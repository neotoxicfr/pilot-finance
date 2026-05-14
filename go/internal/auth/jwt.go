// Package auth gère l'authentification et les sessions
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtSecret        []byte
	ErrInvalidToken  = errors.New("token invalide")
	ErrExpiredToken  = errors.New("token expiré")
	// parseWithClaimsFn est injectable pour les tests (couvre les branches mortes !ok || !token.Valid).
	parseWithClaimsFn = jwt.ParseWithClaims
)

// Claims représente les données du token JWT
type Claims struct {
	UserID         int64  `json:"id"`
	Role           string `json:"role"`
	SessionVersion int    `json:"sessionVersion"`
	Language       string `json:"language"`
	Currency       string `json:"currency"`
	jwt.RegisteredClaims
}

// InitJWT initialise la clé secrète JWT
func InitJWT(secret string) {
	jwtSecret = []byte(secret)
}

// GenerateToken génère un nouveau token JWT
func GenerateToken(userID int64, role, language, currency string, sessionVersion int) (string, error) {
	claims := &Claims{
		UserID:         userID,
		Role:           role,
		SessionVersion: sessionVersion,
		Language:       language,
		Currency:       currency,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateToken valide un token JWT et retourne les claims
func ValidateToken(tokenString string) (*Claims, error) {
	token, err := parseWithClaimsFn(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return jwtSecret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// Pending2FAClaims pour les tokens temporaires 2FA
type Pending2FAClaims struct {
	UserID int64 `json:"uid"`
	jwt.RegisteredClaims
}

// GeneratePending2FAToken génère un token temporaire pour le 2FA
func GeneratePending2FAToken(userID int64) (string, error) {
	claims := &Pending2FAClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidatePending2FAToken valide un token temporaire 2FA
func ValidatePending2FAToken(tokenString string) (int64, error) {
	token, err := parseWithClaimsFn(tokenString, &Pending2FAClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return jwtSecret, nil
	})

	if err != nil {
		return 0, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Pending2FAClaims)
	if !ok || !token.Valid {
		return 0, ErrInvalidToken
	}

	return claims.UserID, nil
}

// MFASetupClaims pour les tokens temporaires d'enrôlement MFA. Stocke le
// secret TOTP côté serveur (cookie signé HS256) pour éviter qu'un client
// malveillant n'envoie un secret de son choix au moment du /enable.
type MFASetupClaims struct {
	UserID int64  `json:"uid"`
	Secret string `json:"sec"`
	jwt.RegisteredClaims
}

// GenerateMFASetupToken signe un cookie contenant le secret TOTP fraîchement
// généré pour l'utilisateur. Expire en 5 minutes.
func GenerateMFASetupToken(userID int64, secret string) (string, error) {
	claims := &MFASetupClaims{
		UserID: userID,
		Secret: secret,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateMFASetupToken vérifie le cookie et retourne (userID, secret) si valide.
func ValidateMFASetupToken(tokenString string) (int64, string, error) {
	token, err := parseWithClaimsFn(tokenString, &MFASetupClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return jwtSecret, nil
	})

	if err != nil {
		return 0, "", ErrInvalidToken
	}

	claims, ok := token.Claims.(*MFASetupClaims)
	if !ok || !token.Valid {
		return 0, "", ErrInvalidToken
	}

	return claims.UserID, claims.Secret, nil
}
