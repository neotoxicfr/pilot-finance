package projection

import (
	"fmt"
	"math"
	"time"

	"pilot-finance/internal/db"
)

// YearData represente les donnees d'une annee de projection
type YearData struct {
	Year     int                `json:"year"`
	Name     string             `json:"name"`
	TotalMin float64            `json:"totalMin"`
	TotalMax float64            `json:"totalMax"`
	TotalAvg float64            `json:"totalAvg"`
	Accounts map[string]float64 `json:"accounts"`
}

// DashboardData contient toutes les donnees du dashboard
type DashboardData struct {
	Accounts       []db.Account `json:"accounts"`
	Projection     []YearData   `json:"projection"`
	TotalInterests float64      `json:"totalInterests"`
	TotalBalance   float64      `json:"totalBalance"`
}

// Calculate calcule les projections sur N annees avec simulation mois par mois.
// Prend en compte les opérations récurrentes et la périodicité des versements.
func Calculate(accounts []db.Account, recurrings []db.RecurringOperation, years int, lang string) DashboardData {
	var totalBalance float64
	for _, acc := range accounts {
		totalBalance += acc.Balance
	}

	balances := make(map[int64]float64)
	accountByID := make(map[int64]*db.Account)
	nameByID := make(map[int64]string)

	for i := range accounts {
		acc := &accounts[i]
		balances[acc.ID] = acc.Balance
		accountByID[acc.ID] = acc
		nameByID[acc.ID] = acc.Name
	}

	useMonths := years <= 2
	totalMonths := years * 12
	var projection []YearData

	monthLabel := func(m int) string { return formatMonthName(m, lang) }

	createYearData := func(index int, name string) YearData {
		yearData := YearData{
			Year:     index,
			Name:     name,
			Accounts: make(map[string]float64),
		}
		for id, balance := range balances {
			accName := nameByID[id]
			yearData.Accounts[accName] = math.Round(balance)
			yearData.TotalAvg += balance
		}
		yearData.TotalMin = math.Round(yearData.TotalAvg)
		yearData.TotalMax = math.Round(yearData.TotalAvg)
		yearData.TotalAvg = math.Round(yearData.TotalAvg)
		return yearData
	}

	if useMonths {
		projection = append(projection, createYearData(0, monthLabel(0)))
	} else {
		projection = append(projection, createYearData(0, formatYearName(0)))
	}

	// Accumulateur pour les versements annuels (remis à zéro chaque année)
	annualPayoutAccum := make(map[int64]float64)

	for m := 1; m <= totalMonths; m++ {
		// 1. Appliquer les opérations récurrentes
		for _, rec := range recurrings {
			if rec.ToAccountID != nil {
				// Virement : source → cible
				amt := math.Abs(rec.Amount)
				if _, ok := balances[rec.AccountID]; ok {
					balances[rec.AccountID] -= amt
				}
				if _, ok := balances[*rec.ToAccountID]; ok {
					balances[*rec.ToAccountID] += amt
				}
			} else {
				// Entrée ou sortie (amount > 0 = entrée, < 0 = sortie)
				if _, ok := balances[rec.AccountID]; ok {
					balances[rec.AccountID] += rec.Amount
				}
			}
		}

		// 2. Calculer les intérêts (compounding mensuel, taux annuel ÷ 12)
		monthlyPayouts := make(map[int64]float64)

		for id, acc := range accountByID {
			if !acc.IsYieldActive {
				continue
			}
			currentBalance := balances[id]

			rate := acc.YieldMin
			if acc.YieldType == "RANGE" {
				rate = (acc.YieldMin + acc.YieldMax) / 2
			}
			monthlyRate := rate / 100 / 12
			monthlyInterest := currentBalance * monthlyRate

			reinvestRatio := float64(acc.ReinvestmentRate) / 100
			reinvested := monthlyInterest * reinvestRatio
			balances[id] = currentBalance + reinvested

			payout := monthlyInterest - reinvested
			if payout > 0 && acc.TargetAccountID != nil {
				freq := acc.PayoutFrequency
				if freq == "YEARLY" {
					annualPayoutAccum[*acc.TargetAccountID] += payout
				} else {
					monthlyPayouts[*acc.TargetAccountID] += payout
				}
			}
		}

		// Verser les payouts mensuels immédiatement
		for targetID, amount := range monthlyPayouts {
			balances[targetID] += amount
		}

		// Verser les payouts annuels accumulés en fin d'année
		if m%12 == 0 {
			for targetID, amount := range annualPayoutAccum {
				balances[targetID] += amount
			}
			annualPayoutAccum = make(map[int64]float64)
		}

		// Enregistrer le point de données
		if useMonths {
			projection = append(projection, createYearData(m, monthLabel(m)))
		} else if m%12 == 0 {
			yearIndex := m / 12
			projection = append(projection, createYearData(yearIndex, formatYearName(yearIndex)))
		}
	}

	var finalTotal float64
	for _, balance := range balances {
		finalTotal += balance
	}
	totalInterests := finalTotal - totalBalance

	return DashboardData{
		Accounts:       accounts,
		Projection:     projection,
		TotalInterests: math.Round(totalInterests),
		TotalBalance:   totalBalance,
	}
}

// YieldPayout represente un paiement d'interets non reinvestis
type YieldPayout struct {
	SourceAccountID   int64
	SourceAccountName string
	TargetAccountID   *int64
	TargetAccountName string
	Amount            float64
	Rate              float64
	PayoutFrequency   string // MONTHLY ou YEARLY
}

// CalculateYieldPayouts calcule les payouts détaillés par compte.
// Pour MONTHLY : montant mensuel (taux annuel ÷ 12).
// Pour YEARLY : montant annuel complet.
func CalculateYieldPayouts(accounts []db.Account, accountNames map[int64]string) []YieldPayout {
	var payouts []YieldPayout

	for _, acc := range accounts {
		if acc.IsYieldActive && acc.ReinvestmentRate < 100 && acc.TargetAccountID != nil {
			rate := acc.YieldMin
			if acc.YieldType == "RANGE" {
				rate = (acc.YieldMin + acc.YieldMax) / 2
			}
			annualGain := acc.Balance * (rate / 100)
			nonReinvested := 1 - float64(acc.ReinvestmentRate)/100

			freq := acc.PayoutFrequency
			if freq == "" {
				freq = "MONTHLY"
			}

			var payout float64
			if freq == "YEARLY" {
				payout = annualGain * nonReinvested
			} else {
				payout = (annualGain / 12) * nonReinvested
			}

			if payout > 0 {
				targetName := accountNames[*acc.TargetAccountID]
				payouts = append(payouts, YieldPayout{
					SourceAccountID:   acc.ID,
					SourceAccountName: accountNames[acc.ID],
					TargetAccountID:   acc.TargetAccountID,
					TargetAccountName: targetName,
					Amount:            payout,
					Rate:              rate,
					PayoutFrequency:   freq,
				})
			}
		}
	}

	return payouts
}

// CalculateMonthlyYieldPayout calcule uniquement les revenus de rendement à versement mensuel
func CalculateMonthlyYieldPayout(accounts []db.Account) float64 {
	var monthlyPayout float64
	for _, acc := range accounts {
		if acc.IsYieldActive && acc.PayoutFrequency != "YEARLY" {
			rate := acc.YieldMin
			if acc.YieldType == "RANGE" {
				rate = (acc.YieldMin + acc.YieldMax) / 2
			}
			annualGain := acc.Balance * (rate / 100)
			monthlyGain := annualGain / 12
			payout := monthlyGain * (1 - float64(acc.ReinvestmentRate)/100)
			monthlyPayout += payout
		}
	}
	return monthlyPayout
}

// CalculateAnnualYieldPayout calcule les revenus de rendement à versement annuel
func CalculateAnnualYieldPayout(accounts []db.Account) float64 {
	var annualPayout float64
	for _, acc := range accounts {
		if acc.IsYieldActive && acc.PayoutFrequency == "YEARLY" {
			rate := acc.YieldMin
			if acc.YieldType == "RANGE" {
				rate = (acc.YieldMin + acc.YieldMax) / 2
			}
			annualGain := acc.Balance * (rate / 100)
			payout := annualGain * (1 - float64(acc.ReinvestmentRate)/100)
			annualPayout += payout
		}
	}
	return annualPayout
}

// MonthlySummary calcule le resume mensuel
type MonthlySummary struct {
	Income      float64 `json:"income"`
	Expenses    float64 `json:"expenses"`
	Net         float64 `json:"net"`
	Yield       float64 `json:"yield"`
	YieldAnnual float64 `json:"yieldAnnual"`
	Transfers   float64 `json:"transfers"`
}

func CalculateMonthlySummary(recurrings []db.RecurringOperation, accounts []db.Account) MonthlySummary {
	var summary MonthlySummary

	yieldAccounts := make(map[int64]bool)
	for _, acc := range accounts {
		if acc.IsYieldActive {
			yieldAccounts[acc.ID] = true
		}
	}

	for _, rec := range recurrings {
		if rec.ToAccountID != nil {
			if yieldAccounts[*rec.ToAccountID] {
				summary.Transfers += math.Abs(rec.Amount)
			}
		} else if rec.Amount > 0 {
			summary.Income += rec.Amount
		} else {
			summary.Expenses += math.Abs(rec.Amount)
		}
	}

	summary.Yield = CalculateMonthlyYieldPayout(accounts)
	summary.YieldAnnual = CalculateAnnualYieldPayout(accounts)
	summary.Income += summary.Yield
	summary.Net = summary.Income - summary.Expenses

	return summary
}

var monthNamesFR = []string{"Jan", "Fév", "Mar", "Avr", "Mai", "Jun", "Jul", "Aoû", "Sep", "Oct", "Nov", "Déc"}
var monthNamesEN = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func formatYearName(year int) string {
	currentYear := time.Now().Year()
	if year == 0 {
		return fmt.Sprintf("%d", currentYear)
	}
	return fmt.Sprintf("%d", currentYear+year)
}

func formatMonthName(monthsFromNow int, lang string) string {
	now := time.Now()
	targetDate := now.AddDate(0, monthsFromNow, 0)
	month := int(targetDate.Month()) - 1
	year := targetDate.Year()
	names := monthNamesFR
	if lang == "en" {
		names = monthNamesEN
	}
	return fmt.Sprintf("%s %d", names[month], year)
}
