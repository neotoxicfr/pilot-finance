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

var sessionCache sync.Map // map[int64]sessionCacheEntry keyed by userID

const sessionCacheCleanupInterval = 5 * time.Minute

// sessionCachePackageStop arrête la goroutine cleanup démarrée par init().
// Réservée pour tests (close uniquement). Les tests veulant valider la boucle
// utilisent leur propre stop channel via sessionCacheCleanupLoop(stop, interval).
var sessionCachePackageStop = make(chan struct{})

func init() {
	go sessionCacheCleanupLoop(sessionCachePackageStop, sessionCacheCleanupInterval)
}

// sessionCacheCleanupLoop purge périodiquement les entrées expirées pour
// éviter une croissance non bornée de la map sur des serveurs long-running.
// H1 fix : sans ça, chaque userID utilisé reste indéfiniment en mémoire.
// Stop et interval sont passés en paramètres pour éviter toute race sur les
// variables globales (-race) — les tests peuvent lancer leur propre instance.
func sessionCacheCleanupLoop(stop <-chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			PurgeExpiredSessionCache()
		case <-stop:
			return
		}
	}
}

// PurgeExpiredSessionCache supprime les entrées expirées de la session cache.
// Appelée périodiquement par sessionCacheCleanupLoop ; exposée pour les tests.
func PurgeExpiredSessionCache() {
	now := time.Now()
	sessionCache.Range(func(key, value any) bool {
		if entry, ok := value.(sessionCacheEntry); ok && entry.expiresAt.Before(now) {
			sessionCache.Delete(key)
		}
		return true
	})
}

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

// loadSessionData resolves the auth data for a user, going through the session
// cache first (30s TTL) and falling back to the DB on a miss/expiry.
//
// Return contract (shared by RequireAuth and OptionalAuth so both apply
// identical cache semantics):
//   - hit:   a live cache entry was used (no DB query was issued).
//   - found: the user exists. A valid user always has sessionVersion >= 1
//     (the column defaults to 1), so getUserAuthData returning (0,"",false,nil)
//     means "user not found / deleted" — that phantom row is NOT cached and
//     found is false.
//   - err:   a real DB error occurred (distinct from "user not found").
//
// When found is false, the returned entry is the zero value and must not be
// trusted; callers must treat it as "user gone" explicitly.
func loadSessionData(userID int64) (entry sessionCacheEntry, hit, found bool, err error) {
	if cached, ok := sessionCache.Load(userID); ok {
		cachedEntry := cached.(sessionCacheEntry)
		if time.Now().Before(cachedEntry.expiresAt) {
			// Only live entries for real users are ever stored (see below),
			// so a cache hit is always a valid, found user.
			return cachedEntry, true, true, nil
		}
	}

	sv, emailEncrypted, emailVerified, dbErr := getUserAuthData(userID)
	if dbErr != nil {
		return sessionCacheEntry{}, false, false, dbErr
	}
	// sessionVersion == 0 is impossible for a real row (DEFAULT 1); it is the
	// sentinel getUserAuthData returns for sql.ErrNoRows. Surface "user gone"
	// distinctly and never cache the phantom sv=0/email="" row.
	if sv < 1 {
		return sessionCacheEntry{}, false, false, nil
	}

	entry = sessionCacheEntry{
		sessionVersion: sv,
		emailEncrypted: emailEncrypted,
		emailVerified:  emailVerified,
		expiresAt:      time.Now().Add(sessionCacheTTL),
	}
	sessionCache.Store(userID, entry)
	return entry, false, true, nil
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

		entry, _, found, dbErr := loadSessionData(claims.UserID)
		if dbErr != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !found || entry.sessionVersion != claims.SessionVersion {
			// Utilisateur introuvable (supprimé) ou session invalidée
			// (changement de mot de passe). Dans les deux cas on déconnecte.
			InvalidateSessionCache(claims.UserID)
			clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		email, _ := crypto.Decrypt(entry.emailEncrypted)

		user := &User{
			ID:             claims.UserID,
			Email:          email,
			Role:           claims.Role,
			SessionVersion: claims.SessionVersion,
			Language:       claims.Language,
			Currency:       claims.Currency,
			EmailVerified:  entry.emailVerified,
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

		entry, _, found, dbErr := loadSessionData(claims.UserID)
		if dbErr != nil || !found || entry.sessionVersion != claims.SessionVersion {
			// DB error, utilisateur introuvable (supprimé) ou session
			// invalidée : on continue sans utilisateur dans le contexte.
			next.ServeHTTP(w, r)
			return
		}
		email, _ := crypto.Decrypt(entry.emailEncrypted)
		user := &User{
			ID:             claims.UserID,
			Email:          email,
			Role:           claims.Role,
			SessionVersion: claims.SessionVersion,
			Language:       claims.Language,
			Currency:       claims.Currency,
			EmailVerified:  entry.emailVerified,
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
