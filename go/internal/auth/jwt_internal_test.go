package auth

import (
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestValidateToken_TokenInvalid covers the !ok || !token.Valid true branch in ValidateToken.
func TestValidateToken_TokenInvalid(t *testing.T) {
	orig := parseWithClaimsFn
	defer func() { parseWithClaimsFn = orig }()

	// Return a token with Valid=false and correct Claims type → hits !token.Valid branch.
	parseWithClaimsFn = func(_ string, _ jwt.Claims, _ jwt.Keyfunc, _ ...jwt.ParserOption) (*jwt.Token, error) {
		return &jwt.Token{Valid: false, Claims: &Claims{}}, nil
	}

	_, err := ValidateToken("any-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("want ErrInvalidToken, got %v", err)
	}
}

// TestValidatePending2FAToken_TokenInvalid covers the !ok || !token.Valid branch.
func TestValidatePending2FAToken_TokenInvalid(t *testing.T) {
	orig := parseWithClaimsFn
	defer func() { parseWithClaimsFn = orig }()

	parseWithClaimsFn = func(_ string, _ jwt.Claims, _ jwt.Keyfunc, _ ...jwt.ParserOption) (*jwt.Token, error) {
		return &jwt.Token{Valid: false, Claims: &Pending2FAClaims{}}, nil
	}

	_, err := ValidatePending2FAToken("any-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("want ErrInvalidToken, got %v", err)
	}
}

// TestValidateMFASetupToken_TokenInvalid covers the !ok || !token.Valid branch
// in ValidateMFASetupToken (M3 fix).
func TestValidateMFASetupToken_TokenInvalid(t *testing.T) {
	orig := parseWithClaimsFn
	defer func() { parseWithClaimsFn = orig }()

	parseWithClaimsFn = func(_ string, _ jwt.Claims, _ jwt.Keyfunc, _ ...jwt.ParserOption) (*jwt.Token, error) {
		return &jwt.Token{Valid: false, Claims: &MFASetupClaims{}}, nil
	}

	_, _, err := ValidateMFASetupToken("any-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("want ErrInvalidToken, got %v", err)
	}
}
