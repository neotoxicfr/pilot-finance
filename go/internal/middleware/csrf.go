package middleware

import (
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
//   - Exception : HOST non configuré (dev local) → laisse passer
func ValidateOrigin(host string) func(http.Handler) http.Handler {
	expected := "https://" + host
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
				// HOST non configuré (dev local) → pas de validation CSRF
				if host == "" {
					break
				}
				if origin := r.Header.Get("Origin"); origin != "" {
					if !strings.HasPrefix(origin, expected) {
						http.Error(w, "Requête cross-origin refusée", http.StatusForbidden)
						return
					}
				} else if referer := r.Header.Get("Referer"); referer != "" {
					if !strings.HasPrefix(referer, expected) {
						http.Error(w, "Requête cross-origin refusée", http.StatusForbidden)
						return
					}
				} else {
					// Les deux absents et HOST configuré : pas de contexte navigateur → rejeter
					http.Error(w, "Requête refusée", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
