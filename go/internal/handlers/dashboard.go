package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

// DashboardAPI retourne les donnees du dashboard en JSON
func DashboardAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	// Nombre d'annees de projection (defaut 5)
	years := 5
	if y := r.URL.Query().Get("years"); y != "" {
		parsed, err := strconv.Atoi(y)
		if err != nil {
			clientError(w, ErrValidation, "years must be an integer", http.StatusBadRequest)
			return
		}
		if parsed < 1 || parsed > 30 {
			clientError(w, ErrValidation, "years must be between 1 and 30", http.StatusBadRequest)
			return
		}
		years = parsed
	}

	// Récupère comptes et récurrents en parallèle (H2 perf)
	accounts, recurrings, accErr, recErr := loadAccountsAndRecurring(user.ID)
	if accErr != nil {
		serverError(w, "get accounts", accErr)
		return
	}
	if recErr != nil {
		slog.Warn("DashboardAPI: recurring", "err", recErr, "userID", user.ID)
	}

	decryptAccountNames(accounts)

	// Calculer les projections
	data := projection.Calculate(accounts, recurrings, years, user.Language)
	summary := projection.CalculateMonthlySummary(recurrings, accounts)

	// Preparer les donnees pour les graphiques
	pieData := buildPieData(accounts)

	// Preparer les donnees de projection pour le graphique
	projectionData := make([]map[string]interface{}, len(data.Projection))
	for i, p := range data.Projection {
		projectionData[i] = map[string]interface{}{
			"year":     p.Year,
			"name":     p.Name,
			"totalAvg": p.TotalAvg,
			"totalMin": p.TotalMin,
			"totalMax": p.TotalMax,
			"accounts": p.Accounts,
		}
	}

	projectionTotal := lastProjectionTotal(data.Projection)

	response := map[string]interface{}{
		"accounts":        accounts,
		"totalBalance":    data.TotalBalance,
		"totalInterests":  data.TotalInterests,
		"projectionTotal": projectionTotal,
		"projection":      projectionData,
		"pieData":         pieData,
		"years":           years,
		"monthly":         summary,
	}

	jsonSuccess(w, response)
}

// AccountsAPI retourne les comptes en JSON
func AccountsAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}

	decryptAccountNames(accounts)

	jsonSuccess(w, accounts)
}

// RecurringAPI retourne les operations recurrentes en JSON
func RecurringAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	recurrings, err := hookGetRecurringByUserID(user.ID)
	if err != nil {
		serverError(w, "get recurrings", err)
		return
	}

	// Early return : si aucun récurrent, pas besoin de fetch+décrypter les
	// comptes (H3 perf : économise une requête DB + N déchiffrements AES).
	if len(recurrings) == 0 {
		jsonSuccess(w, []any{})
		return
	}

	// Dechiffrer les noms de comptes
	accounts, _ := hookGetAccountsByUserID(user.ID)
	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	result := make([]map[string]interface{}, len(recurrings))
	for i, rec := range recurrings {
		description := rec.Description
		if decrypted, err2 := hookDecryptStr(rec.Description); err2 == nil {
			description = decrypted
		}

		result[i] = map[string]interface{}{
			"id":            rec.ID,
			"description":   description,
			"amount":        rec.Amount,
			"dayOfMonth":    rec.DayOfMonth,
			"accountId":     rec.AccountID,
			"accountName":   accountMap[rec.AccountID],
			"toAccountId":   rec.ToAccountID,
			"toAccountName": "",
			"isActive":      rec.IsActive,
		}

		if rec.ToAccountID != nil {
			result[i]["toAccountName"] = accountMap[*rec.ToAccountID]
		}
	}

	jsonSuccess(w, result)
}
