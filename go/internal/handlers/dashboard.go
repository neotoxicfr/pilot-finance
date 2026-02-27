package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

// DashboardAPI retourne les donnees du dashboard en JSON
func DashboardAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	// Nombre d'annees de projection (defaut 5)
	years := 5
	if y := r.URL.Query().Get("years"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil && parsed >= 1 && parsed <= 30 {
			years = parsed
		}
	}

	// Recuperer les comptes
	accounts, err := db.GetAccountsByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	decryptAccountNames(accounts)

	// Recuperer les operations recurrentes pour la projection et le resume mensuel
	recurrings, recErr := db.GetRecurringByUserID(user.ID)
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
				"value": acc.Balance,
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

	// Preparer la liste des comptes avec couleurs pour le graphique
	accountColors := make([]map[string]interface{}, 0)
	for _, acc := range accounts {
		accountColors = append(accountColors, map[string]interface{}{
			"name":  acc.Name,
			"color": acc.Color,
		})
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// AccountsAPI retourne les comptes en JSON
func AccountsAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	accounts, err := db.GetAccountsByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	decryptAccountNames(accounts)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

// RecurringAPI retourne les operations recurrentes en JSON
func RecurringAPI(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	recurrings, err := db.GetRecurringByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Dechiffrer les noms de comptes
	accounts, _ := db.GetAccountsByUserID(user.ID)
	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	result := make([]map[string]interface{}, len(recurrings))
	for i, rec := range recurrings {
		description := rec.Description
		if decrypted, err2 := crypto.Decrypt(rec.Description); err2 == nil {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
