package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// --- cacheStatic ---
// (MaxBodySize, SecurityHeaders et TrustedProxy/formatRemoteAddr ont été déplacés
// dans internal/middleware et y sont testés ; cacheStatic reste ici.)

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
