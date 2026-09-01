package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// PasskeyRegistrationStart démarre l'enregistrement d'une passkey
func PasskeyRegistrationStart(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	// Récupérer les passkeys existantes
	authenticators, err := hookGetAuthenticatorsByUserID(user.ID)
	if err != nil {
		serverError(w, r, "get authenticators", err)
		return
	}

	// Convertir en credentials WebAuthn (base64 decode)
	var credentials []webauthn.Credential
	for _, a := range authenticators {
		credID, _ := base64.StdEncoding.DecodeString(a.CredentialID)
		pubKey, _ := base64.StdEncoding.DecodeString(a.CredentialPublicKey)
		credentials = append(credentials, webauthn.Credential{
			ID:        credID,
			PublicKey: pubKey,
			Flags: webauthn.CredentialFlags{
				BackupEligible: a.BackupEligible,
				BackupState:    a.CredentialBackedUp,
			},
			Authenticator: webauthn.Authenticator{
				SignCount: uint32(a.Counter),
			},
		})
	}

	passkeyUser := &auth.PasskeyUser{
		ID:          user.ID,
		Email:       user.Email,
		Credentials: credentials,
	}

	options, sessionData, err := hookBeginRegistration(passkeyUser)
	if err != nil {
		serverError(w, r, "begin registration", err)
		return
	}

	// L3 fix : limiter au sous-arbre /api/passkey — réduit la surface
	// d'exfiltration et évite d'envoyer le challenge sur toutes les requêtes.
	setScopedCookie(w, "passkey_challenge", sessionData, 300, "/api/passkey") // 5 minutes

	jsonSuccess(w, options)
}

// PasskeyRegistrationFinish termine l'enregistrement d'une passkey
func PasskeyRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	// Récupérer la session
	cookie, err := r.Cookie("passkey_challenge")
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.session_expired", http.StatusBadRequest)
		return
	}

	// Parser la réponse WebAuthn
	var response protocol.CredentialCreationResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		clientErrorT(w, r, ErrValidation, "error.passkey_response_invalid", http.StatusBadRequest)
		return
	}

	parsedResponse, err := hookParseCCR(&response)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.passkey_response_invalid", http.StatusBadRequest)
		return
	}

	passkeyUser := &auth.PasskeyUser{
		ID:    user.ID,
		Email: user.Email,
	}

	credential, err := hookFinishRegistration(passkeyUser, cookie.Value, parsedResponse)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.passkey_registration_failed", http.StatusBadRequest)
		return
	}

	// Sauvegarder la passkey en BDD (base64 encode)
	transports, _ := json.Marshal(credential.Transport)
	err = hookCreateAuthenticator(
		base64.StdEncoding.EncodeToString(credential.ID),
		base64.StdEncoding.EncodeToString(credential.PublicKey),
		int(credential.Authenticator.SignCount),
		"multiDevice",
		credential.Flags.BackupState,
		credential.Flags.BackupEligible,
		string(transports),
		user.ID,
	)
	if err != nil {
		serverError(w, r, "save authenticator", err)
		return
	}

	clearScopedCookie(w, "passkey_challenge", "/api/passkey")

	hookLogAudit(user.ID, db.AuditPasskeyAdd, getClientIP(r), r.UserAgent())

	jsonSuccess(w, map[string]bool{"success": true})
}

// PasskeyLoginStart démarre l'authentification par passkey
func PasskeyLoginStart(w http.ResponseWriter, r *http.Request) {
	options, sessionData, err := hookBeginLogin()
	if err != nil {
		serverError(w, r, "begin login", err)
		return
	}

	// L3 fix : scope /api/passkey (le challenge n'est lu que par /api/passkey/login/finish).
	setScopedCookie(w, "passkey_auth_challenge", sessionData, 300, "/api/passkey") // 5 minutes

	jsonSuccess(w, options)
}

// PasskeyLoginFinish termine l'authentification par passkey
func PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	result := hookRateLimitCheck(clientIP, "login")
	if !result.Allowed {
		clientErrorT(w, r, ErrRateLimited, "error.rate_limited", http.StatusTooManyRequests)
		return
	}

	cookie, err := r.Cookie("passkey_auth_challenge")
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.session_expired", http.StatusBadRequest)
		return
	}

	// Callback pour récupérer l'utilisateur par credential ID (base64 encoded)
	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		// rawID est en bytes, on le convertit en base64 pour chercher en BDD
		credIDBase64 := base64.StdEncoding.EncodeToString(rawID)

		authenticator, err := hookGetAuthByCredentialID(credIDBase64)
		if err != nil {
			return nil, err
		}
		if authenticator == nil {
			return nil, fmt.Errorf("authenticator not found")
		}

		user, err := hookGetUserByID(authenticator.UserID)
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, fmt.Errorf("user not found")
		}

		email, _ := hookDecryptStr(user.EmailEncrypted)

		// Récupérer toutes les credentials de l'utilisateur (base64 decode)
		auths, _ := hookGetAuthenticatorsByUserID(user.ID)
		var credentials []webauthn.Credential
		for _, a := range auths {
			credID, _ := base64.StdEncoding.DecodeString(a.CredentialID)
			pubKey, _ := base64.StdEncoding.DecodeString(a.CredentialPublicKey)
			credentials = append(credentials, webauthn.Credential{
				ID:        credID,
				PublicKey: pubKey,
				Flags: webauthn.CredentialFlags{
					BackupEligible: a.BackupEligible,
					BackupState:    a.CredentialBackedUp,
				},
				Authenticator: webauthn.Authenticator{
					SignCount: uint32(a.Counter),
				},
			})
		}

		return &auth.PasskeyUser{
			ID:          user.ID,
			Email:       email,
			Credentials: credentials,
		}, nil
	}

	passkeyUser, credential, err := hookFinishLogin(cookie.Value, r, userHandler)
	if err != nil {
		clientErrorT(w, r, ErrAuthInvalid, "error.authentication_failed", http.StatusUnauthorized)
		return
	}

	// Mettre à jour le compteur (base64 encode credential ID)
	if err := hookUpdateAuthCounter(base64.StdEncoding.EncodeToString(credential.ID), int(credential.Authenticator.SignCount)); err != nil {
		slog.Warn("passkey: update counter", "err", err)
	}

	// Récupérer l'utilisateur complet
	user, err := hookGetUserByID(passkeyUser.ID)
	if err != nil || user == nil {
		serverError(w, r, "get user", fmt.Errorf("user %d not found", passkeyUser.ID))
		return
	}

	// Rate limiting par compte (protection brute-force distribué multi-IP)
	userIDStr := strconv.FormatInt(user.ID, 10)
	acctResult := hookRateLimitCheck(userIDStr, "loginAccount")
	if !acctResult.Allowed {
		clientErrorT(w, r, ErrRateLimited, "error.rate_limited", http.StatusTooManyRequests)
		return
	}

	// Générer le token JWT
	token, err := hookGenerateToken(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
	if err != nil {
		serverError(w, r, "generate token", err)
		return
	}

	clearScopedCookie(w, "passkey_auth_challenge", "/api/passkey")
	setSessionCookie(w, "session", token, 86400) // 24 heures

	hookRateLimitReset(clientIP, "login")
	hookRateLimitReset(userIDStr, "loginAccount")

	hookLogAudit(user.ID, db.AuditLoginSuccess, getClientIP(r), r.UserAgent())

	jsonSuccess(w, map[string]bool{"success": true})
}

// DeletePasskey supprime une passkey
func DeletePasskey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_id", http.StatusBadRequest)
		return
	}

	err = hookDeleteAuthenticator(id, user.ID)
	if err != nil {
		serverError(w, r, "delete authenticator", err)
		return
	}

	hookLogAudit(user.ID, db.AuditPasskeyRemove, getClientIP(r), r.UserAgent())

	w.WriteHeader(http.StatusNoContent)
}

// RenamePasskey renomme une passkey
func RenamePasskey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_id", http.StatusBadRequest)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_data", http.StatusBadRequest)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 100 {
		clientErrorT(w, r, ErrValidation, "error.passkey_name_invalid", http.StatusBadRequest)
		return
	}

	err = hookRenameAuthenticator(id, user.ID, req.Name)
	if err != nil {
		serverError(w, r, "rename authenticator", err)
		return
	}

	hookLogAudit(user.ID, db.AuditPasskeyRename, getClientIP(r), r.UserAgent())

	jsonSuccess(w, map[string]bool{"success": true})
}
