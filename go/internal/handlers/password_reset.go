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

// ForgotPasswordSubmit traite la demande de reinitialisation.
// M2 fix : exécute le travail crypto (rand + hashToken) même quand l'email
// n'existe pas pour égaliser le temps de réponse, et envoie l'email en
// goroutine pour ne pas exposer la durée du SMTP.
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

	renderSuccess := func() {
		data := baseData(r, nil)
		t := data["T"].(map[string]string)
		data["Title"] = t["forgot.title"]
		data["MailEnabled"] = true
		data["Success"] = true
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "forgot-password.html", data) //nolint:errcheck
	}

	// Chercher l'utilisateur
	blindIndex := hookComputeBlindIndex(email)
	user, err := hookGetUserByBlindIndex(blindIndex)

	// Toujours générer 32 bytes aléatoires + dérivation HMAC pour égaliser
	// les temps user-existe vs user-absent (timing oracle).
	tokenBytes := make([]byte, 32)
	if _, rerr := hookRandRead(tokenBytes); rerr != nil {
		// Si le rand échoue côté serveur, garder réponse générique.
		slog.Error("forgot password: rand read", "err", rerr)
		renderSuccess()
		return
	}
	token := hex.EncodeToString(tokenBytes)
	hashedToken := hookHashToken(token)

	if err != nil || user == nil {
		// User inconnu : on a déjà payé le coût crypto. Réponse identique.
		renderSuccess()
		return
	}

	// Sauvegarder le token avec expiration 1h
	expiry := time.Now().Add(1 * time.Hour)
	if err := hookSetResetToken(user.ID, hashedToken, expiry); err != nil {
		serverError(w, "set reset token", err)
		return
	}

	// Envoyer l'email en arrière-plan pour ne pas exposer la durée du SMTP
	// (timing oracle distinct du chemin user-absent). Les erreurs sont loggées.
	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost:3000"
	}
	lang := user.Language
	if lang == "" {
		lang = "fr"
	}
	go func(email, token, host, lang string) {
		if err := hookSendPasswordReset(email, token, host, lang); err != nil {
			slog.Warn("send password reset email", "err", err)
		}
	}(email, token, host, lang)

	renderSuccess()
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
	clientIP := getClientIP(r)

	// Rate limiting
	result := hookRateLimitCheck(clientIP, "resetPassword")
	if !result.Allowed {
		clientError(w, ErrRateLimited, "Too many attempts", http.StatusTooManyRequests)
		return
	}

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

	// Mettre à jour le mot de passe ET effacer le reset token dans la MÊME
	// transaction. Évite la fenêtre où le token resterait valide en DB si
	// l'effacement échouait après un update réussi.
	if err := hookUpdatePasswordAndClearReset(user.ID, hashedPassword); err != nil {
		serverError(w, "update password and clear reset token", err)
		return
	}

	// Invalidate session cache so stolen tokens are rejected immediately
	hookInvalidateSessionCache(user.ID)

	// Rediriger vers login avec message de succes
	http.Redirect(w, r, "/login?reset=success", http.StatusSeeOther)
}
