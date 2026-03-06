package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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

	getUserAuthData = func(_ int64) (int, string, error) {
		return 0, "", errors.New("db unavailable")
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

	getUserAuthData = func(_ int64) (int, string, error) {
		return 0, "", errors.New("db unavailable")
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
	getUserAuthData = func(userID int64) (int, string, error) {
		callCount++
		return 1, "encrypted-email", nil
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
