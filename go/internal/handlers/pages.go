package handlers

import (
	"log/slog"
	"net/http"
	"os"

	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

// localeMap mappe la langue vers la locale BCP-47 pour JS Intl
var localeMap = map[string]string{
	"fr": "fr-FR",
	"en": "en-US",
}

// baseData construit les données communes à toutes les pages (i18n, devise, langue, nonce CSP)
func baseData(r *http.Request, user *middleware.User) map[string]interface{} {
	lang, currency := userLocale(user)
	locale := localeMap[lang]
	if locale == "" {
		locale = "fr-FR"
	}
	return map[string]interface{}{
		"T":           i18n.Map(lang),
		"Lang":        lang,
		"Locale":      locale,
		"Currency":    currency,
		"Nonce":       middleware.GetNonce(r),
		"CurrentPath": r.URL.Path,
		"AssetVer":    Version,
	}
}

// LoginPage affiche la page de connexion
func LoginPage(w http.ResponseWriter, r *http.Request) {
	data := baseData(r, nil)
	data["Title"] = "Connexion"
	data["CanRegister"] = os.Getenv("ALLOW_REGISTER") == "true"
	data["CanUsePasskeys"] = os.Getenv("HOST") != ""
	data["MailEnabled"] = os.Getenv("SMTP_HOST") != ""
	data["ResetSuccess"] = r.URL.Query().Get("reset") == "success"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hookRender(w, "login.html", data); err != nil {
		serverError(w, "render login", err)
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
	user := middleware.GetUser(r)
	if user != nil {
		hookLogAudit(user.ID, db.AuditLogout, getClientIP(r), r.UserAgent())
	}
	clearCookie(w, "session")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Dashboard affiche le tableau de bord
func Dashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}

	decryptAccountNames(accounts)
	recurrings, recErr := hookGetRecurringByUserID(user.ID)
	if recErr != nil {
		slog.Warn("Dashboard: recurring", "err", recErr, "userID", user.ID)
	}

	// Calculer les projections avec interets composes et opérations récurrentes
	years := 5
	projData := projection.Calculate(accounts, recurrings, years, user.Language)

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

	data := baseData(r, user)
	data["Title"] = "Dashboard"
	data["User"] = map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role}
	data["Accounts"] = accounts
	data["HasAccounts"] = len(accounts) > 0
	data["AccountColors"] = accountColors
	data["TotalBalance"] = projData.TotalBalance
	data["TotalInterests"] = projData.TotalInterests
	data["Years"] = years
	data["ProjectionTotal"] = projectionTotal
	data["ProjectionData"] = projData.Projection
	data["PieData"] = pieData

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hookRender(w, "dashboard.html", data); err != nil {
		serverError(w, "render dashboard", err)
	}
}

// AccountsPage affiche la page des comptes
func AccountsPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	lang, _ := userLocale(user)
	accounts, accErr := hookGetAccountsByUserID(user.ID)
	if accErr != nil {
		slog.Warn("AccountsPage: accounts", "err", accErr, "userID", user.ID)
	}
	recurrings, recErr2 := hookGetRecurringByUserID(user.ID)
	if recErr2 != nil {
		slog.Warn("AccountsPage: recurring", "err", recErr2, "userID", user.ID)
	}

	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	// Calculer les yield payouts (intérêts non réinvestis)
	yieldPayouts := projection.CalculateYieldPayouts(accounts, accountMap)
	interestPrefix := i18n.T(lang, "recurring.interest_prefix")

	// Calculer les totaux : séparer versements mensuels et annuels
	var monthlyIncome, monthlyExpenses, monthlyYield, annualYield float64
	for _, payout := range yieldPayouts {
		if payout.PayoutFrequency == "YEARLY" {
			annualYield += payout.Amount
		} else {
			monthlyIncome += payout.Amount
			monthlyYield += payout.Amount
		}
	}
	for _, rec := range recurrings {
		if rec.ToAccountID != nil {
			continue // Virement interne : ne compte pas dans entrées/sorties
		}
		if rec.Amount > 0 {
			monthlyIncome += rec.Amount
		} else {
			monthlyExpenses += -rec.Amount
		}
	}

	recurringData := buildRecurringData(yieldPayouts, recurrings, accountMap, interestPrefix)

	data := baseData(r, user)
	data["Title"] = "Comptes"
	data["User"] = map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role}
	data["Accounts"] = accounts
	data["Recurrings"] = recurringData
	data["MonthlyIncome"] = monthlyIncome
	data["MonthlyExpenses"] = monthlyExpenses
	data["MonthlyNet"] = monthlyIncome - monthlyExpenses
	data["MonthlyYield"] = monthlyYield
	data["AnnualYield"] = annualYield

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hookRender(w, "accounts.html", data); err != nil {
		serverError(w, "render accounts", err)
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
	dbUser, _ := hookGetUserByID(user.ID)
	mfaEnabled := false
	if dbUser != nil {
		mfaEnabled = dbUser.MFAEnabled
	}

	isAdmin := user.Role == "ADMIN"

	data := baseData(r, user)
	data["Title"] = "Paramètres"
	data["User"] = map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role}
	data["MFAEnabled"] = mfaEnabled
	data["PasskeysEnabled"] = os.Getenv("HOST") != ""
	data["Passkeys"] = []interface{}{}
	data["IsAdmin"] = isAdmin
	data["IsRegisterOpen"] = os.Getenv("ALLOW_REGISTER") == "true"
	data["Users"] = []interface{}{}

	passkeys, _ := hookGetAuthenticatorsByUserID(user.ID)
	data["Passkeys"] = passkeys

	if isAdmin {
		users, err := hookGetAllUsers()
		if err != nil {
			serverError(w, "get all users", err)
			return
		}
		var usersWithEmail []map[string]interface{}
		for _, u := range users {
			uEmail, err := hookDecryptStr(u.EmailEncrypted)
			if err != nil {
				slog.Warn("admin: decrypt email", "userID", u.ID, "err", err)
				continue
			}
			usersWithEmail = append(usersWithEmail, map[string]interface{}{
				"ID":    u.ID,
				"Email": uEmail,
				"Role":  u.Role,
			})
		}
		data["Users"] = usersWithEmail
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hookRender(w, "settings.html", data); err != nil {
		serverError(w, "render settings", err)
	}
}

// VerifyEmailPage verifie l'email avec le token
func VerifyEmailPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	data := baseData(r, nil)
	t := data["T"].(map[string]string)
	data["Title"] = t["page.title_verify_email"]
	data["Success"] = false
	data["Error"] = ""

	if token == "" {
		data["Error"] = t["verify.token_missing"]
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "verify-email.html", data) //nolint:errcheck
		return
	}

	// Hasher le token pour la recherche
	hashedToken := hookHashToken(token)

	// Verifier le token
	err := hookVerifyEmailByToken(hashedToken)
	if err != nil {
		if err == db.ErrTokenInvalid {
			data["Error"] = t["verify.token_invalid"]
		} else {
			data["Error"] = t["verify.server_error"]
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		hookRender(w, "verify-email.html", data) //nolint:errcheck
		return
	}

	data["Success"] = true

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRender(w, "verify-email.html", data) //nolint:errcheck
}

// PrivacyPage affiche la politique de confidentialité
func PrivacyPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	data := baseData(r, user)
	t := data["T"].(map[string]string)
	data["Title"] = t["page.title_privacy"]
	if user != nil {
		data["User"] = map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hookRender(w, "privacy.html", data); err != nil {
		serverError(w, "render privacy", err)
	}
}

// LegalPage affiche les mentions légales
func LegalPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	data := baseData(r, user)
	t := data["T"].(map[string]string)
	data["Title"] = t["page.title_legal"]
	if user != nil {
		data["User"] = map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hookRender(w, "legal.html", data); err != nil {
		serverError(w, "render legal", err)
	}
}
