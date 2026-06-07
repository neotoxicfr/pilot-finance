// Package metrics expose des métriques Prometheus pour le monitoring.
package metrics

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// hookDBStats est une variable de fonction pour faciliter les tests.
var hookDBStats = func() sql.DBStats {
	if getDB == nil {
		return sql.DBStats{}
	}
	d := getDB()
	if d == nil {
		return sql.DBStats{}
	}
	return d.Stats()
}

// getDB retourne la connexion DB — injecté depuis main.go.
var getDB func() *sql.DB

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pilot_http_requests_total",
			Help: "Total HTTP requests by method, route group, and status code.",
		},
		[]string{"method", "route", "code"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pilot_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "route"},
	)

	dbOpenConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pilot_db_open_connections",
		Help: "Current number of open database connections.",
	})

	dbInUseConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pilot_db_in_use_connections",
		Help: "Current number of in-use database connections.",
	})

	dbIdleConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pilot_db_idle_connections",
		Help: "Current number of idle database connections.",
	})

	dbWaitCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pilot_db_wait_count_total",
		Help: "Total number of connections waited for.",
	})

	dbWaitDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pilot_db_wait_duration_seconds_total",
		Help: "Total time blocked waiting for a new connection.",
	})
)

// collectors regroupe tous les collectors pour faciliter les tests.
var collectors = []prometheus.Collector{
	httpRequestsTotal,
	httpRequestDuration,
	dbOpenConnections,
	dbInUseConnections,
	dbIdleConnections,
	dbWaitCount,
	dbWaitDuration,
}

// registerFn est une variable de fonction pour faciliter les tests.
var registerFn = prometheus.Register

// Init enregistre les métriques dans le registre par défaut et configure l'accès DB.
// Idempotent : un double appel ne panique pas (les collectors déjà enregistrés sont ignorés).
func Init(dbFunc func() *sql.DB) {
	getDB = dbFunc
	for _, c := range collectors {
		if err := registerFn(c); err != nil {
			// Un collector déjà enregistré (re-Init) n'est pas une erreur fatale.
			if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
				continue
			}
			// Toute autre erreur d'enregistrement indique un bug de configuration.
			panic(err)
		}
	}
}

// Handler retourne le handler HTTP pour /metrics.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateDBMetrics()
		promhttp.Handler().ServeHTTP(w, r)
	})
}

func updateDBMetrics() {
	stats := hookDBStats()
	dbOpenConnections.Set(float64(stats.OpenConnections))
	dbInUseConnections.Set(float64(stats.InUse))
	dbIdleConnections.Set(float64(stats.Idle))
	dbWaitCount.Set(float64(stats.WaitCount))
	dbWaitDuration.Set(stats.WaitDuration.Seconds())
}

// responseWriter capture le code HTTP pour les métriques.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap permet aux middlewares amont (ex. chi compress) d'accéder au ResponseWriter original.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Middleware enregistre les métriques de chaque requête HTTP.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ne pas mesurer /metrics et /static
		if r.URL.Path == "/metrics" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		route := groupRoute(r.URL.Path)
		duration := time.Since(start).Seconds()
		code := statusBucket(wrapped.statusCode)

		httpRequestsTotal.WithLabelValues(r.Method, route, code).Inc()
		httpRequestDuration.WithLabelValues(r.Method, route).Observe(duration)
	})
}

// groupRoute normalise les chemins pour limiter la cardinalité des labels.
func groupRoute(path string) string {
	switch {
	case path == "/":
		return "dashboard"
	case path == "/login" || path == "/register" || path == "/forgot-password" || path == "/reset-password" || path == "/logout":
		return "auth"
	case path == "/verify-email":
		return "verify_email"
	case path == "/privacy" || path == "/legal":
		return "legal"
	case strings.HasPrefix(path, "/accounts"):
		return "accounts"
	case strings.HasPrefix(path, "/recurring"):
		return "recurring"
	case strings.HasPrefix(path, "/settings"):
		return "settings"
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	case strings.HasPrefix(path, "/api/passkey"):
		return "api_passkey"
	case strings.HasPrefix(path, "/api/"):
		return "api"
	default:
		return "other"
	}
}

// statusBucket regroupe les codes HTTP en catégories.
func statusBucket(code int) string {
	switch {
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}
