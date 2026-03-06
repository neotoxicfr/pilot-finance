package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

// CreateRecurring cree ou met a jour une operation recurrente
func CreateRecurring(w http.ResponseWriter, r *http.Request) {
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
	description := r.FormValue("description")
	amountStr := r.FormValue("amount")
	dayStr := r.FormValue("dayOfMonth")
	opType := r.FormValue("type")
	accountIDStr := r.FormValue("accountId")
	toAccountIDStr := r.FormValue("toAccountId")

	if description == "" || amountStr == "" || accountIDStr == "" {
		clientError(w, ErrValidation, "Champs requis manquants", http.StatusBadRequest)
		return
	}

	// Chiffrer la description
	encryptedDesc, err := hookEncryptStr(description)
	if err != nil {
		clientError(w, ErrEncryption, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	amount, err := parseCents(amountStr)
	if err != nil {
		clientError(w, ErrValidation, "Montant invalide", http.StatusBadRequest)
		return
	}

	day, _ := strconv.Atoi(dayStr)
	if day < 1 || day > 31 {
		day = 1
	}

	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		clientError(w, ErrValidation, "Compte invalide", http.StatusBadRequest)
		return
	}
	if ok, err := hookAccountBelongsToUser(accountID, user.ID); err != nil || !ok {
		clientError(w, ErrValidation, "Compte invalide", http.StatusBadRequest)
		return
	}

	var toAccountID *int64
	if toAccountIDStr != "" {
		id, err := strconv.ParseInt(toAccountIDStr, 10, 64)
		if err != nil {
			clientError(w, ErrValidation, "Compte destinataire invalide", http.StatusBadRequest)
			return
		}
		if ok, err := hookAccountBelongsToUser(id, user.ID); err != nil || !ok {
			clientError(w, ErrValidation, "Compte destinataire invalide", http.StatusBadRequest)
			return
		}
		toAccountID = &id
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
			clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
			return
		}
		err = hookUpdateRecurring(id, user.ID, encryptedDesc, amount, day, toAccountID)
		if err != nil {
			serverError(w, "update recurring", err)
			return
		}
	} else {
		// Creation
		err = hookCreateRecurring(user.ID, accountID, toAccountID, encryptedDesc, amount, day)
		if err != nil {
			serverError(w, "create recurring", err)
			return
		}
	}

	// Retourner la liste mise a jour en HTML
	renderRecurringTable(w, user)
}

// UpdateRecurring met a jour une operation recurrente
func UpdateRecurring(w http.ResponseWriter, r *http.Request) {
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

	description := r.FormValue("description")
	amountStr := r.FormValue("amount")
	dayStr := r.FormValue("dayOfMonth")
	opType := r.FormValue("type")
	toAccountIDStr := r.FormValue("toAccountId")

	// Chiffrer la description
	encryptedDesc, encErr := hookEncryptStr(description)
	if encErr != nil {
		clientError(w, ErrEncryption, "Erreur chiffrement", http.StatusInternalServerError)
		return
	}

	amount, err := parseCents(amountStr)
	if err != nil {
		clientError(w, ErrValidation, "Montant invalide", http.StatusBadRequest)
		return
	}

	day, _ := strconv.Atoi(dayStr)
	if day < 1 || day > 31 {
		day = 1
	}

	var toAccountID *int64
	if toAccountIDStr != "" {
		tid, err := strconv.ParseInt(toAccountIDStr, 10, 64)
		if err == nil {
			toAccountID = &tid
		}
	}

	if opType == "expense" && amount > 0 {
		amount = -amount
	} else if opType == "income" && amount < 0 {
		amount = -amount
	}

	err = hookUpdateRecurring(id, user.ID, encryptedDesc, amount, day, toAccountID)
	if err != nil {
		serverError(w, "update recurring", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteRecurring supprime une operation recurrente
func DeleteRecurring(w http.ResponseWriter, r *http.Request) {
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

	err = hookDeleteRecurring(id, user.ID)
	if err != nil {
		serverError(w, "delete recurring", err)
		return
	}

	// Retourner la liste mise a jour en HTML
	renderRecurringTable(w, user)
}

// renderRecurringTable rend le tableau des operations recurrentes en HTML
func renderRecurringTable(w http.ResponseWriter, user *middleware.User) {
	lang, currency := userLocale(user)

	recurrings, err := hookGetRecurringByUserID(user.ID)
	if err != nil {
		slog.Error("renderRecurringTable: recurring", "err", err)
	}
	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		slog.Error("renderRecurringTable: accounts", "err", err)
	}

	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	// Calculer les yield payouts
	yieldPayouts := projection.CalculateYieldPayouts(accounts, accountMap)
	interestPrefix := i18n.T(lang, "recurring.interest_prefix")

	recurringData := buildRecurringData(yieldPayouts, recurrings, accountMap, interestPrefix)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRenderPartial(w, "accounts.html", "recurring-table", map[string]interface{}{ //nolint:errcheck
		"Recurrings": recurringData,
		"Currency":   currency,
		"T":          i18n.Map(lang),
	})
}
