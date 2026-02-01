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

		// Supprimer le cookie pending_2fa
		http.SetCookie(w, &http.Cookie{
			Name:   "pending_2fa",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		// Déchiffrer l'email pour le token
		decryptedEmail, err := crypto.Decrypt(user.EmailEncrypted)
		if err != nil {
			http.Error(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		// Générer le token JWT
		token, err := auth.GenerateToken(user.ID, decryptedEmail, user.Role, user.SessionVersion)
		if err != nil {
			http.Error(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		// Définir le cookie de session
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			MaxAge:   86400,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		// Réinitialiser le rate limiter
		ratelimit.Reset(clientIP, "login")

		// Rediriger vers le dashboard
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		handleFailedLogin(user)
		http.Error(w, "Identifiants incorrects", http.StatusUnauthorized)
		return
	}

	// Réinitialiser les échecs
	if user.FailedLoginAttempts > 0 || user.LockUntil != nil {
		resetLoginAttempts(user.ID)
	}

	// Vérifier 2FA si activé
	if user.MFAEnabled {
		// Stocker l'ID utilisateur validé dans un cookie temporaire signé
		pendingToken, err := auth.GeneratePending2FAToken(user.ID)
		if err != nil {
			http.Error(w, "Erreur serveur", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "pending_2fa",
			Value:    pendingToken,
			Path:     "/",
			MaxAge:   300, // 5 minutes
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})

		// Rendre la page login avec le formulaire 2FA visible
		data := map[string]interface{}{
			"Title":          "Connexion",
			"CanRegister":    os.Getenv("ALLOW_REGISTER") == "true",
			"CanUsePasskeys": os.Getenv("HOST") != "",
			"MailEnabled":    os.Getenv("SMTP_HOST") != "",
			"Requires2FA":    true,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "login.html", data)
		return
	}

	// Déchiffrer l'email pour le token
	decryptedEmail, err := crypto.Decrypt(user.EmailEncrypted)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Générer le token JWT
	token, err := auth.GenerateToken(user.ID, decryptedEmail, user.Role, user.SessionVersion)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Définir le cookie de session
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   86400, // 24 heures
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// Réinitialiser le rate limiter
	ratelimit.Reset(clientIP, "login")

	// Rediriger vers le dashboard
	http.Redirect(w, r, "/", http.StatusSeeOther)
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

	if password != confirmPassword {
		http.Error(w, "Les mots de passe ne correspondent pas", http.StatusBadRequest)
		return
	}

	if len(password) < 8 {
		http.Error(w, "Mot de passe trop court (min 8 caractères)", http.StatusBadRequest)
		return
	}

	// Vérifier si c'est le premier utilisateur
	userCount, err := db.CountUsers()
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	isFirstUser := userCount == 0

	// Vérifier si l'email existe déjà
	blindIndex := crypto.ComputeBlindIndex(email)
	existingUser, err := db.GetUserByBlindIndex(blindIndex)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	if existingUser != nil {
		http.Error(w, "Email déjà utilisé", http.StatusConflict)
		return
	}

	// Hasher le mot de passe
	hashedPassword, err := crypto.HashPassword(password)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Chiffrer l'email
	encryptedEmail, err := crypto.Encrypt(email)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Créer l'utilisateur
	role := "USER"
	if isFirstUser {
		role = "ADMIN"
	}

	userID, err := db.CreateUser(encryptedEmail, blindIndex, hashedPassword, role)
	if err != nil {
		http.Error(w, "Erreur création compte", http.StatusInternalServerError)
		return
	}

	// Générer le token et connecter
	token, err := auth.GenerateToken(userID, email, role, 1)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
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
