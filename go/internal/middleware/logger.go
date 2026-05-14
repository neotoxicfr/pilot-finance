package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// sensitiveLogPaths liste les paths dont la query string ne doit JAMAIS être
// loggée — elle contient des secrets (tokens email, reset, verification).
// M1 fix : sans sanitization, les tokens fuitent dans les logs chi.Logger.
var sensitiveLogPaths = map[string]bool{
	"/reset-password": true,
	"/verify-email":   true,
}

// SanitizedLogger logge chaque requête (status, durée, taille) en redactant la
// query string sur les paths sensibles AVANT le log. Pour les autres paths, il
// délègue à chi/middleware.Logger.
//
// Note : on ne peut pas se contenter d'envelopper chimw.Logger en clonant la
// requête, car le handler en aval lit `r.URL.Query().Get("token")`. On loggue
// donc nous-mêmes pour les paths sensibles.
func SanitizedLogger(next http.Handler) http.Handler {
	chiLogged := chimw.Logger(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sensitiveLogPaths[r.URL.Path] && r.URL.RawQuery != "" {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			slog.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"query", "[REDACTED]",
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
				"reqID", chimw.GetReqID(r.Context()),
			)
			return
		}
		chiLogged.ServeHTTP(w, r)
	})
}
