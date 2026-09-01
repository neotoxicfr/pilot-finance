package handlers

import (
	"testing"

	"pilot-finance/internal/db"
	"pilot-finance/internal/projection"
)

// buildPieData ne retient que les comptes au solde strictement positif.
func TestBuildPieData(t *testing.T) {
	accounts := []db.Account{
		{ID: 1, Name: "Positif", Balance: 10000, Color: "#3b82f6"},
		{ID: 2, Name: "Zero", Balance: 0, Color: "#000000"},
		{ID: 3, Name: "Negatif", Balance: -500, Color: "#ff0000"},
	}

	pie := buildPieData(accounts)
	if len(pie) != 1 {
		t.Fatalf("want 1 pie entry (positive balance only), got %d", len(pie))
	}
	if pie[0]["name"] != "Positif" {
		t.Errorf("name: want Positif, got %v", pie[0]["name"])
	}
	if pie[0]["value"].(float64) != 100.0 {
		t.Errorf("value: want 100.0, got %v", pie[0]["value"])
	}
	if pie[0]["color"] != "#3b82f6" {
		t.Errorf("color: want #3b82f6, got %v", pie[0]["color"])
	}
}

// buildPieData renvoie une slice vide (non nil) quand aucun compte n'a un solde positif.
func TestBuildPieData_Empty(t *testing.T) {
	pie := buildPieData(nil)
	if pie == nil {
		t.Fatal("want non-nil empty slice")
	}
	if len(pie) != 0 {
		t.Errorf("want 0 entries, got %d", len(pie))
	}
}

// lastProjectionTotal retourne le TotalAvg de la dernière année.
func TestLastProjectionTotal(t *testing.T) {
	proj := []projection.YearData{
		{Year: 1, TotalAvg: 1000},
		{Year: 2, TotalAvg: 2500},
	}
	if got := lastProjectionTotal(proj); got != 2500 {
		t.Errorf("want 2500, got %v", got)
	}
}

// lastProjectionTotal retourne 0 pour une projection vide.
func TestLastProjectionTotal_Empty(t *testing.T) {
	if got := lastProjectionTotal(nil); got != 0 {
		t.Errorf("want 0 for empty projection, got %v", got)
	}
}

// --- Unité monétaire de la liste des opérations récurrentes (audit S-40) ---
//
// buildRecurringData fusionne DEUX sources dont les unités divergent en amont —
// projection.YieldPayout.Amount est un float64 en unité de devise, tandis que
// db.RecurringOperation.Amount est un int64 en centimes — sous une clé
// « Amount » unique que le template rend avec un seul appel. Tant que le
// formateur devinait l'unité d'après le type dynamique, l'écran était juste par
// coïncidence et une inversion serait passée inaperçue.
//
// Le test fige donc l'invariant : après construction, « Amount » est toujours un
// int64 en centimes, quelle que soit la branche.
func TestBuildRecurringData_AmountAlwaysCents(t *testing.T) {
	origDecrypt := hookDecryptStr
	defer func() { hookDecryptStr = origDecrypt }()
	hookDecryptStr = func(s string) (string, error) { return s, nil }

	target := int64(1)
	payouts := []projection.YieldPayout{
		// 12 345,67 EUR à 3 % l'an, versés mensuellement : 30,864175 EUR,
		// arrondis à 3086 centimes.
		{SourceAccountID: 2, SourceAccountName: "Livret A", TargetAccountID: &target,
			TargetAccountName: "Courant", Amount: 30.864175, Rate: 3, PayoutFrequency: "MONTHLY"},
	}
	recs := []db.RecurringOperation{
		{ID: 7, Description: "Loyer", Amount: -123456, DayOfMonth: 3, AccountID: 1, IsActive: true},
	}

	rows := buildRecurringData(payouts, recs, map[int64]string{1: "Courant", 2: "Livret A"}, "Intérêts")
	if len(rows) != 2 {
		t.Fatalf("want 2 lignes, got %d", len(rows))
	}

	for i, row := range rows {
		if _, ok := row["Amount"].(int64); !ok {
			t.Fatalf("ligne %d: Amount doit être un int64 (centimes), got %T", i, row["Amount"])
		}
	}
	if got := rows[0]["Amount"].(int64); got != 3086 {
		t.Errorf("versement d'intérêts: want 3086 centimes, got %d", got)
	}
	if got := rows[1]["Amount"].(int64); got != -123456 {
		t.Errorf("opération stockée: want -123456 centimes (inchangé), got %d", got)
	}
}

// unitsToCents arrondit au centime le plus proche : les intérêts périodiques
// sont des quotients (taux annuel ÷ 12) qu'une troncature raboterait
// systématiquement vers le bas.
func TestUnitsToCents(t *testing.T) {
	cases := []struct {
		in   float64
		want int64
	}{
		{0, 0},
		{30.864175, 3086},
		{30.865, 3087},
		{750, 75000},
		{-12.345, -1235},
		{0.004, 0},
		{0.005, 1},
	}
	for _, tc := range cases {
		if got := unitsToCents(tc.in); got != tc.want {
			t.Errorf("unitsToCents(%v): want %d, got %d", tc.in, tc.want, got)
		}
	}
}
