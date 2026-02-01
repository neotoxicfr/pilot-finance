// Package auth gère l'authentification et les sessions
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	jwtSecret      []byte
	ErrInvalidToken = errors.New("token invalide")
	ErrExpiredToken = errors.New("token expiré")
)

// Claims représente les données du token JWT
type Claims struct {
	UserID         int64  `json:"id"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	SessionVersion int    `json:"sessionVersion"`
	jwt.RegisteredClaims
}

// InitJWT initialise la clé secrète JWT
func InitJWT(secret string) {
	jwtSecret = []byte(secret)
}

// GenerateToken génère un nouveau token JWT
func GenerateToken(userID int64, email, role string, sessionVersion int) (string, error) {
	claims := &Claims{
		UserID:         userID,
		Email:          email,
		Role:           role,
		SessionVersion: sessionVersion,
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
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
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
	token, err := jwt.ParseWithClaims(tokenString, &Pending2FAClaims{}, func(token *jwt.Token) (interface{}, error) {
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
