package projection

import (
	"fmt"
	"math"
	"sort"
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
//
// Modèle de précision : contrairement au reste de l'application qui stocke les
// montants en int64 centimes, la projection accumule délibérément les soldes en
// float64 euros. L'invariant int64-centimes est volontairement relâché sur tout
// l'horizon de projection : la composition mois-par-mois des intérêts produit des
// fractions de centime, et la projection est une estimation prospective (affichée
// arrondie) plutôt qu'un registre comptable exact. Ce choix est assumé.
//
// Granularité temporelle : la simulation est purement mensuelle. Le champ
// DayOfMonth des opérations récurrentes n'est PAS pris en compte / proraté : toute
// opération d'un mois donné est appliquée en bloc une fois par mois (choix de
// conception, pas un bug).
//
// Soldes négatifs : aucun plancher n'est appliqué. Un solde peut devenir négatif
// (p. ex. via des transferts/dépenses récurrents), et les intérêts sont alors
// calculés sur le solde signé — un solde négatif génère donc un « intérêt »
// négatif. Ce comportement est délibéré (cohérence arithmétique du modèle).
//
// Déterminisme : toutes les itérations qui dépendaient de l'ordre d'itération des
// maps (boucle d'intérêts mutant des soldes cibles, sommes des totaux) parcourent
// désormais une tranche d'IDs triée (sortedIDs), ce qui rend le résultat
// reproductible d'une exécution à l'autre.
func Calculate(accounts []db.Account, recurrings []db.RecurringOperation, years int, lang string) DashboardData {
	var totalBalance float64
	for _, acc := range accounts {
		totalBalance += float64(acc.Balance) / 100.0
	}

	accountByID := make(map[int64]*db.Account)
	nameByID := make(map[int64]string)
	sortedIDs := make([]int64, 0, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		accountByID[acc.ID] = acc
		nameByID[acc.ID] = acc.Name
		sortedIDs = append(sortedIDs, acc.ID)
	}
	// Ordre stable pour toutes les itérations dépendantes de l'ordre (déterminisme).
	sort.Slice(sortedIDs, func(i, j int) bool { return sortedIDs[i] < sortedIDs[j] })

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
			scens[s].balances[acc.ID] = float64(acc.Balance) / 100.0
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
				annualRate = effectiveRate(*acc)
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
		for _, id := range sortedIDs {
			bal := scens[1].balances[id]
			yd.Accounts[nameByID[id]] = math.Round(bal)
			yd.TotalAvg += bal
			yd.TotalMin += scens[0].balances[id]
			yd.TotalMax += scens[2].balances[id]
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
				amt := math.Abs(float64(rec.Amount) / 100.0)
				for s := range scens {
					if _, ok := scens[s].balances[rec.AccountID]; ok {
						scens[s].balances[rec.AccountID] -= amt
					}
					if _, ok := scens[s].balances[*rec.ToAccountID]; ok {
						scens[s].balances[*rec.ToAccountID] += amt
					}
				}
			} else {
				recAmt := float64(rec.Amount) / 100.0
				for s := range scens {
					if _, ok := scens[s].balances[rec.AccountID]; ok {
						scens[s].balances[rec.AccountID] += recAmt
					}
				}
			}
		}

		// 2. Intérêts : taux différent par scénario pour les comptes RANGE.
		// Itération sur sortedIDs (ordre stable) : la boucle mute des soldes cibles
		// (payout mensuel), donc l'ordre doit être déterministe.
		for _, id := range sortedIDs {
			acc := accountByID[id]
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

		// 3. Versements annuels en fin d'année (ordre stable : tri des cibles).
		if m%12 == 0 {
			for s := range scens {
				targetIDs := make([]int64, 0, len(scens[s].annualAccum))
				for targetID := range scens[s].annualAccum {
					targetIDs = append(targetIDs, targetID)
				}
				sort.Slice(targetIDs, func(i, j int) bool { return targetIDs[i] < targetIDs[j] })
				for _, targetID := range targetIDs {
					scens[s].balances[targetID] += scens[s].annualAccum[targetID]
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
	for _, id := range sortedIDs {
		finalTotal += scens[1].balances[id]
	}

	return DashboardData{
		Accounts:       accounts,
		Projection:     projection,
		TotalInterests: math.Round(finalTotal - totalBalance),
		TotalBalance:   totalBalance,
	}
}

// effectiveRate retourne le taux annuel « moyen » d'un compte : YieldMin pour les
// comptes FIXED, et la moyenne (YieldMin+YieldMax)/2 pour les comptes RANGE.
// C'est le taux utilisé par le scénario avg et par tous les calculs de payout.
func effectiveRate(acc db.Account) float64 {
	if acc.YieldType == "RANGE" {
		return (acc.YieldMin + acc.YieldMax) / 2
	}
	return acc.YieldMin
}

// annualPayout retourne le gain annuel non réinvesti d'un compte :
// solde × taux effectif × (part non réinvestie).
func annualPayout(acc db.Account) float64 {
	rate := effectiveRate(acc)
	annualGain := (float64(acc.Balance) / 100.0) * (rate / 100)
	return annualGain * (1 - float64(acc.ReinvestmentRate)/100)
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
			rate := effectiveRate(acc)
			annual := annualPayout(acc)

			freq := acc.PayoutFrequency
			if freq == "" {
				freq = "MONTHLY"
			}

			var payout float64
			if freq == "YEARLY" {
				payout = annual
			} else {
				payout = annual / 12
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
			monthlyPayout += annualPayout(acc) / 12
		}
	}
	return monthlyPayout
}

// CalculateAnnualYieldPayout calcule les revenus de rendement à versement annuel
func CalculateAnnualYieldPayout(accounts []db.Account) float64 {
	var total float64
	for _, acc := range accounts {
		if acc.IsYieldActive && acc.PayoutFrequency == "YEARLY" {
			total += annualPayout(acc)
		}
	}
	return total
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

// CalculateMonthlySummary agrège les opérations récurrentes en revenus, dépenses et transferts mensuels
func CalculateMonthlySummary(recurrings []db.RecurringOperation, accounts []db.Account) MonthlySummary {
	var summary MonthlySummary

	yieldAccounts := make(map[int64]bool)
	for _, acc := range accounts {
		if acc.IsYieldActive {
			yieldAccounts[acc.ID] = true
		}
	}

	for _, rec := range recurrings {
		recAmt := float64(rec.Amount) / 100.0
		if rec.ToAccountID != nil {
			if yieldAccounts[*rec.ToAccountID] {
				summary.Transfers += math.Abs(recAmt)
			}
		} else if recAmt > 0 {
			summary.Income += recAmt
		} else {
			summary.Expenses += math.Abs(recAmt)
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
