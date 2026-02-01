// Package auth - 2FA TOTP
package auth

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/url"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	totpIssuer = "Pilot Finance"
	totpDigits = 6
	totpPeriod = 30
)

// GenerateTOTPSecret génère un nouveau secret TOTP
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// GenerateTOTPURI génère l'URI otpauth:// pour le QR code
func GenerateTOTPURI(secret, email string) string {
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", totpIssuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", totpPeriod))

	return fmt.Sprintf("otpauth://totp/%s:%s?%s",
		url.PathEscape(totpIssuer),
		url.PathEscape(email),
		params.Encode(),
	)
}

// ValidateTOTP vérifie un code TOTP
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// ValidateTOTPWithWindow vérifie un code TOTP avec une fenêtre de tolérance
func ValidateTOTPWithWindow(secret, code string) (bool, error) {
	return totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      1, // Accepte 1 période avant/après
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
}
