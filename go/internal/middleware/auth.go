// Package middleware contient les middlewares HTTP
package middleware

import (
	"context"
	"net/http"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
)

type contextKey string

const UserContextKey contextKey = "user"

// User représente l'utilisateur authentifié dans le contexte
type User struct {
	ID             int64
	Email          string
	Role           string
	SessionVersion int
}

// RequireAuth vérifie que l'utilisateur est authentifié
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		claims, err := auth.ValidateToken(cookie.Value)
		if err != nil {
			// Token invalide ou expiré, supprimer le cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "session",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Vérifier que la version de session correspond
		dbUser, err := db.GetUserByID(claims.UserID)
		if err != nil || dbUser == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if dbUser.SessionVersion != claims.SessionVersion {
			// Session invalidée (changement de mot de passe)
			http.SetCookie(w, &http.Cookie{
				Name:     "session",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Ajouter l'utilisateur au contexte
		user := &User{
			ID:             claims.UserID,
			Email:          claims.Email,
			Role:           claims.Role,
			SessionVersion: claims.SessionVersion,
		}
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin vérifie que l'utilisateur est admin
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil || user.Role != "ADMIN" {
			http.Error(w, "Accès refusé", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUser récupère l'utilisateur du contexte
func GetUser(r *http.Request) *User {
	user, ok := r.Context().Value(UserContextKey).(*User)
	if !ok {
		return nil
	}
	return user
}
