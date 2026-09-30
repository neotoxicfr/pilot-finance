package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func passthrough(next http.Handler) http.Handler { return next }

// PERF-01 : 130 assets statiques d'affilée ne doivent jamais prendre de 429,
// alors que les routes dynamiques restent sous le limiteur global.
func TestRouter_StaticNotRateLimited(t *testing.T) {
	t.Chdir("../..") // dossier go/ : static/ y est servi
	r := newRouter(passthrough, "localhost", false)

	for i := range 130 {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/static/css/app.css", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("requête %d sur /static : code %d", i+1, rr.Code)
		}
	}

	limited := false
	for range 130 {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/csp-report", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		r.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("les routes dynamiques doivent rester limitées à 120 req/min")
	}
}

func TestRouter_RateLimitDisabled(t *testing.T) {
	r := newRouter(passthrough, "localhost", true)
	for i := range 130 {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/csp-report", nil)
		r.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			t.Fatalf("429 à la requête %d alors que le limiteur est désactivé", i+1)
		}
	}
}

// SEC-06 : DISABLE_RATE_LIMIT=true est refusé en production.
func TestRateLimitDisabled(t *testing.T) {
	cases := []struct {
		env, flag string
		want      bool
		wantErr   bool
	}{
		{"", "", false, false},
		{"production", "", false, false},
		{"production", "false", false, false},
		{"", "true", true, false},
		{"development", "true", true, false},
		{"production", "true", false, true},
	}
	for _, c := range cases {
		got, err := rateLimitDisabled(c.env, c.flag)
		if got != c.want || (err != nil) != c.wantErr {
			t.Errorf("rateLimitDisabled(%q, %q) = %v, %v", c.env, c.flag, got, err)
		}
	}
}
