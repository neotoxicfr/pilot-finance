package handlers

import (
	"net/http"
	"os"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
	"pilot-finance/internal/templates"
)

// LoginPage affiche la page de connexion
func LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title":          "Connexion",
		"CanRegister":    os.Getenv("ALLOW_REGISTER") == "true",
		"CanUsePasskeys": os.Getenv("HOST") != "",
		"MailEnabled":    os.Getenv("SMTP_HOST") != "",
		"ResetSuccess":   r.URL.Query().Get("reset") == "success",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.Render(w, "login.html", data); err != nil {
		http.Error(w, "Erreur template", http.StatusInternalServerError)
	}
}

// LoginSubmit traite la soumission du formulaire de connexion
func LoginSubmit(w http.ResponseWriter, r *http.Request) {
	HandleLogin(w, r)
}

// RegisterPage affiche la page d'inscription
func RegisterPage(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("ALLOW_REGISTER") != "true" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	LoginPage(w, r)
}

// RegisterSubmit traite la soumission du formulaire d'inscription
func RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	HandleRegister(w, r)
}

// Logout deconnecte l'utilisateur
func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Dashboard affiche le tableau de bord
func Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	accounts, err := db.GetAccountsByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Déchiffrer les noms des comptes
	for i := range accounts {
		if decrypted, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		}
	}

	// Calculer les projections avec interets composes
	years := 5
	projData := projection.Calculate(accounts, years)

	// Donnees pour le graphique camembert
	var pieData []map[string]interface{}
	for _, acc := range accounts {
		if acc.Balance > 0 {
			pieData = append(pieData, map[string]interface{}{
				"name":  acc.Name,
				"value": acc.Balance,
				"color": acc.Color,
			})
		}
	}

	// Projection finale (annee N)
	var projectionTotal float64
	if len(projData.Projection) > 0 {
		projectionTotal = projData.Projection[len(projData.Projection)-1].TotalAvg
	}

	// Preparer la liste des comptes avec couleurs pour le graphique
	accountColors := make([]map[string]interface{}, 0)
	for _, acc := range accounts {
		accountColors = append(accountColors, map[string]interface{}{
			"name":  acc.Name,
			"color": acc.Color,
		})
	}

	data := map[string]interface{}{
		"Title":           "Dashboard",
		"User":            map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role},
		"Accounts":        accounts,
		"AccountColors":   accountColors,
		"TotalBalance":    projData.TotalBalance,
		"TotalInterests":  projData.TotalInterests,
		"Years":           years,
		"ProjectionTotal": projectionTotal,
		"ProjectionData":  projData.Projection,
		"PieData":         pieData,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.Render(w, "dashboard.html", data); err != nil {
		http.Error(w, "Erreur template: "+err.Error(), http.StatusInternalServerError)
	}
}

// AccountsPage affiche la page des comptes
func AccountsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	accounts, _ := db.GetAccountsByUserID(user.ID)
	recurrings, _ := db.GetRecurringByUserID(user.ID)

	// Déchiffrer les noms des comptes et créer un map pour lookup
	accountMap := make(map[int64]string)
	for i := range accounts {
		if decrypted, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		}
		accountMap[accounts[i].ID] = accounts[i].Name
	}

	// Préparer les récurrents avec déchiffrement et nom de compte
	var monthlyIncome, monthlyExpenses float64
	recurringData := make([]map[string]interface{}, 0, len(recurrings))
	for _, rec := range recurrings {
		description := rec.Description
		if decrypted, err := crypto.Decrypt(rec.Description); err == nil {
			description = decrypted
		}

		if rec.Amount > 0 {
			monthlyIncome += rec.Amount
		} else {
			monthlyExpenses += -rec.Amount
		}

		toAccountName := ""
		if rec.ToAccountID != nil {
			toAccountName = accountMap[*rec.ToAccountID]
		}

		recurringData = append(recurringData, map[string]interface{}{
			"ID":            rec.ID,
			"Description":   description,
			"Amount":        rec.Amount,
			"DayOfMonth":    rec.DayOfMonth,
			"AccountID":     rec.AccountID,
			"AccountName":   accountMap[rec.AccountID],
			"ToAccountID":   rec.ToAccountID,
			"ToAccountName": toAccountName,
			"IsActive":      rec.IsActive,
		})
	}

	data := map[string]interface{}{
		"Title":           "Comptes",
		"User":            map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role},
		"Accounts":        accounts,
		"Recurrings":      recurringData,
		"MonthlyIncome":   monthlyIncome,
		"MonthlyExpenses": monthlyExpenses,
		"MonthlyNet":      monthlyIncome - monthlyExpenses,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.Render(w, "accounts.html", data); err != nil {
		http.Error(w, "Erreur template: "+err.Error(), http.StatusInternalServerError)
	}
}

// SettingsPage affiche la page des parametres
func SettingsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Récupérer l'utilisateur complet pour MFAEnabled
	dbUser, _ := db.GetUserByID(user.ID)
	mfaEnabled := false
	if dbUser != nil {
		mfaEnabled = dbUser.MFAEnabled
	}

	isAdmin := user.Role == "ADMIN"

	data := map[string]interface{}{
		"Title":           "Parametres",
		"User":            map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role},
		"MFAEnabled":      mfaEnabled,
		"PasskeysEnabled": os.Getenv("HOST") != "",
		"Passkeys":        []interface{}{},
		"IsAdmin":         isAdmin,
		"IsRegisterOpen":  os.Getenv("ALLOW_REGISTER") == "true",
		"Users":           []interface{}{},
	}

	passkeys, _ := db.GetAuthenticatorsByUserID(user.ID)
	data["Passkeys"] = passkeys

	if isAdmin {
		users, _ := db.GetAllUsers()
		var usersWithEmail []map[string]interface{}
		for _, u := range users {
			uEmail, _ := crypto.Decrypt(u.EmailEncrypted)
			usersWithEmail = append(usersWithEmail, map[string]interface{}{
				"ID":    u.ID,
				"Email": uEmail,
				"Role":  u.Role,
			})
		}
		data["Users"] = usersWithEmail
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.Render(w, "settings.html", data); err != nil {
		http.Error(w, "Erreur template: "+err.Error(), http.StatusInternalServerError)
	}
}

// AdminPage affiche la page d'administration
func AdminPage(w http.ResponseWriter, r *http.Request) {
	SettingsPage(w, r)
}

// RecurringPage redirige vers la page des comptes
func RecurringPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// VerifyEmailPage verifie l'email avec le token
func VerifyEmailPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	data := map[string]interface{}{
		"Title":   "Verification email",
		"Success": false,
		"Error":   "",
	}

	if token == "" {
		data["Error"] = "Jeton manquant."
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "verify-email.html", data)
		return
	}

	// Hasher le token pour la recherche
	hashedToken := crypto.HashToken(token)

	// Verifier le token
	err := db.VerifyEmailByToken(hashedToken)
	if err != nil {
		if err == db.ErrTokenInvalid {
			data["Error"] = "Jeton invalide ou expire."
		} else {
			data["Error"] = "Erreur serveur."
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		templates.Render(w, "verify-email.html", data)
		return
	}

	data["Success"] = true

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.Render(w, "verify-email.html", data)
}
