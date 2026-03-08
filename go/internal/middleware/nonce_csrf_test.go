package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"

	"pilot-finance/internal/middleware"
)

var passThroughHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestGenerateNonce_PanicOnRandError(t *testing.T) {
	restore := middleware.SetRandReader(iotest.ErrReader(iotest.ErrTimeout))
	defer restore()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when rand reader fails")
		}
	}()
	middleware.GenerateNonce()
}

// --- GenerateNonce ---

func TestGenerateNonce_NonEmpty(t *testing.T) {
	n := middleware.GenerateNonce()
	if n == "" {
		t.Error("want non-empty nonce")
	}
}

func TestGenerateNonce_Unique(t *testing.T) {
	n1 := middleware.GenerateNonce()
	n2 := middleware.GenerateNonce()
	if n1 == n2 {
		t.Error("two consecutive nonces should differ")
	}
}

// --- WithNonce / GetNonce ---

func TestWithNonce_GetNonce_RoundTrip(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := middleware.WithNonce(req.Context(), "abc123")
	req = req.WithContext(ctx)

	got := middleware.GetNonce(req)
	if got != "abc123" {
		t.Errorf("want abc123, got %q", got)
	}
}

func TestGetNonce_NoContext_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if n := middleware.GetNonce(req); n != "" {
		t.Errorf("want empty string with no nonce, got %q", n)
	}
}

// --- ValidateOrigin ---

func TestValidateOrigin_GET_AlwaysPasses(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET should always pass, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_CorrectOrigin(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("correct origin should pass, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_WrongOrigin(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("wrong origin should return 403, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_CorrectReferer(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Referer", "https://example.com/page")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("correct referer should pass, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_WrongReferer(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Referer", "https://evil.com/page")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("wrong referer should return 403, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_NoOriginNoReferer_WithHost_Rejects(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("no origin/referer with configured host should return 403, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_NoOriginNoReferer_NoHost_Rejects(t *testing.T) {
	// CSRF is always active: even with empty configured host, requests without
	// Origin/Referer are rejected (uses request Host or "localhost" as fallback).
	mw := middleware.ValidateOrigin("")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("POST without Origin/Referer should be rejected, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_NoHost_NonLocalhost_Rejected(t *testing.T) {
	// When HOST env is empty, non-localhost hosts are rejected (fail-closed)
	mw := middleware.ValidateOrigin("")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "myapp.local"
	req.Header.Set("Origin", "https://myapp.local")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("Non-localhost host without HOST env should be rejected, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_NoHost_DevAllowsHTTP(t *testing.T) {
	// In dev mode (empty HOST), both HTTP and HTTPS origins are accepted
	mw := middleware.ValidateOrigin("")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set("Origin", "http://localhost:8080")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("HTTP origin should pass in dev mode, got %d", rr.Code)
	}
}

func TestValidateOrigin_DELETE_WrongOrigin(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("DELETE with wrong origin should return 403, got %d", rr.Code)
	}
}

func TestValidateOrigin_PATCH_CorrectOrigin(t *testing.T) {
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPatch, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("PATCH with correct origin should pass, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_NoHost_EmptyRequestHost_FallsBackToLocalhost(t *testing.T) {
	// When HOST is empty and request Host header is also empty,
	// effectiveHost falls back to "localhost". Origin must match.
	mw := middleware.ValidateOrigin("")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = ""
	req.Header.Set("Origin", "https://localhost")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Origin matching localhost fallback should pass, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_NoHost_EmptyRequestHost_HTTPLocalhost(t *testing.T) {
	// Dev mode (empty HOST), empty request Host → localhost fallback, HTTP origin
	mw := middleware.ValidateOrigin("")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = ""
	req.Header.Set("Origin", "http://localhost")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("HTTP origin matching localhost fallback should pass in dev mode, got %d", rr.Code)
	}
}

func TestValidateOrigin_PUT_CorrectOrigin(t *testing.T) {
	// Cover the PUT method branch
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPut, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("PUT with correct origin should pass, got %d", rr.Code)
	}
}

func TestValidateOrigin_POST_ProductionRejectsHTTP(t *testing.T) {
	// In production (HOST configured), HTTP origins should be rejected
	mw := middleware.ValidateOrigin("example.com")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	rr := httptest.NewRecorder()
	mw(passThroughHandler).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("production should reject HTTP origin, got %d", rr.Code)
	}
}
