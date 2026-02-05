package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
	"pilot-finance/internal/templates"
)

// CreateRecurring cree ou met a jour une operation recurrente
func CreateRecurring(w http.ResponseWriter, r *http.Request) {
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
	description := r.FormValue("description")
	amountStr := r.FormValue("amount")
	dayStr := r.FormValue("dayOfMonth")
	opType := r.FormValue("type")
	accountIDStr := r.FormValue("accountId")
	toAccountIDStr := r.FormValue("toAccountId")

	if description == "" || amountStr == "" || accountIDStr == "" {
		http.Error(w, "Champs requis manquants", http.StatusBadRequest)
		return
	}

	// Chiffrer la description
	encryptedDesc, err := crypto.Encrypt(description)
	if err != nil {
		http.Error(w, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		http.Error(w, "Montant invalide", http.StatusBadRequest)
		return
	}

	day, _ := strconv.Atoi(dayStr)
	if day < 1 || day > 31 {
		day = 1
	}

	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Compte invalide", http.StatusBadRequest)
		return
	}

	var toAccountID *int64
	if toAccountIDStr != "" {
		id, err := strconv.ParseInt(toAccountIDStr, 10, 64)
		if err == nil {
			toAccountID = &id
		}
	}

	// Ajuster le signe selon le type
	if opType == "expense" && amount > 0 {
		amount = -amount
	} else if opType == "income" && amount < 0 {
		amount = -amount
	}

	// Si un ID est fourni, c'est une mise a jour
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "ID invalide", http.StatusBadRequest)
			return
		}
		err = db.UpdateRecurring(id, user.ID, encryptedDesc, amount, day, toAccountID)
		if err != nil {
			http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
			return
		}
	} else {
		// Creation
		err = db.CreateRecurring(user.ID, accountID, toAccountID, encryptedDesc, amount, day)
		if err != nil {
			http.Error(w, "Erreur creation", http.StatusInternalServerError)
			return
		}
	}

	// Retourner la liste mise a jour en HTML
	renderRecurringTable(w, user.ID)
}

// UpdateRecurring met a jour une operation recurrente
func UpdateRecurring(w http.ResponseWriter, r *http.Request) {
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

	description := r.FormValue("description")
	amountStr := r.FormValue("amount")
	dayStr := r.FormValue("dayOfMonth")
	opType := r.FormValue("type")
	toAccountIDStr := r.FormValue("toAccountId")

	// Chiffrer la description
	encryptedDesc, encErr := crypto.Encrypt(description)
	if encErr != nil {
		http.Error(w, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	amount, _ := strconv.ParseFloat(amountStr, 64)
	day, _ := strconv.Atoi(dayStr)

	var toAccountID *int64
	if toAccountIDStr != "" {
		tid, err := strconv.ParseInt(toAccountIDStr, 10, 64)
		if err == nil {
			toAccountID = &tid
		}
	}

	if opType == "expense" {
		amount = -amount
	}

	err = db.UpdateRecurring(id, user.ID, encryptedDesc, amount, day, toAccountID)
	if err != nil {
		http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteRecurring supprime une operation recurrente
func DeleteRecurring(w http.ResponseWriter, r *http.Request) {
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

	err = db.DeleteRecurring(id, user.ID)
	if err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	// Retourner la liste mise a jour en HTML
	renderRecurringTable(w, user.ID)
}

// renderRecurringTable rend le tableau des operations recurrentes en HTML
func renderRecurringTable(w http.ResponseWriter, userID int64) {
	recurrings, _ := db.GetRecurringByUserID(userID)
	accounts, _ := db.GetAccountsByUserID(userID)

	// Creer un map des noms de comptes
	accountMap := make(map[int64]string)
	for _, acc := range accounts {
		name := acc.Name
		if decrypted, err := crypto.Decrypt(acc.Name); err == nil {
			name = decrypted
		}
		accountMap[acc.ID] = name
	}

	// Calculer les yield payouts
	yieldPayouts := projection.CalculateYieldPayouts(accounts, accountMap)

	// Preparer les donnees avec noms de comptes
	recurringData := make([]map[string]interface{}, 0, len(recurrings)+len(yieldPayouts))

	// Ajouter les yield payouts en premier
	for _, payout := range yieldPayouts {
		recurringData = append(recurringData, map[string]interface{}{
			"ID":            int64(0),
			"Description":   "Interets " + payout.SourceAccountName,
			"Amount":        payout.Amount,
			"DayOfMonth":    1,
			"AccountID":     payout.SourceAccountID,
			"AccountName":   payout.SourceAccountName,
			"ToAccountID":   payout.TargetAccountID,
			"ToAccountName": payout.TargetAccountName,
			"IsActive":      true,
			"IsYieldPayout": true,
			"YieldRate":     payout.Rate,
		})
	}

	for _, rec := range recurrings {
		description := rec.Description
		if decrypted, err := crypto.Decrypt(rec.Description); err == nil {
			description = decrypted
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
			"IsYieldPayout": false,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.RenderPartial(w, "accounts.html", "recurring-table", map[string]interface{}{
		"Recurrings": recurringData,
	})
}
