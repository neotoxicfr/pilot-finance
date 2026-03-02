package metrics

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	_ "modernc.org/sqlite" // driver pour TestHookDBStats_WithDB
)

// resetRegistry recrée un registre vide pour éviter les collisions entre tests.
func resetRegistry(t *testing.T) {
	t.Helper()
	// Désenregistrer et ré-enregistrer les collectors
	for _, c := range collectors {
		prometheus.Unregister(c)
	}
	for _, c := range collectors {
		prometheus.MustRegister(c)
	}
}

// --- Init ---

func TestInit(t *testing.T) {
	// Unregister first to avoid double-registration
	for _, c := range collectors {
		prometheus.Unregister(c)
	}
	called := false
	Init(func() *sql.DB {
		called = true
		return nil
	})
	defer func() {
		getDB = nil
		for _, c := range collectors {
			prometheus.Unregister(c)
		}
	}()

	if getDB == nil {
		t.Error("getDB should be set after Init")
	}
	getDB()
	if !called {
		t.Error("getDB function should be callable")
	}
}

// --- Handler ---

func TestHandler_ServesMetrics(t *testing.T) {
	resetRegistry(t)
	getDB = func() *sql.DB { return nil }
	defer func() { getDB = nil }()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Handler: want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "pilot_db_open_connections") {
		t.Error("Handler should include pilot_db metrics")
	}
}

// --- hookDBStats ---

func TestHookDBStats_WithDB(t *testing.T) {
	origGetDB := getDB
	defer func() { getDB = origGetDB }()

	d, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	getDB = func() *sql.DB { return d }
	stats := hookDBStats()
	if stats.OpenConnections < 0 {
		t.Error("unexpected negative connections")
	}
}

func TestHookDBStats_NilGetDB(t *testing.T) {
	origGetDB := getDB
	defer func() { getDB = origGetDB }()
	getDB = nil

	stats := hookDBStats()
	if stats.OpenConnections != 0 {
		t.Error("hookDBStats should return zero stats when getDB is nil")
	}
}

func TestHookDBStats_NilDB(t *testing.T) {
	origGetDB := getDB
	defer func() { getDB = origGetDB }()
	getDB = func() *sql.DB { return nil }

	stats := hookDBStats()
	if stats.OpenConnections != 0 {
		t.Error("hookDBStats should return zero stats when DB is nil")
	}
}

// --- updateDBMetrics ---

func TestUpdateDBMetrics(t *testing.T) {
	resetRegistry(t)
	origHook := hookDBStats
	defer func() { hookDBStats = origHook }()

	hookDBStats = func() sql.DBStats {
		return sql.DBStats{
			OpenConnections: 3,
			InUse:           2,
			Idle:            1,
			WaitCount:       5,
		}
	}

	updateDBMetrics()
	// If it doesn't panic, gauges were set correctly
}

// --- Middleware ---

func TestMiddleware_RecordsMetrics(t *testing.T) {
	resetRegistry(t)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	}))

	req := httptest.NewRequest("GET", "/accounts", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Middleware: want 200, got %d", rr.Code)
	}
}

func TestMiddleware_SkipsMetricsPath(t *testing.T) {
	resetRegistry(t)

	called := false
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Middleware should still call next handler for /metrics")
	}
}

func TestMiddleware_SkipsStaticPath(t *testing.T) {
	resetRegistry(t)

	called := false
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/static/app.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("Middleware should still call next handler for /static/")
	}
}

func TestMiddleware_ImplicitOK(t *testing.T) {
	resetRegistry(t)

	// Handler that writes body without explicit WriteHeader → implicit 200
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello")) //nolint:errcheck
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- responseWriter ---

func TestResponseWriter_WriteHeaderOnce(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rr, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	rw.WriteHeader(http.StatusOK) // should be ignored

	if rw.statusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", rw.statusCode)
	}
}

func TestResponseWriter_Unwrap(t *testing.T) {
	rr := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rr, statusCode: http.StatusOK}

	if rw.Unwrap() != rr {
		t.Error("Unwrap should return original ResponseWriter")
	}
}

// --- groupRoute ---

func TestGroupRoute(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "dashboard"},
		{"/login", "auth"},
		{"/register", "auth"},
		{"/forgot-password", "auth"},
		{"/reset-password", "auth"},
		{"/logout", "auth"},
		{"/verify-email", "verify_email"},
		{"/privacy", "legal"},
		{"/legal", "legal"},
		{"/accounts", "accounts"},
		{"/accounts/123/balance", "accounts"},
		{"/recurring", "recurring"},
		{"/recurring/456", "recurring"},
		{"/settings", "settings"},
		{"/settings/mfa/setup", "settings"},
		{"/admin", "admin"},
		{"/admin/users/1", "admin"},
		{"/api/passkey/login/start", "api_passkey"},
		{"/api/health", "api"},
		{"/api/dashboard", "api"},
		{"/unknown-path", "other"},
	}

	for _, tt := range tests {
		got := groupRoute(tt.path)
		if got != tt.want {
			t.Errorf("groupRoute(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- statusBucket ---

func TestStatusBucket(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{100, "1xx"},
		{200, "2xx"},
		{301, "3xx"},
		{404, "4xx"},
		{500, "5xx"},
	}

	for _, tt := range tests {
		got := statusBucket(tt.code)
		if got != tt.want {
			t.Errorf("statusBucket(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}
