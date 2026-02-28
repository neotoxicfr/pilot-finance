package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	qrcode "github.com/skip2/go-qrcode"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// MFASetup retourne le secret et le QR code pour configurer le 2FA
func MFASetup(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	// Generer un nouveau secret
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Generer l'URI pour le QR code
	otpauthURI := auth.GenerateTOTPURI(secret, user.Email)

	// Generer le QR code localement (zéro dépendance externe)
	png, err := qrcode.Encode(otpauthURI, qrcode.Medium, 200)
	if err != nil {
		http.Error(w, "Erreur generation QR", http.StatusInternalServerError)
		return
	}
	qrDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"secret":   secret,
		"imageUrl": qrDataURI,
	})
}

// MFAEnable active le 2FA apres verification du code
func MFAEnable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Donnees invalides"})
		return
	}

	// Verifier le code
	if !auth.ValidateTOTP(req.Secret, req.Code) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Code invalide"})
		return
	}

	// Chiffrer et sauvegarder le secret
	encryptedSecret, err := crypto.Encrypt(req.Secret)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Erreur serveur"})
		return
	}

	if err := db.EnableMFA(user.ID, encryptedSecret); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": "Erreur sauvegarde"})
		return
	}

	db.LogAudit(user.ID, db.AuditMFAEnable, getClientIP(r), r.UserAgent())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// MFADisable desactive le 2FA
func MFADisable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	if err := db.DisableMFA(user.ID); err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	db.LogAudit(user.ID, db.AuditMFADisable, getClientIP(r), r.UserAgent())

	// Rediriger vers settings pour recharger la page
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
