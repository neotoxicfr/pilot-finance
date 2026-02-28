package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"pilot-finance/internal/auth"
)

const testJWTSecret = "test-jwt-secret-32bytes-padding!!"

func init() {
	auth.InitJWT(testJWTSecret)
}

func TestGenerateAndValidateToken(t *testing.T) {
	token, err := auth.GenerateToken(42, "user", "fr", "EUR", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("UserID: want 42, got %d", claims.UserID)
	}
	if claims.Role != "user" {
		t.Errorf("Role: want user, got %s", claims.Role)
	}
	if claims.Language != "fr" {
		t.Errorf("Language: want fr, got %s", claims.Language)
	}
	if claims.Currency != "EUR" {
		t.Errorf("Currency: want EUR, got %s", claims.Currency)
	}
	if claims.SessionVersion != 1 {
		t.Errorf("SessionVersion: want 1, got %d", claims.SessionVersion)
	}
}

func TestValidateToken_InvalidString(t *testing.T) {
	_, err := auth.ValidateToken("not.a.valid.jwt")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("want ErrInvalidToken, got %v", err)
	}
}

func TestValidateToken_Empty(t *testing.T) {
	_, err := auth.ValidateToken("")
	if err == nil {
		t.Error("want error for empty token")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	// Craft an expired token signed with the same secret
	claims := &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	_, err = auth.ValidateToken(signed)
	if !errors.Is(err, auth.ErrExpiredToken) {
		t.Errorf("want ErrExpiredToken, got %v", err)
	}
}

func TestValidateToken_WrongAlgorithm(t *testing.T) {
	// Token with "none" algorithm header — should be rejected
	_, err := auth.ValidateToken("eyJhbGciOiJub25lIn0.eyJpZCI6MX0.")
	if err == nil {
		t.Error("want error for none-algorithm token")
	}
}

func TestGeneratePending2FAToken_RoundTrip(t *testing.T) {
	token, err := auth.GeneratePending2FAToken(99)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}
	if token == "" {
		t.Fatal("want non-empty token")
	}

	userID, err := auth.ValidatePending2FAToken(token)
	if err != nil {
		t.Fatalf("ValidatePending2FAToken: %v", err)
	}
	if userID != 99 {
		t.Errorf("want userID 99, got %d", userID)
	}
}

func TestValidatePending2FAToken_InvalidToken(t *testing.T) {
	_, err := auth.ValidatePending2FAToken("garbage")
	if err == nil {
		t.Error("want error for invalid 2FA token")
	}
}

func TestValidatePending2FAToken_ExpiredToken(t *testing.T) {
	claims := &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(-10 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-15 * time.Minute)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	_, err = auth.ValidatePending2FAToken(signed)
	if err == nil {
		t.Error("want error for expired 2FA token")
	}
}

func TestGenerateToken_MultipleUsers_Independent(t *testing.T) {
	tok1, _ := auth.GenerateToken(1, "user", "fr", "EUR", 1)
	tok2, _ := auth.GenerateToken(2, "ADMIN", "en", "USD", 3)

	c1, err := auth.ValidateToken(tok1)
	if err != nil {
		t.Fatalf("ValidateToken user1: %v", err)
	}
	c2, err := auth.ValidateToken(tok2)
	if err != nil {
		t.Fatalf("ValidateToken user2: %v", err)
	}

	if c1.UserID == c2.UserID {
		t.Error("tokens should have different userIDs")
	}
	if c1.Role == c2.Role {
		t.Error("tokens should have different roles")
	}
}
