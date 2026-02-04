package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
	"pilot-finance/internal/templates"
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

	// Dechiffrer les noms des comptes
	for i := range accounts {
		if decrypted, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		}
	}

	// Calculer les projections
	data := projection.Calculate(accounts, years)

	// Recuperer les operations recurrentes pour le resume mensuel
	recurrings, _ := db.GetRecurringByUserID(user.ID)
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

	response := map[string]interface{}{
		"accounts":        accounts,
		"totalBalance":    data.TotalBalance,
		"totalInterests":  data.TotalInterests,
		"projectionTotal": data.Projection[len(data.Projection)-1].TotalAvg,
		"projection":      projectionData,
		"pieData":         pieData,
		"years":           years,
		"monthly":         summary,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DashboardPartial retourne le HTML partiel du dashboard (pour HTMX)
func DashboardPartial(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	years := 5
	if y := r.URL.Query().Get("years"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil && parsed >= 1 && parsed <= 30 {
			years = parsed
		}
	}

	section := r.URL.Query().Get("section")

	accounts, err := db.GetAccountsByUserID(user.ID)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	for i := range accounts {
		if decrypted, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		}
	}

	data := projection.Calculate(accounts, years)

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

	templateData := map[string]interface{}{
		"Accounts":        accounts,
		"AccountColors":   accountColors,
		"TotalBalance":    data.TotalBalance,
		"TotalInterests":  data.TotalInterests,
		"ProjectionTotal": data.Projection[len(data.Projection)-1].TotalAvg,
		"ProjectionData":  projectionData,
		"PieData":         pieData,
		"Years":           years,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Rendre la section demandee
	switch section {
	case "kpi":
		templates.RenderPartial(w, "dashboard.html", "kpi-cards", templateData)
	case "projection":
		templates.RenderPartial(w, "dashboard.html", "projection-chart", templateData)
	default:
		templates.RenderPartial(w, "dashboard.html", "dashboard-cards", templateData)
	}
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

	// Dechiffrer les noms
	for i := range accounts {
		if decrypted, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		}
	}

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

	// Dechiffrer les descriptions et ajouter les noms de compte
	accounts, _ := db.GetAccountsByUserID(user.ID)
	accountMap := make(map[int64]string)
	for _, acc := range accounts {
		name := acc.Name
		if decrypted, err := crypto.Decrypt(acc.Name); err == nil {
			name = decrypted
		}
		accountMap[acc.ID] = name
	}

	result := make([]map[string]interface{}, len(recurrings))
	for i, rec := range recurrings {
		description := rec.Description
		if decrypted, err := crypto.Decrypt(rec.Description); err == nil {
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
