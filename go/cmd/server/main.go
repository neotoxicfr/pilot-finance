package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/config"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/handlers"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/ratelimit"
	"pilot-finance/internal/templates"
)

// Version est définie par ldflags au build (-X main.Version=x.y.z)
var Version = "dev"

func main() {
	// JSON logs en production, texte en dev
	var logHandler slog.Handler
	if os.Getenv("ENV") == "production" {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(logHandler))

	// Propager la version au package handlers (health check)
	handlers.Version = Version

	slog.Info("Pilot Finance démarrage", "version", Version)

	// Charger la configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "err", err)
		os.Exit(1)
	}

	// Initialiser le chiffrement
	if err := crypto.Init(cfg.EncryptionKey, cfg.BlindIndexKey); err != nil {
		slog.Error("crypto init", "err", err)
		os.Exit(1)
	}
	slog.Info("chiffrement initialisé")

	// Initialiser JWT
	auth.InitJWT(cfg.AuthSecret)
	slog.Info("JWT initialisé")

	// Connexion à la base de données
	dbPath := cfg.DatabaseURL
	if len(dbPath) > 5 && dbPath[:5] == "file:" {
		dbPath = dbPath[5:]
	}
	if err := db.Init(db.Config{Path: dbPath}); err != nil {
		slog.Error("base de données", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("base de données connectée")

	// Initialiser les templates
	if err := templates.Init("templates"); err != nil {
		slog.Error("templates", "err", err)
		os.Exit(1)
	}
	slog.Info("templates chargés")

	// Charger les traductions
	if err := i18n.Load("locales"); err != nil {
		slog.Error("i18n", "err", err)
		os.Exit(1)
	}
	slog.Info("traductions chargées")

	// Initialiser le mail (optionnel)
	if err := mail.Init(); err != nil {
		slog.Warn("mail non configuré", "err", err)
	} else if mail.IsEnabled() {
		slog.Info("mail configuré")
	}

	// Initialiser WebAuthn/Passkeys si HOST est configuré
	host := os.Getenv("HOST")
	if host != "" {
		rpOrigin := "https://" + host
		if err := auth.InitWebAuthn(host, rpOrigin, "Pilot Finance"); err != nil {
			slog.Warn("passkeys non configurés", "err", err)
		} else {
			slog.Info("passkeys configurés")
		}
	}

	// Créer le routeur
	r := chi.NewRouter()

	// Middlewares globaux — doivent être déclarés AVANT NotFound/MethodNotAllowed
	// pour que chi les applique aux handlers d'erreur
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	r.Use(securityHeaders)
	r.Use(maxBodySize)
	r.Use(httprate.LimitByRealIP(120, time.Minute)) // 120 req/min global (2/s, suffisant pour usage actif)

	r.NotFound(handlers.NotFound)
	r.MethodNotAllowed(handlers.MethodNotAllowed)

	// Fichiers statiques avec cache (pas de rate limit)
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", cacheStatic(fileServer)))

	// Health check (pas de rate limit strict)
	r.Get("/api/health", handlers.HealthCheck)

	// Routes auth avec rate limit (10 req/min anti-bruteforce, humain = ~1 essai/6s)
	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByRealIP(10, time.Minute))

		r.Get("/login", handlers.LoginPage)
		r.Post("/login", handlers.LoginSubmit)
		r.Get("/register", handlers.RegisterPage)
		r.Post("/register", handlers.RegisterSubmit)
		r.Get("/forgot-password", handlers.ForgotPasswordPage)
		r.Post("/forgot-password", handlers.ForgotPasswordSubmit)
		r.Get("/reset-password", handlers.ResetPasswordPage)
		r.Post("/reset-password", handlers.ResetPasswordSubmit)
		r.Post("/api/passkey/login/start", handlers.PasskeyLoginStart)
		r.Post("/api/passkey/login/finish", handlers.PasskeyLoginFinish)
	})

	// Routes publiques sans rate limit strict
	r.With(middleware.ValidateOrigin(host), middleware.OptionalAuth).Post("/logout", handlers.Logout)
	r.Get("/verify-email", handlers.VerifyEmailPage)
	r.With(middleware.OptionalAuth).Get("/privacy", handlers.PrivacyPage)
	r.With(middleware.OptionalAuth).Get("/legal", handlers.LegalPage)

	// Routes protégées
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Use(middleware.ValidateOrigin(host))

		r.Get("/", handlers.Dashboard)
		r.Get("/accounts", handlers.AccountsPage)
		r.Post("/accounts", handlers.CreateAccount)
		r.Post("/accounts/reorder", handlers.ReorderAccounts)
		r.Delete("/accounts/{id}", handlers.DeleteAccount)
		r.Post("/accounts/{id}/balance", handlers.UpdateBalance)
		r.Post("/accounts/{id}/move", handlers.MoveAccount)

		r.Post("/recurring", handlers.CreateRecurring)
		r.Put("/recurring/{id}", handlers.UpdateRecurring)
		r.Delete("/recurring/{id}", handlers.DeleteRecurring)

		r.Get("/settings", handlers.SettingsPage)
		r.Post("/settings/password", handlers.ChangePassword)
		r.Post("/settings/preferences", handlers.UpdatePreferences)
		r.With(httprate.LimitByRealIP(10, time.Minute)).Get("/settings/export", handlers.ExportData)
		r.Delete("/settings/account", handlers.DeleteSelfAccount)

		// Routes MFA
		r.Get("/settings/mfa/setup", handlers.MFASetup)
		r.Post("/settings/mfa/enable", handlers.MFAEnable)
		r.Post("/settings/mfa/disable", handlers.MFADisable)

		// Routes Passkey (protégées pour l'enregistrement)
		r.Post("/api/passkey/register/start", handlers.PasskeyRegistrationStart)
		r.Post("/api/passkey/register/finish", handlers.PasskeyRegistrationFinish)
		r.Delete("/api/passkey/{id}", handlers.DeletePasskey)
		r.Put("/api/passkey/{id}/rename", handlers.RenamePasskey)

		// API endpoints
		r.Get("/api/dashboard", handlers.DashboardAPI)
		r.Get("/api/accounts", handlers.AccountsAPI)
		r.Get("/api/recurring", handlers.RecurringAPI)
	})

	// Routes admin
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Use(middleware.RequireAdmin)

		r.Get("/admin", handlers.SettingsPage)
		r.Delete("/admin/users/{id}", handlers.DeleteUser)
		r.Get("/admin/audit", handlers.AuditPage)
	})

	// Démarrer le serveur
	addr := ":" + cfg.Port
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown via signal.NotifyContext
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("serveur démarré", "addr", "http://localhost"+addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("serveur", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("arrêt en cours")
	ratelimit.StopAll()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
	slog.Info("serveur arrêté proprement")
}

// maxBodySize limite la taille du body HTTP à 1MB pour prévenir les attaques DoS
func maxBodySize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
		next.ServeHTTP(w, r)
	})
}

// staticETags calcule les ETag au démarrage pour chaque fichier statique
var staticETags = func() map[string]string {
	tags := make(map[string]string)
	fs.WalkDir(os.DirFS("static"), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			hash := md5.Sum([]byte(fmt.Sprintf("%s-%d-%d", path, info.Size(), info.ModTime().UnixNano())))
			tags["/"+path] = fmt.Sprintf(`"%x"`, hash)
		}
		return nil
	})
	return tags
}()

// cacheStatic ajoute des headers de cache et ETag pour les fichiers statiques
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // 1 an (hash busting)
		} else {
			w.Header().Set("Cache-Control", "public, max-age=2592000") // 30 jours (images, icônes)
		}

		// ETag pour cache conditionnel
		if etag, ok := staticETags[path]; ok {
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// securityHeaders génère un nonce par requête et l'intègre dans la CSP.
// Pour les routes /api/ (JSON), le nonce et la CSP HTML ne sont pas nécessaires.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers communs à toutes les routes
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Cache-Control", "no-store")

		// Les routes /api/ retournent du JSON : pas de CSP HTML ni de nonce
		if strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		nonce := middleware.GenerateNonce()
		r = r.WithContext(middleware.WithNonce(r.Context(), nonce))

		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; "+
				"script-src 'self' 'nonce-"+nonce+"' 'strict-dynamic'; "+
				"style-src 'self' 'unsafe-inline'; "+ // unsafe-inline requis par Tailwind CSS v4 (styles inline générés)

				"img-src 'self' blob: data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"manifest-src 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")

		next.ServeHTTP(w, r)
	})
}
