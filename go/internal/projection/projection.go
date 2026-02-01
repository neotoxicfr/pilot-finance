package projection

import (
	"math"

	"pilot-finance/internal/db"
)

// YearData represente les donnees d'une annee de projection
type YearData struct {
	Year     int                    `json:"year"`
	Name     string                 `json:"name"`
	TotalMin float64                `json:"totalMin"`
	TotalMax float64                `json:"totalMax"`
	TotalAvg float64                `json:"totalAvg"`
	Accounts map[string]float64     `json:"accounts"`
}

// DashboardData contient toutes les donnees du dashboard
type DashboardData struct {
	Accounts       []db.Account      `json:"accounts"`
	Projection     []YearData        `json:"projection"`
	TotalInterests float64           `json:"totalInterests"`
	TotalBalance   float64           `json:"totalBalance"`
}

// Calculate calcule les projections sur N annees
func Calculate(accounts []db.Account, years int) DashboardData {
	projection := make([]YearData, 0, years+1)
	var totalInterests float64
	var totalBalance float64

	// Calculer le solde total actuel
	for _, acc := range accounts {
		totalBalance += acc.Balance
	}

	for i := 0; i <= years; i++ {
		yearData := YearData{
			Year:     i,
			Name:     formatYearName(i),
			Accounts: make(map[string]float64),
		}

		for _, acc := range accounts {
			if !acc.IsYieldActive {
				// Compte sans rendement : solde constant
				yearData.Accounts[acc.Name] = acc.Balance
				yearData.TotalMin += acc.Balance
				yearData.TotalMax += acc.Balance
				yearData.TotalAvg += acc.Balance
			} else {
				// Compte avec rendement : interets composes
				rateMin := acc.YieldMin
				rateMax := acc.YieldMax

				// Si type FIXED, min = max
				if acc.YieldType == "FIXED" || acc.YieldType == "" {
					rateMax = rateMin
				}

				// Formule des interets composes : P * (1 + r)^n
				compoundMin := acc.Balance * math.Pow(1+rateMin/100, float64(i))
				compoundMax := acc.Balance * math.Pow(1+rateMax/100, float64(i))
				compoundAvg := (compoundMin + compoundMax) / 2

				yearData.Accounts[acc.Name] = math.Round(compoundAvg)
				yearData.TotalMin += compoundMin
				yearData.TotalMax += compoundMax
				yearData.TotalAvg += compoundAvg

				// Calculer les interets totaux sur la derniere annee
				if i == years {
					totalInterests += (compoundAvg - acc.Balance)
				}
			}
		}

		yearData.TotalMin = math.Round(yearData.TotalMin)
		yearData.TotalMax = math.Round(yearData.TotalMax)
		yearData.TotalAvg = math.Round(yearData.TotalAvg)

		projection = append(projection, yearData)
	}

	return DashboardData{
		Accounts:       accounts,
		Projection:     projection,
		TotalInterests: math.Round(totalInterests),
		TotalBalance:   totalBalance,
	}
}

// CalculateMonthlyYieldPayout calcule les revenus mensuels de rendement
func CalculateMonthlyYieldPayout(accounts []db.Account) float64 {
	var monthlyPayout float64

	for _, acc := range accounts {
		if acc.IsYieldActive {
			// Taux moyen
			rate := acc.YieldMin
			if acc.YieldType == "RANGE" {
				rate = (acc.YieldMin + acc.YieldMax) / 2
			}

			// Gain annuel
			annualGain := acc.Balance * (rate / 100)
			// Gain mensuel
			monthlyGain := annualGain / 12
			// Payout (partie non reinvestie)
			payout := monthlyGain * (1 - float64(acc.ReinvestmentRate)/100)
			monthlyPayout += payout
		}
	}

	return monthlyPayout
}

// CalculateMonthlySummary calcule le resume mensuel
type MonthlySummary struct {
	Income    float64 `json:"income"`
	Expenses  float64 `json:"expenses"`
	Net       float64 `json:"net"`
	Yield     float64 `json:"yield"`
	Transfers float64 `json:"transfers"`
}

func CalculateMonthlySummary(recurrings []db.RecurringOperation, accounts []db.Account) MonthlySummary {
	var summary MonthlySummary

	// Creer une map des comptes avec rendement
	yieldAccounts := make(map[int64]bool)
	for _, acc := range accounts {
		if acc.IsYieldActive {
			yieldAccounts[acc.ID] = true
		}
	}

	for _, rec := range recurrings {
		if rec.ToAccountID != nil {
			// C'est un virement
			if yieldAccounts[*rec.ToAccountID] {
				summary.Transfers += math.Abs(rec.Amount)
			}
		} else if rec.Amount > 0 {
			summary.Income += rec.Amount
		} else {
			summary.Expenses += math.Abs(rec.Amount)
		}
	}

	// Ajouter les revenus de rendement
	summary.Yield = CalculateMonthlyYieldPayout(accounts)
	summary.Income += summary.Yield
	summary.Net = summary.Income - summary.Expenses

	return summary
}

func formatYearName(year int) string {
	if year == 0 {
		return "Aujourd'hui"
	}
	return "Annee " + itoa(year)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
