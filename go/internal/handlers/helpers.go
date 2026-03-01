package handlers

import (
	"log/slog"
	"net/http"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/projection"
)

// serverError logue l'erreur interne et renvoie une 500 générique au client.
func serverError(w http.ResponseWriter, context string, err error) {
	slog.Error(context, "err", err)
	http.Error(w, "Erreur serveur", http.StatusInternalServerError)
}

// setSessionCookie pose un cookie de session avec les flags de sécurité appropriés
func setSessionCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie supprime un cookie en le posant avec MaxAge=-1
func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// decryptAccountNames déchiffre les noms de comptes en place
func decryptAccountNames(accounts []db.Account) {
	for i := range accounts {
		if decrypted, err := crypto.Decrypt(accounts[i].Name); err == nil {
			accounts[i].Name = decrypted
		}
	}
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
		if decrypted, err := crypto.Decrypt(rec.Description); err == nil {
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
