package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
)

// ChangePassword change le mot de passe de l'utilisateur
func ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("newPassword")
	confirmPassword := r.FormValue("confirmPassword")

	if currentPassword == "" || newPassword == "" {
		clientError(w, ErrValidation, "Tous les champs sont requis", http.StatusBadRequest)
		return
	}

	if newPassword != confirmPassword {
		clientError(w, ErrValidation, "Les mots de passe ne correspondent pas", http.StatusBadRequest)
		return
	}

	if err := hookValidatePassword(newPassword); err != nil {
		lang, _ := userLocale(user)
		clientError(w, ErrValidation, i18n.T(lang, "pwd_error."+err.Error()), http.StatusBadRequest)
		return
	}

	// Récupérer l'utilisateur complet pour vérifier le mot de passe
	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		clientError(w, ErrNotFound, "Utilisateur non trouvé", http.StatusNotFound)
		return
	}

	// Verifier le mot de passe actuel
	if !hookVerifyPassword(currentPassword, dbUser.Password) {
		clientError(w, ErrAuthInvalid, "Mot de passe actuel incorrect", http.StatusUnauthorized)
		return
	}

	// Hasher le nouveau mot de passe
	hashedPassword, err := hookHashPassword(newPassword)
	if err != nil {
		serverError(w, "hash password", err)
		return
	}

	// Mettre a jour
	err = hookUpdatePassword(user.ID, hashedPassword)
	if err != nil {
		serverError(w, "update password", err)
		return
	}

	// Invalidate session cache so the new session_version takes effect immediately
	hookInvalidateSessionCache(user.ID)

	hookLogAudit(user.ID, db.AuditPasswordChange, getClientIP(r), r.UserAgent())

	// Re-issue JWT with new session version. UpdatePassword a déjà bumpé la
	// session_version ; on relit uniquement ce compteur (pas tout l'utilisateur).
	if sv, svErr := hookGetSessionVersion(user.ID); svErr == nil {
		if token, err := hookGenerateToken(user.ID, user.Role, user.Language, user.Currency, sv); err == nil {
			setSessionCookie(w, "session", token, 86400)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// UpdatePreferences met à jour la langue et la devise de l'utilisateur
func UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
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

	if err := hookUpdateUserPrefs(user.ID, language, currency); err != nil {
		serverError(w, "update preferences", err)
		return
	}

	// Re-émettre le JWT avec les nouvelles préférences (Language/Currency dans les claims)
	token, err := hookGenerateToken(user.ID, user.Role, language, currency, user.SessionVersion)
	if err != nil {
		serverError(w, "generate token", err)
		return
	}
	setSessionCookie(w, "session", token, 86400)

	w.WriteHeader(http.StatusOK)
}

// ExportData exporte toutes les données de l'utilisateur en JSON (GDPR).
func ExportData(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		clientError(w, ErrNotFound, "Utilisateur non trouvé", http.StatusNotFound)
		return
	}
	email, err := hookDecryptStr(dbUser.EmailEncrypted)
	if err != nil {
		slog.Warn("ExportData: decrypt email", "err", err)
	}

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}
	recurrings, err := hookGetRecurringByUserID(user.ID)
	if err != nil {
		serverError(w, "get recurrings", err)
		return
	}

	// Déchiffrer les noms
	for i := range accounts {
		if dec, err := hookDecryptStr(accounts[i].Name); err == nil {
			accounts[i].Name = dec
		}
	}
	for i := range recurrings {
		if dec, err := hookDecryptStr(recurrings[i].Description); err == nil {
			recurrings[i].Description = dec
		}
	}

	auditEntries, err := hookGetAuditLogByUserID(user.ID)
	if err != nil {
		slog.Warn("ExportData: audit log", "err", err)
	}

	// Passkeys : exporter sans la clé publique brute (non intelligible)
	type passkeyExport struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		DeviceType string `json:"device_type"`
		BackedUp   bool   `json:"backed_up"`
	}
	rawPasskeys, err := hookGetAuthenticatorsByUserID(user.ID)
	if err != nil {
		slog.Warn("ExportData: passkeys", "err", err)
	}
	passkeys := make([]passkeyExport, len(rawPasskeys))
	for i, pk := range rawPasskeys {
		passkeys[i] = passkeyExport{
			ID:         pk.ID,
			Name:       pk.Name,
			DeviceType: pk.CredentialDeviceType,
			BackedUp:   pk.CredentialBackedUp,
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
		"audit_log":  auditEntries,
		"passkeys":   passkeys,
	}

	hookLogAudit(user.ID, db.AuditGDPRExport, getClientIP(r), r.UserAgent())

	w.Header().Set("Content-Disposition", `attachment; filename="pilot-finance-export.json"`)
	jsonSuccess(w, export)
}

// DeleteSelfAccount supprime le compte de l'utilisateur connecté et toutes ses données (GDPR).
// Requires current password for re-authentication (irreversible action).
func DeleteSelfAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := parseFormAny(r); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	if currentPassword == "" {
		clientError(w, ErrValidation, "Mot de passe requis", http.StatusBadRequest)
		return
	}

	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		clientError(w, ErrNotFound, "Utilisateur non trouvé", http.StatusNotFound)
		return
	}

	if !hookVerifyPassword(currentPassword, dbUser.Password) {
		clientError(w, ErrAuthInvalid, "Mot de passe incorrect", http.StatusUnauthorized)
		return
	}

	userID := user.ID
	clientIP := getClientIP(r)
	ua := r.UserAgent()

	hookInvalidateSessionCache(userID)

	if err := hookDeleteUserAndData(userID); err != nil {
		serverError(w, "delete user", err)
		return
	}

	hookLogAudit(userID, db.AuditGDPRDelete, clientIP, ua)

	clearCookie(w, "session")
	w.Header().Set("HX-Redirect", "/login")
	w.WriteHeader(http.StatusOK)
}

// DeleteUser supprime un utilisateur (admin uniquement)
func DeleteUser(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil || user.Role != "ADMIN" {
		clientError(w, ErrForbidden, "Non autorisé", http.StatusForbidden)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
		return
	}

	// Ne pas permettre de supprimer un admin
	targetUser, err := hookGetUserByID(id)
	if err != nil || targetUser == nil {
		clientError(w, ErrNotFound, "Utilisateur non trouvé", http.StatusNotFound)
		return
	}

	if targetUser.Role == "ADMIN" {
		clientError(w, ErrForbidden, "Impossible de supprimer un administrateur", http.StatusForbidden)
		return
	}

	hookInvalidateSessionCache(id)

	err = hookDeleteUserAndData(id)
	if err != nil {
		serverError(w, "delete user", err)
		return
	}

	hookLogAudit(user.ID, db.AuditAdminDeleteUser, getClientIP(r), r.UserAgent())

	w.WriteHeader(http.StatusNoContent)
}
