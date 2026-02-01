package handlers

import (
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

// PasskeyRegistrationStart démarre l'enregistrement d'une passkey
func PasskeyRegistrationStart(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Récupérer les passkeys existantes
	authenticators, err := db.GetAuthenticatorsByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Convertir en credentials WebAuthn
	var credentials []webauthn.Credential
	for _, a := range authenticators {
		credentials = append(credentials, webauthn.Credential{
			ID:        []byte(a.CredentialID),
			PublicKey: []byte(a.CredentialPublicKey),
		})
	}

	passkeyUser := &auth.PasskeyUser{
		ID:          user.ID,
		Email:       user.Email,
		Credentials: credentials,
	}

	options, sessionData, err := auth.BeginRegistration(passkeyUser)
	if err != nil {
		http.Error(w, "Erreur WebAuthn", http.StatusInternalServerError)
		return
	}

	// Stocker la session dans un cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "passkey_challenge",
		Value:    sessionData,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

// PasskeyRegistrationFinish termine l'enregistrement d'une passkey
func PasskeyRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Récupérer la session
	cookie, err := r.Cookie("passkey_challenge")
	if err != nil {
		http.Error(w, "Session expirée", http.StatusBadRequest)
		return
	}

	// Parser la réponse WebAuthn
	var response protocol.CredentialCreationResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil {
		http.Error(w, "Réponse invalide", http.StatusBadRequest)
		return
	}

	parsedResponse, err := response.Parse()
	if err != nil {
		http.Error(w, "Réponse invalide", http.StatusBadRequest)
		return
	}

	passkeyUser := &auth.PasskeyUser{
		ID:    user.ID,
		Email: user.Email,
	}

	credential, err := auth.FinishRegistration(passkeyUser, cookie.Value, parsedResponse)
	if err != nil {
		http.Error(w, "Enregistrement échoué", http.StatusBadRequest)
		return
	}

	// Sauvegarder la passkey en BDD
	transports, _ := json.Marshal(credential.Transport)
	err = db.CreateAuthenticator(
		string(credential.ID),
		string(credential.PublicKey),
		int(credential.Authenticator.SignCount),
		"multiDevice",
		credential.Flags.BackupState,
		string(transports),
		user.ID,
	)
	if err != nil {
		http.Error(w, "Erreur sauvegarde", http.StatusInternalServerError)
		return
	}

	// Supprimer le cookie de challenge
	http.SetCookie(w, &http.Cookie{
		Name:   "passkey_challenge",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// PasskeyLoginStart démarre l'authentification par passkey
func PasskeyLoginStart(w http.ResponseWriter, r *http.Request) {
	options, sessionData, err := auth.BeginLogin()
	if err != nil {
		http.Error(w, "Erreur WebAuthn", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "passkey_auth_challenge",
		Value:    sessionData,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

// PasskeyLoginFinish termine l'authentification par passkey
func PasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("passkey_auth_challenge")
	if err != nil {
		http.Error(w, "Session expirée", http.StatusBadRequest)
		return
	}

	// Callback pour récupérer l'utilisateur par credential ID
	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		authenticator, err := db.GetAuthenticatorByCredentialID(string(rawID))
		if err != nil || authenticator == nil {
			return nil, err
		}

		user, err := db.GetUserByID(authenticator.UserID)
		if err != nil || user == nil {
			return nil, err
		}

		email, _ := crypto.Decrypt(user.EmailEncrypted)

		// Récupérer toutes les credentials de l'utilisateur
		auths, _ := db.GetAuthenticatorsByUserID(user.ID)
		var credentials []webauthn.Credential
		for _, a := range auths {
			credentials = append(credentials, webauthn.Credential{
				ID:        []byte(a.CredentialID),
				PublicKey: []byte(a.CredentialPublicKey),
			})
		}

		return &auth.PasskeyUser{
			ID:          user.ID,
			Email:       email,
			Credentials: credentials,
		}, nil
	}

	passkeyUser, credential, err := auth.FinishLogin(cookie.Value, r, userHandler)
	if err != nil {
		http.Error(w, "Authentification échouée", http.StatusUnauthorized)
		return
	}

	// Mettre à jour le compteur
	db.UpdateAuthenticatorCounter(string(credential.ID), int(credential.Authenticator.SignCount))

	// Récupérer l'utilisateur complet
	user, err := db.GetUserByID(passkeyUser.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Générer le token JWT
	token, err := auth.GenerateToken(user.ID, passkeyUser.Email, user.Role, user.SessionVersion)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Supprimer le cookie de challenge
	http.SetCookie(w, &http.Cookie{
		Name:   "passkey_auth_challenge",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	// Définir le cookie de session
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// DeletePasskey supprime une passkey
func DeletePasskey(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	err = db.DeleteAuthenticator(id, user.ID)
	if err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
