package metrics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// PERF-02 : 1000 méthodes inventées ne doivent créer qu'une seule série "OTHER".
func TestMiddleware_UnknownMethodsBounded(t *testing.T) {
	resetRegistry(t)
	const series = `pilot_http_requests_total{code="2xx",method="OTHER",route="auth"}`
	before := metricValue(t, series)
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for i := range 1000 {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(fmt.Sprintf("X%d", i), "/login", nil))
	}
	if got := metricValue(t, series); got != before+1000 {
		t.Errorf("%s : want %v, got %v", series, before+1000, got)
	}
	if got := metricValue(t, `pilot_http_requests_total{code="2xx",method="X1",route="auth"}`); got != 0 {
		t.Errorf("la méthode brute X1 ne doit pas devenir un label, got %v", got)
	}
}

func TestMethodLabel(t *testing.T) {
	for _, m := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		if got := methodLabel(m); got != m {
			t.Errorf("methodLabel(%q) = %q", m, got)
		}
	}
	for _, m := range []string{"TRACE", "CONNECT", "X1", "get"} {
		if got := methodLabel(m); got != "OTHER" {
			t.Errorf("methodLabel(%q) = %q, want OTHER", m, got)
		}
	}
}
