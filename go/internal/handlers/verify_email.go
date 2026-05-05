package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"pilot-finance/internal/middleware"
)

// ResendVerificationEmail régénère un token et renvoie un email de vérification.
// Rate-limité à 1 par 5 min/IP via verifyEmailResend. Best-effort : si l'email est
// déjà vérifié, on retourne 400. Si SMTP n'est pas configuré, 400.
func ResendVerificationEmail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if !hookMailIsEnabled() {
		clientError(w, ErrDisabled, "Mail non configuré", http.StatusBadRequest)
		return
	}

	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		clientError(w, ErrNotFound, "Utilisateur introuvable", http.StatusNotFound)
		return
	}
	if dbUser.EmailVerified {
		clientError(w, ErrConflict, "Email déjà vérifié", http.StatusBadRequest)
		return
	}

	// Rate limit (1 par 5 min) basé sur l'ID utilisateur (plus précis que l'IP).
	result := hookRateLimitCheck(strconv.FormatInt(user.ID, 10), "verifyEmailResend")
	if !result.Allowed {
		clientError(w, ErrRateLimited, "Veuillez patienter avant un nouvel envoi", http.StatusTooManyRequests)
		return
	}

	email, err := hookDecryptStr(dbUser.EmailEncrypted)
	if err != nil {
		serverError(w, "decrypt email", err)
		return
	}

	// sendVerificationToken applique le fallback "fr" si lang est vide.
	if err := sendVerificationToken(user.ID, email, dbUser.Language); err != nil {
		slog.Warn("resend verification email", "err", err, "userID", user.ID)
		serverError(w, "send verification email", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
