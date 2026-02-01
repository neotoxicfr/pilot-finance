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

	// Routes protégées
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)

		r.Get("/", handlers.Dashboard)
		r.Get("/accounts", handlers.AccountsPage)
		r.Post("/accounts", handlers.CreateAccount)
		r.Put("/accounts/{id}", handlers.UpdateAccount)
		r.Delete("/accounts/{id}", handlers.DeleteAccount)
		r.Post("/accounts/{id}/balance", handlers.UpdateBalance)

		r.Get("/recurring", handlers.RecurringPage)
		r.Post("/recurring", handlers.CreateRecurring)
		r.Put("/recurring/{id}", handlers.UpdateRecurring)
		r.Delete("/recurring/{id}", handlers.DeleteRecurring)

		r.Get("/settings", handlers.SettingsPage)
		r.Post("/settings/password", handlers.ChangePassword)

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
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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

// securityHeaders ajoute les headers de sécurité
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSP compatible HTMX + Alpine.js
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
			"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://unpkg.com https://cdn.tailwindcss.com; "+
			"style-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com; "+
			"img-src 'self' blob: data:; "+
			"font-src 'self'; "+
			"connect-src 'self'; "+
			"frame-ancestors 'none'; "+
			"base-uri 'self'; "+
			"form-action 'self'")

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// HSTS (à activer en production avec HTTPS)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}
