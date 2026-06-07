package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestMaxBodySize(t *testing.T) {
	called := false
	h := MaxBodySize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called {
		t.Fatal("next handler not called")
	}
}

func TestSecurityHeaders_HTMLRoute(t *testing.T) {
	var ctxNonce string
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxNonce = GetNonce(r)
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: %q", got)
	}
	if got := rr.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("missing HSTS")
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP missing default-src: %q", csp)
	}
	if ctxNonce == "" || !strings.Contains(csp, "nonce-"+ctxNonce) {
		t.Errorf("nonce not propagated to CSP/context: ctx=%q csp=%q", ctxNonce, csp)
	}
	if rr.Header().Get("Reporting-Endpoints") == "" {
		t.Error("missing Reporting-Endpoints")
	}
}

func TestSecurityHeaders_APIRoute(t *testing.T) {
	h := SecurityHeaders(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// Common headers still set...
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("common headers should be set on /api/ routes")
	}
	// ...but no HTML CSP / nonce for JSON routes.
	if rr.Header().Get("Content-Security-Policy") != "" {
		t.Error("/api/ routes should not get an HTML CSP")
	}
}
