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
