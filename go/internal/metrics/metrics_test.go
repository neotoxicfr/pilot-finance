package metrics

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

// TestInit_DoubleInit vérifie qu'un second appel à Init ne panique pas
// (les collectors déjà enregistrés déclenchent AlreadyRegisteredError, ignoré).
func TestInit_DoubleInit(t *testing.T) {
	for _, c := range collectors {
		prometheus.Unregister(c)
	}
	defer func() {
		getDB = nil
		for _, c := range collectors {
			prometheus.Unregister(c)
		}
	}()

	Init(func() *sql.DB { return nil })
	// Second appel : ne doit pas paniquer.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("double Init paniqued: %v", r)
		}
	}()
	Init(func() *sql.DB { return nil })
}

// TestInit_RegisterFatalError vérifie qu'une erreur d'enregistrement
// autre que AlreadyRegisteredError provoque bien un panic.
func TestInit_RegisterFatalError(t *testing.T) {
	orig := registerFn
	defer func() { registerFn = orig }()
	registerFn = func(_ prometheus.Collector) error {
		return errors.New("boom")
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Init should panic on non-AlreadyRegistered error")
		}
	}()
	Init(func() *sql.DB { return nil })
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

// --- S-14 : valeurs réellement enregistrées dans les métriques ---
//
// Les tests TestMiddleware_RecordsMetrics et TestUpdateDBMetrics ci-dessus
// n'observent que le code HTTP / l'absence de panic : neutraliser les appels
// .Inc(), .Observe() ou .Set() ne les faisait pas rougir. Les tests suivants
// lisent la valeur exposée par /metrics et assertent l'état des séries.

// metricValue lit la valeur courante d'une série dans l'exposition
// Prometheus. Retourne 0 quand la série n'a pas encore été observée.
func metricValue(t *testing.T, series string) float64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, req)

	for _, line := range strings.Split(rr.Body.String(), "\n") {
		rest, ok := strings.CutPrefix(line, series+" ")
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
		if err != nil {
			t.Fatalf("valeur illisible pour %s : %q", series, line)
		}
		return v
	}
	return 0
}

// TestMiddleware_IncrementsCounters vérifie que chaque requête mesurée
// incrémente exactement d'une unité le compteur et l'histogramme portant les
// bons labels (méthode, groupe de route, classe de code).
func TestMiddleware_IncrementsCounters(t *testing.T) {
	resetRegistry(t)

	cases := []struct {
		name      string
		method    string
		path      string
		status    int
		wantRoute string
		wantCode  string
	}{
		{"comptes_2xx", http.MethodGet, "/accounts", http.StatusOK, "accounts", "2xx"},
		{"recurrentes_4xx", http.MethodPost, "/recurring/12", http.StatusBadRequest, "recurring", "4xx"},
		{"admin_5xx", http.MethodDelete, "/admin/users/1", http.StatusInternalServerError, "admin", "5xx"},
		{"inconnu_3xx", http.MethodGet, "/nulle-part", http.StatusFound, "other", "3xx"},
		{"passkey_2xx", http.MethodPost, "/api/passkey/login/start", http.StatusOK, "api_passkey", "2xx"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter := fmt.Sprintf("pilot_http_requests_total{code=%q,method=%q,route=%q}", tc.wantCode, tc.method, tc.wantRoute)
			histogram := fmt.Sprintf("pilot_http_request_duration_seconds_count{method=%q,route=%q}", tc.method, tc.wantRoute)
			beforeCounter := metricValue(t, counter)
			beforeHistogram := metricValue(t, histogram)

			handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tc.method, tc.path, nil))

			if got, want := metricValue(t, counter), beforeCounter+1; got != want {
				t.Errorf("%s : want %v, got %v", counter, want, got)
			}
			if got, want := metricValue(t, histogram), beforeHistogram+1; got != want {
				t.Errorf("%s : want %v, got %v", histogram, want, got)
			}
		})
	}
}

// TestMiddleware_SkippedPathsDoNotIncrement vérifie que /metrics et /static/
// traversent bien le middleware SANS être comptabilisés (l'ancien test se
// contentait de vérifier que le handler suivant était appelé).
func TestMiddleware_SkippedPathsDoNotIncrement(t *testing.T) {
	resetRegistry(t)

	// /metrics et /static/... tombent tous deux dans le groupe "other".
	const series = `pilot_http_requests_total{code="2xx",method="GET",route="other"}`

	for _, path := range []string{"/metrics", "/static/app.js"} {
		t.Run(path, func(t *testing.T) {
			before := metricValue(t, series)

			handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))

			if got := metricValue(t, series); got != before {
				t.Errorf("%s ne doit pas être comptabilisé : %v → %v", path, before, got)
			}
		})
	}
}

// TestUpdateDBMetrics_SetsGaugeValues vérifie que chaque jauge reçoit la
// statistique qui lui correspond. Les valeurs sont volontairement toutes
// distinctes pour détecter un croisement de câblage entre deux jauges.
func TestUpdateDBMetrics_SetsGaugeValues(t *testing.T) {
	resetRegistry(t)
	origHook := hookDBStats
	t.Cleanup(func() { hookDBStats = origHook })

	hookDBStats = func() sql.DBStats {
		return sql.DBStats{
			OpenConnections: 7,
			InUse:           4,
			Idle:            3,
			WaitCount:       9,
			WaitDuration:    1500 * time.Millisecond,
		}
	}
	updateDBMetrics()

	cases := []struct {
		series string
		want   float64
	}{
		{"pilot_db_open_connections", 7},
		{"pilot_db_in_use_connections", 4},
		{"pilot_db_idle_connections", 3},
		{"pilot_db_wait_count_total", 9},
		{"pilot_db_wait_duration_seconds_total", 1.5},
	}
	for _, tc := range cases {
		t.Run(tc.series, func(t *testing.T) {
			if got := metricValue(t, tc.series); got != tc.want {
				t.Errorf("%s : want %v, got %v", tc.series, tc.want, got)
			}
		})
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
