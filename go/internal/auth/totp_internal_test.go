package auth

import (
	"errors"
	"testing"
)

// TestGenerateTOTPSecret_RandError covers the rand.Read error branch.
func TestGenerateTOTPSecret_RandError(t *testing.T) {
	orig := totpRandRead
	defer func() { totpRandRead = orig }()

	totpRandRead = func(b []byte) (int, error) {
		return 0, errors.New("rand unavailable")
	}

	_, err := GenerateTOTPSecret()
	if err == nil || err.Error() != "rand unavailable" {
		t.Errorf("want 'rand unavailable', got %v", err)
	}
}
