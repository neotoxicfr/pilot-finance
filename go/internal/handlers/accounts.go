package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// CreateAccount cree un nouveau compte
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

	name := r.FormValue("name")
	balanceStr := r.FormValue("balance")
	color := r.FormValue("color")

	if name == "" {
		http.Error(w, "Nom requis", http.StatusBadRequest)
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

	// Recuperer la position max
	accounts, _ := db.GetAccountsByUserID(user.ID)
	position := len(accounts)

	err := db.CreateAccount(user.ID, name, balance, color, position)
	if err != nil {
		http.Error(w, "Erreur creation", http.StatusInternalServerError)
		return
	}

	// Retourner la liste mise a jour
	accounts, _ = db.GetAccountsByUserID(user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
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

	balance := 0.0
	if balanceStr != "" {
		balance, _ = strconv.ParseFloat(balanceStr, 64)
	}

	err = db.UpdateAccount(id, user.ID, name, balance, color)
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

	// Retourner la liste mise a jour
	accounts, _ := db.GetAccountsByUserID(user.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
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
