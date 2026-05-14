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

// MFASetup retourne le QR code pour configurer le 2FA et stocke le secret
// dans un cookie signé HS256 (mfa_setup, 5 min). Le secret n'est PLUS exposé
// dans la réponse JSON (M3 fix : empêche le client de choisir un secret
// arbitraire au moment du /enable).
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

	// Signer le secret dans un cookie HS256 — il n'est PAS renvoyé au client.
	mfaToken, err := hookGenerateMFASetupToken(user.ID, secret)
	if err != nil {
		slog.Error("generate MFA setup token", "err", err)
		jsonError(w, ErrInternal, "Erreur serveur", http.StatusInternalServerError)
		return
	}
	// L4 fix : cookie limité à /settings/mfa pour réduire la surface
	// d'exfiltration du secret TOTP (claim "sec" dans le JWT).
	setScopedCookie(w, "mfa_setup", mfaToken, 300, "/settings/mfa") // 5 minutes

	jsonSuccess(w, map[string]string{
		"imageUrl": qrDataURI,
	})
}

// MFAEnable active le 2FA après vérification du code TOTP. Le secret est lu
// depuis le cookie `mfa_setup` signé (M3 fix), pas depuis le body.
func MFAEnable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		jsonError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	// Lire le secret depuis le cookie signé posé par MFASetup
	cookie, err := r.Cookie("mfa_setup")
	if err != nil {
		jsonError(w, ErrAuthInvalid, "Session de configuration MFA expirée", http.StatusBadRequest)
		return
	}
	tokenUserID, secret, err := hookValidateMFASetupToken(cookie.Value)
	if err != nil {
		jsonError(w, ErrAuthInvalid, "Session de configuration MFA invalide", http.StatusBadRequest)
		return
	}
	// Le cookie doit appartenir à l'utilisateur courant
	if tokenUserID != user.ID {
		jsonError(w, ErrAuthInvalid, "Session de configuration MFA invalide", http.StatusBadRequest)
		return
	}

	// Verifier le code TOTP contre le secret côté serveur
	if !hookValidateTOTP(secret, req.Code) {
		jsonError(w, ErrAuthInvalid, "Code invalide", http.StatusBadRequest)
		return
	}

	// Chiffrer et sauvegarder le secret
	encryptedSecret, err := hookEncryptStr(secret)
	if err != nil {
		jsonError(w, ErrEncryption, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	if err := hookEnableMFA(user.ID, encryptedSecret); err != nil {
		jsonError(w, ErrInternal, "Erreur sauvegarde", http.StatusInternalServerError)
		return
	}

	// Effacer le cookie de setup (single-use). Le Path doit matcher
	// celui posé par MFASetup pour que le navigateur efface l'entrée.
	clearScopedCookie(w, "mfa_setup", "/settings/mfa")

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
		clientError(w, ErrAuthInvalid, "Mot de passe incorrect", http.StatusUnauthorized)
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
