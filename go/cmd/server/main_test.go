package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pilot-finance/internal/middleware"
)

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// --- maxBodySize ---

func TestMaxBodySize_PassesThrough(t *testing.T) {
	handler := maxBodySize(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("maxBodySize: want 200, got %d", rr.Code)
	}
}

// --- cacheStatic ---

func TestCacheStatic_JS_LongCache(t *testing.T) {
	handler := cacheStatic(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	want := "public, max-age=31536000, immutable"
	if got := rr.Header().Get("Cache-Control"); got != want {
		t.Errorf("JS Cache-Control: want %q, got %q", want, got)
	}
}

func TestCacheStatic_CSS_LongCache(t *testing.T) {
	handler := cacheStatic(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	want := "public, max-age=31536000, immutable"
	if got := rr.Header().Get("Cache-Control"); got != want {
		t.Errorf("CSS Cache-Control: want %q, got %q", want, got)
	}
}

func TestCacheStatic_Image_ShortCache(t *testing.T) {
	handler := cacheStatic(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/static/logo.png", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	want := "public, max-age=2592000"
	if got := rr.Header().Get("Cache-Control"); got != want {
		t.Errorf("image Cache-Control: want %q, got %q", want, got)
	}
}

// --- securityHeaders ---

func TestSecurityHeaders_API_NoCSP(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("API route should have no CSP, got %q", got)
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: want DENY, got %q", got)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: want nosniff, got %q", got)
	}
}

func TestSecurityHeaders_HTML_HasCSP(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify nonce is in context
		nonce := middleware.GetNonce(r)
		if nonce == "" {
			t.Error("nonce should be set in context for HTML routes")
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("HTML route should have CSP header")
	}
	if !strings.Contains(csp, "nonce-") {
		t.Errorf("CSP should contain nonce, got: %q", csp)
	}
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP should have default-src none, got: %q", csp)
	}
}

func TestSecurityHeaders_CommonHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	checks := map[string]string{
		"Referrer-Policy":              "strict-origin-when-cross-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cache-Control":                "no-store",
	}
	for header, want := range checks {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
}
