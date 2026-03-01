package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// Hooks injectables pour les tests (couvrent les branches d'erreur des handlers passkey).
var (
	dbGetAuthsByUserIDFn = db.GetAuthenticatorsByUserID
	authBeginRegistrationFn = auth.BeginRegistration
	parseCCRFn = func(r *protocol.CredentialCreationResponse) (*protocol.ParsedCredentialCreationData, error) {
		return r.Parse()
	}
	authFinishRegistrationFn = auth.FinishRegistration
	dbCreateAuthFn           = db.CreateAuthenticator
	authBeginLoginFn         = auth.BeginLogin
	authFinishLoginFn        = auth.FinishLogin
	dbGetAuthByCredIDFn      = db.GetAuthenticatorByCredentialID
	dbGetUserByIDFn          = db.GetUserByID
	authGenerateTokenFn      = auth.GenerateToken
)

// PasskeyRegistrationStart démarre l'enregistrement d'une passkey
func PasskeyRegistrationStart(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Récupérer les passkeys existantes
	authenticators, err := dbGetAuthsByUserIDFn(user.ID)
	if err != nil {
		serverError(w, "get authenticators", err)
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

	options, sessionData, err := authBeginRegistrationFn(passkeyUser)
	if err != nil {
		srvError(w, "begin registration", err)
		return
	}

	setSessionCookie(w, "passkey_challenge", sessionData, 300) // 5 minutes

	jsonSuccess(w, options)
}

// PasskeyRegistrationFinish termine l'enregistrement d'une passkey
func PasskeyRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Récupérer la session
	cookie, err := r.Cookie("passkey_challenge")
	if err != nil {
		clientError(w, ErrValidation, "Session expirée", http.StatusBadRequest)
		return
	}

	// Parser la réponse WebAuthn
	var response protocol.CredentialCreationResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		clientError(w, ErrValidation, "Réponse invalide", http.StatusBadRequest)
		return
	}

	parsedResponse, err := parseCCRFn(&response)
	if err != nil {
		clientError(w, ErrValidation, "Réponse invalide", http.StatusBadRequest)
		return
	}

	passkeyUser := &auth.PasskeyUser{
		ID:    user.ID,
		Email: user.Email,
	}

	credential, err := authFinishRegistrationFn(passkeyUser, cookie.Value, parsedResponse)
	if err != nil {
		clientError(w, ErrValidation, "Enregistrement échoué", http.StatusBadRequest)
		return
	}

	// Sauvegarder la passkey en BDD (base64 encode)
	transports, _ := json.Marshal(credential.Transport)
	err = dbCreateAuthFn(
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
		srvError(w, "save authenticator", err)
		return
	}

	clearCookie(w, "passkey_challenge")

	db.LogAudit(user.ID, db.AuditPasskeyAdd, getClientIP(r), r.UserAgent())

	jsonSuccess(w, map[string]bool{"success": true})
}

// PasskeyLoginStart démarre l'authentification par passkey
func PasskeyLoginStart(w http.ResponseWriter, r *http.Request) {
	options, sessionData, err := authBeginLoginFn()
	if err != nil {
		srvError(w, "begin login", err)
		return
	}

	setSessionCookie(w, "passkey_auth_challenge", sessionData, 300) // 5 minutes

	jsonSuccess(w, options)
}

// PasskeyLoginFinish termine l'authentification par passkey
func PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("passkey_auth_challenge")
	if err != nil {
		clientError(w, ErrValidation, "Session expirée", http.StatusBadRequest)
		return
	}

	// Callback pour récupérer l'utilisateur par credential ID (base64 encoded)
	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		// rawID est en bytes, on le convertit en base64 pour chercher en BDD
		credIDBase64 := base64.StdEncoding.EncodeToString(rawID)

		authenticator, err := dbGetAuthByCredIDFn(credIDBase64)
		if err != nil || authenticator == nil {
			return nil, err
		}

		user, err := dbGetUserByIDFn(authenticator.UserID)
		if err != nil || user == nil {
			return nil, err
		}

		email, _ := crypto.Decrypt(user.EmailEncrypted)

		// Récupérer toutes les credentials de l'utilisateur (base64 decode)
		auths, _ := db.GetAuthenticatorsByUserID(user.ID)
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

	passkeyUser, credential, err := authFinishLoginFn(cookie.Value, r, userHandler)
	if err != nil {
		clientError(w, ErrAuthInvalid, "Authentification échouée", http.StatusUnauthorized)
		return
	}

	// Mettre à jour le compteur (base64 encode credential ID)
	hookUpdateAuthCounter(base64.StdEncoding.EncodeToString(credential.ID), int(credential.Authenticator.SignCount)) //nolint:errcheck

	// Récupérer l'utilisateur complet
	user, err := dbGetUserByIDFn(passkeyUser.ID)
	if err != nil {
		serverError(w, "get user", err)
		return
	}

	// Générer le token JWT
	token, err := authGenerateTokenFn(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
	if err != nil {
		serverError(w, "generate token", err)
		return
	}

	clearCookie(w, "passkey_auth_challenge")
	setSessionCookie(w, "session", token, 86400) // 24 heures

	jsonSuccess(w, map[string]bool{"success": true})
}

// DeletePasskey supprime une passkey
func DeletePasskey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
		return
	}

	err = hookDeleteAuthenticator(id, user.ID)
	if err != nil {
		srvError(w, "delete authenticator", err)
		return
	}

	db.LogAudit(user.ID, db.AuditPasskeyRemove, getClientIP(r), r.UserAgent())

	w.WriteHeader(http.StatusNoContent)
}

// RenamePasskey renomme une passkey
func RenamePasskey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	err = hookRenameAuthenticator(id, user.ID, req.Name)
	if err != nil {
		srvError(w, "rename authenticator", err)
		return
	}

	jsonSuccess(w, map[string]bool{"success": true})
}
