package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
)

// Codes d'erreur structurés pour les réponses API et HTMX.
// Le header X-Error-Code est systématiquement positionné.
const (
	ErrValidation       = "VALIDATION_FAILED"
	ErrAuthRequired     = "AUTH_REQUIRED"
	ErrAuthExpired      = "AUTH_EXPIRED"
	ErrAuthInvalid      = "AUTH_INVALID"
	ErrForbidden        = "FORBIDDEN"
	ErrNotFound         = "NOT_FOUND"
	ErrMethodNotAllowed = "METHOD_NOT_ALLOWED"
	ErrConflict         = "CONFLICT"
	ErrRateLimited      = "RATE_LIMITED"
	ErrAccountLocked    = "ACCOUNT_LOCKED"
	ErrInternal         = "SERVER_ERROR"
	ErrEncryption       = "ENCRYPTION_ERROR"
	ErrDisabled         = "FEATURE_DISABLED"
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

// requestLang résout la langue dans laquelle rendre un message destiné à
// l'utilisateur (audit FIN-14).
//
// Deux sources, dans cet ordre :
//  1. la préférence enregistrée sur le compte, disponible dès que le middleware
//     d'authentification a peuplé le contexte de la requête ;
//  2. l'en-tête Accept-Language du navigateur, via detectLanguage. Indispensable
//     sur les chemins NON authentifiés (login, register, forgot/reset password)
//     où middleware.GetUser renvoie nil : sans ce repli, un visiteur anglophone
//     lirait « Identifiants incorrects » sur l'écran de connexion.
//
// Le repli final de detectLanguage est "en" ; i18n.T retombe de toute façon sur
// le français si une clé manque dans la langue demandée.
func requestLang(r *http.Request) string {
	if u := middleware.GetUser(r); u != nil && u.Language != "" {
		return u.Language
	}
	return detectLanguage(r)
}

// clientErrorT est la variante traduite de clientError : le message est résolu
// via i18n.T dans la langue de l'utilisateur. Une clé i18n distincte est
// attribuée à CHAQUE site d'appel — le code machine-readable (X-Error-Code) est
// trop grossier pour porter le libellé (ErrAuthInvalid couvre à lui seul
// « Identifiants incorrects », « Session expirée », « Code 2FA invalide »…).
func clientErrorT(w http.ResponseWriter, r *http.Request, code, key string, status int) {
	clientError(w, code, i18n.T(requestLang(r), key), status)
}

// clientErrorTn est la variante paramétrée de clientErrorT : le marqueur « {n} »
// de la traduction est remplacé par n, selon la même convention que la fonction
// de template `replace` et que le JS côté client.
func clientErrorTn(w http.ResponseWriter, r *http.Request, code, key string, status int, n int64) {
	msg := strings.ReplaceAll(i18n.T(requestLang(r), key), "{n}", strconv.FormatInt(n, 10))
	clientError(w, code, msg, status)
}

// jsonErrorT est la variante traduite de jsonError (endpoints /api/).
func jsonErrorT(w http.ResponseWriter, r *http.Request, code, key string, status int) {
	jsonError(w, code, i18n.T(requestLang(r), key), status)
}

// jsonSuccess renvoie une réponse JSON de succès.
// L'encodage se fait en mémoire d'abord : une donnée non sérialisable (float
// NaN/Inf, cf. audit FIN-1/EDGE-003) doit produire un 500 explicite plutôt
// qu'un 200 au corps tronqué et silencieux.
func jsonSuccess(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		slog.Error("jsonSuccess: encodage", "err", err)
		clientError(w, ErrInternal, "Erreur interne", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(buf)
}
