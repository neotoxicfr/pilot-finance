package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
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

	// Si un ID est fourni, c'est une mise a jour
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "ID invalide", http.StatusBadRequest)
			return
		}
		err = db.UpdateAccountWithYield(id, user.ID, encryptedName, balance, color, isYieldActive, yieldType, yieldMin, yieldMax, reinvestmentRate)
		if err != nil {
			http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
			return
		}
	} else {
		// Creation d'un nouveau compte
		accounts, _ := db.GetAccountsByUserID(user.ID)
		position := len(accounts)

		err := db.CreateAccountWithYield(user.ID, encryptedName, balance, color, position, isYieldActive, yieldType, yieldMin, yieldMax, reinvestmentRate)
		if err != nil {
			http.Error(w, "Erreur creation", http.StatusInternalServerError)
			return
		}
	}

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user.ID)
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
	renderAccountsList(w, user.ID)
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
		renderAccountsList(w, user.ID)
		return
	}

	// Echanger les positions
	err = db.SwapAccountPositions(accounts[currentIdx].ID, accounts[targetIdx].ID, user.ID)
	if err != nil {
		http.Error(w, "Erreur deplacement", http.StatusInternalServerError)
		return
	}

	// Retourner la liste mise a jour
	renderAccountsList(w, user.ID)
}

// renderAccountsList rend la liste des comptes en HTML
func renderAccountsList(w http.ResponseWriter, userID int64) {
	accounts, _ := db.GetAccountsByUserID(userID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for _, acc := range accounts {
		// Dechiffrer le nom
		if decrypted, err := crypto.Decrypt(acc.Name); err == nil {
			acc.Name = decrypted
		}
		templates.RenderPartial(w, "accounts.html", "account-row", acc)
	}
}
