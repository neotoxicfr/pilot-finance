package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pilot-finance/internal/middleware"
)

// captureLogs redirige slog vers un buffer le temps du test puis restaure.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestSanitizedLogger_RedactsResetPasswordToken(t *testing.T) {
	buf := captureLogs(t)
	mw := middleware.SanitizedLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Le handler doit toujours voir le token (handler-side reads token).
		if got := r.URL.Query().Get("token"); got != "secret-token-abc" {
			t.Errorf("handler should still see token, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/reset-password?token=secret-token-abc", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	out := buf.String()
	if strings.Contains(out, "secret-token-abc") {
		t.Errorf("log output must not contain the raw token: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Errorf("log output must contain [REDACTED] marker: %s", out)
	}
}

func TestSanitizedLogger_RedactsVerifyEmailToken(t *testing.T) {
	buf := captureLogs(t)
	mw := middleware.SanitizedLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=verify-xyz", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	if strings.Contains(buf.String(), "verify-xyz") {
		t.Errorf("verify-email token leaked in logs: %s", buf.String())
	}
}

func TestSanitizedLogger_KeepsNormalPaths(t *testing.T) {
	// Les paths non sensibles passent par chi.Logger (qui logge sur le logger
	// par défaut de chi). On vérifie au moins que le handler exécute et que
	// le code de statut est correct.
	mw := middleware.SanitizedLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/accounts?foo=bar", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestSanitizedLogger_NoQueryString_NoRedaction(t *testing.T) {
	// Sensitive path mais sans query : pas de redaction nécessaire, fallback
	// sur chi.Logger.
	mw := middleware.SanitizedLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/reset-password", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}
