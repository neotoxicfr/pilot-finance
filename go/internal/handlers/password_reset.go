package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"
)

// ForgotPasswordPage affiche la page de mot de passe oublie
func ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	data := baseData(r, nil)
	data["Title"] = "Mot de passe oublie"
	data["MailEnabled"] = hookMailIsEnabled()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "forgot-password.html", data)
}

// ForgotPasswordSubmit traite la demande de reinitialisation
func ForgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if !hookMailIsEnabled() {
		clientError(w, ErrDisabled, "Fonction desactivee", http.StatusBadRequest)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := hookRateLimitCheck(clientIP, "forgotPassword")
	if !result.Allowed {
		clientError(w, ErrRateLimited, "Trop de tentatives. Reessayez plus tard.", http.StatusTooManyRequests)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if email == "" {
		clientError(w, ErrValidation, "Email requis", http.StatusBadRequest)
		return
	}

	// Chercher l'utilisateur
	blindIndex := hookComputeBlindIndex(email)
	user, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil || user == nil {
		// Ne pas reveler si l'email existe ou non
		data := baseData(r, nil)
		data["Title"] = "Mot de passe oublie"
		data["MailEnabled"] = true
		data["Success"] = true
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "forgot-password.html", data)
		return
	}

	// Generer un token
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)
	hashedToken := hookHashToken(token)

	// Sauvegarder le token avec expiration 1h
	expiry := time.Now().Add(1 * time.Hour)
	hookSetResetToken(user.ID, hashedToken, expiry)

	// Envoyer l'email
	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost:3000"
	}
	hookSendPasswordReset(email, token, host)

	data := baseData(r, nil)
	data["Title"] = "Mot de passe oublie"
	data["MailEnabled"] = true
	data["Success"] = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "forgot-password.html", data)
}

// ResetPasswordPage affiche la page de reinitialisation
func ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Verifier le token
	hashedToken := hookHashToken(token)
	user, err := hookGetUserByResetToken(hashedToken)
	if err != nil || user == nil {
		data := baseData(r, nil)
		data["Title"] = "Lien expire"
		data["Error"] = "Ce lien de reinitialisation a expire ou est invalide."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data)
		return
	}

	data := baseData(r, nil)
	data["Title"] = "Nouveau mot de passe"
	data["Token"] = token
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "reset-password.html", data)
}

// ResetPasswordSubmit traite la reinitialisation
func ResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	if token == "" || password == "" {
		clientError(w, ErrValidation, "Donnees manquantes", http.StatusBadRequest)
		return
	}

	if password != confirmPassword {
		data := baseData(r, nil)
		data["Title"] = "Nouveau mot de passe"
		data["Token"] = token
		data["Error"] = "Les mots de passe ne correspondent pas"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data)
		return
	}

	if err := hookValidatePassword(password); err != nil {
		data := baseData(r, nil)
		data["Title"] = "Nouveau mot de passe"
		data["Token"] = token
		data["Error"] = err.Error()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data)
		return
	}

	// Verifier le token
	hashedToken := hookHashToken(token)
	user, err := hookGetUserByResetToken(hashedToken)
	if err != nil || user == nil {
		data := baseData(r, nil)
		data["Title"] = "Lien expire"
		data["Error"] = "Ce lien de reinitialisation a expire ou est invalide."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data)
		return
	}

	// Hasher le nouveau mot de passe
	hashedPassword, err := hookHashPassword(password)
	if err != nil {
		serverError(w, "hash password", err)
		return
	}

	// Mettre a jour le mot de passe
	if err := hookUpdatePassword(user.ID, hashedPassword); err != nil {
		serverError(w, "update password", err)
		return
	}

	// Effacer le token
	hookClearResetToken(user.ID)

	// Rediriger vers login avec message de succes
	http.Redirect(w, r, "/login?reset=success", http.StatusSeeOther)
}
