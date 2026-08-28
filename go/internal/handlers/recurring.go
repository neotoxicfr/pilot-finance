package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
)

// recurringForm regroupe les champs validés d'une opération récurrente,
// communs à la création et à la mise à jour.
type recurringForm struct {
	encryptedDesc string
	amount        int64 // déjà ajusté selon le signe du type
	day           int
	toAccountID   *int64
}

// parseRecurringForm lit et valide les champs partagés par CreateRecurring et
// UpdateRecurring : description (non vide, ≤500 runes, chiffrée), montant (parse
// + ajustement de signe selon le type), jour du mois (clampé 1-31) et compte
// destinataire (ownership vérifié pour les virements). En cas d'erreur cliente,
// la réponse est déjà écrite sur w et ok vaut false.
func parseRecurringForm(w http.ResponseWriter, r *http.Request, user *middleware.User) (recurringForm, bool) {
	var f recurringForm

	description := r.FormValue("description")
	amountStr := r.FormValue("amount")
	dayStr := r.FormValue("dayOfMonth")
	opType := r.FormValue("type")
	toAccountIDStr := r.FormValue("toAccountId")

	if description == "" || amountStr == "" {
		clientErrorT(w, r, ErrValidation, "error.required_fields_missing", http.StatusBadRequest)
		return f, false
	}
	if len([]rune(description)) > 500 {
		clientErrorT(w, r, ErrValidation, "error.description_too_long", http.StatusBadRequest)
		return f, false
	}

	// Chiffrer la description
	encryptedDesc, err := hookEncryptStr(description)
	if err != nil {
		clientErrorT(w, r, ErrEncryption, "error.encryption", http.StatusInternalServerError)
		return f, false
	}

	amount, err := parseCents(amountStr)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.amount_invalid", http.StatusBadRequest)
		return f, false
	}

	day, _ := strconv.Atoi(dayStr)
	if day < 1 || day > 31 {
		day = 1
	}

	// Le compte destinataire n'est valide que pour les virements.
	// Pour income/expense, on ignore toute valeur résiduelle envoyée par le formulaire
	// (le select reste dans le DOM même quand x-show le cache).
	var toAccountID *int64
	if opType == "transfer" {
		// audit S-04 : un virement sans destinataire n'était pas refusé — il
		// était enregistré avec ToAccountID nil et un montant laissé positif
		// (l'ajustement de signe ne traite que expense/income), donc compté
		// comme un revenu récurrent fantôme dans le résumé et la projection.
		if toAccountIDStr == "" {
			clientErrorT(w, r, ErrValidation, "error.to_account_required", http.StatusBadRequest)
			return f, false
		}
		id, err := strconv.ParseInt(toAccountIDStr, 10, 64)
		if err != nil {
			clientErrorT(w, r, ErrValidation, "error.to_account_invalid", http.StatusBadRequest)
			return f, false
		}
		// Virement vers soi-même : mouvement net nul que la projection
		// compterait quand même. Refusé (audit S-04). Le compte source n'est
		// présent dans le formulaire que sur le chemin POST /recurring ; le
		// chemin PUT ne le transmet pas et ne le modifie pas.
		if srcStr := r.FormValue("accountId"); srcStr != "" {
			if srcID, srcErr := strconv.ParseInt(srcStr, 10, 64); srcErr == nil && srcID == id {
				clientErrorT(w, r, ErrValidation, "error.to_account_same", http.StatusBadRequest)
				return f, false
			}
		}
		if ok, err := hookAccountBelongsToUser(id, user.ID); err != nil || !ok {
			clientErrorT(w, r, ErrValidation, "error.to_account_invalid", http.StatusBadRequest)
			return f, false
		}
		toAccountID = &id
	}

	// Ajuster le signe selon le type
	if opType == "expense" && amount > 0 {
		amount = -amount
	} else if opType == "income" && amount < 0 {
		amount = -amount
	}

	f.encryptedDesc = encryptedDesc
	f.amount = amount
	f.day = day
	f.toAccountID = toAccountID
	return f, true
}

// CreateRecurring cree ou met a jour une operation recurrente
func CreateRecurring(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_data", http.StatusBadRequest)
		return
	}

	idStr := r.FormValue("id")
	accountIDStr := r.FormValue("accountId")

	if accountIDStr == "" {
		clientErrorT(w, r, ErrValidation, "error.required_fields_missing", http.StatusBadRequest)
		return
	}

	f, ok := parseRecurringForm(w, r, user)
	if !ok {
		return
	}

	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.account_invalid", http.StatusBadRequest)
		return
	}
	if ok, err := hookAccountBelongsToUser(accountID, user.ID); err != nil || !ok {
		clientErrorT(w, r, ErrValidation, "error.account_invalid", http.StatusBadRequest)
		return
	}

	// Si un ID est fourni, c'est une mise a jour
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			clientErrorT(w, r, ErrValidation, "error.invalid_id", http.StatusBadRequest)
			return
		}
		err = hookUpdateRecurring(id, user.ID, accountID, f.encryptedDesc, f.amount, f.day, f.toAccountID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				clientErrorT(w, r, ErrNotFound, "error.recurring_not_found", http.StatusNotFound)
				return
			}
			serverError(w, r, "update recurring", err)
			return
		}
	} else {
		// Creation
		err = hookCreateRecurring(user.ID, accountID, f.toAccountID, f.encryptedDesc, f.amount, f.day)
		if err != nil {
			serverError(w, r, "create recurring", err)
			return
		}
	}

	// Retourner la liste mise a jour en HTML
	renderRecurringTable(w, r, user)
}

// UpdateRecurring met a jour une operation recurrente
func UpdateRecurring(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_data", http.StatusBadRequest)
		return
	}

	f, ok := parseRecurringForm(w, r, user)
	if !ok {
		return
	}

	// accountID=0 : le chemin PUT ne modifie pas le compte source (le formulaire
	// d'édition de l'UI passe par POST /recurring). Voir db.UpdateRecurring.
	err = hookUpdateRecurring(id, user.ID, 0, f.encryptedDesc, f.amount, f.day, f.toAccountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			clientErrorT(w, r, ErrNotFound, "error.recurring_not_found", http.StatusNotFound)
			return
		}
		serverError(w, r, "update recurring", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteRecurring supprime une operation recurrente
func DeleteRecurring(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.invalid_id", http.StatusBadRequest)
		return
	}

	err = hookDeleteRecurring(id, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			clientErrorT(w, r, ErrNotFound, "error.recurring_not_found", http.StatusNotFound)
			return
		}
		serverError(w, r, "delete recurring", err)
		return
	}

	// Retourner la liste mise a jour en HTML
	renderRecurringTable(w, r, user)
}

// renderRecurringTable rend le tableau des operations recurrentes en HTML avec OOB summary
func renderRecurringTable(w http.ResponseWriter, r *http.Request, user *middleware.User) {
	lang, currency := userLocale(user)

	accounts, recurrings, accErr, recErr := loadAccountsAndRecurring(user.ID)
	if recErr != nil {
		serverError(w, r, "renderRecurringTable: recurring", recErr)
		return
	}
	if accErr != nil {
		serverError(w, r, "renderRecurringTable: accounts", accErr)
		return
	}

	computed := computeAccountsSummary(lang, accounts, recurrings)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hookRenderPartial(w, "accounts.html", "recurring-table", map[string]interface{}{ //nolint:errcheck
		"Recurrings": computed.recurringData,
		"Currency":   currency,
		"Locale":     localeTag(lang),
		"T":          i18n.Map(lang),
	})

	// OOB: Rendre le summary card
	renderSummaryCardOOB(w, lang, currency, computed.summary)
}
