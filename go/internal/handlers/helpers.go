package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

// loadAccountsAndRecurring récupère comptes et opérations récurrentes en
// parallèle (H2 perf). Sur SQLite, deux requêtes read peuvent s'exécuter
// concurremment ; en sériel on observe une latence cumulative. Les erreurs
// sont retournées séparément pour que les handlers puissent traiter chaque
// échec différemment (compte critique vs récurrent non-bloquant).
func loadAccountsAndRecurring(userID int64) ([]db.Account, []db.RecurringOperation, error, error) {
	var accs []db.Account
	var recs []db.RecurringOperation
	var accErr, recErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		accs, accErr = hookGetAccountsByUserID(userID)
	}()
	go func() {
		defer wg.Done()
		recs, recErr = hookGetRecurringByUserID(userID)
	}()
	wg.Wait()
	return accs, recs, accErr, recErr
}

// parseFormAny parses form data from the request body for any HTTP method.
// Go's ParseForm only reads the body for POST/PUT/PATCH; this extends it to DELETE.
func parseFormAny(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	if r.Method == http.MethodDelete && r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return err
		}
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return err
		}
		for k, v := range vals {
			r.Form[k] = v
		}
	}
	return nil
}

// serverError logue l'erreur interne et renvoie une 500 générique au client.
// Inclut le header X-Error-Code pour le suivi structuré.
func serverError(w http.ResponseWriter, context string, err error) {
	slog.Error(context, "err", err)
	clientError(w, ErrInternal, "Erreur serveur", http.StatusInternalServerError)
}

// setSessionCookie pose un cookie de session avec les flags de sécurité appropriés
func setSessionCookie(w http.ResponseWriter, name, value string, maxAge int) {
	setScopedCookie(w, name, value, maxAge, "/")
}

// setScopedCookie pose un cookie de session limité à un Path donné. Utilisé
// pour les cookies qui ne sont pertinents que sur une sous-arborescence
// (mfa_setup → /settings/mfa, passkey_* → /api/passkey) afin de réduire la
// surface d'exfiltration et d'éviter de les envoyer sur toutes les requêtes.
// path doit être non-vide (utiliser "/" pour cookies globaux).
func setScopedCookie(w http.ResponseWriter, name, value string, maxAge int, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie supprime un cookie en le posant avec MaxAge=-1
func clearCookie(w http.ResponseWriter, name string) {
	clearScopedCookie(w, name, "/")
}

// clearScopedCookie supprime un cookie posé avec un Path spécifique. Le Path
// du cookie d'effacement DOIT correspondre à celui du cookie d'origine,
// sinon le navigateur n'efface pas l'entrée. path doit être non-vide.
func clearScopedCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// decryptAccountNames déchiffre les noms de comptes en place
func decryptAccountNames(accounts []db.Account) {
	for i := range accounts {
		if decrypted, err := hookDecryptStr(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		} else {
			slog.Warn("decryptAccountNames: failed", "accountID", accounts[i].ID, "err", err)
			accounts[i].Name = "???"
		}
	}
}

// summaryTotals contient les totaux mensuels/annuels pour le summary card
type summaryTotals struct {
	MonthlyIncome   float64
	MonthlyExpenses float64
	MonthlyYield    float64
	AnnualYield     float64
}

// calculateSummary calcule les totaux à partir des yield payouts et opérations récurrentes
func calculateSummary(yieldPayouts []projection.YieldPayout, recurrings []db.RecurringOperation) summaryTotals {
	var s summaryTotals
	for _, payout := range yieldPayouts {
		if payout.PayoutFrequency == "YEARLY" {
			s.AnnualYield += payout.Amount
		} else {
			s.MonthlyIncome += payout.Amount
			s.MonthlyYield += payout.Amount
		}
	}
	for _, rec := range recurrings {
		if rec.ToAccountID != nil {
			continue
		}
		recAmt := float64(rec.Amount) / 100.0
		if recAmt > 0 {
			s.MonthlyIncome += recAmt
		} else {
			s.MonthlyExpenses += -recAmt
		}
	}
	return s
}

// buildAccountMap construit un map ID→nom depuis des comptes déjà déchiffrés
func buildAccountMap(accounts []db.Account) map[int64]string {
	m := make(map[int64]string, len(accounts))
	for _, acc := range accounts {
		m[acc.ID] = acc.Name
	}
	return m
}

// buildRecurringData construit la liste des opérations récurrentes pour les templates.
// Les noms dans accountMap doivent être déjà déchiffrés.
func buildRecurringData(payouts []projection.YieldPayout, recs []db.RecurringOperation, accountMap map[int64]string, interestPrefix string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(recs)+len(payouts))
	for _, payout := range payouts {
		result = append(result, map[string]interface{}{
			"ID":              int64(0),
			"Description":     interestPrefix + " " + payout.SourceAccountName,
			"Amount":          payout.Amount,
			"DayOfMonth":      1,
			"AccountID":       payout.SourceAccountID,
			"AccountName":     payout.SourceAccountName,
			"ToAccountID":     payout.TargetAccountID,
			"ToAccountName":   payout.TargetAccountName,
			"IsActive":        true,
			"IsYieldPayout":   true,
			"YieldRate":       payout.Rate,
			"PayoutFrequency": payout.PayoutFrequency,
		})
	}
	for _, rec := range recs {
		description := rec.Description
		if decrypted, err := hookDecryptStr(rec.Description); err == nil {
			description = decrypted
		}
		toAccountName := ""
		if rec.ToAccountID != nil {
			toAccountName = accountMap[*rec.ToAccountID]
		}
		result = append(result, map[string]interface{}{
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
	return result
}

// accountsComputed regroupe les données dérivées partagées par renderAccountsList
// et renderRecurringTable : comptes déchiffrés, totaux du summary et liste des
// opérations récurrentes prêtes pour les templates.
type accountsComputed struct {
	accounts      []db.Account
	summary       summaryTotals
	recurringData []map[string]interface{}
	// linkedCounts[id] = nombre d'opérations récurrentes que la suppression du
	// compte id détruirait aussi (audit S-20).
	linkedCounts map[int64]int
}

// countLinkedRecurrings compte, pour chaque compte, les opérations récurrentes
// que sa suppression emporterait.
//
// db.DeleteAccount supprime les récurrentes dans LES DEUX SENS
// (`account_id = ? OR to_account_id = ?`) : un virement dont ce compte est la
// destination disparaît lui aussi, alors qu'il « appartient » visuellement à un
// autre compte. L'interface ne le disait pas (audit S-20) ; on compte donc les
// deux sens, comme la requête de suppression.
func countLinkedRecurrings(recurrings []db.RecurringOperation) map[int64]int {
	counts := make(map[int64]int)
	for _, r := range recurrings {
		counts[r.AccountID]++
		if r.ToAccountID != nil && *r.ToAccountID != r.AccountID {
			counts[*r.ToAccountID]++
		}
	}
	return counts
}

// computeAccountsSummary déchiffre les noms de comptes, calcule les yield payouts,
// le summary et la liste récurrente. Les comptes/récurrents doivent déjà être chargés.
func computeAccountsSummary(lang string, accounts []db.Account, recurrings []db.RecurringOperation) accountsComputed {
	decryptAccountNames(accounts)
	accountMap := buildAccountMap(accounts)

	yieldPayouts := projection.CalculateYieldPayouts(accounts, accountMap)
	interestPrefix := i18n.T(lang, "recurring.interest_prefix")
	s := calculateSummary(yieldPayouts, recurrings)
	recurringData := buildRecurringData(yieldPayouts, recurrings, accountMap, interestPrefix)

	return accountsComputed{accounts: accounts, summary: s, recurringData: recurringData, linkedCounts: countLinkedRecurrings(recurrings)}
}

// renderSummaryCardOOB rend la carte de résumé en fragment OOB (id + hx-swap-oob
// inclus dans le template "summary-card-oob").
func renderSummaryCardOOB(w io.Writer, lang, currency string, s summaryTotals) {
	hookRenderPartial(w, "accounts.html", "summary-card-oob", map[string]interface{}{ //nolint:errcheck
		"T":               i18n.Map(lang),
		"MonthlyIncome":   s.MonthlyIncome,
		"MonthlyExpenses": s.MonthlyExpenses,
		"MonthlyNet":      s.MonthlyIncome - s.MonthlyExpenses,
		"MonthlyYield":    s.MonthlyYield,
		"AnnualYield":     s.AnnualYield,
		"Currency":        currency,
		"Locale":          localeTag(lang),
	})
}

// buildPieData construit les données du graphique camembert (comptes au solde positif).
func buildPieData(accounts []db.Account) []map[string]interface{} {
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
	return pieData
}

// lastProjectionTotal retourne le TotalAvg de la dernière année de projection (0 si vide).
func lastProjectionTotal(proj []projection.YearData) float64 {
	if len(proj) > 0 {
		return proj[len(proj)-1].TotalAvg
	}
	return 0
}

// userLocale extrait la langue et la devise de l'utilisateur avec leurs valeurs par défaut
func userLocale(user *middleware.User) (lang, currency string) {
	lang, currency = "fr", "EUR"
	if user != nil {
		if user.Language != "" {
			lang = user.Language
		}
		if user.Currency != "" {
			currency = user.Currency
		}
	}
	return
}
