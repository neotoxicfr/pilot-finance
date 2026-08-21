package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
)

// errInvalidAmount : montant/taux non fini (NaN, ±Inf) ou hors bornes.
var errInvalidAmount = errors.New("valeur numérique invalide")

// maxCents borne les montants pour empêcher tout débordement int64 lors des
// accumulations (soldes cumulés, projections sur 30 ans) : 10^15 centimes =
// 10 000 milliards €, très au-delà de tout usage réel tout en laissant une
// marge énorme avant 2^63.
const maxCents = 1e15

// maxRate borne les taux de rendement (pourcentages) pour éviter la propagation
// de valeurs absurdes dans le moteur de projection.
const maxRate = 1000.0

// parseCents parses a decimal string ("1234.56") into centimes (123456).
// Rejette NaN/±Inf et les valeurs dont l'arrondi dépasse ±maxCents (une
// conversion int64 hors plage donnerait un solde indéfini, cf. audit FIN-1).
func parseCents(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errInvalidAmount
	}
	cents := math.Round(f * 100)
	if math.Abs(cents) > maxCents {
		return 0, errInvalidAmount
	}
	return int64(cents), nil
}

// parseRate parse un taux de rendement en pourcentage. Rejette NaN/±Inf et les
// magnitudes > maxRate (le signe reste validé par l'appelant selon isYieldActive).
func parseRate(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > maxRate {
		return 0, errInvalidAmount
	}
	return f, nil
}

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// accountForm regroupe les champs validés d'un compte (nom chiffré, solde,
// couleur, paramètres de rendement et compte cible).
type accountForm struct {
	encryptedName    string
	balance          int64
	color            string
	isYieldActive    bool
	yieldType        string
	yieldMin         float64
	yieldMax         float64
	reinvestmentRate int
	targetAccountID  *int64
	payoutFrequency  string
}

// parseAccountForm lit et valide les champs du formulaire de compte : nom
// (non vide, ≤100 runes, chiffré), solde, couleur hex, mapping/validation des
// taux de rendement et de réinvestissement, et compte cible (ownership vérifié).
// En cas d'erreur cliente, la réponse est déjà écrite sur w et ok vaut false.
func parseAccountForm(w http.ResponseWriter, r *http.Request, user *middleware.User) (accountForm, bool) {
	var f accountForm

	name := r.FormValue("name")
	balanceStr := r.FormValue("balance")
	color := r.FormValue("color")

	// Champs rendement
	isYieldActive := r.FormValue("isYieldActive") == "on" || r.FormValue("isYieldActive") == "true"
	yieldType := r.FormValue("yieldType")
	yieldMinStr := r.FormValue("yieldMin")
	yieldMaxStr := r.FormValue("yieldMax")
	reinvestmentRateStr := r.FormValue("reinvestmentRate")
	targetAccountIDStr := r.FormValue("targetAccountId")

	if name == "" {
		clientError(w, ErrValidation, "Nom requis", http.StatusBadRequest)
		return f, false
	}

	if len([]rune(name)) > 100 {
		clientError(w, ErrValidation, "Nom trop long (100 caractères max)", http.StatusBadRequest)
		return f, false
	}

	// Chiffrer le nom du compte
	encryptedName, err := hookEncryptStr(name)
	if err != nil {
		slog.Error("encrypt name", "err", err)
		clientError(w, ErrEncryption, "Erreur chiffrement", http.StatusInternalServerError)
		return f, false
	}

	var balance int64
	if balanceStr != "" {
		var err error
		balance, err = parseCents(balanceStr)
		if err != nil {
			clientError(w, ErrValidation, "Solde invalide", http.StatusBadRequest)
			return f, false
		}
	}

	if color == "" {
		color = "#3b82f6"
	} else if !hexColorRegex.MatchString(color) {
		clientError(w, ErrValidation, "Format couleur invalide (ex: #3b82f6)", http.StatusBadRequest)
		return f, false
	}

	// Parser les valeurs de rendement
	if yieldType == "" {
		yieldType = "FIXED"
	}
	yieldMin := 0.0
	yieldMax := 0.0
	reinvestmentRate := 100
	var parseErr error
	if yieldMinStr != "" {
		if yieldMin, parseErr = parseRate(yieldMinStr); parseErr != nil {
			clientError(w, ErrValidation, "Taux minimum invalide", http.StatusBadRequest)
			return f, false
		}
	}
	if yieldMaxStr != "" {
		if yieldMax, parseErr = parseRate(yieldMaxStr); parseErr != nil {
			clientError(w, ErrValidation, "Taux maximum invalide", http.StatusBadRequest)
			return f, false
		}
	}
	if reinvestmentRateStr != "" {
		if reinvestmentRate, parseErr = strconv.Atoi(reinvestmentRateStr); parseErr != nil {
			clientError(w, ErrValidation, "Taux de réinvestissement invalide", http.StatusBadRequest)
			return f, false
		}
	}

	// Validation des taux de rendement
	if isYieldActive {
		if yieldMin < 0 || yieldMax < 0 {
			clientError(w, ErrValidation, "Les taux de rendement ne peuvent pas être négatifs", http.StatusBadRequest)
			return f, false
		}
		if yieldType == "RANGE" && yieldMin > yieldMax {
			clientError(w, ErrValidation, "Le taux minimum doit être inférieur ou égal au taux maximum", http.StatusBadRequest)
			return f, false
		}
	}

	// Validation du taux de réinvestissement
	if reinvestmentRate < 0 || reinvestmentRate > 100 {
		clientError(w, ErrValidation, "Le taux de réinvestissement doit être compris entre 0 et 100", http.StatusBadRequest)
		return f, false
	}

	// Parser le compte cible pour les interets non reinvestis
	var targetAccountID *int64
	if targetAccountIDStr != "" && targetAccountIDStr != "0" {
		targetID, err := strconv.ParseInt(targetAccountIDStr, 10, 64)
		if err != nil {
			clientError(w, ErrValidation, "Compte cible invalide", http.StatusBadRequest)
			return f, false
		}
		// Vérifier que le compte cible appartient à l'utilisateur
		ok, err := hookAccountBelongsToUser(targetID, user.ID)
		if err != nil || !ok {
			clientError(w, ErrValidation, "Compte cible invalide", http.StatusBadRequest)
			return f, false
		}
		targetAccountID = &targetID
	}

	payoutFrequency := r.FormValue("payoutFrequency")
	if payoutFrequency != "YEARLY" {
		payoutFrequency = "MONTHLY"
	}

	f = accountForm{
		encryptedName:    encryptedName,
		balance:          balance,
		color:            color,
		isYieldActive:    isYieldActive,
		yieldType:        yieldType,
		yieldMin:         yieldMin,
		yieldMax:         yieldMax,
		reinvestmentRate: reinvestmentRate,
		targetAccountID:  targetAccountID,
		payoutFrequency:  payoutFrequency,
	}
	return f, true
}

// CreateAccount cree ou met a jour un compte
func CreateAccount(w http.ResponseWriter, r *http.Request) {
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

	f, ok := parseAccountForm(w, r, user)
	if !ok {
		return
	}

	// Si un ID est fourni, c'est une mise a jour
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			clientError(w, ErrValidation, "ID invalide", http.StatusBadRequest)
			return
		}
		err = hookUpdateAccountWithYield(id, user.ID, f.encryptedName, f.balance, f.color, f.isYieldActive, f.yieldType, f.yieldMin, f.yieldMax, f.reinvestmentRate, f.targetAccountID, f.payoutFrequency)
		if err != nil {
			serverError(w, "update account", err)
			return
		}
		hookLogAudit(user.ID, db.AuditAccountUpdate, getClientIP(r), r.UserAgent())
	} else {
		// Creation d'un nouveau compte
		accountCount, posErr := hookCountAccountsByUserID(user.ID)
		if posErr != nil {
			slog.Warn("CreateAccount: position lookup", "err", posErr)
		}
		position := accountCount

		err := hookCreateAccountWithYield(user.ID, f.encryptedName, f.balance, f.color, position, f.isYieldActive, f.yieldType, f.yieldMin, f.yieldMax, f.reinvestmentRate, f.targetAccountID, f.payoutFrequency)
		if err != nil {
			serverError(w, "create account", err)
			return
		}
		hookLogAudit(user.ID, db.AuditAccountCreate, getClientIP(r), r.UserAgent())
	}

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user)
}

// DeleteAccount supprime un compte
func DeleteAccount(w http.ResponseWriter, r *http.Request) {
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

	err = hookDeleteAccount(id, user.ID)
	if err != nil {
		serverError(w, "delete account", err)
		return
	}

	hookLogAudit(user.ID, db.AuditAccountDelete, getClientIP(r), r.UserAgent())

	// Retourner la liste mise a jour en HTML
	renderAccountsList(w, user)
}

// UpdateBalance met a jour le solde d'un compte
func UpdateBalance(w http.ResponseWriter, r *http.Request) {
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

	// Un solde vide est rejeté (400) : sinon parseCents("") renverrait 0 et
	// écraserait silencieusement le solde existant (audit FIN-2, « vide ≠ 0 »).
	balanceStr := r.FormValue("balance")
	if balanceStr == "" {
		clientError(w, ErrValidation, "Solde requis", http.StatusBadRequest)
		return
	}
	balance, err := parseCents(balanceStr)
	if err != nil {
		clientError(w, ErrValidation, "Solde invalide", http.StatusBadRequest)
		return
	}

	err = hookUpdateAccountBalance(id, user.ID, balance)
	if err != nil {
		serverError(w, "update balance", err)
		return
	}

	renderAccountsList(w, user)
}

// MoveAccount deplace un compte vers le haut ou le bas
func MoveAccount(w http.ResponseWriter, r *http.Request) {
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

	direction := r.URL.Query().Get("direction")
	if direction != "up" && direction != "down" {
		clientError(w, ErrValidation, "Direction invalide", http.StatusBadRequest)
		return
	}

	// Recuperer tous les comptes tries par position
	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}

	// Trouver l'index du compte a deplacer
	currentIdx := -1
	for i, acc := range accounts {
		if acc.ID == id {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		clientError(w, ErrNotFound, "Compte non trouvé", http.StatusNotFound)
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
		renderAccountsList(w, user)
		return
	}

	// Echanger les positions
	err = hookSwapAccountPositions(accounts[currentIdx].ID, accounts[targetIdx].ID, user.ID)
	if err != nil {
		serverError(w, "swap positions", err)
		return
	}

	// Retourner la liste mise a jour
	renderAccountsList(w, user)
}

// ReorderAccounts reordonne les comptes selon un tableau d'IDs
func ReorderAccounts(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		clientError(w, ErrValidation, "Données invalides", http.StatusBadRequest)
		return
	}

	if err := hookReorderAccounts(user.ID, body.IDs); err != nil {
		serverError(w, "reorder accounts", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// renderAccountsList rend la liste des comptes en HTML avec OOB updates
func renderAccountsList(w http.ResponseWriter, user *middleware.User) {
	lang, currency := userLocale(user)

	accounts, recurrings, accErr, recErr := loadAccountsAndRecurring(user.ID)
	if accErr != nil {
		serverError(w, "renderAccountsList: accounts", accErr)
		return
	}
	if recErr != nil {
		serverError(w, "renderAccountsList: recurring", recErr)
		return
	}

	computed := computeAccountsSummary(lang, accounts, recurrings)
	accounts = computed.accounts
	s := computed.summary
	recurringData := computed.recurringData

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Rendre la liste des comptes
	if len(accounts) == 0 {
		hookRenderPartial(w, "accounts.html", "accounts-empty", map[string]interface{}{ //nolint:errcheck
			"T": i18n.Map(lang),
		})
	}
	lastIdx := len(accounts) - 1
	for i, acc := range accounts {
		hookRenderPartial(w, "accounts.html", "account-row", map[string]interface{}{ //nolint:errcheck
			"Account":  acc,
			"Currency": currency,
			"T":        i18n.Map(lang),
			"IsFirst":  i == 0,
			"IsLast":   i == lastIdx,
		})
	}

	// OOB: Rendre le summary card
	renderSummaryCardOOB(w, lang, currency, s)

	// OOB: Rendre le tableau des recurrents
	hookRenderPartial(w, "accounts.html", "recurring-table-oob", map[string]interface{}{ //nolint:errcheck
		"Recurrings": recurringData,
		"Currency":   currency,
		"T":          i18n.Map(lang),
	})

	// OOB: Mettre a jour les selects de comptes (formulaires recurrent + compte)
	accountSelectData := map[string]interface{}{
		"Accounts": accounts,
		"T":        i18n.Map(lang),
	}
	hookRenderPartial(w, "accounts.html", "account-options-recurring", accountSelectData) //nolint:errcheck
	hookRenderPartial(w, "accounts.html", "account-options-target", accountSelectData)    //nolint:errcheck
}
