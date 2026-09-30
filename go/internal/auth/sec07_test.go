package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"pilot-finance/internal/auth"
)

// signTest signe des claims arbitraires avec le secret de test.
func signTest(t *testing.T, claims jwt.Claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// SEC-07 : un jeton pending_2fa ou mfa_setup n'est pas accepté comme session.
func TestSEC07_SessionRejectsOtherAudiences(t *testing.T) {
	p2fa, _ := auth.GeneratePending2FAToken(7)
	if _, err := auth.ValidateToken(p2fa); err == nil {
		t.Error("pending_2fa accepté comme session")
	}
	setup, _ := auth.GenerateMFASetupToken(7, "SECRET")
	if _, err := auth.ValidateToken(setup); err == nil {
		t.Error("mfa_setup accepté comme session")
	}
}

// SEC-07 : une session émise avant l'aud (legacy) reste valide.
func TestSEC07_LegacySessionWithoutAudienceAccepted(t *testing.T) {
	legacy := signTest(t, &auth.Claims{
		UserID: 9,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	c, err := auth.ValidateToken(legacy)
	if err != nil || c.UserID != 9 {
		t.Fatalf("session legacy refusée : %v", err)
	}
	tok, _ := auth.GenerateToken(3, "user", "fr", "EUR", 1)
	c, err = auth.ValidateToken(tok)
	if err != nil || len(c.Audience) != 1 || c.Audience[0] != auth.AudienceSession {
		t.Fatalf("session neuve sans aud session : %v %v", err, c)
	}
}

// SEC-07 : 2fa et mfa-setup exigent strictement leur audience.
func TestSEC07_ShortTokensRequireAudience(t *testing.T) {
	exp := jwt.NewNumericDate(time.Now().Add(time.Minute))
	noAud := signTest(t, &auth.MFASetupClaims{UserID: 1, Secret: "S", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp}})
	if _, err := auth.ValidatePending2FAToken(noAud); err == nil {
		t.Error("pending_2fa sans aud accepté")
	}
	if _, _, err := auth.ValidateMFASetupToken(noAud); err == nil {
		t.Error("mfa_setup sans aud accepté")
	}
	p2fa, _ := auth.GeneratePending2FAToken(1)
	if _, _, err := auth.ValidateMFASetupToken(p2fa); err == nil {
		t.Error("pending_2fa accepté comme mfa_setup")
	}
	setup, _ := auth.GenerateMFASetupToken(1, "S")
	if _, err := auth.ValidatePending2FAToken(setup); err == nil {
		t.Error("mfa_setup accepté comme pending_2fa")
	}
	sess, _ := auth.GenerateToken(1, "user", "fr", "EUR", 1)
	if _, err := auth.ValidatePending2FAToken(sess); err == nil {
		t.Error("session acceptée comme pending_2fa")
	}
}
