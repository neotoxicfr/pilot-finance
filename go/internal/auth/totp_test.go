package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"pilot-finance/internal/auth"
)

func TestGenerateTOTPSecret_NonEmpty(t *testing.T) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if secret == "" {
		t.Error("want non-empty secret")
	}
}

func TestGenerateTOTPSecret_ValidBase32(t *testing.T) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	const base32Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	for i, c := range secret {
		if !strings.ContainsRune(base32Chars, c) {
			t.Errorf("invalid character %q at position %d in secret", c, i)
		}
	}
}

func TestGenerateTOTPSecret_Unique(t *testing.T) {
	s1, _ := auth.GenerateTOTPSecret()
	s2, _ := auth.GenerateTOTPSecret()
	if s1 == s2 {
		t.Error("two generated secrets should differ")
	}
}

func TestGenerateTOTPURI_Format(t *testing.T) {
	uri := auth.GenerateTOTPURI("JBSWY3DPEHPK3PXP", "test@example.com")

	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("URI should start with otpauth://totp/, got %s", uri)
	}
	if !strings.Contains(uri, "JBSWY3DPEHPK3PXP") {
		t.Error("URI should contain the secret")
	}
	if !strings.Contains(uri, "issuer=") {
		t.Error("URI should contain issuer parameter")
	}
	if !strings.Contains(uri, "digits=6") {
		t.Error("URI should specify 6 digits")
	}
	if !strings.Contains(uri, "period=30") {
		t.Error("URI should specify 30s period")
	}
}

func TestGenerateTOTPURI_ContainsEmail(t *testing.T) {
	uri := auth.GenerateTOTPURI("TESTSECRET", "user@pilot.app")
	// Email is URL-encoded in path
	if !strings.Contains(uri, "pilot.app") {
		t.Errorf("URI should contain email domain, got %s", uri)
	}
}

func TestValidateTOTP_ValidCode(t *testing.T) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !auth.ValidateTOTP(secret, code) {
		t.Error("valid current TOTP code should be accepted")
	}
}

func TestValidateTOTP_NonNumericCode(t *testing.T) {
	secret, _ := auth.GenerateTOTPSecret()
	if auth.ValidateTOTP(secret, "abcdef") {
		t.Error("non-numeric code should be rejected")
	}
}

func TestValidateTOTP_EmptyCode(t *testing.T) {
	secret, _ := auth.GenerateTOTPSecret()
	if auth.ValidateTOTP(secret, "") {
		t.Error("empty code should be rejected")
	}
}

func TestValidateTOTP_WrongLength(t *testing.T) {
	secret, _ := auth.GenerateTOTPSecret()
	if auth.ValidateTOTP(secret, "123") {
		t.Error("3-digit code should be rejected (needs 6)")
	}
}

// TestValidateTOTP_PreviousWindow pins the Skew:1 tolerance: a code generated
// for the previous 30s window (now-30s) must still be accepted.
func TestValidateTOTP_PreviousWindow(t *testing.T) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now().Add(-30*time.Second))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if !auth.ValidateTOTP(secret, code) {
		t.Error("code from previous window (now-30s) should be accepted with Skew:1")
	}
}

// TestValidateTOTP_FarOutWindow pins that a code from far outside the tolerance
// window (now-120s, i.e. 4 windows back) is rejected.
func TestValidateTOTP_FarOutWindow(t *testing.T) {
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now().Add(-120*time.Second))
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if auth.ValidateTOTP(secret, code) {
		t.Error("code from far-out window (now-120s) should be rejected")
	}
}
