package middleware

import (
	"net/http"
	"strings"
)

// MaxBodySize limite la taille du body HTTP à 1MB pour prévenir les attaques DoS.
func MaxBodySize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders pose les en-têtes de sécurité communs, puis (hors routes /api/)
// génère un nonce par requête et l'intègre dans la CSP HTML.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers communs à toutes les routes
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		w.Header().Set("Cache-Control", "no-store")

		// Les routes /api/ retournent du JSON : pas de CSP HTML ni de nonce
		if strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		nonce := GenerateNonce()
		r = r.WithContext(WithNonce(r.Context(), nonce))

		// Reporting-Endpoints (moderne, remplace report-uri deprecated)
		w.Header().Set("Reporting-Endpoints", `csp-endpoint="/api/csp-report"`)

		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; "+
				"script-src 'self' 'nonce-"+nonce+"' 'strict-dynamic'; "+
				"style-src 'self' 'unsafe-inline'; "+ // unsafe-inline requis par Tailwind CSS v4 (styles inline générés)
				"img-src 'self' blob: data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"manifest-src 'self'; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'; "+
				"report-uri /api/csp-report; "+
				"report-to csp-endpoint")

		next.ServeHTTP(w, r)
	})
}
