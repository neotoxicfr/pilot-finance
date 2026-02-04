package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/ratelimit"
	"pilot-finance/internal/templates"
)

// ForgotPasswordPage affiche la page de mot de passe oublie
func ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":       "Mot de passe oublie",
		"MailEnabled": mail.IsEnabled(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.Render(w, "forgot-password.html", data)
}

// ForgotPasswordSubmit traite la demande de reinitialisation
func ForgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if !mail.IsEnabled() {
		http.Error(w, "Fonction desactivee", http.StatusBadRequest)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := ratelimit.Check(clientIP, "forgotPassword")
	if !result.Allowed {
		http.Error(w, "Trop de tentatives. Reessayez plus tard.", http.StatusTooManyRequests)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if email == "" {
		http.Error(w, "Email requis", http.StatusBadRequest)
		return
	}

	// Chercher l'utilisateur
	blindIndex := crypto.ComputeBlindIndex(email)
	user, err := db.GetUserByBlindIndex(blindIndex)
	if err != nil || user == nil {
		// Ne pas reveler si l'email existe ou non
		data := map[string]interface{}{
			"Title":       "Mot de passe oublie",
			"MailEnabled": true,
			"Success":     true,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "forgot-password.html", data)
		return
	}

	// Generer un token
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	hashedToken := crypto.HashToken(token)

	// Sauvegarder le token avec expiration 1h
	expiry := time.Now().Add(1 * time.Hour)
	db.SetResetToken(user.ID, hashedToken, expiry)

	// Envoyer l'email
	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost:3000"
	}
	mail.SendPasswordReset(email, token, host)

	data := map[string]interface{}{
		"Title":       "Mot de passe oublie",
		"MailEnabled": true,
		"Success":     true,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.Render(w, "forgot-password.html", data)
}

// ResetPasswordPage affiche la page de reinitialisation
func ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Verifier le token
	hashedToken := crypto.HashToken(token)
	user, err := db.GetUserByResetToken(hashedToken)
	if err != nil || user == nil {
		data := map[string]interface{}{
			"Title": "Lien expire",
			"Error": "Ce lien de reinitialisation a expire ou est invalide.",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "reset-password.html", data)
		return
	}

	data := map[string]interface{}{
		"Title": "Nouveau mot de passe",
		"Token": token,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.Render(w, "reset-password.html", data)
}

// ResetPasswordSubmit traite la reinitialisation
func ResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	if token == "" || password == "" {
		http.Error(w, "Donnees manquantes", http.StatusBadRequest)
		return
	}

	if password != confirmPassword {
		data := map[string]interface{}{
			"Title": "Nouveau mot de passe",
			"Token": token,
			"Error": "Les mots de passe ne correspondent pas",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "reset-password.html", data)
		return
	}

	if err := crypto.ValidatePassword(password); err != nil {
		data := map[string]interface{}{
			"Title": "Nouveau mot de passe",
			"Token": token,
			"Error": err.Error(),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "reset-password.html", data)
		return
	}

	// Verifier le token
	hashedToken := crypto.HashToken(token)
	user, err := db.GetUserByResetToken(hashedToken)
	if err != nil || user == nil {
		data := map[string]interface{}{
			"Title": "Lien expire",
			"Error": "Ce lien de reinitialisation a expire ou est invalide.",
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "reset-password.html", data)
		return
	}

	// Hasher le nouveau mot de passe
	hashedPassword, err := crypto.HashPassword(password)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Mettre a jour le mot de passe
	db.UpdatePassword(user.ID, hashedPassword)

	// Effacer le token
	db.ClearResetToken(user.ID)

	// Rediriger vers login avec message de succes
	http.Redirect(w, r, "/login?reset=success", http.StatusSeeOther)
}
