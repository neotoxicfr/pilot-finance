package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// ValidateOrigin retourne un middleware qui vérifie l'en-tête Origin (ou Referer)
// sur les requêtes mutantes (POST/PUT/DELETE/PATCH). Defense-in-depth contre le CSRF.
//
// Règles :
//   - Si Origin est présent et ne correspond pas à l'hôte attendu → 403
//   - Si Origin est absent mais Referer est présent et ne correspond pas → 403
//   - Si les deux sont absents → 403 (requête mutante sans contexte navigateur)
//   - CSRF est toujours actif : si HOST n'est pas configuré, on utilise le Host header de la requête
func ValidateOrigin(host string) func(http.Handler) http.Handler {
	if host == "" {
		slog.Warn("HOST not configured, using request host for CSRF origin check")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
				// Determine the expected host: use configured HOST, or fall back to request Host header
				effectiveHost := host
				if effectiveHost == "" {
					effectiveHost = r.Host
					if effectiveHost == "" {
						effectiveHost = "localhost"
					}
				}

				// Build expected origin prefixes (support both http and https for localhost/dev)
				expectedHTTPS := "https://" + effectiveHost
				expectedHTTP := "http://" + effectiveHost

				matchesOrigin := func(value string) bool {
					if host != "" {
						// Production: only allow HTTPS
						return strings.HasPrefix(value, expectedHTTPS)
					}
					// Dev fallback: allow both HTTP and HTTPS
					return strings.HasPrefix(value, expectedHTTPS) || strings.HasPrefix(value, expectedHTTP)
				}

				if origin := r.Header.Get("Origin"); origin != "" {
					if !matchesOrigin(origin) {
						http.Error(w, "Requête cross-origin refusée", http.StatusForbidden)
						return
					}
				} else if referer := r.Header.Get("Referer"); referer != "" {
					if !matchesOrigin(referer) {
						http.Error(w, "Requête cross-origin refusée", http.StatusForbidden)
						return
					}
				} else {
					// Les deux absents : pas de contexte navigateur → rejeter
					http.Error(w, "Requête refusée", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
