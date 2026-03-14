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

	// Recuperer les comptes
	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}

	decryptAccountNames(accounts)

	// Recuperer les operations recurrentes pour la projection et le resume mensuel
	recurrings, recErr := hookGetRecurringByUserID(user.ID)
	if recErr != nil {
		slog.Warn("DashboardAPI: recurring", "err", recErr, "userID", user.ID)
	}

	// Calculer les projections
	data := projection.Calculate(accounts, recurrings, years, user.Language)
	summary := projection.CalculateMonthlySummary(recurrings, accounts)

	// Preparer les donnees pour les graphiques
	pieData := make([]map[string]interface{}, 0)
	for _, acc := range accounts {
		if acc.Balance > 0 {
			pieData = append(pieData, map[string]interface{}{
				"name":  acc.Name,
				"value": float64(acc.Balance) / 100.0,
				"color": acc.Color,
			})
		}
	}

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

	var projectionTotal float64
	if len(data.Projection) > 0 {
		projectionTotal = data.Projection[len(data.Projection)-1].TotalAvg
	}

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
