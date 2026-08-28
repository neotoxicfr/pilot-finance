package handlers

import (
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
		clientError(w, ErrRateLimited, "Trop de tentatives. Réessayez dans "+strconv.FormatInt(waitMin, 10)+" min.", http.StatusTooManyRequests)
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
			clientError(w, ErrRateLimited, "Trop de tentatives 2FA. Réessayez dans "+strconv.FormatInt(waitMin, 10)+" min.", http.StatusTooManyRequests)
			return
		}

		// Validation du code 2FA
		pendingUserID, err := hookValidatePending2FAToken(pendingCookie.Value)
		if err != nil {
			clientError(w, ErrAuthInvalid, "Session expirée, veuillez vous reconnecter", http.StatusUnauthorized)
			return
		}

		user, err := hookGetUserByID(pendingUserID)
		if err != nil || user == nil {
			clientError(w, ErrAuthInvalid, "Utilisateur non trouvé", http.StatusUnauthorized)
			return
		}

		// Déchiffrer le secret MFA
		if user.MFASecret == nil {
			clientError(w, ErrAuthInvalid, "Configuration MFA incomplète", http.StatusUnauthorized)
			return
		}
		secret, err := hookDecryptStr(*user.MFASecret)
		if err != nil {
			serverError(w, "decrypt MFA secret", err)
			return
		}

		if !hookValidateTOTP(secret, twoFactorCode) {
			clientError(w, ErrAuthInvalid, "Code 2FA invalide", http.StatusUnauthorized)
			return
		}

		clearCookie(w, "pending_2fa")

		// Générer le token JWT
		token, err := hookGenerateToken(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
		if err != nil {
			serverError(w, "generate token", err)
			return
		}

		setSessionCookie(w, "session", token, 86400)

		// Réinitialiser les rate limiters (IP + compte)
		hookRateLimitReset(clientIP, "login")
		hookRateLimitReset(strconv.FormatInt(user.ID, 10), "loginAccount")

		hookLogAudit(user.ID, db.AuditLoginSuccess, clientIP, r.UserAgent())

		// Rediriger vers le dashboard
		htmxRedirect(w, r, "/")
		return
	}

	// Login normal (première étape)
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		clientError(w, ErrValidation, "Email et mot de passe requis", http.StatusBadRequest)
		return
	}

	// Chercher l'utilisateur par blind index
	blindIndex := hookComputeBlindIndex(email)
	user, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil {
		serverError(w, "get user", err)
		return
	}

	if user == nil {
		// Égaliser le temps de réponse : exécuter un bcrypt dummy pour empêcher
		// la détection d'email existant via timing oracle (~100ms vs ~1ms).
		// Le résultat est ignoré.
		_ = hookVerifyPassword(password, dummyPasswordHash)
		clientError(w, ErrAuthInvalid, "Identifiants incorrects", http.StatusUnauthorized)
		return
	}

	// Rate limiting par compte (protection brute-force distribué multi-IP)
	accountKey := strconv.FormatInt(user.ID, 10)
	acctResult := hookRateLimitCheck(accountKey, "loginAccount")
	if !acctResult.Allowed {
		waitMin := (acctResult.RetryAfterMs / 60000) + 1
		clientError(w, ErrRateLimited, "Trop de tentatives sur ce compte. Réessayez dans "+strconv.FormatInt(waitMin, 10)+" min.", http.StatusTooManyRequests)
		return
	}

	// Vérifier le verrouillage
	if user.LockUntil != nil && user.LockUntil.After(time.Now()) {
		waitMin := int(time.Until(*user.LockUntil).Minutes()) + 1
		clientError(w, ErrAccountLocked, "Compte verrouillé. Réessayez dans "+strconv.Itoa(waitMin)+" min.", http.StatusTooManyRequests)
		return
	}

	// Vérifier le mot de passe
	if !hookVerifyPassword(password, user.Password) {
		hookLogAudit(user.ID, db.AuditLoginFail, clientIP, r.UserAgent())
		handleFailedLogin(user)
		clientError(w, ErrAuthInvalid, "Identifiants incorrects", http.StatusUnauthorized)
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
			serverError(w, "generate pending 2FA token", err)
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
		serverError(w, "generate token", err)
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
		clientError(w, ErrForbidden, "Inscription désactivée", http.StatusForbidden)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := hookRateLimitCheck(clientIP, "register")
	if !result.Allowed {
		clientError(w, ErrRateLimited, "Trop de tentatives. Réessayez plus tard.", http.StatusTooManyRequests)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	// Validation
	if email == "" || password == "" {
		clientError(w, ErrValidation, "Email et mot de passe requis", http.StatusBadRequest)
		return
	}

	if !strings.Contains(email, "@") || len(email) < 3 || len(email) > 254 {
		clientError(w, ErrValidation, "Email invalide", http.StatusBadRequest)
		return
	}

	if password != confirmPassword {
		clientError(w, ErrValidation, "Les mots de passe ne correspondent pas", http.StatusBadRequest)
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
		serverError(w, "get user", err)
		return
	}

	if existingUser != nil {
		clientError(w, ErrConflict, "Email déjà utilisé", http.StatusConflict)
		return
	}

	// Hasher le mot de passe
	hashedPassword, err := hookHashPassword(password)
	if err != nil {
		serverError(w, "hash password", err)
		return
	}

	// Chiffrer l'email
	encryptedEmail, err := hookEncryptStr(email)
	if err != nil {
		serverError(w, "encrypt email", err)
		return
	}

	// Créer l'utilisateur en assignant le rôle ADMIN dans la même transaction
	// si aucun admin n'existe encore (L2 fix : sérialise l'attribution ADMIN
	// même sous deux inscriptions concurrentes).
	userID, role, err := hookCreateUserAtomic(encryptedEmail, blindIndex, hashedPassword)
	if err != nil {
		serverError(w, "create user", err)
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
		serverError(w, "generate token", err)
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
