// Package middleware contient les middlewares HTTP
package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

// sessionCacheEntry stores cached session data with TTL
type sessionCacheEntry struct {
	sessionVersion int
	emailEncrypted string
	emailVerified  bool
	expiresAt      time.Time
}

const sessionCacheTTL = 30 * time.Second

var (
	sessionCache   sync.Map // map[int64]sessionCacheEntry keyed by userID
)

type contextKey string

const UserContextKey contextKey = "user"

// getUserAuthData est une variable de fonction pour faciliter les tests.
var getUserAuthData = db.GetUserAuthData

// User représente l'utilisateur authentifié dans le contexte
type User struct {
	ID             int64
	Email          string
	Role           string
	SessionVersion int
	Language       string
	Currency       string
	EmailVerified  bool
}

// clearSessionCookie supprime le cookie de session
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireAuth vérifie que l'utilisateur est authentifié.
// Uses an in-memory cache (30s TTL) to avoid querying the DB on every request.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		claims, err := auth.ValidateToken(cookie.Value)
		if err != nil {
			// Token invalide ou expiré, supprimer le cookie
			clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Try the session cache first (30s TTL to reduce DB queries)
		var sv int
		var emailEncrypted string
		var emailVerified bool
		cacheHit := false

		if cached, ok := sessionCache.Load(claims.UserID); ok {
			entry := cached.(sessionCacheEntry)
			if time.Now().Before(entry.expiresAt) {
				sv = entry.sessionVersion
				emailEncrypted = entry.emailEncrypted
				emailVerified = entry.emailVerified
				cacheHit = true
			}
		}

		if !cacheHit {
			// Cache miss or expired: query DB and update cache
			var dbErr error
			sv, emailEncrypted, emailVerified, dbErr = getUserAuthData(claims.UserID)
			if dbErr != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			sessionCache.Store(claims.UserID, sessionCacheEntry{
				sessionVersion: sv,
				emailEncrypted: emailEncrypted,
				emailVerified:  emailVerified,
				expiresAt:      time.Now().Add(sessionCacheTTL),
			})
		}

		if sv != claims.SessionVersion {
			// Session invalidée (changement de mot de passe) ou utilisateur introuvable (sv==0)
			InvalidateSessionCache(claims.UserID)
			clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		email, _ := crypto.Decrypt(emailEncrypted)

		user := &User{
			ID:             claims.UserID,
			Email:          email,
			Role:           claims.Role,
			SessionVersion: claims.SessionVersion,
			Language:       claims.Language,
			Currency:       claims.Currency,
			EmailVerified:  emailVerified,
		}
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// InvalidateSessionCache removes a user's entry from the session cache.
// Should be called on logout or session invalidation.
func InvalidateSessionCache(userID int64) {
	sessionCache.Delete(userID)
}

// OptionalAuth tente de parser le JWT si présent, sans rediriger si absent ou invalide.
// Uses the same session cache as RequireAuth to avoid redundant DB queries.
func OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		claims, err := auth.ValidateToken(cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		var sv int
		var emailEncrypted string
		var emailVerified bool
		if cached, ok := sessionCache.Load(claims.UserID); ok {
			entry := cached.(sessionCacheEntry)
			if time.Now().Before(entry.expiresAt) {
				sv = entry.sessionVersion
				emailEncrypted = entry.emailEncrypted
				emailVerified = entry.emailVerified
			}
		}
		if emailEncrypted == "" {
			sv, emailEncrypted, emailVerified, err = getUserAuthData(claims.UserID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			sessionCache.Store(claims.UserID, sessionCacheEntry{
				sessionVersion: sv,
				emailEncrypted: emailEncrypted,
				emailVerified:  emailVerified,
				expiresAt:      time.Now().Add(sessionCacheTTL),
			})
		}

		if sv != claims.SessionVersion {
			next.ServeHTTP(w, r)
			return
		}
		email, _ := crypto.Decrypt(emailEncrypted)
		user := &User{
			ID:             claims.UserID,
			Email:          email,
			Role:           claims.Role,
			SessionVersion: claims.SessionVersion,
			Language:       claims.Language,
			Currency:       claims.Currency,
			EmailVerified:  emailVerified,
		}
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin vérifie que l'utilisateur est admin
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil || user.Role != "ADMIN" {
			w.Header().Set("X-Error-Code", "FORBIDDEN")
			http.Error(w, "Accès refusé", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUser récupère l'utilisateur du contexte
func GetUser(r *http.Request) *User {
	user, ok := r.Context().Value(UserContextKey).(*User)
	if !ok {
		return nil
	}
	return user
}
