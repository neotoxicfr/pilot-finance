package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
	"pilot-finance/internal/templates"
)

// CreateAccount cree ou met a jour un compte
func CreateAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Donnees invalides", http.StatusBadRequest)
		return
	}

	idStr := r.FormValue("id")
	name := r.FormValue("name")
	balanceStr := r.FormValue("balance")
	color := r.FormValue("color")

	// Champs rendement
	isYieldActive := r.FormValue("isYieldActive") == "on" || r.FormValue("isYieldActive") == "true"
	yieldType := r.FormValue("yieldType")
	yieldMinStr := r.FormValue("yieldMin")
	yieldMaxStr := r.FormValue("yieldMax")
	reinvestmentRateStr := r.FormValue("reinvestmentRate")
	targetAccountIDStr := r.FormValue("targetAccountId")

	if name == "" {
		http.Error(w, "Nom requis", http.StatusBadRequest)
		return
	}

	// Chiffrer le nom du compte
	encryptedName, err := crypto.Encrypt(name)
	if err != nil {
		http.Error(w, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	balance := 0.0
	if balanceStr != "" {
		var err error
		balance, err = strconv.ParseFloat(balanceStr, 64)
		if err != nil {
			http.Error(w, "Solde invalide", http.StatusBadRequest)
			return
		}
	}

	if color == "" {
		color = "#3b82f6"
	}

	// Parser les valeurs de rendement
	if yieldType == "" {
		yieldType = "FIXED"
	}
	yieldMin := 0.0
	yieldMax := 0.0
	reinvestmentRate := 100
	if yieldMinStr != "" {
		yieldMin, _ = strconv.ParseFloat(yieldMinStr, 64)
	}
	if yieldMaxStr != "" {
		yieldMax, _ = strconv.ParseFloat(yieldMaxStr, 64)
	}
	if reinvestmentRateStr != "" {
		reinvestmentRate, _ = strconv.Atoi(reinvestmentRateStr)
	}

	// Validation des taux pour le type RANGE
	if isYieldActive && yieldType == "RANGE" {
		if yieldMin < 0 || yieldMax < 0 {
			http.Error(w, "Les taux de rendement ne peuvent pas être négatifs", http.StatusBadRequest)
			return
		}
		if yieldMin > yieldMax {
			http.Error(w, "Le taux minimum doit être inférieur ou égal au taux maximum", http.StatusBadRequest)
			return
		}
	}

	// Parser le compte cible pour les interets non reinvestis
	var targetAccountID *int64
	if targetAccountIDStr != "" && targetAccountIDStr != "0" {
		targetID, err := strconv.ParseInt(targetAccountIDStr, 10, 64)
		if err == nil {
			targetAccountID = &targetID
		}
	}

	// Si un ID est fourni, c'est une mise a jour
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "ID invalide", http.StatusBadRequest)
			return
		}
		err = db.UpdateAccountWithYield(id, user.ID, encryptedName, balance, color, isYieldActive, yieldType, yieldMin, yieldMax, reinvestmentRate, targetAccountID)
		if err != nil {
			http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
			return
		}
	} else {
		// Creation d'un nouveau compte
		accounts, _ := db.GetAccountsByUserID(user.ID)
		position := len(accounts)

		err := db.CreateAccountWithYield(user.ID, encryptedName, balance, color, position, isYieldActive, yieldType, yieldMin, yieldMax, reinvestmentRate, targetAccountID)
		if err != nil {
			http.Error(w, "Erreur creation", http.StatusInternalServerError)
			return
		}
	}

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user)
}

// UpdateAccount met a jour un compte
func UpdateAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Donnees invalides", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	balanceStr := r.FormValue("balance")
	color := r.FormValue("color")

	// Chiffrer le nom du compte
	encryptedName, encErr := crypto.Encrypt(name)
	if encErr != nil {
		http.Error(w, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	balance := 0.0
	if balanceStr != "" {
		balance, _ = strconv.ParseFloat(balanceStr, 64)
	}

	err = db.UpdateAccount(id, user.ID, encryptedName, balance, color)
	if err != nil {
		http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteAccount supprime un compte
func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	err = db.DeleteAccount(id, user.ID)
	if err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user)
}

// UpdateBalance met a jour le solde d'un compte
func UpdateBalance(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Donnees invalides", http.StatusBadRequest)
		return
	}

	balanceStr := r.FormValue("balance")
	balance, err := strconv.ParseFloat(balanceStr, 64)
	if err != nil {
		http.Error(w, "Solde invalide", http.StatusBadRequest)
		return
	}

	err = db.UpdateAccountBalance(id, user.ID, balance)
	if err != nil {
		http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// MoveAccount deplace un compte vers le haut ou le bas
func MoveAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	direction := r.URL.Query().Get("direction")
	if direction != "up" && direction != "down" {
		http.Error(w, "Direction invalide", http.StatusBadRequest)
		return
	}

	// Recuperer tous les comptes tries par position
	accounts, err := db.GetAccountsByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Trouver l'index du compte a deplacer
	var currentIdx int = -1
	for i, acc := range accounts {
		if acc.ID == id {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		http.Error(w, "Compte non trouve", http.StatusNotFound)
		return
	}

	// Calculer l'index cible
	var targetIdx int
	if direction == "up" {
		targetIdx = currentIdx - 1
	} else {
		targetIdx = currentIdx + 1
	}

	// Verifier les limites
	if targetIdx < 0 || targetIdx >= len(accounts) {
		// Pas de changement, retourner la liste actuelle
		renderAccountsList(w, user)
		return
	}

	// Echanger les positions
	err = db.SwapAccountPositions(accounts[currentIdx].ID, accounts[targetIdx].ID, user.ID)
	if err != nil {
		http.Error(w, "Erreur deplacement", http.StatusInternalServerError)
		return
	}

	// Retourner la liste mise a jour
	renderAccountsList(w, user)
}

// renderAccountsList rend la liste des comptes en HTML avec OOB updates
func renderAccountsList(w http.ResponseWriter, user *middleware.User) {
	lang, currency := userLocale(user)

	accounts, _ := db.GetAccountsByUserID(user.ID)
	recurrings, _ := db.GetRecurringByUserID(user.ID)

	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	// Calculer les yield payouts
	yieldPayouts := projection.CalculateYieldPayouts(accounts, accountMap)
	interestPrefix := i18n.T(lang, "recurring.interest_prefix")

	// Calculer les totaux mensuels
	var monthlyIncome, monthlyExpenses float64
	for _, payout := range yieldPayouts {
		monthlyIncome += payout.Amount
	}
	for _, rec := range recurrings {
		if rec.Amount > 0 {
			monthlyIncome += rec.Amount
		} else {
			monthlyExpenses += -rec.Amount
		}
	}

	recurringData := buildRecurringData(yieldPayouts, recurrings, accountMap, interestPrefix)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Rendre la liste des comptes
	for _, acc := range accounts {
		templates.RenderPartial(w, "accounts.html", "account-row", acc)
	}

	// OOB: Rendre le summary card
	w.Write([]byte(`<div id="summary-card" hx-swap-oob="innerHTML">`))
	templates.RenderPartial(w, "accounts.html", "summary-card", map[string]interface{}{
		"MonthlyIncome":   monthlyIncome,
		"MonthlyExpenses": monthlyExpenses,
		"MonthlyNet":      monthlyIncome - monthlyExpenses,
		"Currency":        currency,
	})
	w.Write([]byte(`</div>`))

	// OOB: Rendre le tableau des recurrents
	w.Write([]byte(`<div id="recurring-list" hx-swap-oob="innerHTML">`))
	templates.RenderPartial(w, "accounts.html", "recurring-table", map[string]interface{}{
		"Recurrings": recurringData,
		"Currency":   currency,
	})
	w.Write([]byte(`</div>`))
}
