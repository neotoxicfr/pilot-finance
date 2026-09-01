package main

import (
	"context"
	"crypto/md5"
	"database/sql"
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
	"pilot-finance/internal/metrics"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/ratelimit"
	"pilot-finance/internal/templates"
)

// Version est définie par ldflags au build (-X main.Version=x.y.z)
var Version = "dev"

func main() {
	// Mode healthcheck intégré (pour scratch/distroless sans wget)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		resp, err := http.Get("http://127.0.0.1:3000/api/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

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
	handlers.AssetVersion = computeAssetVersion()

	slog.Info("Pilot Finance démarrage", "version", Version, "assets", handlers.AssetVersion)

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
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("fermeture base de données", "err", err)
		}
	}()
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

	// Désactiver le rate limiting applicatif si demandé (tests E2E)
	disableRL := os.Getenv("DISABLE_RATE_LIMIT") == "true"
	if disableRL {
		ratelimit.Disabled = true
		slog.Warn("rate limiting désactivé (DISABLE_RATE_LIMIT=true)")
	}

	// Initialiser les métriques Prometheus
	metrics.Init(func() *sql.DB { return db.DB })
	slog.Info("métriques Prometheus initialisées")

	// Créer le routeur
	r := chi.NewRouter()

	// Middlewares globaux — doivent être déclarés AVANT NotFound/MethodNotAllowed
	// pour que chi les applique aux handlers d'erreur
	tp, err := middleware.TrustedProxy(os.Getenv("TRUSTED_PROXIES"), os.Getenv("ENV") == "production")
	if err != nil {
		slog.Error("trusted proxies", "err", err)
		os.Exit(1)
	}
	r.Use(tp)
	r.Use(chimw.RequestID)
	r.Use(middleware.SanitizedLogger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	r.Use(metrics.Middleware)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxBodySize)
	if !disableRL {
		r.Use(httprate.LimitBy(120, time.Minute, middleware.ClientIPKey)) // 120 req/min global
	}

	r.NotFound(handlers.NotFound)
	r.MethodNotAllowed(handlers.MethodNotAllowed)

	// Fichiers statiques avec cache (pas de rate limit)
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", cacheStatic(fileServer)))

	// Health check, CSP report (pas de rate limit strict)
	r.Get("/api/health", handlers.HealthCheck)
	r.Post("/api/csp-report", handlers.CSPReport)

	// Metrics endpoint (admin auth required)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Use(middleware.RequireAdmin)
		r.Get("/metrics", metrics.Handler().ServeHTTP)
	})

	// Routes auth avec rate limit (10 req/min anti-bruteforce, humain = ~1 essai/6s)
	r.Group(func(r chi.Router) {
		if !disableRL {
			r.Use(httprate.LimitBy(10, time.Minute, middleware.ClientIPKey))
		}
		// ValidateOrigin protège aussi les POST d'auth (login CSRF) : sans lui,
		// un POST cross-site vers /login pouvait connecter la victime sur le
		// compte de l'attaquant (cookie SameSite=Lax insuffisant, audit FIN-18).
		// N'agit que sur les méthodes mutantes ; les GET de page passent.
		r.Use(middleware.ValidateOrigin(host))

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
	r.With(middleware.OptionalAuth).Get("/verify-email", handlers.VerifyEmailPage)
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
		r.Post("/settings/verify-email/resend", handlers.ResendVerificationEmail)
		exportRoute := r.With()
		if !disableRL {
			exportRoute = r.With(httprate.LimitBy(10, time.Minute, middleware.ClientIPKey))
		}
		exportRoute.Get("/settings/export", handlers.ExportData)
		exportRoute.Post("/settings/import", handlers.ImportBalances)
		r.Delete("/settings/account", handlers.DeleteSelfAccount)

		// Routes MFA
		r.Get("/settings/mfa/setup", handlers.MFASetup)
		r.Post("/settings/mfa/enable", handlers.MFAEnable)
		r.Post("/settings/mfa/disable", handlers.MFADisable)
		r.Get("/settings/mfa/recovery/count", handlers.MFARecoveryCount)
		r.Post("/settings/mfa/recovery/regenerate", handlers.MFARecoveryRegenerate)

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
		r.Use(middleware.ValidateOrigin(host))

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

	// Rotation automatique du journal d'audit (purge > 90 jours, toutes les 24h)
	db.StartAuditRotation(ctx)

	go func() {
		slog.Info("serveur démarré", "addr", "http://localhost"+addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("serveur", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("arrêt en cours")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
	// Stop rate-limiter background workers only after in-flight requests have
	// drained (server.Shutdown above), since handlers may still touch limiters.
	ratelimit.StopAll()
	// Drain les tâches forgot-password en vol (audit S-33). Le handler répond
	// immédiatement puis fait le travail en arrière-plan pour ne pas exposer un
	// oracle temporel ; sans ce drain, un arrêt pendant cette fenêtre laissait
	// l'écriture du jeton ou l'envoi du mail inachevés, sur une base déjà fermée.
	handlers.FlushForgotPassword()
	// M6 : drain les écritures audit en vol avant de quitter (fire-and-forget).
	db.FlushAuditLog()
	slog.Info("serveur arrêté proprement")
}

// staticETags calcule les ETag au démarrage pour chaque fichier statique
var staticETags = func() map[string]string {
	tags := make(map[string]string)
	if err := fs.WalkDir(os.DirFS("static"), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			hash := md5.Sum([]byte(fmt.Sprintf("%s-%d-%d", path, info.Size(), info.ModTime().UnixNano())))
			tags["/"+path] = fmt.Sprintf(`"%x"`, hash)
		}
		return nil
	}); err != nil {
		slog.Warn("static ETag calculation", "err", err)
	}
	return tags
}()

// computeAssetVersion calcule un hash court de TOUS les .css/.js servis (y
// compris les libs vendorées htmx/alpine/chart/sortable) pour le cache-busting.
// Servis en Cache-Control immutable/1 an, ils doivent changer d'URL à chaque
// bump : les hasher tous évite qu'un bump vendor reste invisible jusqu'à un an
// côté navigateur (audit FIN-13). Parcours déterministe (fs.WalkDir trie).
func computeAssetVersion() string {
	h := md5.New()
	if err := fs.WalkDir(os.DirFS("static"), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".css") && !strings.HasSuffix(path, ".js") {
			return nil
		}
		data, rerr := os.ReadFile("static/" + path)
		if rerr != nil {
			return nil
		}
		h.Write([]byte(path))
		h.Write(data)
		return nil
	}); err != nil {
		slog.Warn("computeAssetVersion", "err", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

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

// computeAssetVersion et cacheStatic restent ici : plomberie d'assets liée au
// dossier "static" et calculée au démarrage. Les middlewares de sécurité
// (TrustedProxy, SecurityHeaders, MaxBodySize) sont dans internal/middleware.
