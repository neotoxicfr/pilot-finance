package handlers

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"

	qrcode "github.com/skip2/go-qrcode"

	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// MFASetup retourne le secret et le QR code pour configurer le 2FA
func MFASetup(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		jsonError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Generer un nouveau secret
	secret, err := hookGenerateTOTPSecret()
	if err != nil {
		slog.Error("generate TOTP secret", "err", err)
		jsonError(w, ErrInternal, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Generer l'URI pour le QR code
	otpauthURI := hookGenerateTOTPURI(secret, user.Email)

	// Generer le QR code localement (zéro dépendance externe)
	png, err := hookQREncode(otpauthURI, qrcode.Medium, 200)
	if err != nil {
		slog.Error("generate QR code", "err", err)
		jsonError(w, ErrInternal, "Erreur generation QR", http.StatusInternalServerError)
		return
	}
	qrDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	jsonSuccess(w, map[string]string{
		"secret":   secret,
		"imageUrl": qrDataURI,
	})
}

// MFAEnable active le 2FA apres verification du code
func MFAEnable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		jsonError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	// Verifier le code
	if !hookValidateTOTP(req.Secret, req.Code) {
		jsonError(w, ErrAuthInvalid, "Code invalide", http.StatusBadRequest)
		return
	}

	// Chiffrer et sauvegarder le secret
	encryptedSecret, err := hookEncryptStr(req.Secret)
	if err != nil {
		jsonError(w, ErrEncryption, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	if err := hookEnableMFA(user.ID, encryptedSecret); err != nil {
		jsonError(w, ErrInternal, "Erreur sauvegarde", http.StatusInternalServerError)
		return
	}

	hookLogAudit(user.ID, db.AuditMFAEnable, getClientIP(r), r.UserAgent())

	jsonSuccess(w, map[string]bool{"success": true})
}

// MFADisable desactive le 2FA after verifying the user's current password
func MFADisable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	// Require password re-verification before disabling MFA
	currentPassword := r.FormValue("current_password")
	if currentPassword == "" {
		clientError(w, ErrValidation, "Mot de passe requis pour désactiver le 2FA", http.StatusBadRequest)
		return
	}

	// Retrieve the stored password hash to verify
	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		clientError(w, ErrNotFound, "Utilisateur non trouvé", http.StatusNotFound)
		return
	}

	if !hookVerifyPassword(currentPassword, dbUser.Password) {
		clientError(w, ErrAuthInvalid, "Mot de passe incorrect", http.StatusForbidden)
		return
	}

	if err := hookDisableMFA(user.ID); err != nil {
		serverError(w, "disable MFA", err)
		return
	}

	hookLogAudit(user.ID, db.AuditMFADisable, getClientIP(r), r.UserAgent())

	// Rediriger vers settings pour recharger la page
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
