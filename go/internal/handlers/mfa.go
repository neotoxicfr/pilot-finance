package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"log/slog"
	"net/http"

	"github.com/pquerna/otp"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
)

// Géométrie du QR code d'enrôlement TOTP. L'image reste de 200×200 px, comme
// avec skip2/go-qrcode. qrQuietZone est la marge blanche réservée sur chaque
// bord : boombuler ne dessine aucune quiet zone alors que la norme ISO/IEC
// 18004 en exige 4 modules. 14 px garantit ≥ 4 modules jusqu'à la version 10
// (57×57), soit bien au-delà des URI otpauth:// produites ici.
const (
	qrSize      = 200
	qrQuietZone = 14
)

// qrEncodePNG génère le PNG (size × size) du QR code d'enrôlement TOTP à
// partir de l'URI otpauth://.
//
// Remplace github.com/skip2/go-qrcode (upstream sans commit depuis 2020) par
// otp.Key.Image, qui s'appuie sur github.com/boombuler/barcode — déjà présent
// dans l'arbre via pquerna/otp, et activement maintenu. Key.Image encode
// k.orig : l'URI est donc reprise à l'octet près, avec le même niveau de
// correction d'erreur (M) qu'auparavant.
//
// boombuler met la matrice à l'échelle avec un facteur entier puis la centre,
// mais sans quiet zone. On encode donc sur une surface réduite de
// 2*qrQuietZone, que l'on recentre ensuite sur un fond blanc.
func qrEncodePNG(otpauthURI string, size int) ([]byte, error) {
	key, err := otp.NewKeyFromURL(otpauthURI)
	if err != nil {
		return nil, err
	}

	inner := size - 2*qrQuietZone
	code, err := key.Image(inner, inner)
	if err != nil {
		return nil, err
	}

	// Palette 1 bit (blanc, noir) : le pixel zéro vaut déjà blanc, donc seule
	// la matrice reste à dessiner. Sortie PNG compacte, comme avec skip2.
	img := image.NewPaletted(
		image.Rect(0, 0, size, size),
		color.Palette{color.White, color.Black},
	)
	draw.Draw(img,
		image.Rect(qrQuietZone, qrQuietZone, size-qrQuietZone, size-qrQuietZone),
		code, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := hookPNGEncode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MFASetup retourne le QR code pour configurer le 2FA et stocke le secret
// dans un cookie signé HS256 (mfa_setup, 5 min). Le secret n'est PLUS exposé
// dans la réponse JSON (M3 fix : empêche le client de choisir un secret
// arbitraire au moment du /enable).
func MFASetup(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		jsonErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	// Generer un nouveau secret
	secret, err := hookGenerateTOTPSecret()
	if err != nil {
		slog.Error("generate TOTP secret", "err", err)
		jsonErrorT(w, r, ErrInternal, "error.internal", http.StatusInternalServerError)
		return
	}

	// Generer l'URI pour le QR code
	otpauthURI := hookGenerateTOTPURI(secret, user.Email)

	// Generer le QR code localement (zéro dépendance externe)
	png, err := hookQREncode(otpauthURI, qrSize)
	if err != nil {
		slog.Error("generate QR code", "err", err)
		jsonErrorT(w, r, ErrInternal, "error.qr_generation", http.StatusInternalServerError)
		return
	}
	qrDataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)

	// Signer le secret dans un cookie HS256 — il n'est PAS renvoyé au client.
	mfaToken, err := hookGenerateMFASetupToken(user.ID, secret)
	if err != nil {
		slog.Error("generate MFA setup token", "err", err)
		jsonErrorT(w, r, ErrInternal, "error.internal", http.StatusInternalServerError)
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
		jsonErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErrorT(w, r, ErrValidation, "error.invalid_data", http.StatusBadRequest)
		return
	}

	// Lire le secret depuis le cookie signé posé par MFASetup
	cookie, err := r.Cookie("mfa_setup")
	if err != nil {
		jsonErrorT(w, r, ErrAuthInvalid, "error.mfa_setup_expired", http.StatusBadRequest)
		return
	}
	tokenUserID, secret, err := hookValidateMFASetupToken(cookie.Value)
	if err != nil {
		jsonErrorT(w, r, ErrAuthInvalid, "error.mfa_setup_invalid", http.StatusBadRequest)
		return
	}
	// Le cookie doit appartenir à l'utilisateur courant
	if tokenUserID != user.ID {
		jsonErrorT(w, r, ErrAuthInvalid, "error.mfa_setup_invalid", http.StatusBadRequest)
		return
	}

	// Verifier le code TOTP contre le secret côté serveur
	if !hookValidateTOTP(secret, req.Code) {
		jsonErrorT(w, r, ErrAuthInvalid, "error.code_invalid", http.StatusBadRequest)
		return
	}

	// Chiffrer et sauvegarder le secret
	encryptedSecret, err := hookEncryptStr(secret)
	if err != nil {
		jsonErrorT(w, r, ErrEncryption, "error.internal", http.StatusInternalServerError)
		return
	}

	// Codes de récupération (audit S-22) : générés et stockés AVANT
	// l'activation du TOTP. Dans cet ordre, un échec d'écriture laisse le
	// compte exactement dans son état d'origine (2FA toujours inactive) ;
	// l'ordre inverse pouvait activer le TOTP sans le moindre code de secours,
	// c'est-à-dire recréer précisément la situation que S-22 corrige.
	codes, err := generateAndStoreRecoveryCodes(user.ID)
	if err != nil {
		slog.Error("generate recovery codes", "err", err, "userID", user.ID)
		jsonErrorT(w, r, ErrInternal, "error.mfa_save_failed", http.StatusInternalServerError)
		return
	}

	if err := hookEnableMFA(user.ID, encryptedSecret); err != nil {
		jsonErrorT(w, r, ErrInternal, "error.mfa_save_failed", http.StatusInternalServerError)
		return
	}

	// Effacer le cookie de setup (single-use). Le Path doit matcher
	// celui posé par MFASetup pour que le navigateur efface l'entrée.
	clearScopedCookie(w, "mfa_setup", "/settings/mfa")

	hookLogAudit(user.ID, db.AuditMFAEnable, getClientIP(r), r.UserAgent())

	// Unique occasion où les codes transitent en clair : la base n'en garde
	// que le hash, ils ne pourront plus jamais être réaffichés.
	jsonSuccess(w, map[string]interface{}{
		"success":       true,
		"recoveryCodes": codes,
	})
}

// generateAndStoreRecoveryCodes tire un lot de codes de récupération, n'en
// persiste que les hashes et retourne les codes en clair à l'appelant.
//
// CHOIX DU HACHAGE — SHA-256 (crypto.HashToken), pas bcrypt, contrairement aux
// mots de passe. Deux raisons, toutes deux propres à ces codes :
//
//  1. Entropie. Un code fait 60 bits tirés de crypto/rand, là où un mot de
//     passe humain en vaut couramment moins de 30 et se retrouve dans les
//     dictionnaires. Le facteur de travail de bcrypt sert exactement à rendre
//     une attaque par dictionnaire coûteuse ; sur un secret uniformément
//     aléatoire de 60 bits il n'y a pas de dictionnaire, et l'énumération est
//     hors de portée même à des milliards de hashes par seconde. Le bénéfice de
//     bcrypt est donc quasi nul ici.
//
//  2. Coût. bcrypt est salé, donc non consultable : vérifier une saisie
//     imposerait de comparer contre LES DIX hashes de l'utilisateur, soit
//     ~10 × 250 ms de CPU serveur par tentative — une amplification de déni de
//     service offerte à un endpoint non authentifié. Le hash déterministe
//     SHA-256 se résout en un seul UPDATE indexé.
//
// Le garde-fou anti-force-brute ne repose de toute façon pas sur le coût du
// hachage mais sur le rate limiting « twoFactor » partagé avec le TOTP
// (5 tentatives / 5 min, blocage 15 min), appliqué dans HandleLogin.
func generateAndStoreRecoveryCodes(userID int64) ([]string, error) {
	codes, err := hookGenerateRecoveryCodes()
	if err != nil {
		return nil, err
	}

	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = hookHashToken(auth.NormalizeRecoveryCode(c))
	}

	if err := hookReplaceRecoveryCodes(userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

// MFADisable desactive le 2FA after verifying the user's current password
func MFADisable(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_data", http.StatusBadRequest)
		return
	}

	// Require password re-verification before disabling MFA
	currentPassword := r.FormValue("current_password")
	if currentPassword == "" {
		clientErrorT(w, r, ErrValidation, "error.password_required_disable_mfa", http.StatusBadRequest)
		return
	}

	// Retrieve the stored password hash to verify
	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		clientErrorT(w, r, ErrNotFound, "error.user_not_found", http.StatusNotFound)
		return
	}

	if !hookVerifyPassword(currentPassword, dbUser.Password) {
		clientErrorT(w, r, ErrAuthInvalid, "error.password_incorrect", http.StatusUnauthorized)
		return
	}

	if err := hookDisableMFA(user.ID); err != nil {
		serverError(w, r, "disable MFA", err)
		return
	}

	// Les codes de récupération ne servent QU'À contourner le second facteur :
	// les laisser vivre après une désactivation du 2FA reviendrait à garder des
	// identifiants de secours valides pour un compte qui n'a plus de 2FA, et à
	// les voir ressusciter à la réactivation. On les supprime dans la foulée.
	// L'échec n'annule pas la désactivation (déjà persistée) mais doit être
	// tracé : il laisse des codes orphelins.
	if err := hookDeleteRecoveryCodes(user.ID); err != nil {
		slog.Error("suppression des codes de récupération après désactivation MFA",
			"err", err, "userID", user.ID)
	}

	hookLogAudit(user.ID, db.AuditMFADisable, getClientIP(r), r.UserAgent())

	// Rediriger vers settings pour recharger la page
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// MFARecoveryCount rend le compteur de codes de récupération inutilisés sous
// forme de fragment HTMX.
//
// Le compte n'est pas injecté par SettingsPage : le fragment est chargé par la
// page via hx-trigger="load", ce qui évite d'alourdir le rendu initial et
// permet de rafraîchir le compteur après une régénération sans recharger.
func MFARecoveryCount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	remaining, err := hookCountRecoveryCodes(user.ID)
	if err != nil {
		serverError(w, r, "count recovery codes", err)
		return
	}

	lang, currency := userLocale(user)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRenderPartial(w, "settings.html", "mfa-recovery-count", map[string]interface{}{ //nolint:errcheck
		"T":         i18n.Map(lang),
		"Currency":  currency,
		"Remaining": remaining,
		"Depleted":  remaining == 0,
	})
}

// MFARecoveryRegenerate remplace le lot de codes de récupération après
// re-vérification du mot de passe, et renvoie le nouveau lot en clair.
//
// La ré-authentification est exigée pour la même raison que sur MFADisable :
// une session volée ne doit pas suffire à se fabriquer un jeu de codes qui
// contournera durablement le TOTP.
func MFARecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		jsonErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	var req struct {
		CurrentPassword string `json:"currentPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErrorT(w, r, ErrValidation, "error.invalid_data", http.StatusBadRequest)
		return
	}
	if req.CurrentPassword == "" {
		jsonErrorT(w, r, ErrValidation, "error.password_required", http.StatusBadRequest)
		return
	}

	dbUser, err := hookGetUserByID(user.ID)
	if err != nil || dbUser == nil {
		jsonErrorT(w, r, ErrNotFound, "error.user_not_found", http.StatusNotFound)
		return
	}

	if !hookVerifyPassword(req.CurrentPassword, dbUser.Password) {
		jsonErrorT(w, r, ErrAuthInvalid, "error.password_incorrect", http.StatusUnauthorized)
		return
	}

	// Sans 2FA active, un lot de codes n'aurait aucune contrepartie à
	// contourner : refuser évite de laisser traîner des secrets inutiles.
	if !dbUser.MFAEnabled {
		jsonErrorT(w, r, ErrConflict, "error.mfa_not_enabled", http.StatusConflict)
		return
	}

	codes, err := generateAndStoreRecoveryCodes(user.ID)
	if err != nil {
		slog.Error("regenerate recovery codes", "err", err, "userID", user.ID)
		jsonErrorT(w, r, ErrInternal, "error.mfa_save_failed", http.StatusInternalServerError)
		return
	}

	hookLogAudit(user.ID, db.AuditMFARecoveryRegen, getClientIP(r), r.UserAgent())

	jsonSuccess(w, map[string]interface{}{
		"success":       true,
		"recoveryCodes": codes,
	})
}
