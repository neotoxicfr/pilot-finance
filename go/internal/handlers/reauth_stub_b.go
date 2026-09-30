package handlers

// STUB TEMPORAIRE (mission B) — À SUPPRIMER à la fusion : verifyCurrentPassword
// est fourni par helpers.go (mission A, SEC-03 : limiteur + audit d'échec).
// Ce stub ne sert qu'à compiler PasskeyRegistrationStart dans ce worktree.

import (
	"net/http"

	"pilot-finance/internal/middleware"
)

// verifyCurrentPassword vérifie le mot de passe courant ; écrit la réponse
// d'erreur et renvoie false en cas d'échec.
func verifyCurrentPassword(w http.ResponseWriter, r *http.Request, user *middleware.User, password string) bool {
	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil || !hookVerifyPassword(password, dbUser.Password) {
		clientErrorT(w, r, ErrAuthInvalid, "error.current_password_incorrect", http.StatusUnauthorized)
		return false
	}
	return true
}
