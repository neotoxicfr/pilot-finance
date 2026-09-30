// Package auth - Passkeys WebAuthn
package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
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

// SEC-02 : la SessionData WebAuthn (challenge, userVerification, expiration)
// voyageait en base64 brut dans un cookie : un client pouvait la forger avec
// le challenge d'une assertion déjà observée et rejouer celle-ci. Elle est
// désormais signée (HS256, secret d'auth, aud dédiée, 5 min) et son challenge
// est à usage unique côté serveur.
const passkeySessionTTL = 5 * time.Minute

// passkeySessionClaims porte la SessionData sérialisée dans un JWT signé.
type passkeySessionClaims struct {
	Session json.RawMessage `json:"sd"`
	jwt.RegisteredClaims
}

// usedChallenges retient les challenges déjà consommés jusqu'à leur expiration.
var (
	usedChallengesMu sync.Mutex
	usedChallenges   = map[string]time.Time{}
)

// consumeChallenge marque le challenge comme utilisé ; false s'il l'était déjà.
// Purge au passage les entrées expirées (la table reste bornée par le TTL).
func consumeChallenge(challenge string, expires time.Time) bool {
	usedChallengesMu.Lock()
	defer usedChallengesMu.Unlock()
	now := time.Now()
	for c, exp := range usedChallenges {
		if now.After(exp) {
			delete(usedChallenges, c)
		}
	}
	if _, seen := usedChallenges[challenge]; seen {
		return false
	}
	usedChallenges[challenge] = expires
	return true
}

// signPasskeySession sérialise et signe la SessionData pour l'audience donnée.
func signPasskeySession(session *webauthn.SessionData, audience string) (string, error) {
	data, err := marshalJSON(session)
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := &passkeySessionClaims{
		Session: data,
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(now.Add(passkeySessionTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
}

// openPasskeySession vérifie signature, audience et expiration, puis consomme
// le challenge (usage unique) avant de rendre la SessionData.
func openPasskeySession(token, audience string) (webauthn.SessionData, error) {
	var session webauthn.SessionData
	claims, err := parseClaims(token, &passkeySessionClaims{},
		jwt.WithAudience(audience), jwt.WithExpirationRequired())
	if err != nil {
		return session, ErrInvalidToken
	}
	if err := json.Unmarshal(claims.Session, &session); err != nil {
		return session, err
	}
	if !consumeChallenge(session.Challenge, claims.ExpiresAt.Time) {
		return session, ErrInvalidToken
	}
	return session, nil
}

// BeginRegistration démarre l'enregistrement d'une passkey
func BeginRegistration(user *PasskeyUser) (*protocol.CredentialCreation, string, error) {
	options, session, err := beginRegistrationFn(webAuthn, user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			// SEC-09 : PIN/biométrie exigés, sinon une clé « simple toucher »
			// suffirait à ouvrir une session qui court-circuite mot de passe et 2FA.
			UserVerification: protocol.VerificationRequired,
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
		}),
	)
	if err != nil {
		return nil, "", err
	}

	token, err := signPasskeySession(session, AudiencePasskeyRegister)
	if err != nil {
		return nil, "", err
	}
	return options, token, nil
}

// FinishRegistration termine l'enregistrement d'une passkey
func FinishRegistration(user *PasskeyUser, sessionToken string, response *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
	session, err := openPasskeySession(sessionToken, AudiencePasskeyRegister)
	if err != nil {
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
		// SEC-09 : vérification utilisateur exigée (flag UV contrôlé au Finish).
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		return nil, "", err
	}

	token, err := signPasskeySession(session, AudiencePasskeyLogin)
	if err != nil {
		return nil, "", err
	}
	return options, token, nil
}

// FinishLogin termine l'authentification par passkey
// Utilise la nouvelle API go-webauthn v0.10+
func FinishLogin(sessionToken string, r *http.Request, userHandler func(rawID, userHandle []byte) (webauthn.User, error)) (*PasskeyUser, *webauthn.Credential, error) {
	session, err := openPasskeySession(sessionToken, AudiencePasskeyLogin)
	if err != nil {
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
