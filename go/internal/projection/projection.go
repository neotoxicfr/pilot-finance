package projection

import (
	"fmt"
	"math"
	"time"

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

// Calculate calcule les projections sur N annees avec simulation mois par mois
func Calculate(accounts []db.Account, years int) DashboardData {
	var totalBalance float64

	// Calculer le solde total actuel
	for _, acc := range accounts {
		totalBalance += acc.Balance
	}

	// Creer des maps pour la simulation
	// balances[id] = solde courant du compte
	balances := make(map[int64]float64)
	accountByID := make(map[int64]*db.Account)
	nameByID := make(map[int64]string)

	for i := range accounts {
		acc := &accounts[i]
		balances[acc.ID] = acc.Balance
		accountByID[acc.ID] = acc
		nameByID[acc.ID] = acc.Name
	}

	// Pour les projections courtes (<=2 ans), afficher par mois
	// Pour les projections longues, afficher par annee
	useMonths := years <= 2
	totalMonths := years * 12
	var projection []YearData

	// Fonction pour creer un YearData a partir des soldes actuels
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
		yearData.TotalMin = yearData.TotalAvg
		yearData.TotalMax = yearData.TotalAvg
		yearData.TotalMin = math.Round(yearData.TotalMin)
		yearData.TotalMax = math.Round(yearData.TotalMax)
		yearData.TotalAvg = math.Round(yearData.TotalAvg)
		return yearData
	}

	// Point de depart (mois 0)
	if useMonths {
		projection = append(projection, createYearData(0, formatMonthName(0)))
	} else {
		projection = append(projection, createYearData(0, formatYearName(0)))
	}

	// Simuler mois par mois
	for m := 1; m <= totalMonths; m++ {
		// Calculer les interets de chaque compte avec rendement
		// et les redistribuer selon le taux de reinvestissement
		payouts := make(map[int64]float64) // payouts a ajouter aux comptes cibles

		for id, acc := range accountByID {
			if !acc.IsYieldActive {
				continue
			}

			currentBalance := balances[id]

			// Taux moyen mensuel
			rate := acc.YieldMin
			if acc.YieldType == "RANGE" {
				rate = (acc.YieldMin + acc.YieldMax) / 2
			}
			monthlyRate := rate / 100 / 12

			// Interet du mois
			monthlyInterest := currentBalance * monthlyRate

			// Partie reinvestie (reste sur le compte)
			reinvestRatio := float64(acc.ReinvestmentRate) / 100
			reinvested := monthlyInterest * reinvestRatio
			balances[id] = currentBalance + reinvested

			// Partie non reinvestie (va vers le compte cible si defini)
			payout := monthlyInterest - reinvested
			if payout > 0 && acc.TargetAccountID != nil {
				payouts[*acc.TargetAccountID] += payout
			}
		}

		// Ajouter les payouts aux comptes cibles
		for targetID, amount := range payouts {
			balances[targetID] += amount
		}

		// Enregistrer le point de donnees selon le mode d'affichage
		if useMonths {
			projection = append(projection, createYearData(m, formatMonthName(m)))
		} else if m%12 == 0 {
			yearIndex := m / 12
			projection = append(projection, createYearData(yearIndex, formatYearName(yearIndex)))
		}
	}

	// Calculer les interets totaux (difference entre solde final et initial)
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

// CalculateYieldPayouts calcule les payouts detailles par compte
func CalculateYieldPayouts(accounts []db.Account, accountNames map[int64]string) []YieldPayout {
	var payouts []YieldPayout

	for _, acc := range accounts {
		if acc.IsYieldActive && acc.ReinvestmentRate < 100 && acc.TargetAccountID != nil {
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

			if payout > 0 {
				targetName := ""
				if acc.TargetAccountID != nil {
					targetName = accountNames[*acc.TargetAccountID]
				}

				payouts = append(payouts, YieldPayout{
					SourceAccountID:   acc.ID,
					SourceAccountName: accountNames[acc.ID],
					TargetAccountID:   acc.TargetAccountID,
					TargetAccountName: targetName,
					Amount:            payout,
					Rate:              rate,
				})
			}
		}
	}

	return payouts
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

var monthNames = []string{"Jan", "Fev", "Mar", "Avr", "Mai", "Jun", "Jul", "Aou", "Sep", "Oct", "Nov", "Dec"}

func formatYearName(year int) string {
	currentYear := time.Now().Year()
	if year == 0 {
		return fmt.Sprintf("%d", currentYear)
	}
	return fmt.Sprintf("%d", currentYear+year)
}

func formatMonthName(monthsFromNow int) string {
	now := time.Now()
	targetDate := now.AddDate(0, monthsFromNow, 0)
	month := int(targetDate.Month()) - 1
	year := targetDate.Year()
	return fmt.Sprintf("%s %d", monthNames[month], year)
}
