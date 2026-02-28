package projection_test

import (
	"testing"

	"pilot-finance/internal/db"
	"pilot-finance/internal/projection"
)

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
	// With no yield and no recurring, balance should stay constant
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
	// With 100% reinvestment at 5% annual, balance grows
	last := result.Projection[len(result.Projection)-1]
	if last.TotalAvg <= 10000 {
		t.Errorf("yield account should grow: got %v", last.TotalAvg)
	}
	// All three scenarios should be equal for FIXED rate
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
	// TotalMin < TotalAvg < TotalMax for RANGE
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
	// After 12 months of +1000/month, balance = 12000
	last := result.Projection[len(result.Projection)-1]
	if last.TotalAvg != 12000 {
		t.Errorf("12 months of 1000/month: want 12000, got %v", last.TotalAvg)
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

func TestCalculateYieldPayoutsNoTarget(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Savings", Balance: 5000, IsYieldActive: true,
			YieldType: "FIXED", YieldMin: 4.0, YieldMax: 4.0,
			ReinvestmentRate: 100, TargetAccountID: nil, PayoutFrequency: "MONTHLY"},
	}
	names := map[int64]string{1: "Savings"}

	// 100% reinvested and no target → no payout
	payouts := projection.CalculateYieldPayouts(accounts, names)
	if len(payouts) != 0 {
		t.Errorf("want 0 payouts (100%% reinvested), got %d", len(payouts))
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
		t.Errorf("income (should include yield): want 30, got %v", summary.Income)
	}
}
