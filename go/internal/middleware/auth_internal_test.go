package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pilot-finance/internal/auth"
)

const (
	internalJWTKey = "internal-test-jwt-secret-32chars!"
)

func setupInternalAuth(t *testing.T) func() {
	t.Helper()
	auth.InitJWT(internalJWTKey)
	orig := getUserAuthData
	return func() { getUserAuthData = orig }
}

// TestRequireAuth_DBError covers the err != nil path in db.GetUserAuthData.
func TestRequireAuth_DBError_Redirects(t *testing.T) {
	restore := setupInternalAuth(t)
	defer restore()

	getUserAuthData = func(_ int64) (int, string, bool, error) {
		return 0, "", false, errors.New("db unavailable")
	}

	token, err := auth.GenerateToken(1, "user", "fr", "EUR", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 on DB error, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("want redirect to /login, got %s", loc)
	}
}

// TestOptionalAuth_DBError covers the err != nil path in OptionalAuth's getUserAuthData.
func TestOptionalAuth_DBError_NoUserInContext(t *testing.T) {
	restore := setupInternalAuth(t)
	defer restore()

	getUserAuthData = func(_ int64) (int, string, bool, error) {
		return 0, "", false, errors.New("db unavailable")
	}

	token, err := auth.GenerateToken(1, "user", "fr", "EUR", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	var gotUser *User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = GetUser(r)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	OptionalAuth(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotUser != nil {
		t.Error("user should be nil on DB error in OptionalAuth")
	}
}

// TestRequireAuth_SessionCacheHit covers the cache hit path in RequireAuth.
// On the second request with the same token, the session data should come from
// the in-memory cache rather than calling getUserAuthData again.
func TestRequireAuth_SessionCacheHit(t *testing.T) {
	restore := setupInternalAuth(t)
	defer restore()

	// Track how many times getUserAuthData is called
	callCount := 0
	getUserAuthData = func(userID int64) (int, string, bool, error) {
		callCount++
		return 1, "encrypted-email", true, nil
	}

	token, err := auth.GenerateToken(42, "user", "fr", "EUR", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := GetUser(r)
		if u == nil {
			t.Error("user should be set in context")
		}
		w.WriteHeader(http.StatusOK)
	})

	// First request: cache miss → getUserAuthData called
	req1 := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req1.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr1 := httptest.NewRecorder()

	// Clear the cache to ensure a clean start
	InvalidateSessionCache(42)

	RequireAuth(handler).ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", rr1.Code)
	}
	if callCount != 1 {
		t.Fatalf("first request: want 1 DB call, got %d", callCount)
	}

	// Second request: cache hit → getUserAuthData should NOT be called again
	req2 := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req2.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr2 := httptest.NewRecorder()

	RequireAuth(handler).ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request: want 200, got %d", rr2.Code)
	}
	if callCount != 1 {
		t.Errorf("second request: want 1 total DB call (cache hit), got %d", callCount)
	}

	// Clean up
	InvalidateSessionCache(42)
}

// TestSessionCacheCleanupLoop_TickerPurges (H1 fix) : démarre une goroutine
// dédiée avec un intervalle court, dépose une entrée expirée, attend un tick,
// et vérifie qu'elle est purgée. Couvre la branche ticker.C du select.
func TestSessionCacheCleanupLoop_TickerPurges(t *testing.T) {
	// Stop channel local pour éviter toute race sur le globaux (-race detector).
	stop := make(chan struct{})
	defer close(stop)

	go sessionCacheCleanupLoop(stop, 20*time.Millisecond)

	sessionCache.Store(int64(2001), sessionCacheEntry{
		sessionVersion: 1,
		emailEncrypted: "z",
		emailVerified:  true,
		expiresAt:      time.Now().Add(-1 * time.Hour),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, ok := sessionCache.Load(int64(2001)); !ok {
			return // purged
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("ticker did not purge expired entry within deadline")
}

// TestPurgeExpiredSessionCache_RemovesExpired (H1 fix) : vérifie que la
// fonction de purge supprime les entrées expirées et conserve les valides.
func TestPurgeExpiredSessionCache_RemovesExpired(t *testing.T) {
	// Entrée déjà expirée
	sessionCache.Store(int64(1001), sessionCacheEntry{
		sessionVersion: 1,
		emailEncrypted: "x",
		emailVerified:  true,
		expiresAt:      time.Now().Add(-1 * time.Hour),
	})
	// Entrée encore valide
	sessionCache.Store(int64(1002), sessionCacheEntry{
		sessionVersion: 1,
		emailEncrypted: "y",
		emailVerified:  true,
		expiresAt:      time.Now().Add(1 * time.Hour),
	})

	PurgeExpiredSessionCache()

	if _, ok := sessionCache.Load(int64(1001)); ok {
		t.Error("expired entry should have been purged")
	}
	if _, ok := sessionCache.Load(int64(1002)); !ok {
		t.Error("valid entry should still be present")
	}
	sessionCache.Delete(int64(1002))
}
