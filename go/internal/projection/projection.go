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
// Trois scénarios parallèles : pessimiste (yieldMin), moyen (avg), optimiste (yieldMax).
// Pour les comptes FIXED, les trois scénarios sont identiques.
func Calculate(accounts []db.Account, recurrings []db.RecurringOperation, years int, lang string) DashboardData {
	var totalBalance float64
	for _, acc := range accounts {
		totalBalance += acc.Balance
	}

	accountByID := make(map[int64]*db.Account)
	nameByID := make(map[int64]string)
	for i := range accounts {
		acc := &accounts[i]
		accountByID[acc.ID] = acc
		nameByID[acc.ID] = acc.Name
	}

	// Trois scénarios : balances et accumulateurs annuels indépendants
	type scenario struct {
		balances    map[int64]float64
		annualAccum map[int64]float64
	}
	scens := [3]scenario{
		{balances: make(map[int64]float64), annualAccum: make(map[int64]float64)},
		{balances: make(map[int64]float64), annualAccum: make(map[int64]float64)},
		{balances: make(map[int64]float64), annualAccum: make(map[int64]float64)},
	}
	for i := range accounts {
		acc := &accounts[i]
		for s := range scens {
			scens[s].balances[acc.ID] = acc.Balance
		}
	}

	// Taux mensuel selon le scénario (0=min, 1=avg, 2=max)
	scenRate := func(acc *db.Account, s int) float64 {
		var annualRate float64
		if acc.YieldType == "RANGE" {
			switch s {
			case 0:
				annualRate = acc.YieldMin
			case 1:
				annualRate = (acc.YieldMin + acc.YieldMax) / 2
			case 2:
				annualRate = acc.YieldMax
			}
		} else {
			annualRate = acc.YieldMin
		}
		return annualRate / 100 / 12
	}

	useMonths := years <= 2
	totalMonths := years * 12
	var projection []YearData
	monthLabel := func(m int) string { return formatMonthName(m, lang) }

	// Crée un point de données à partir de l'état courant des trois scénarios.
	// Les données par compte utilisent le scénario avg (index 1).
	createYearData := func(index int, name string) YearData {
		yd := YearData{Year: index, Name: name, Accounts: make(map[string]float64)}
		for id, bal := range scens[1].balances {
			yd.Accounts[nameByID[id]] = math.Round(bal)
			yd.TotalAvg += bal
		}
		for _, bal := range scens[0].balances {
			yd.TotalMin += bal
		}
		for _, bal := range scens[2].balances {
			yd.TotalMax += bal
		}
		yd.TotalMin = math.Round(yd.TotalMin)
		yd.TotalAvg = math.Round(yd.TotalAvg)
		yd.TotalMax = math.Round(yd.TotalMax)
		return yd
	}

	if useMonths {
		projection = append(projection, createYearData(0, monthLabel(0)))
	} else {
		projection = append(projection, createYearData(0, formatYearName(0)))
	}

	for m := 1; m <= totalMonths; m++ {
		// 1. Opérations récurrentes : identiques pour les trois scénarios
		for _, rec := range recurrings {
			if rec.ToAccountID != nil {
				amt := math.Abs(rec.Amount)
				for s := range scens {
					if _, ok := scens[s].balances[rec.AccountID]; ok {
						scens[s].balances[rec.AccountID] -= amt
					}
					if _, ok := scens[s].balances[*rec.ToAccountID]; ok {
						scens[s].balances[*rec.ToAccountID] += amt
					}
				}
			} else {
				for s := range scens {
					if _, ok := scens[s].balances[rec.AccountID]; ok {
						scens[s].balances[rec.AccountID] += rec.Amount
					}
				}
			}
		}

		// 2. Intérêts : taux différent par scénario pour les comptes RANGE
		for id, acc := range accountByID {
			if !acc.IsYieldActive {
				continue
			}
			reinvest := float64(acc.ReinvestmentRate) / 100
			for s := range scens {
				currentBal := scens[s].balances[id]
				monthlyInterest := currentBal * scenRate(acc, s)
				reinvested := monthlyInterest * reinvest
				scens[s].balances[id] = currentBal + reinvested
				payout := monthlyInterest - reinvested
				if payout > 0 && acc.TargetAccountID != nil {
					if acc.PayoutFrequency == "YEARLY" {
						scens[s].annualAccum[*acc.TargetAccountID] += payout
					} else {
						scens[s].balances[*acc.TargetAccountID] += payout
					}
				}
			}
		}

		// 3. Versements annuels en fin d'année
		if m%12 == 0 {
			for s := range scens {
				for targetID, amount := range scens[s].annualAccum {
					scens[s].balances[targetID] += amount
				}
				scens[s].annualAccum = make(map[int64]float64)
			}
		}

		if useMonths {
			projection = append(projection, createYearData(m, monthLabel(m)))
		} else if m%12 == 0 {
			yearIndex := m / 12
			projection = append(projection, createYearData(yearIndex, formatYearName(yearIndex)))
		}
	}

	var finalTotal float64
	for _, balance := range scens[1].balances {
		finalTotal += balance
	}

	return DashboardData{
		Accounts:       accounts,
		Projection:     projection,
		TotalInterests: math.Round(finalTotal - totalBalance),
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
