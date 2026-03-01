package projection_test

import (
	"testing"

	"pilot-finance/internal/db"
	"pilot-finance/internal/projection"
)

// TestCalculate_YearlyPayoutFlush couvre projection.go:159 — inner loop body du flush annuel.
// Nécessite annualAccum non vide à m=12 (years=1, useMonths=true, PayoutFrequency=YEARLY).
func TestCalculate_YearlyPayoutFlush(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{
			ID: 1, Name: "Source", Balance: 12000,
			IsYieldActive: true, YieldType: "FIXED", YieldMin: 12,
			ReinvestmentRate: 0, PayoutFrequency: "YEARLY",
			TargetAccountID: &targetID,
		},
		{ID: 2, Name: "Target", Balance: 1000},
	}
	result := projection.Calculate(accounts, nil, 1, "fr")
	if len(result.Projection) == 0 {
		t.Fatal("projection vide")
	}
	// Le flush annuel à m=12 doit avoir crédité Target — TotalInterests > 0
	if result.TotalInterests <= 0 {
		t.Errorf("TotalInterests: want >0 (flush YEARLY), got %v", result.TotalInterests)
	}
}

func TestCalculateEmpty(t *testing.T) {
	result := projection.Calculate(nil, nil, 5, "en")
	if result.TotalBalance != 0 {
		t.Errorf("TotalBalance: want 0, got %v", result.TotalBalance)
	}
	if result.TotalInterests != 0 {
		t.Errorf("TotalInterests: want 0, got %v", result.TotalInterests)
	}
	if len(result.Projection) == 0 {
		t.Error("projection should have at least 1 data point")
	}
}

func TestCalculateSimpleNoYield(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Checking", Balance: 10000, IsYieldActive: false},
	}
	result := projection.Calculate(accounts, nil, 3, "en")
	if result.TotalBalance != 10000 {
		t.Errorf("TotalBalance: want 10000, got %v", result.TotalBalance)
	}
	// 3 years → year 0 + years 1,2,3 = 4 data points
	if len(result.Projection) != 4 {
		t.Errorf("want 4 data points, got %d", len(result.Projection))
	}
	last := result.Projection[len(result.Projection)-1]
	if last.TotalAvg != 10000 {
		t.Errorf("final balance: want 10000, got %v", last.TotalAvg)
	}
}

func TestCalculateMonthlyGranularity(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Checking", Balance: 5000, IsYieldActive: false},
	}
	// years <= 2 → monthly granularity
	result := projection.Calculate(accounts, nil, 1, "fr")
	// year 0 + 12 months = 13 data points
	if len(result.Projection) != 13 {
		t.Errorf("want 13 data points (monthly), got %d", len(result.Projection))
	}
}

func TestCalculateWithYieldFixed(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 10000, IsYieldActive: true, YieldType: "FIXED",
			YieldMin: 5.0, YieldMax: 5.0, ReinvestmentRate: 100, PayoutFrequency: "MONTHLY"},
	}
	result := projection.Calculate(accounts, nil, 3, "en")
	last := result.Projection[len(result.Projection)-1]
	if last.TotalAvg <= 10000 {
		t.Errorf("yield account should grow: got %v", last.TotalAvg)
	}
	// All three scenarios equal for FIXED rate
	if last.TotalMin != last.TotalAvg || last.TotalAvg != last.TotalMax {
		t.Errorf("FIXED rate: min/avg/max should be equal: %v/%v/%v", last.TotalMin, last.TotalAvg, last.TotalMax)
	}
}

func TestCalculateWithYieldRange(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "PEA", Balance: 10000, IsYieldActive: true, YieldType: "RANGE",
			YieldMin: 2.0, YieldMax: 8.0, ReinvestmentRate: 100, PayoutFrequency: "MONTHLY"},
	}
	result := projection.Calculate(accounts, nil, 5, "en")
	last := result.Projection[len(result.Projection)-1]
	if last.TotalMin >= last.TotalAvg {
		t.Errorf("TotalMin should be < TotalAvg: %v >= %v", last.TotalMin, last.TotalAvg)
	}
	if last.TotalAvg >= last.TotalMax {
		t.Errorf("TotalAvg should be < TotalMax: %v >= %v", last.TotalAvg, last.TotalMax)
	}
}

func TestCalculateWithRecurringIncome(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Checking", Balance: 0, IsYieldActive: false},
	}
	recurrings := []db.RecurringOperation{
		{ID: 1, UserID: 1, AccountID: 1, Amount: 1000, DayOfMonth: 1},
	}
	result := projection.Calculate(accounts, recurrings, 1, "en")
	last := result.Projection[len(result.Projection)-1]
	if last.TotalAvg != 12000 {
		t.Errorf("12 months of 1000/month: want 12000, got %v", last.TotalAvg)
	}
}

// TestCalculateWithTransfer covers the rec.ToAccountID != nil branch in Calculate.
func TestCalculateWithTransfer(t *testing.T) {
	fromID := int64(1)
	toID := int64(2)
	accounts := []db.Account{
		{ID: fromID, Name: "Checking", Balance: 5000, IsYieldActive: false},
		{ID: toID, Name: "Savings", Balance: 0, IsYieldActive: false},
	}
	recurrings := []db.RecurringOperation{
		{ID: 1, UserID: 1, AccountID: fromID, ToAccountID: &toID, Amount: 500, DayOfMonth: 1},
	}
	result := projection.Calculate(accounts, recurrings, 1, "en")
	last := result.Projection[len(result.Projection)-1]
	// From: 5000 - 12*500 = -1000, To: 0 + 12*500 = 6000, total stays at 5000
	if last.TotalAvg != 5000 {
		t.Errorf("transfer: total should stay 5000, got %v", last.TotalAvg)
	}
}

// TestCalculateWithYearlyPayout covers the YEARLY payout / annualAccum flush branch.
func TestCalculateWithYearlyPayout(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 10000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 12.0, YieldMax: 12.0,
			ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: "YEARLY"},
		{ID: targetID, Name: "Checking", Balance: 0, IsYieldActive: false},
	}
	result := projection.Calculate(accounts, nil, 3, "en")
	last := result.Projection[len(result.Projection)-1]
	// Each year: 10000 * 12% = 1200 flushed to Checking. After 3 years: 3600 + 10000 = 13600
	if last.TotalAvg != 13600 {
		t.Errorf("yearly payout after 3 years: want 13600, got %v", last.TotalAvg)
	}
}

func TestCalculateYieldPayoutsMonthly(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 12000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 3.0, YieldMax: 3.0,
			ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: "MONTHLY"},
		{ID: 2, Name: "Checking", Balance: 1000},
	}
	names := map[int64]string{1: "Savings", 2: "Checking"}

	payouts := projection.CalculateYieldPayouts(accounts, names)
	if len(payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(payouts))
	}
	// 12000 * 3% / 12 = 30
	if payouts[0].Amount != 30 {
		t.Errorf("monthly payout: want 30, got %v", payouts[0].Amount)
	}
	if payouts[0].PayoutFrequency != "MONTHLY" {
		t.Errorf("frequency: want MONTHLY, got %q", payouts[0].PayoutFrequency)
	}
}

func TestCalculateYieldPayoutsYearly(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "Livret", Balance: 10000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 2.0, YieldMax: 2.0,
			ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: "YEARLY"},
		{ID: 2, Name: "Checking", Balance: 500},
	}
	names := map[int64]string{1: "Livret", 2: "Checking"}

	payouts := projection.CalculateYieldPayouts(accounts, names)
	if len(payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(payouts))
	}
	// 10000 * 2% = 200 annually
	if payouts[0].Amount != 200 {
		t.Errorf("yearly payout: want 200, got %v", payouts[0].Amount)
	}
	if payouts[0].PayoutFrequency != "YEARLY" {
		t.Errorf("frequency: want YEARLY, got %q", payouts[0].PayoutFrequency)
	}
}

// TestCalculateYieldPayoutsRange covers the YieldType=="RANGE" branch in CalculateYieldPayouts.
func TestCalculateYieldPayoutsRange(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "PEA", Balance: 10000, IsYieldActive: true, YieldType: "RANGE",
			YieldMin: 2.0, YieldMax: 8.0, ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: "MONTHLY"},
		{ID: 2, Name: "Checking", Balance: 0},
	}
	names := map[int64]string{1: "PEA", 2: "Checking"}

	payouts := projection.CalculateYieldPayouts(accounts, names)
	if len(payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(payouts))
	}
	// avg rate = (2+8)/2 = 5%
	if payouts[0].Rate != 5.0 {
		t.Errorf("RANGE avg rate: want 5.0, got %v", payouts[0].Rate)
	}
}

// TestCalculateYieldPayoutsEmptyFrequency covers the freq=="" → "MONTHLY" default.
func TestCalculateYieldPayoutsEmptyFrequency(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 12000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 3.0, YieldMax: 3.0,
			ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: ""},
		{ID: 2, Name: "Checking", Balance: 0},
	}
	names := map[int64]string{1: "Savings", 2: "Checking"}

	payouts := projection.CalculateYieldPayouts(accounts, names)
	if len(payouts) != 1 {
		t.Fatalf("want 1 payout, got %d", len(payouts))
	}
	if payouts[0].PayoutFrequency != "MONTHLY" {
		t.Errorf("empty freq should default to MONTHLY, got %q", payouts[0].PayoutFrequency)
	}
}

func TestCalculateYieldPayoutsNoTarget(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 5000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 4.0, YieldMax: 4.0,
			ReinvestmentRate: 100, TargetAccountID: nil, PayoutFrequency: "MONTHLY"},
	}
	names := map[int64]string{1: "Savings"}

	payouts := projection.CalculateYieldPayouts(accounts, names)
	if len(payouts) != 0 {
		t.Errorf("want 0 payouts (100%% reinvested), got %d", len(payouts))
	}
}

// TestCalculateMonthlyYieldPayoutRange covers the YieldType=="RANGE" branch.
func TestCalculateMonthlyYieldPayoutRange(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "PEA", Balance: 12000, IsYieldActive: true, YieldType: "RANGE",
			YieldMin: 2.0, YieldMax: 10.0, ReinvestmentRate: 0, PayoutFrequency: "MONTHLY"},
	}
	// avg rate = (2+10)/2 = 6%, annual = 720, monthly = 60, payout (0% reinvested) = 60
	got := projection.CalculateMonthlyYieldPayout(accounts)
	if got != 60 {
		t.Errorf("monthly payout RANGE: want 60, got %v", got)
	}
}

// TestCalculateAnnualYieldPayoutRange covers the YieldType=="RANGE" branch.
func TestCalculateAnnualYieldPayoutRange(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "PEA", Balance: 10000, IsYieldActive: true, YieldType: "RANGE",
			YieldMin: 3.0, YieldMax: 7.0, ReinvestmentRate: 0, PayoutFrequency: "YEARLY"},
	}
	// avg rate = (3+7)/2 = 5%, annual = 500
	got := projection.CalculateAnnualYieldPayout(accounts)
	if got != 500 {
		t.Errorf("annual payout RANGE: want 500, got %v", got)
	}
}

func TestCalculateMonthlySummary(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Checking", Balance: 5000, IsYieldActive: false},
	}
	recurrings := []db.RecurringOperation{
		{ID: 1, UserID: 1, AccountID: 1, Amount: 3000, DayOfMonth: 1},
		{ID: 2, UserID: 1, AccountID: 1, Amount: -1500, DayOfMonth: 15},
	}
	summary := projection.CalculateMonthlySummary(recurrings, accounts)

	if summary.Income != 3000 {
		t.Errorf("income: want 3000, got %v", summary.Income)
	}
	if summary.Expenses != 1500 {
		t.Errorf("expenses: want 1500, got %v", summary.Expenses)
	}
	if summary.Net != 1500 {
		t.Errorf("net: want 1500, got %v", summary.Net)
	}
}

func TestCalculateMonthlySummaryWithYield(t *testing.T) {
	targetID := int64(1)
	accounts := []db.Account{
		{ID: 1, Name: "Checking", Balance: 12000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 3.0, YieldMax: 3.0,
			ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: "MONTHLY"},
	}
	summary := projection.CalculateMonthlySummary(nil, accounts)
	// Yield: 12000 * 3% / 12 = 30
	if summary.Yield != 30 {
		t.Errorf("yield: want 30, got %v", summary.Yield)
	}
	if summary.Income != 30 {
		t.Errorf("income (includes yield): want 30, got %v", summary.Income)
	}
}

// TestCalculateMonthlySummaryTransferToYield covers the yieldAccounts[*rec.ToAccountID] branch.
func TestCalculateMonthlySummaryTransferToYield(t *testing.T) {
	toID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "Checking", Balance: 5000, IsYieldActive: false},
		{ID: 2, Name: "Investment", Balance: 10000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 5.0, YieldMax: 5.0,
			ReinvestmentRate: 100, PayoutFrequency: "MONTHLY"},
	}
	recurrings := []db.RecurringOperation{
		{ID: 1, UserID: 1, AccountID: 1, ToAccountID: &toID, Amount: 500, DayOfMonth: 1},
	}
	summary := projection.CalculateMonthlySummary(recurrings, accounts)
	if summary.Transfers != 500 {
		t.Errorf("transfer to yield account: want 500, got %v", summary.Transfers)
	}
}

// TestCalculate_MonthlyPayoutToTarget couvre projection.go:149-151 — else branch :
// versement mensuel (non YEARLY) vers un TargetAccountID.
func TestCalculate_MonthlyPayoutToTarget(t *testing.T) {
	targetID := int64(2)
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 12000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 12, ReinvestmentRate: 0,
			PayoutFrequency: "MONTHLY", TargetAccountID: &targetID},
		{ID: 2, Name: "Current", Balance: 0},
	}
	result := projection.Calculate(accounts, nil, 1, "fr")
	if result.TotalInterests <= 0 {
		t.Errorf("TotalInterests: want >0 (monthly payout to target), got %v", result.TotalInterests)
	}
}

// TestCalculateMonthlySummaryWithAnnualYield covers CalculateAnnualYieldPayout via MonthlySummary.
func TestCalculateMonthlySummaryWithAnnualYield(t *testing.T) {
	targetID := int64(1)
	accounts := []db.Account{
		{ID: 1, Name: "Livret", Balance: 10000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 2.0, YieldMax: 2.0,
			ReinvestmentRate: 0, TargetAccountID: &targetID, PayoutFrequency: "YEARLY"},
	}
	summary := projection.CalculateMonthlySummary(nil, accounts)
	// 10000 * 2% = 200 annual
	if summary.YieldAnnual != 200 {
		t.Errorf("annual yield: want 200, got %v", summary.YieldAnnual)
	}
}

// ----- Benchmarks -----

// BenchmarkCalculate_5years benchmarks a 5-year projection with 10 accounts and recurring ops.
func BenchmarkCalculate_5years(b *testing.B) {
	targetID := int64(10)
	accounts := make([]db.Account, 10)
	for i := range accounts {
		accounts[i] = db.Account{
			ID: int64(i + 1), Name: "Account",
			Balance: float64((i + 1) * 10000), IsYieldActive: true,
			YieldType: "RANGE", YieldMin: 2.0, YieldMax: 8.0,
			ReinvestmentRate: 80, PayoutFrequency: "MONTHLY",
			TargetAccountID: &targetID,
		}
	}
	recurrings := []db.RecurringOperation{
		{ID: 1, UserID: 1, AccountID: 1, Amount: 2000, DayOfMonth: 1},
		{ID: 2, UserID: 1, AccountID: 1, Amount: -1200, DayOfMonth: 15},
		{ID: 3, UserID: 1, AccountID: 2, Amount: 500, DayOfMonth: 1},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		projection.Calculate(accounts, recurrings, 5, "fr")
	}
}

// BenchmarkCalculate_30years benchmarks a 30-year projection (worst case).
func BenchmarkCalculate_30years(b *testing.B) {
	targetID := int64(5)
	accounts := make([]db.Account, 5)
	for i := range accounts {
		accounts[i] = db.Account{
			ID: int64(i + 1), Name: "Account",
			Balance: float64((i + 1) * 50000), IsYieldActive: true,
			YieldType: "RANGE", YieldMin: 1.0, YieldMax: 10.0,
			ReinvestmentRate: 50, PayoutFrequency: "YEARLY",
			TargetAccountID: &targetID,
		}
	}
	recurrings := []db.RecurringOperation{
		{ID: 1, UserID: 1, AccountID: 1, Amount: 3000, DayOfMonth: 1},
		{ID: 2, UserID: 1, AccountID: 1, Amount: -2000, DayOfMonth: 15},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		projection.Calculate(accounts, recurrings, 30, "fr")
	}
}
