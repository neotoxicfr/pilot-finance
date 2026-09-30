package db

import (
	"strings"
	"testing"
)

// PERF-07 : la suppression d'un compte ne doit plus scanner recurring_operations.
func TestRecurringAccountIndexes_UsedByFKChecks(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	var applied int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = '015_recurring_account_indexes'`).Scan(&applied); err != nil || applied != 1 {
		t.Fatalf("migration 015 non enregistrée (%d, %v)", applied, err)
	}
	for _, idx := range []string{"idx_recurring_account", "idx_recurring_to_account"} {
		var n int
		if err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("index %s absent (%d, %v)", idx, n, err)
		}
	}

	rows, err := DB.Query(`EXPLAIN QUERY PLAN DELETE FROM accounts WHERE user_id = 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if joined := strings.Join(plan, " | "); strings.Contains(joined, "SCAN recurring_operations") {
		t.Errorf("scan complet de recurring_operations : %s", joined)
	}
}
