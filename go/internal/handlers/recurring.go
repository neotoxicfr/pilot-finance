package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// CreateRecurring cree une operation recurrente
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
	if opType == "expense" {
		amount = -amount
	}

	err = db.CreateRecurring(user.ID, accountID, toAccountID, description, amount, day)
	if err != nil {
		http.Error(w, "Erreur creation", http.StatusInternalServerError)
		return
	}

	// Retourner la liste mise a jour
	recurrings, _ := db.GetRecurringByUserID(user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recurrings)
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

	err = db.UpdateRecurring(id, user.ID, description, amount, day, toAccountID)
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

	// Retourner la liste mise a jour
	recurrings, _ := db.GetRecurringByUserID(user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recurrings)
}
