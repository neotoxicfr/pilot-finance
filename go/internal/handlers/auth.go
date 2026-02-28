package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/ratelimit"
	"pilot-finance/internal/templates"
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

// getClientIP extrait l'IP du client
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

// HandleLogin gère la soumission du formulaire de connexion
func HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := ratelimit.Check(clientIP, "login")
	if !result.Allowed {
		waitMin := (result.RetryAfterMs / 60000) + 1
		http.Error(w, "Trop de tentatives. Réessayez dans "+strconv.FormatInt(waitMin, 10)+" min.", http.StatusTooManyRequests)
		return
	}

	twoFactorCode := r.FormValue("twoFactorCode")

	// Vérifier s'il y a un pending_2fa cookie (deuxième étape de login avec 2FA)
	pendingCookie, _ := r.Cookie("pending_2fa")
	if pendingCookie != nil && twoFactorCode != "" {
		// Validation du code 2FA
		pendingUserID, err := auth.ValidatePending2FAToken(pendingCookie.Value)
		if err != nil {
			http.Error(w, "Session expirée, veuillez vous reconnecter", http.StatusUnauthorized)
			return
		}

		user, err := db.GetUserByID(pendingUserID)
		if err != nil || user == nil {
			http.Error(w, "Utilisateur non trouvé", http.StatusUnauthorized)
			return
		}

		// Déchiffrer le secret MFA
		secret, err := crypto.Decrypt(*user.MFASecret)
		if err != nil {
			http.Error(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		if !auth.ValidateTOTP(secret, twoFactorCode) {
			http.Error(w, "Code 2FA invalide", http.StatusUnauthorized)
			return
		}

		clearCookie(w, "pending_2fa")

		// Générer le token JWT
		token, err := auth.GenerateToken(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
		if err != nil {
			http.Error(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, "session", token, 86400)

		// Réinitialiser le rate limiter
		ratelimit.Reset(clientIP, "login")

		db.LogAudit(user.ID, db.AuditLoginSuccess, clientIP, r.UserAgent())

		// Rediriger vers le dashboard
		htmxRedirect(w, r, "/")
		return
	}

	// Login normal (première étape)
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		http.Error(w, "Email et mot de passe requis", http.StatusBadRequest)
		return
	}

	// Chercher l'utilisateur par blind index
	blindIndex := crypto.ComputeBlindIndex(email)
	user, err := db.GetUserByBlindIndex(blindIndex)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	if user == nil {
		http.Error(w, "Identifiants incorrects", http.StatusUnauthorized)
		return
	}

	// Vérifier le verrouillage
	if user.LockUntil != nil && user.LockUntil.After(time.Now()) {
		waitMin := int(time.Until(*user.LockUntil).Minutes()) + 1
		http.Error(w, "Compte verrouillé. Réessayez dans "+strconv.Itoa(waitMin)+" min.", http.StatusTooManyRequests)
		return
	}

	// Vérifier le mot de passe
	if !crypto.VerifyPassword(password, user.Password) {
		db.LogAudit(user.ID, db.AuditLoginFail, clientIP, r.UserAgent())
		handleFailedLogin(user)
		http.Error(w, "Identifiants incorrects", http.StatusUnauthorized)
		return
	}

	// Réinitialiser les échecs
	if user.FailedLoginAttempts > 0 || user.LockUntil != nil {
		resetLoginAttempts(user.ID)
	}

	// Mise à niveau silencieuse du hash bcrypt (cost 10 → 12) sans invalider les sessions
	if crypto.NeedsRehash(user.Password) {
		if newHash, err := crypto.HashPassword(password); err == nil {
			db.UpdatePasswordHash(user.ID, newHash)
		}
	}

	// Vérifier 2FA si activé
	if user.MFAEnabled {
		// Stocker l'ID utilisateur validé dans un cookie temporaire signé
		pendingToken, err := auth.GeneratePending2FAToken(user.ID)
		if err != nil {
			http.Error(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, "pending_2fa", pendingToken, 300) // 5 minutes

		// Rendre la page login avec le formulaire 2FA visible et les traductions correctes
		data := baseData(r, nil)
		data["Title"] = "Connexion"
		data["CanRegister"] = os.Getenv("ALLOW_REGISTER") == "true"
		data["CanUsePasskeys"] = os.Getenv("HOST") != ""
		data["MailEnabled"] = os.Getenv("SMTP_HOST") != ""
		data["Requires2FA"] = true
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "login.html", data)
		return
	}

	// Générer le token JWT
	token, err := auth.GenerateToken(user.ID, user.Role, user.Language, user.Currency, user.SessionVersion)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	setSessionCookie(w, "session", token, 86400) // 24 heures

	// Réinitialiser le rate limiter
	ratelimit.Reset(clientIP, "login")

	db.LogAudit(user.ID, db.AuditLoginSuccess, clientIP, r.UserAgent())

	// Rediriger vers le dashboard
	htmxRedirect(w, r, "/")
}

// HandleRegister gère l'inscription
func HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	clientIP := getClientIP(r)

	// Rate limiting
	result := ratelimit.Check(clientIP, "register")
	if !result.Allowed {
		http.Error(w, "Trop de tentatives. Réessayez plus tard.", http.StatusTooManyRequests)
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	// Validation
	if email == "" || password == "" {
		http.Error(w, "Email et mot de passe requis", http.StatusBadRequest)
		return
	}

	if !strings.Contains(email, "@") || len(email) < 3 || len(email) > 254 {
		http.Error(w, "Email invalide", http.StatusBadRequest)
		return
	}

	if password != confirmPassword {
		http.Error(w, "Les mots de passe ne correspondent pas", http.StatusBadRequest)
		return
	}

	if err := crypto.ValidatePassword(password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Vérifier si c'est le premier utilisateur
	userCount, err := hookCountUsers()
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	isFirstUser := userCount == 0

	// Vérifier si l'email existe déjà
	blindIndex := crypto.ComputeBlindIndex(email)
	existingUser, err := hookGetUserByBlindIndex(blindIndex)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	if existingUser != nil {
		http.Error(w, "Email déjà utilisé", http.StatusConflict)
		return
	}

	// Hasher le mot de passe
	hashedPassword, err := hookHashPassword(password)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Chiffrer l'email
	encryptedEmail, err := hookEncryptStr(email)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Créer l'utilisateur
	role := "USER"
	if isFirstUser {
		role = "ADMIN"
	}

	userID, err := hookCreateUser(encryptedEmail, blindIndex, hashedPassword, role)
	if err != nil {
		http.Error(w, "Erreur création compte", http.StatusInternalServerError)
		return
	}

	// Générer le token et connecter (nouveaux utilisateurs : langue/devise par défaut)
	token, err := hookGenerateToken(userID, role, "fr", "EUR", 1)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	setSessionCookie(w, "session", token, 86400)

	htmxRedirect(w, r, "/")
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

	db.UpdateLoginAttempts(user.ID, newCount, lockUntil)
}

// resetLoginAttempts réinitialise les tentatives de connexion
func resetLoginAttempts(userID int64) {
	db.UpdateLoginAttempts(userID, 0, nil)
}
