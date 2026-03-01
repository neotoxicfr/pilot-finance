package handlers

import (
	"encoding/json"
	"net/http"
)

// Codes d'erreur structurés pour les réponses API et HTMX.
// Le header X-Error-Code est systématiquement positionné.
const (
	ErrValidation    = "VALIDATION_FAILED"
	ErrAuthRequired  = "AUTH_REQUIRED"
	ErrAuthExpired   = "AUTH_EXPIRED"
	ErrAuthInvalid   = "AUTH_INVALID"
	ErrForbidden     = "FORBIDDEN"
	ErrNotFound      = "NOT_FOUND"
	ErrConflict      = "CONFLICT"
	ErrRateLimited   = "RATE_LIMITED"
	ErrAccountLocked = "ACCOUNT_LOCKED"
	ErrInternal      = "SERVER_ERROR"
	ErrEncryption    = "ENCRYPTION_ERROR"
	ErrDisabled      = "FEATURE_DISABLED"
)

// clientError renvoie une erreur HTTP avec un code machine-readable dans le header.
func clientError(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set("X-Error-Code", code)
	http.Error(w, message, status)
}

// jsonError renvoie une erreur JSON avec code structuré (pour les endpoints /api/).
func jsonError(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Error-Code", code)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"code": code, "error": message})
}

// jsonSuccess renvoie une réponse JSON de succès.
func jsonSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
