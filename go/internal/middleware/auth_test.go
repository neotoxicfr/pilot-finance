package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

const (
	mwEncKey   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	mwBlindKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	mwJWTKey   = "test-jwt-secret-for-middleware-32!"
)

func setupMW(t *testing.T) func() {
	t.Helper()
	crypto.ResetForTest()
	db.ResetForTest()
	if err := crypto.Init(mwEncKey, mwBlindKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
	auth.InitJWT(mwJWTKey)
	dir := t.TempDir()
	if err := db.Init(db.Config{Path: dir + "/mw_test.db"}); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	return func() { db.Close() }
}

func createMWUser(t *testing.T) int64 {
	t.Helper()
	enc, err := crypto.Encrypt("mw@example.com")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	bi := crypto.ComputeBlindIndex("mw@example.com")
	id, err := db.CreateUser(enc, bi, "bcrypt-hash", "user")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// --- RequireAuth ---

func TestRequireAuth_NoCookie_Redirects(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()

	middleware.RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("want Location /login, got %s", loc)
	}
}

func TestRequireAuth_InvalidJWT_Redirects(t *testing.T) {
	defer setupMW(t)()

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "not.valid.jwt"})
	rr := httptest.NewRecorder()

	middleware.RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

func TestRequireAuth_SessionVersionMismatch_Redirects(t *testing.T) {
	defer setupMW(t)()
	userID := createMWUser(t)

	// Wrong session version
	token, err := auth.GenerateToken(userID, "user", "fr", "EUR", 999)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	middleware.RequireAuth(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 on session mismatch, got %d", rr.Code)
	}
}

func TestRequireAuth_ValidSession_PassesUser(t *testing.T) {
	defer setupMW(t)()
	userID := createMWUser(t)

	sv, _, err := db.GetUserAuthData(userID)
	if err != nil {
		t.Fatalf("GetUserAuthData: %v", err)
	}

	token, err := auth.GenerateToken(userID, "user", "fr", "EUR", sv)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	var gotUser *middleware.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.GetUser(r)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("user should be set in context")
	}
	if gotUser.ID != userID {
		t.Errorf("user.ID: want %d, got %d", userID, gotUser.ID)
	}
	if gotUser.Role != "user" {
		t.Errorf("user.Role: want user, got %s", gotUser.Role)
	}
}

// --- OptionalAuth ---

func TestOptionalAuth_NoCookie_NoUserInContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	var gotUser *middleware.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.GetUser(r)
		w.WriteHeader(http.StatusOK)
	})

	middleware.OptionalAuth(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotUser != nil {
		t.Error("user should be nil with no cookie")
	}
}

func TestOptionalAuth_InvalidJWT_NoUserInContext(t *testing.T) {
	defer setupMW(t)()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "garbage"})
	rr := httptest.NewRecorder()

	var gotUser *middleware.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.GetUser(r)
		w.WriteHeader(http.StatusOK)
	})

	middleware.OptionalAuth(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotUser != nil {
		t.Error("user should be nil with invalid JWT")
	}
}

func TestOptionalAuth_SessionVersionMismatch_NoUserInContext(t *testing.T) {
	defer setupMW(t)()

	// Token for non-existent user: DB returns sv=0, token has sv=1 → mismatch
	token, err := auth.GenerateToken(999999, "user", "fr", "EUR", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	var gotUser *middleware.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.GetUser(r)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	middleware.OptionalAuth(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200 on session mismatch, got %d", rr.Code)
	}
	if gotUser != nil {
		t.Error("user should be nil on session version mismatch")
	}
}

func TestOptionalAuth_ValidSession_SetsUser(t *testing.T) {
	defer setupMW(t)()
	userID := createMWUser(t)

	sv, _, err := db.GetUserAuthData(userID)
	if err != nil {
		t.Fatalf("GetUserAuthData: %v", err)
	}

	token, err := auth.GenerateToken(userID, "user", "fr", "EUR", sv)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	var gotUser *middleware.User
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = middleware.GetUser(r)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	middleware.OptionalAuth(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotUser == nil {
		t.Fatal("user should be set in context")
	}
	if gotUser.ID != userID {
		t.Errorf("user.ID: want %d, got %d", userID, gotUser.ID)
	}
}

// --- RequireAdmin ---

func TestRequireAdmin_NoUser_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestRequireAdmin_RegularUser_Forbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, &middleware.User{
		ID:   1,
		Role: "user",
	})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestRequireAdmin_AdminUser_Passes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, &middleware.User{
		ID:   1,
		Role: "ADMIN",
	})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- GetUser ---

func TestGetUser_NoContext_ReturnsNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if middleware.GetUser(req) != nil {
		t.Error("want nil when no user in context")
	}
}

func TestGetUser_WithUser_ReturnsUser(t *testing.T) {
	u := &middleware.User{ID: 42, Role: "user", Email: "test@example.com"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), middleware.UserContextKey, u)
	req = req.WithContext(ctx)

	got := middleware.GetUser(req)
	if got == nil {
		t.Fatal("want non-nil user")
	}
	if got.ID != 42 {
		t.Errorf("ID: want 42, got %d", got.ID)
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email: want test@example.com, got %s", got.Email)
	}
}
