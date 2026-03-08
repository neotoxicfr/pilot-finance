package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
)

type nonceContextKey struct{}

var randReader io.Reader = rand.Reader

// GenerateNonce génère un nonce aléatoire de 16 octets (base64url, ~22 chars)
func GenerateNonce() string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(randReader, b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// WithNonce stocke le nonce dans le contexte de la requête
func WithNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, nonceContextKey{}, nonce)
}

// GetNonce récupère le nonce depuis le contexte de la requête
func GetNonce(r *http.Request) string {
	nonce, _ := r.Context().Value(nonceContextKey{}).(string)
	return nonce
}
