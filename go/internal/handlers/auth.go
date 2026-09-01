package handlers

import (
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
)

// htmxRedirect envoie HX-Redirect pour les requêtes HTMX (navigation complète),
// ou un 303 standard sinon. Évite le swap body HTMX qui casserait les nonces CSP.
func htmxRedirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// getClientIP extrait l'IP du client, résolue par le middleware TrustedProxy
// (contexte chi ClientIPFrom*) avec repli sur l'hôte de r.RemoteAddr.
// X-Forwarded-For / X-Real-IP ne sont plus lus ici : seul TrustedProxy les
// honore, et uniquement quand le pair direct est dans l'allowlist.
func getClientIP(r *http.Request) string {
	return middleware.ClientIP(r)
}

// dummyPasswordHash est un hash bcrypt valide (cost 12) utilisé pour égaliser
// le temps de réponse quand l'email saisi n'existe pas. Sans ce dummy, un attaquant
// peut détecter la présence d'un email en mesurant la durée de la requête
// (~100ms avec hash, ~1ms sans).
// Le mot de passe en clair correspondant n'a aucune importance — on ne vérifie
// jamais le résultat de la comparaison dummy.
var dummyPasswordHash = "$2a$12$abcdefghijklmnopqrstuuQYO7c5T7C0YyJzUu/2eSoYI8.7qONXi"

// HandleLogin gère la soumission du formulaire de connexion
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)

	// Rate limiting
	result := hookRateLimitCheck(clientIP, "login")
	if !result.Allowed {
		waitMin := (result.RetryAfterMs / 60000) + 1
		clientErrorTn(w, r, ErrRateLimited, "error.rate_limited_min", http.StatusTooManyRequests, waitMin)
		return
	}

	twoFactorCode := r.FormValue("twoFactorCode")

	// Vérifier s'il y a un pending_2fa cookie (deuxième étape de login avec 2FA)
	pendingCookie, _ := r.Cookie("pending_2fa")
	if pendingCookie != nil && twoFactorCode != "" {
		// Rate limit on 2FA verification (prevents TOTP brute-force)
		twoFAResult := hookRateLimitCheck(clientIP, "twoFactor")
		if !twoFAResult.Allowed {
			waitMin := (twoFAResult.RetryAfterMs / 60000) + 1
			clientErrorTn(w, r, ErrRateLimited, "error.rate_limited_2fa_min", http.StatusTooManyRequests, waitMin)
			return
		}

		// Validation du code 2FA
		pendingUserID, err := hookValidatePending2FAToken(pendingCookie.Value)
		if err != nil {
			clientErrorT(w, r, ErrAuthInvalid, "error.session_expired_relogin", http.StatusUnauthorized)
			return
		}

		user, err := hookGetUserByID(pendingUserID)
		if err != nil || user == nil {
			clientErrorT(w, r, ErrAuthInvalid, "error.user_not_found", http.StatusUnauthorized)
			return
		}

		// Déchiffrer le secret MFA
		if user.MFASecret == nil {
			clientErrorT(w, r, ErrAuthInvalid, "error.mfa_config_incomplete", http.StatusUnauthorized)
			return
		}
		secret, err := hookDecryptStr(*user.MFASecret)
		if err != nil {
			serverError(w, r, "decrypt MFA secret", err)
			return
		}

		// Un code de récupération est accepté À LA PLACE du code TOTP
		// (audit S-22). Il est tenté seulement après l'échec du TOTP, et
		// SURTOUT après le hookRateLimitCheck(clientIP, "twoFactor") ci-dessus,
		// qui garde donc les deux formes de vérification sous le même compteur
		// (5 tentatives / 5 min, blocage 15 min). Aucun chemin ne permet
		// d'essayer un code de récupération sans avoir consommé un jeton de ce
		// rate limiter : sans cela, l'ajout des codes ouvrirait un
		// contournement du garde-fou anti-force-brute du TOTP.
		usedRecoveryCode := false
		if !hookValidateTOTP(secret, twoFactorCode) {
			if !consumeRecoveryCode(user.ID, twoFactorCode) {
				clientErrorT(w, r, ErrAuthInvalid, "error.totp_invalid", http.StatusUnauthorized)
				return
			}
			usedRecoveryCode = true
		}

		clearCookie(w, "pending_2fa")

		// Générer le token JWT
		token, err := hookGenerateToken(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
		if err != nil {
			serverError(w, r, "generate token", err)
			return
		}

		setSessionCookie(w, "session", token, 86400)

		// Réinitialiser les rate limiters (IP + compte)
		hookRateLimitReset(clientIP, "login")
		hookRateLimitReset(strconv.FormatInt(user.ID, 10), "loginAccount")

		if usedRecoveryCode {
			hookLogAudit(user.ID, db.AuditMFARecoveryUsed, clientIP, r.UserAgent())
		}
		hookLogAudit(user.ID, db.AuditLoginSuccess, clientIP, r.UserAgent())

		// Rediriger vers le dashboard
		htmxRedirect(w, r, "/")
		return
	}

	// Login normal (première étape)
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		clientErrorT(w, r, ErrValidation, "error.email_password_required", http.StatusBadRequest)
		return
	}

	// Chercher l'utilisateur par blind index
	blindIndex := hookComputeBlindIndex(email)
	user, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil {
		serverError(w, r, "get user", err)
		return
	}

	if user == nil {
		// Égaliser le temps de réponse : exécuter un bcrypt dummy pour empêcher
		// la détection d'email existant via timing oracle (~100ms vs ~1ms).
		// Le résultat est ignoré.
		_ = hookVerifyPassword(password, dummyPasswordHash)
		clientErrorT(w, r, ErrAuthInvalid, "error.credentials_invalid", http.StatusUnauthorized)
		return
	}

	// Rate limiting par compte (protection brute-force distribué multi-IP)
	accountKey := strconv.FormatInt(user.ID, 10)
	acctResult := hookRateLimitCheck(accountKey, "loginAccount")
	if !acctResult.Allowed {
		waitMin := (acctResult.RetryAfterMs / 60000) + 1
		clientErrorTn(w, r, ErrRateLimited, "error.rate_limited_account_min", http.StatusTooManyRequests, waitMin)
		return
	}

	// Vérifier le verrouillage
	if user.LockUntil != nil && user.LockUntil.After(time.Now()) {
		waitMin := int64(time.Until(*user.LockUntil).Minutes()) + 1
		clientErrorTn(w, r, ErrAccountLocked, "error.account_locked_min", http.StatusTooManyRequests, waitMin)
		return
	}

	// Vérifier le mot de passe
	if !hookVerifyPassword(password, user.Password) {
		hookLogAudit(user.ID, db.AuditLoginFail, clientIP, r.UserAgent())
		handleFailedLogin(user)
		clientErrorT(w, r, ErrAuthInvalid, "error.credentials_invalid", http.StatusUnauthorized)
		return
	}

	// Réinitialiser les échecs
	if user.FailedLoginAttempts > 0 || user.LockUntil != nil {
		resetLoginAttempts(user.ID)
	}

	// Mise à niveau silencieuse du hash bcrypt (cost 10 → 12) sans invalider les sessions
	if hookNeedsRehash(user.Password) {
		if newHash, err := hookHashPassword(password); err == nil {
			if err := hookUpdatePasswordHash(user.ID, newHash); err != nil {
				slog.Warn("rehash: update password hash", "err", err, "userID", user.ID)
			}
		}
	}

	// Vérifier 2FA si activé
	if user.MFAEnabled {
		// Stocker l'ID utilisateur validé dans un cookie temporaire signé
		pendingToken, err := hookGeneratePending2FAToken(user.ID)
		if err != nil {
			serverError(w, r, "generate pending 2FA token", err)
			return
		}

		setSessionCookie(w, "pending_2fa", pendingToken, 300) // 5 minutes

		// Rendre la page login avec le formulaire 2FA visible et les traductions correctes
		data := loginPageData(r)
		data["Requires2FA"] = true
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "login.html", data) //nolint:errcheck
		return
	}

	// Générer le token JWT
	token, err := hookGenerateToken(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
	if err != nil {
		serverError(w, r, "generate token", err)
		return
	}

	setSessionCookie(w, "session", token, 86400) // 24 heures

	// Réinitialiser les rate limiters (IP + compte)
	hookRateLimitReset(clientIP, "login")
	hookRateLimitReset(strconv.FormatInt(user.ID, 10), "loginAccount")

	hookLogAudit(user.ID, db.AuditLoginSuccess, clientIP, r.UserAgent())

	// Rediriger vers le dashboard
	htmxRedirect(w, r, "/")
}

// consumeRecoveryCode tente de consommer la saisie comme code de récupération
// 2FA à usage unique (audit S-22).
//
// NormalizeRecoveryCode fait office de filtre : une saisie qui n'a pas la forme
// d'un code (un TOTP à 6 chiffres, typiquement) est écartée sans requête. Un
// code consommé est marqué utilisé côté base et ne peut plus resservir ; une
// erreur SQL est journalisée et traitée comme un échec de vérification.
func consumeRecoveryCode(userID int64, input string) bool {
	normalized := auth.NormalizeRecoveryCode(input)
	if normalized == "" {
		return false
	}

	ok, err := hookConsumeRecoveryCode(userID, hookHashToken(normalized))
	if err != nil {
		slog.Error("consume recovery code", "err", err, "userID", userID)
		return false
	}
	return ok
}

// registrationOpen indique si l'inscription est autorisée, et constitue l'unique
// source de vérité partagée par le GET (RegisterPage) et le POST
// (HandleRegister) — avant, l'UI annonçait « fermé » là où l'endpoint acceptait
// encore (audit S-08).
//
// Deux cas seulement :
//   - ALLOW_REGISTER=true : inscription ouverte en permanence ;
//   - base sans aucun utilisateur : bootstrap du premier compte (qui reçoit
//     ADMIN). Sans cette dérogation une instance fraîche serait ininstallable ;
//     elle se referme d'elle-même dès que ce compte existe.
//
// Fail-closed : toute erreur de comptage referme l'inscription.
func registrationOpen() bool {
	if os.Getenv("ALLOW_REGISTER") == "true" {
		return true
	}
	count, err := hookCountUsers()
	return err == nil && count == 0
}

// HandleRegister gère l'inscription
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if !registrationOpen() {
		clientErrorT(w, r, ErrForbidden, "error.register_disabled", http.StatusForbidden)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := hookRateLimitCheck(clientIP, "register")
	if !result.Allowed {
		clientErrorT(w, r, ErrRateLimited, "error.rate_limited_later", http.StatusTooManyRequests)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	// Validation
	if email == "" || password == "" {
		clientErrorT(w, r, ErrValidation, "error.email_password_required", http.StatusBadRequest)
		return
	}

	if !strings.Contains(email, "@") || len(email) < 3 || len(email) > 254 {
		clientErrorT(w, r, ErrValidation, "error.email_invalid", http.StatusBadRequest)
		return
	}

	if password != confirmPassword {
		clientErrorT(w, r, ErrValidation, "error.passwords_mismatch", http.StatusBadRequest)
		return
	}

	// Detect user language from Accept-Language header
	detectedLang := detectLanguage(r)

	if err := hookValidatePassword(password); err != nil {
		clientError(w, ErrValidation, i18n.T(detectedLang, "pwd_error."+err.Error()), http.StatusBadRequest)
		return
	}

	// Vérifier si l'email existe déjà
	blindIndex := hookComputeBlindIndex(email)
	existingUser, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil {
		serverError(w, r, "get user", err)
		return
	}

	if existingUser != nil {
		clientErrorT(w, r, ErrConflict, "error.email_taken", http.StatusConflict)
		return
	}

	// Hasher le mot de passe
	hashedPassword, err := hookHashPassword(password)
	if err != nil {
		serverError(w, r, "hash password", err)
		return
	}

	// Chiffrer l'email
	encryptedEmail, err := hookEncryptStr(email)
	if err != nil {
		serverError(w, r, "encrypt email", err)
		return
	}

	// Créer l'utilisateur en assignant le rôle ADMIN dans la même transaction
	// si aucun admin n'existe encore (L2 fix : sérialise l'attribution ADMIN
	// même sous deux inscriptions concurrentes).
	userID, role, err := hookCreateUserAtomic(encryptedEmail, blindIndex, hashedPassword)
	if err != nil {
		serverError(w, r, "create user", err)
		return
	}

	// Persist the detected language preference
	if detectedLang != "fr" {
		_ = hookUpdateUserPrefs(userID, detectedLang, "EUR")
	}

	// Email de vérification (best-effort) : génère un token et l'envoie si SMTP configuré.
	// L'échec n'empêche PAS l'inscription : l'utilisateur peut renvoyer depuis Settings.
	if hookMailIsEnabled() {
		if err := sendVerificationToken(userID, email, detectedLang); err != nil {
			slog.Warn("send verification email", "err", err, "userID", userID)
		}
	}

	// Générer le token et connecter (langue détectée depuis Accept-Language, devise par défaut)
	token, err := hookGenerateToken(userID, role, detectedLang, "EUR", 1)
	if err != nil {
		serverError(w, r, "generate token", err)
		return
	}

	setSessionCookie(w, "session", token, 86400)

	htmxRedirect(w, r, "/")
}

// sendVerificationToken génère un token aléatoire 32 bytes, stocke son hash SHA-256
// et envoie un email contenant le token brut. Best-effort : retourne l'erreur de l'envoi.
func sendVerificationToken(userID int64, email, lang string) error {
	tokenBytes := make([]byte, 32)
	if _, err := hookRandRead(tokenBytes); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes)
	hashed := hookHashToken(token)
	if err := hookSetVerificationToken(userID, hashed); err != nil {
		return err
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost:3000"
	}
	if lang == "" {
		lang = "fr"
	}
	return hookSendVerification(email, token, host, lang)
}

// handleFailedLogin gère un échec de connexion
func handleFailedLogin(user *db.User) {
	newCount := user.FailedLoginAttempts + 1
	var lockUntil *time.Time

	if newCount >= 5 {
		t := time.Now().Add(15 * time.Minute)
		lockUntil = &t
		newCount = 0
	}

	if err := hookUpdateLoginAttempts(user.ID, newCount, lockUntil); err != nil {
		slog.Error("handleFailedLogin: update attempts", "err", err, "userID", user.ID)
	}
}

// resetLoginAttempts réinitialise les tentatives de connexion
func resetLoginAttempts(userID int64) {
	if err := hookUpdateLoginAttempts(userID, 0, nil); err != nil {
		slog.Error("resetLoginAttempts: update attempts", "err", err, "userID", userID)
	}
}

// detectLanguage parses the Accept-Language header and returns "fr" if French
// is preferred, otherwise defaults to "en".
func detectLanguage(r *http.Request) string {
	accept := strings.ToLower(r.Header.Get("Accept-Language"))
	if accept == "" {
		return "en"
	}
	for _, part := range strings.Split(accept, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if tag == "fr" || strings.HasPrefix(tag, "fr-") {
			return "fr"
		}
	}
	return "en"
}
