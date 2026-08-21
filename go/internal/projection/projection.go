package projection

import (
	"fmt"
	"math"
	"slices"
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
// Modèle de précision : les soldes sont accumulés en int64 centimes (cohérent
// avec le reste de l'application), et l'intérêt mensuel est arrondi au centime à
// chaque itération. Les totaux ne sont convertis et arrondis à l'euro qu'à la
// sortie — YearData/DashboardData restent en float64 euros pour l'affichage.
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
	var totalBalanceCents int64
	for _, acc := range accounts {
		totalBalanceCents += acc.Balance
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
	slices.Sort(sortedIDs)

	// Trois scénarios : balances et accumulateurs annuels indépendants (centimes int64)
	type scenario struct {
		balances    map[int64]int64
		annualAccum map[int64]int64
	}
	scens := [3]scenario{
		{balances: make(map[int64]int64), annualAccum: make(map[int64]int64)},
		{balances: make(map[int64]int64), annualAccum: make(map[int64]int64)},
		{balances: make(map[int64]int64), annualAccum: make(map[int64]int64)},
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
		var minC, avgC, maxC int64
		for _, id := range sortedIDs {
			bal := scens[1].balances[id]
			yd.Accounts[nameByID[id]] = math.Round(float64(bal) / 100.0)
			avgC += bal
			minC += scens[0].balances[id]
			maxC += scens[2].balances[id]
		}
		yd.TotalMin = math.Round(float64(minC) / 100.0)
		yd.TotalAvg = math.Round(float64(avgC) / 100.0)
		yd.TotalMax = math.Round(float64(maxC) / 100.0)
		return yd
	}

	if useMonths {
		projection = append(projection, createYearData(0, monthLabel(0)))
	} else {
		projection = append(projection, createYearData(0, formatYearName(0)))
	}

	// Somme des intérêts effectivement générés par le scénario avg (FIN-10).
	var avgInterestCents int64

	for m := 1; m <= totalMonths; m++ {
		// 1. Opérations récurrentes : identiques pour les trois scénarios
		for _, rec := range recurrings {
			if rec.ToAccountID != nil {
				amt := rec.Amount
				if amt < 0 {
					amt = -amt
				}
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
				// Intérêt mensuel arrondi au centime (les soldes restent en int64 centimes).
				monthlyInterest := int64(math.Round(float64(currentBal) * scenRate(acc, s)))
				// TotalInterests = somme réelle des intérêts du scénario avg, et
				// non « solde final − initial » qui incluait aussi les flux
				// récurrents (épargne comptée comme intérêts, audit FIN-10).
				if s == 1 {
					avgInterestCents += monthlyInterest
				}
				reinvested := int64(math.Round(float64(monthlyInterest) * reinvest))
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
				slices.Sort(targetIDs)
				for _, targetID := range targetIDs {
					scens[s].balances[targetID] += scens[s].annualAccum[targetID]
				}
				scens[s].annualAccum = make(map[int64]int64)
			}
		}

		if useMonths {
			projection = append(projection, createYearData(m, monthLabel(m)))
		} else if m%12 == 0 {
			yearIndex := m / 12
			projection = append(projection, createYearData(yearIndex, formatYearName(yearIndex)))
		}
	}

	return DashboardData{
		Accounts:       accounts,
		Projection:     projection,
		TotalInterests: math.Round(float64(avgInterestCents) / 100.0),
		TotalBalance:   float64(totalBalanceCents) / 100.0,
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
	// Ancrer au 1er du mois courant : AddDate normalise les débordements
	// (31 janv + 1 mois = 3 mars), ce qui ferait sauter février et afficher
	// deux « mars » sur le graphe les 29-31 du mois (audit FIN-16).
	base := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	targetDate := base.AddDate(0, monthsFromNow, 0)
	month := int(targetDate.Month()) - 1
	year := targetDate.Year()
	names := monthNamesFR
	if lang == "en" {
		names = monthNamesEN
	}
	return fmt.Sprintf("%s %d", names[month], year)
}
