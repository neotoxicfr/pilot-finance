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

// parseClaims factorise la logique commune aux trois validateurs : appel du
// hook injectable parseWithClaimsFn, pin de l'algorithme HMAC dans la keyfunc,
// et vérification !ok || !token.Valid. Le type concret des claims est passé par
// l'appelant (Go 1.26 generics). En cas d'échec, l'erreur brute de parsing est
// remontée telle quelle (ErrInvalidToken pour la keyfunc, jwt.ErrTokenExpired
// pour un token expiré) afin que les appelants conservent leur sémantique.
func parseClaims[T jwt.Claims](tokenString string, claims T) (T, error) {
	token, err := parseWithClaimsFn(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return jwtSecret, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}

	parsed, ok := token.Claims.(T)
	if !ok || !token.Valid {
		var zero T
		return zero, ErrInvalidToken
	}

	return parsed, nil
}

// ValidateToken valide un token JWT et retourne les claims
func ValidateToken(tokenString string) (*Claims, error) {
	claims, err := parseClaims(tokenString, &Claims{})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
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
	claims, err := parseClaims(tokenString, &Pending2FAClaims{})
	if err != nil {
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
	claims, err := parseClaims(tokenString, &MFASetupClaims{})
	if err != nil {
		return 0, "", ErrInvalidToken
	}
	return claims.UserID, claims.Secret, nil
}
