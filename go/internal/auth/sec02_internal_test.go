package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
)

func sec02Init(t *testing.T) {
	t.Helper()
	if len(jwtSecret) == 0 {
		InitJWT("test-jwt-secret-32bytes-padding!!")
	}
}

// SEC-02 : l'ancien format (SessionData base64 non signée) est refusé.
func TestSEC02_UnsignedSessionRejected(t *testing.T) {
	sec02Init(t)
	raw, _ := json.Marshal(webauthn.SessionData{Challenge: "forged", Expires: time.Now().Add(time.Hour)})
	if _, err := openPasskeySession(base64.StdEncoding.EncodeToString(raw), AudiencePasskeyLogin); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("session non signée acceptée : %v", err)
	}
}

// SEC-02 : un jeton d'enregistrement ne sert pas au login (et inversement).
func TestSEC02_WrongAudienceRejected(t *testing.T) {
	sec02Init(t)
	tok, _ := signPasskeySession(&webauthn.SessionData{Challenge: "aud-" + time.Now().String()}, AudiencePasskeyRegister)
	if _, err := openPasskeySession(tok, AudiencePasskeyLogin); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("audience croisée acceptée : %v", err)
	}
}

// SEC-02 : le challenge est à usage unique (rejeu refusé).
func TestSEC02_ReplayRejected(t *testing.T) {
	sec02Init(t)
	tok, _ := signPasskeySession(&webauthn.SessionData{Challenge: "replay-" + time.Now().String()}, AudiencePasskeyLogin)
	if _, err := openPasskeySession(tok, AudiencePasskeyLogin); err != nil {
		t.Fatalf("1re ouverture : %v", err)
	}
	if _, err := openPasskeySession(tok, AudiencePasskeyLogin); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("rejeu accepté : %v", err)
	}
}

// SEC-02 : jeton sans exp ou expiré refusé ; charge sd non décodable → erreur.
func TestSEC02_ExpiryAndPayload(t *testing.T) {
	sec02Init(t)
	sign := func(c *passkeySessionClaims) string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(jwtSecret)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	aud := jwt.ClaimStrings{AudiencePasskeyLogin}
	noExp := sign(&passkeySessionClaims{Session: json.RawMessage(`{}`), RegisteredClaims: jwt.RegisteredClaims{Audience: aud}})
	if _, err := openPasskeySession(noExp, AudiencePasskeyLogin); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("jeton sans exp accepté : %v", err)
	}
	expired := sign(&passkeySessionClaims{Session: json.RawMessage(`{}`), RegisteredClaims: jwt.RegisteredClaims{
		Audience: aud, ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}})
	if _, err := openPasskeySession(expired, AudiencePasskeyLogin); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("jeton expiré accepté : %v", err)
	}
	badSD := sign(&passkeySessionClaims{Session: json.RawMessage(`"str"`), RegisteredClaims: jwt.RegisteredClaims{
		Audience: aud, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}})
	if _, err := openPasskeySession(badSD, AudiencePasskeyLogin); err == nil || errors.Is(err, ErrInvalidToken) {
		t.Errorf("sd invalide : erreur de décodage attendue, got %v", err)
	}
}

// SEC-02 : les challenges expirés sont purgés de la table d'usage unique.
func TestSEC02_ConsumeChallengePrunes(t *testing.T) {
	usedChallengesMu.Lock()
	usedChallenges["old-challenge"] = time.Now().Add(-time.Second)
	usedChallengesMu.Unlock()
	if !consumeChallenge("new-"+time.Now().String(), time.Now().Add(time.Minute)) {
		t.Fatal("challenge neuf refusé")
	}
	usedChallengesMu.Lock()
	_, still := usedChallenges["old-challenge"]
	usedChallengesMu.Unlock()
	if still {
		t.Error("challenge expiré non purgé")
	}
}
