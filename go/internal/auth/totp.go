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

// totpRandRead est injectable pour les tests (couvre la branche d'erreur rand.Read).
var totpRandRead = rand.Read

// GenerateTOTPSecret génère un nouveau secret TOTP
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := totpRandRead(secret); err != nil {
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

// ValidateTOTP vérifie un code TOTP. Les paramètres de validation sont rendus
// explicites (totp.ValidateCustom) pour rester alignés sur GenerateTOTPURI :
// période 30s, 6 chiffres, SHA1. Skew:1 (±1 fenêtre) est le défaut pquerna déjà
// en vigueur — la tolérance d'une fenêtre couvre la dérive d'horloge.
func ValidateTOTP(secret, code string) bool {
	valid, _ := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid
}

