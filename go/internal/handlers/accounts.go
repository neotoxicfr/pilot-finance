package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CreateAccount cree ou met a jour un compte
func CreateAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
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
		clientError(w, ErrValidation, "Nom requis", http.StatusBadRequest)
		return
	}

	// Chiffrer le nom du compte
	encryptedName, err := hookEncryptStr(name)
	if err != nil {
		slog.Error("encrypt name", "err", err)
		clientError(w, ErrEncryption, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	balance := 0.0
	if balanceStr != "" {
		var err error
		balance, err = strconv.ParseFloat(balanceStr, 64)
		if err != nil {
			clientError(w, ErrValidation, "Solde invalide", http.StatusBadRequest)
			return
		}
	}

	if color == "" {
		color = "#3b82f6"
	} else if !hexColorRegex.MatchString(color) {
		clientError(w, ErrValidation, "Format couleur invalide (ex: #3b82f6)", http.StatusBadRequest)
		return
	}

	// Parser les valeurs de rendement
	if yieldType == "" {
		yieldType = "FIXED"
	}
	yieldMin := 0.0
	yieldMax := 0.0
	reinvestmentRate := 100
	var parseErr error
	if yieldMinStr != "" {
		if yieldMin, parseErr = strconv.ParseFloat(yieldMinStr, 64); parseErr != nil {
			clientError(w, ErrValidation, "Taux minimum invalide", http.StatusBadRequest)
			return
		}
	}
	if yieldMaxStr != "" {
		if yieldMax, parseErr = strconv.ParseFloat(yieldMaxStr, 64); parseErr != nil {
			clientError(w, ErrValidation, "Taux maximum invalide", http.StatusBadRequest)
			return
		}
	}
	if reinvestmentRateStr != "" {
		if reinvestmentRate, parseErr = strconv.Atoi(reinvestmentRateStr); parseErr != nil {
			clientError(w, ErrValidation, "Taux de réinvestissement invalide", http.StatusBadRequest)
			return
		}
	}

	// Validation des taux de rendement
	if isYieldActive {
		if yieldMin < 0 || yieldMax < 0 {
			clientError(w, ErrValidation, "Les taux de rendement ne peuvent pas être négatifs", http.StatusBadRequest)
			return
		}
		if yieldType == "RANGE" && yieldMin > yieldMax {
			clientError(w, ErrValidation, "Le taux minimum doit être inférieur ou égal au taux maximum", http.StatusBadRequest)
			return
		}
	}

	// Validation du taux de réinvestissement
	if reinvestmentRate < 0 || reinvestmentRate > 100 {
		clientError(w, ErrValidation, "Le taux de réinvestissement doit être compris entre 0 et 100", http.StatusBadRequest)
		return
	}

	// Parser le compte cible pour les interets non reinvestis
	var targetAccountID *int64
	if targetAccountIDStr != "" && targetAccountIDStr != "0" {
		targetID, err := strconv.ParseInt(targetAccountIDStr, 10, 64)
		if err != nil {
			clientError(w, ErrValidation, "Compte cible invalide", http.StatusBadRequest)
			return
		}
		// Vérifier que le compte cible appartient à l'utilisateur
		ok, err := hookAccountBelongsToUser(targetID, user.ID)
		if err != nil || !ok {
			clientError(w, ErrValidation, "Compte cible invalide", http.StatusBadRequest)
			return
		}
		targetAccountID = &targetID
	}

	payoutFrequency := r.FormValue("payoutFrequency")
	if payoutFrequency != "YEARLY" {
		payoutFrequency = "MONTHLY"
	}

	// Si un ID est fourni, c'est une mise a jour
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
			return
		}
		err = hookUpdateAccountWithYield(id, user.ID, encryptedName, balance, color, isYieldActive, yieldType, yieldMin, yieldMax, reinvestmentRate, targetAccountID, payoutFrequency)
		if err != nil {
			serverError(w, "update account", err)
			return
		}
		hookLogAudit(user.ID, db.AuditAccountUpdate, getClientIP(r), r.UserAgent())
	} else {
		// Creation d'un nouveau compte
		existingAccounts, posErr := hookGetAccountsByUserID(user.ID)
		if posErr != nil {
			slog.Warn("CreateAccount: position lookup", "err", posErr)
		}
		position := len(existingAccounts)

		err := hookCreateAccountWithYield(user.ID, encryptedName, balance, color, position, isYieldActive, yieldType, yieldMin, yieldMax, reinvestmentRate, targetAccountID, payoutFrequency)
		if err != nil {
			serverError(w, "create account", err)
			return
		}
		hookLogAudit(user.ID, db.AuditAccountCreate, getClientIP(r), r.UserAgent())
	}

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user)
}

// DeleteAccount supprime un compte
func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
		return
	}

	err = hookDeleteAccount(id, user.ID)
	if err != nil {
		serverError(w, "delete account", err)
		return
	}

	hookLogAudit(user.ID, db.AuditAccountDelete, getClientIP(r), r.UserAgent())

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user)
}

// UpdateBalance met a jour le solde d'un compte
func UpdateBalance(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	balanceStr := r.FormValue("balance")
	balance, err := strconv.ParseFloat(balanceStr, 64)
	if err != nil {
		clientError(w, ErrValidation, "Solde invalide", http.StatusBadRequest)
		return
	}

	err = hookUpdateAccountBalance(id, user.ID, balance)
	if err != nil {
		serverError(w, "update balance", err)
		return
	}

	renderAccountsList(w, user)
}

// MoveAccount deplace un compte vers le haut ou le bas
func MoveAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
		return
	}

	direction := r.URL.Query().Get("direction")
	if direction != "up" && direction != "down" {
		clientError(w, ErrValidation, "Direction invalide", http.StatusBadRequest)
		return
	}

	// Recuperer tous les comptes tries par position
	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}

	// Trouver l'index du compte a deplacer
	currentIdx := -1
	for i, acc := range accounts {
		if acc.ID == id {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		clientError(w, ErrNotFound, "Compte non trouvé", http.StatusNotFound)
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
	err = hookSwapAccountPositions(accounts[currentIdx].ID, accounts[targetIdx].ID, user.ID)
	if err != nil {
		serverError(w, "swap positions", err)
		return
	}

	// Retourner la liste mise a jour
	renderAccountsList(w, user)
}

// ReorderAccounts reordonne les comptes selon un tableau d'IDs
func ReorderAccounts(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	if err := hookReorderAccounts(user.ID, body.IDs); err != nil {
		serverError(w, "reorder accounts", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// renderAccountsList rend la liste des comptes en HTML avec OOB updates
func renderAccountsList(w http.ResponseWriter, user *middleware.User) {
	lang, currency := userLocale(user)

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		slog.Error("renderAccountsList: accounts", "err", err)
	}
	recurrings, err := hookGetRecurringByUserID(user.ID)
	if err != nil {
		slog.Error("renderAccountsList: recurring", "err", err)
	}

	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	// Calculer les yield payouts
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Rendre la liste des comptes
	if len(accounts) == 0 {
		hookRenderPartial(w, "accounts.html", "accounts-empty", map[string]interface{}{ //nolint:errcheck
			"T": i18n.Map(lang),
		})
	}
	lastIdx := len(accounts) - 1
	for i, acc := range accounts {
		hookRenderPartial(w, "accounts.html", "account-row", map[string]interface{}{ //nolint:errcheck
			"Account":  acc,
			"Currency": currency,
			"T":        i18n.Map(lang),
			"IsFirst":  i == 0,
			"IsLast":   i == lastIdx,
		})
	}

	// OOB: Rendre le summary card
	w.Write([]byte(`<div id="summary-card" hx-swap-oob="innerHTML">`))
	hookRenderPartial(w, "accounts.html", "summary-card", map[string]interface{}{ //nolint:errcheck
		"T":               i18n.Map(lang),
		"MonthlyIncome":   monthlyIncome,
		"MonthlyExpenses": monthlyExpenses,
		"MonthlyNet":      monthlyIncome - monthlyExpenses,
		"MonthlyYield":    monthlyYield,
		"AnnualYield":     annualYield,
		"Currency":        currency,
	})
	w.Write([]byte(`</div>`))

	// OOB: Rendre le tableau des recurrents
	w.Write([]byte(`<div id="recurring-list" hx-swap-oob="innerHTML">`))
	hookRenderPartial(w, "accounts.html", "recurring-table", map[string]interface{}{ //nolint:errcheck
		"Recurrings": recurringData,
		"Currency":   currency,
		"T":          i18n.Map(lang),
	})
	w.Write([]byte(`</div>`))
}
