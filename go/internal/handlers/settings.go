package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// ChangePassword change le mot de passe de l'utilisateur
func ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Donnees invalides", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("currentPassword")
	newPassword := r.FormValue("newPassword")
	confirmPassword := r.FormValue("confirmPassword")

	if currentPassword == "" || newPassword == "" {
		http.Error(w, "Tous les champs sont requis", http.StatusBadRequest)
		return
	}

	if newPassword != confirmPassword {
		http.Error(w, "Les mots de passe ne correspondent pas", http.StatusBadRequest)
		return
	}

	if err := crypto.ValidatePassword(newPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Récupérer l'utilisateur complet pour vérifier le mot de passe
	dbUser, err := db.GetUserByID(user.ID)
	if err != nil || dbUser == nil {
		http.Error(w, "Utilisateur non trouve", http.StatusNotFound)
		return
	}

	// Verifier le mot de passe actuel
	if !crypto.VerifyPassword(currentPassword, dbUser.Password) {
		http.Error(w, "Mot de passe actuel incorrect", http.StatusUnauthorized)
		return
	}

	// Hasher le nouveau mot de passe
	hashedPassword, err := crypto.HashPassword(newPassword)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Mettre a jour
	err = db.UpdatePassword(user.ID, hashedPassword)
	if err != nil {
		http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
		return
	}

	db.LogAudit(user.ID, db.AuditPasswordChange, getClientIP(r), r.UserAgent())

	w.WriteHeader(http.StatusOK)
}

// UpdatePreferences met à jour la langue et la devise de l'utilisateur
func UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Donnees invalides", http.StatusBadRequest)
		return
	}

	language := r.FormValue("language")
	currency := r.FormValue("currency")

	validLangs := map[string]bool{"fr": true, "en": true}
	validCurrencies := map[string]bool{
		"EUR": true, "USD": true, "GBP": true, "CHF": true,
		"JPY": true, "CAD": true, "AUD": true,
	}

	if !validLangs[language] {
		language = "fr"
	}
	if !validCurrencies[currency] {
		currency = "EUR"
	}

	if err := db.UpdateUserPreferences(user.ID, language, currency); err != nil {
		http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
		return
	}

	// Re-émettre le JWT avec les nouvelles préférences (Language/Currency dans les claims)
	token, err := auth.GenerateToken(user.ID, user.Role, language, currency, user.SessionVersion)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, "session", token, 86400)

	w.WriteHeader(http.StatusOK)
}

// ExportData exporte toutes les données de l'utilisateur en JSON (GDPR).
func ExportData(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	dbUser, err := db.GetUserByID(user.ID)
	if err != nil || dbUser == nil {
		http.Error(w, "Utilisateur non trouvé", http.StatusNotFound)
		return
	}
	email, _ := crypto.Decrypt(dbUser.EmailEncrypted)

	accounts, _ := db.GetAccountsByUserID(user.ID)
	recurrings, _ := db.GetRecurringByUserID(user.ID)

	// Déchiffrer les noms
	for i := range accounts {
		if dec, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = dec
		}
	}
	for i := range recurrings {
		if dec, err := crypto.Decrypt(recurrings[i].Description); err == nil {
			recurrings[i].Description = dec
		}
	}

	export := map[string]interface{}{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"user": map[string]interface{}{
			"id":       user.ID,
			"email":    email,
			"language": dbUser.Language,
			"currency": dbUser.Currency,
		},
		"accounts":   accounts,
		"recurrings": recurrings,
	}

	db.LogAudit(user.ID, db.AuditGDPRExport, getClientIP(r), r.UserAgent())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="pilot-finance-export.json"`)
	json.NewEncoder(w).Encode(export)
}

// DeleteSelfAccount supprime le compte de l'utilisateur connecté et toutes ses données (GDPR).
func DeleteSelfAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifié", http.StatusUnauthorized)
		return
	}

	db.LogAudit(user.ID, db.AuditGDPRDelete, getClientIP(r), r.UserAgent())

	if err := db.DeleteUserAndData(user.ID); err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	clearCookie(w, "session")
	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

// DeleteUser supprime un utilisateur (admin uniquement)
func DeleteUser(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil || user.Role != "ADMIN" {
		http.Error(w, "Non autorise", http.StatusForbidden)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	// Ne pas permettre de supprimer un admin
	targetUser, err := db.GetUserByID(id)
	if err != nil {
		http.Error(w, "Utilisateur non trouve", http.StatusNotFound)
		return
	}

	if targetUser.Role == "ADMIN" {
		http.Error(w, "Impossible de supprimer un administrateur", http.StatusForbidden)
		return
	}

	err = db.DeleteUser(id)
	if err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
