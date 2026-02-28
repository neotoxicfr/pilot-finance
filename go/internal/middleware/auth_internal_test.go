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
