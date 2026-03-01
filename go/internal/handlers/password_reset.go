package handlers

import (
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"pilot-finance/internal/i18n"
)

// ForgotPasswordPage affiche la page de mot de passe oublie
func ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	data := baseData(r, nil)
	t := data["T"].(map[string]string)
	data["Title"] = t["forgot.title"]
	data["MailEnabled"] = hookMailIsEnabled()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "forgot-password.html", data) //nolint:errcheck
}

// ForgotPasswordSubmit traite la demande de reinitialisation
func ForgotPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	if !hookMailIsEnabled() {
		clientError(w, ErrDisabled, "Feature disabled", http.StatusBadRequest)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := hookRateLimitCheck(clientIP, "forgotPassword")
	if !result.Allowed {
		clientError(w, ErrRateLimited, "Too many attempts", http.StatusTooManyRequests)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if email == "" {
		clientError(w, ErrValidation, "Email required", http.StatusBadRequest)
		return
	}

	// Chercher l'utilisateur
	blindIndex := hookComputeBlindIndex(email)
	user, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil || user == nil {
		// Ne pas reveler si l'email existe ou non
		data := baseData(r, nil)
		t := data["T"].(map[string]string)
		data["Title"] = t["forgot.title"]
		data["MailEnabled"] = true
		data["Success"] = true
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "forgot-password.html", data) //nolint:errcheck
		return
	}

	// Generer un token
	tokenBytes := make([]byte, 32)
	if _, err := hookRandRead(tokenBytes); err != nil {
		serverError(w, "generate token", err)
		return
	}
	token := hex.EncodeToString(tokenBytes)
	hashedToken := hookHashToken(token)

	// Sauvegarder le token avec expiration 1h
	expiry := time.Now().Add(1 * time.Hour)
	if err := hookSetResetToken(user.ID, hashedToken, expiry); err != nil {
		serverError(w, "set reset token", err)
		return
	}

	// Envoyer l'email
	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost:3000"
	}
	lang := user.Language
	if lang == "" {
		lang = "fr"
	}
	if err := hookSendPasswordReset(email, token, host, lang); err != nil {
		slog.Warn("send password reset email", "err", err)
	}

	data := baseData(r, nil)
	t := data["T"].(map[string]string)
	data["Title"] = t["forgot.title"]
	data["MailEnabled"] = true
	data["Success"] = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "forgot-password.html", data) //nolint:errcheck
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
		t := data["T"].(map[string]string)
		data["Title"] = t["reset.link_expired"]
		data["Error"] = t["reset.link_expired_desc"]
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data) //nolint:errcheck
		return
	}

	data := baseData(r, nil)
	t := data["T"].(map[string]string)
	data["Title"] = t["reset.new_password"]
	data["Token"] = token
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "reset-password.html", data) //nolint:errcheck
}

// ResetPasswordSubmit traite la reinitialisation
func ResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	if token == "" || password == "" {
		clientError(w, ErrValidation, "Missing data", http.StatusBadRequest)
		return
	}

	lang := "fr" // default for unauthenticated context

	if password != confirmPassword {
		data := baseData(r, nil)
		t := data["T"].(map[string]string)
		data["Title"] = t["reset.new_password"]
		data["Token"] = token
		data["Error"] = t["reset.passwords_mismatch"]
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data) //nolint:errcheck
		return
	}

	if err := hookValidatePassword(password); err != nil {
		data := baseData(r, nil)
		t := data["T"].(map[string]string)
		data["Title"] = t["reset.new_password"]
		data["Token"] = token
		data["Error"] = i18n.T(lang, "pwd_error."+err.Error())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data) //nolint:errcheck
		return
	}

	// Verifier le token
	hashedToken := hookHashToken(token)
	user, err := hookGetUserByResetToken(hashedToken)
	if err != nil || user == nil {
		data := baseData(r, nil)
		t := data["T"].(map[string]string)
		data["Title"] = t["reset.link_expired"]
		data["Error"] = t["reset.link_expired_desc"]
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "reset-password.html", data) //nolint:errcheck
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
	hookClearResetToken(user.ID) //nolint:errcheck

	// Rediriger vers login avec message de succes
	http.Redirect(w, r, "/login?reset=success", http.StatusSeeOther)
}
