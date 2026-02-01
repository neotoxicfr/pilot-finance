// Package auth - Passkeys WebAuthn
package auth

import (
	"encoding/json"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var webAuthn *webauthn.WebAuthn

// PasskeyUser implémente l'interface webauthn.User
type PasskeyUser struct {
	ID          int64
	Email       string
	Credentials []webauthn.Credential
}

func (u *PasskeyUser) WebAuthnID() []byte {
	return []byte(string(rune(u.ID)))
}

func (u *PasskeyUser) WebAuthnName() string {
	return u.Email
}

func (u *PasskeyUser) WebAuthnDisplayName() string {
	return u.Email
}

func (u *PasskeyUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

// InitWebAuthn initialise le module WebAuthn
func InitWebAuthn(rpID, rpOrigin, rpName string) error {
	var err error
	webAuthn, err = webauthn.New(&webauthn.Config{
		RPDisplayName: rpName,
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce:    true,
				Timeout:    time.Minute * 5,
				TimeoutUVD: time.Minute * 5,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce:    true,
				Timeout:    time.Minute * 5,
				TimeoutUVD: time.Minute * 5,
			},
		},
	})
	return err
}

// BeginRegistration démarre l'enregistrement d'une passkey
func BeginRegistration(user *PasskeyUser) (*protocol.CredentialCreation, string, error) {
	options, session, err := webAuthn.BeginRegistration(user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationPreferred,
		}),
	)
	if err != nil {
		return nil, "", err
	}

	// Sérialiser la session pour stockage en cookie
	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, "", err
	}

	return options, string(sessionData), nil
}

// FinishRegistration termine l'enregistrement d'une passkey
func FinishRegistration(user *PasskeyUser, sessionDataJSON string, response *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionDataJSON), &session); err != nil {
		return nil, err
	}

	credential, err := webAuthn.CreateCredential(user, session, response)
	if err != nil {
		return nil, err
	}

	return credential, nil
}

// BeginLogin démarre l'authentification par passkey
func BeginLogin() (*protocol.CredentialAssertion, string, error) {
	options, session, err := webAuthn.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationPreferred),
	)
	if err != nil {
		return nil, "", err
	}

	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, "", err
	}

	return options, string(sessionData), nil
}

// FinishLogin termine l'authentification par passkey
func FinishLogin(sessionDataJSON string, response *protocol.ParsedCredentialAssertionData, userHandler func(rawID []byte) (*PasskeyUser, error)) (*PasskeyUser, *webauthn.Credential, error) {
	var session webauthn.SessionData
	if err := json.Unmarshal([]byte(sessionDataJSON), &session); err != nil {
		return nil, nil, err
	}

	user, credential, err := webAuthn.FinishDiscoverableLogin(
		func(rawID, userHandle []byte) (webauthn.User, error) {
			return userHandler(rawID)
		},
		session,
		response,
	)
	if err != nil {
		return nil, nil, err
	}

	passkeyUser, ok := user.(*PasskeyUser)
	if !ok {
		return nil, nil, err
	}

	return passkeyUser, credential, nil
}
