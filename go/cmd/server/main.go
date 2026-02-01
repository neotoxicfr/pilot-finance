package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/config"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/handlers"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/templates"
)

func main() {
	log.Println("Pilot Finance v2.0 - Démarrage...")

	// Charger la configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Erreur configuration: %v", err)
	}

	// Initialiser le chiffrement
	if err := crypto.Init(cfg.EncryptionKey, cfg.BlindIndexKey); err != nil {
		log.Fatalf("Erreur crypto: %v", err)
	}
	log.Println("✓ Chiffrement initialisé")

	// Initialiser JWT
	auth.InitJWT(cfg.AuthSecret)
	log.Println("✓ JWT initialisé")

	// Connexion à la base de données
	dbPath := cfg.DatabaseURL
	if len(dbPath) > 5 && dbPath[:5] == "file:" {
		dbPath = dbPath[5:]
	}
	if err := db.Init(db.Config{Path: dbPath}); err != nil {
		log.Fatalf("Erreur base de données: %v", err)
	}
	defer db.Close()
	log.Println("✓ Base de données connectée")

	// Initialiser les templates
	if err := templates.Init("templates"); err != nil {
		log.Fatalf("Erreur templates: %v", err)
	}
	log.Println("✓ Templates chargés")

	// Initialiser le mail (optionnel)
	if err := mail.Init(); err != nil {
		log.Printf("⚠ Mail non configuré: %v", err)
	} else if mail.IsEnabled() {
		log.Println("✓ Mail configuré")
	}

	// Initialiser WebAuthn/Passkeys si HOST est configuré
	host := os.Getenv("HOST")
	if host != "" {
		rpOrigin := "https://" + host
		if err := auth.InitWebAuthn(host, rpOrigin, "Pilot Finance"); err != nil {
			log.Printf("⚠ Passkeys non configurés: %v", err)
		} else {
			log.Println("✓ Passkeys configurés")
		}
	}

	// Créer le routeur
	r := chi.NewRouter()

	// Middlewares globaux
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	r.Use(securityHeaders)

	// Fichiers statiques
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Routes publiques
	r.Get("/login", handlers.LoginPage)
	r.Post("/login", handlers.LoginSubmit)
	r.Get("/register", handlers.RegisterPage)
	r.Post("/register", handlers.RegisterSubmit)
	r.Post("/logout", handlers.Logout)
	r.Get("/api/health", handlers.HealthCheck)

	// Routes mot de passe oublié
	r.Get("/forgot-password", handlers.ForgotPasswordPage)
	r.Post("/forgot-password", handlers.ForgotPasswordSubmit)
	r.Get("/reset-password", handlers.ResetPasswordPage)
	r.Post("/reset-password", handlers.ResetPasswordSubmit)

	// Verification email
	r.Get("/verify-email", handlers.VerifyEmailPage)

	// Routes Passkey (publiques pour le login)
	r.Post("/api/passkey/login/start", handlers.PasskeyLoginStart)
	r.Post("/api/passkey/login/finish", handlers.PasskeyLoginFinish)

	// Routes protégées
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		r.Get("/", handlers.Dashboard)
		r.Get("/dashboard-partial", handlers.DashboardPartial)
		r.Get("/accounts", handlers.AccountsPage)
		r.Post("/accounts", handlers.CreateAccount)
		r.Put("/accounts/{id}", handlers.UpdateAccount)
		r.Delete("/accounts/{id}", handlers.DeleteAccount)
		r.Post("/accounts/{id}/balance", handlers.UpdateBalance)
		r.Post("/accounts/{id}/move", handlers.MoveAccount)

		r.Get("/recurring", handlers.RecurringPage)
		r.Post("/recurring", handlers.CreateRecurring)
		r.Put("/recurring/{id}", handlers.UpdateRecurring)
		r.Delete("/recurring/{id}", handlers.DeleteRecurring)

		r.Get("/settings", handlers.SettingsPage)
		r.Post("/settings/password", handlers.ChangePassword)

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

		r.Get("/admin", handlers.AdminPage)
		r.Delete("/admin/users/{id}", handlers.DeleteUser)
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

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Arrêt en cours...")
		server.Close()
	}()

	log.Printf("✓ Serveur démarré sur http://localhost%s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Erreur serveur: %v", err)
	}
}

// securityHeaders ajoute le CSP (les autres headers sont gérés par Traefik)
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP spécifique à l'app (HTMX + Alpine.js + Chart.js + Tailwind)
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com https://cdn.tailwindcss.com https://cdn.jsdelivr.net; "+
				"style-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com; "+
				"img-src 'self' blob: data: https://chart.googleapis.com; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")

		next.ServeHTTP(w, r)
	})
}
