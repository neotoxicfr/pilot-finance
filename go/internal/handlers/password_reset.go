package handlers

import (
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
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

// forgotPasswordWG suit les goroutines de reset-password en vol pour les tests
// (synchronisation déterministe) et pour un éventuel drain au shutdown.
var forgotPasswordWG sync.WaitGroup

// FlushForgotPassword attend que toutes les tâches forgot-password en vol
// soient terminées. Utilisé par les tests pour synchroniser sur la goroutine
// background (timing-oracle hardening, M5).
func FlushForgotPassword() {
	forgotPasswordWG.Wait()
}

// ForgotPasswordSubmit traite la demande de reinitialisation.
// M5 fix : toute la chaîne (lookup user, génération token, INSERT DB, SMTP)
// est exécutée APRÈS l'envoi de la réponse, dans une goroutine background.
// La latence du handler est donc constante (~1ms) quelle que soit l'existence
// de l'email : impossible pour un attaquant de distinguer "user existe" vs
// "user inconnu" via un timing oracle, même résiduel (~1-5ms d'UPDATE DB).
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

	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost:3000"
	}

	// Capturer les données nécessaires AVANT la goroutine (r ne doit pas être
	// utilisé après la fin du handler — son contexte sera annulé).
	forgotPasswordWG.Add(1)
	go func(email, host string) {
		defer forgotPasswordWG.Done()
		processForgotPassword(email, host)
	}(email, host)

	// Réponse immédiate : latence constante, indistinguable du chemin user-absent.
	data := baseData(r, nil)
	t := data["T"].(map[string]string)
	data["Title"] = t["forgot.title"]
	data["MailEnabled"] = true
	data["Success"] = true
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "forgot-password.html", data) //nolint:errcheck
}

// processForgotPassword effectue le travail effectif (lookup + token + email).
// Découpé en fonction privée pour permettre le test isolé (sans timing) et
// surtout pour s'exécuter en background.
func processForgotPassword(email, host string) {
	// Toujours payer le coût crypto pour équilibrer le travail CPU même si
	// la réponse est déjà envoyée — défense en profondeur côté logs/metrics.
	tokenBytes := make([]byte, 32)
	if _, rerr := hookRandRead(tokenBytes); rerr != nil {
		slog.Error("forgot password: rand read", "err", rerr)
		return
	}
	token := hex.EncodeToString(tokenBytes)
	hashedToken := hookHashToken(token)

	blindIndex := hookComputeBlindIndex(email)
	user, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil || user == nil {
		// User inconnu : rien à faire, on a déjà payé le coût crypto.
		return
	}

	expiry := time.Now().Add(1 * time.Hour)
	if err := hookSetResetToken(user.ID, hashedToken, expiry); err != nil {
		slog.Error("forgot password: set reset token", "err", err)
		return
	}

	lang := user.Language
	if lang == "" {
		lang = "fr"
	}
	if err := hookSendPasswordReset(email, token, host, lang); err != nil {
		slog.Warn("send password reset email", "err", err)
	}
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
