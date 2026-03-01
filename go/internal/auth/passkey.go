// Package auth - Passkeys WebAuthn
package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var webAuthn *webauthn.WebAuthn

// marshalJSON est injectable pour les tests (couvre la branche d'erreur json.Marshal).
var marshalJSON = json.Marshal

// Hooks injectables pour les tests — couvrent les branches d'erreur des appels webauthn.
var (
	beginRegistrationFn = func(wa *webauthn.WebAuthn, user webauthn.User, opts ...webauthn.RegistrationOption) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
		return wa.BeginRegistration(user, opts...)
	}
	beginDiscoverableLoginFn = func(wa *webauthn.WebAuthn, opts ...webauthn.LoginOption) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
		return wa.BeginDiscoverableLogin(opts...)
	}
	createCredentialFn = func(wa *webauthn.WebAuthn, user webauthn.User, session webauthn.SessionData, response *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
		return wa.CreateCredential(user, session, response)
	}
	finishDiscoverableLoginFn = func(wa *webauthn.WebAuthn, handler webauthn.DiscoverableUserHandler, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
		return wa.FinishDiscoverableLogin(handler, session, r)
	}
)

// PasskeyUser implémente l'interface webauthn.User
type PasskeyUser struct {
	ID          int64
	Email       string
	Credentials []webauthn.Credential
}

func (u *PasskeyUser) WebAuthnID() []byte {
	return []byte(strconv.FormatInt(u.ID, 10))
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

func (u *PasskeyUser) WebAuthnIcon() string {
	return ""
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
	options, session, err := beginRegistrationFn(webAuthn, user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationPreferred,
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
		}),
	)
	if err != nil {
		return nil, "", err
	}

	// Sérialiser la session et encoder en base64 pour le cookie
	sessionData, err := marshalJSON(session)
	if err != nil {
		return nil, "", err
	}

	return options, base64.StdEncoding.EncodeToString(sessionData), nil
}

// FinishRegistration termine l'enregistrement d'une passkey
func FinishRegistration(user *PasskeyUser, sessionDataBase64 string, response *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
	// Décoder depuis base64
	sessionDataJSON, err := base64.StdEncoding.DecodeString(sessionDataBase64)
	if err != nil {
		return nil, err
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionDataJSON, &session); err != nil {
		return nil, err
	}

	credential, err := createCredentialFn(webAuthn, user, session, response)
	if err != nil {
		return nil, err
	}

	return credential, nil
}

// BeginLogin démarre l'authentification par passkey
func BeginLogin() (*protocol.CredentialAssertion, string, error) {
	options, session, err := beginDiscoverableLoginFn(webAuthn,
		webauthn.WithUserVerification(protocol.VerificationPreferred),
	)
	if err != nil {
		return nil, "", err
	}

	sessionData, err := marshalJSON(session)
	if err != nil {
		return nil, "", err
	}

	return options, base64.StdEncoding.EncodeToString(sessionData), nil
}

// FinishLogin termine l'authentification par passkey
// Utilise la nouvelle API go-webauthn v0.10+
func FinishLogin(sessionDataBase64 string, r *http.Request, userHandler func(rawID, userHandle []byte) (webauthn.User, error)) (*PasskeyUser, *webauthn.Credential, error) {
	// Décoder depuis base64
	sessionDataJSON, err := base64.StdEncoding.DecodeString(sessionDataBase64)
	if err != nil {
		return nil, nil, err
	}

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionDataJSON, &session); err != nil {
		return nil, nil, err
	}

	credential, err := finishDiscoverableLoginFn(webAuthn, userHandler, session, r)
	if err != nil {
		return nil, nil, err
	}

	// Récupérer l'utilisateur via le handler
	user, err := userHandler(credential.ID, nil)
	if err != nil {
		return nil, nil, err
	}

	passkeyUser, ok := user.(*PasskeyUser)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected user type: %T", user)
	}

	return passkeyUser, credential, nil
}
