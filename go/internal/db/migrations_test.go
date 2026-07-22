package db

import "testing"

func TestVersionedMigrations(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	rows, err := DB.Query(`SELECT name FROM schema_migrations ORDER BY name`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		rows.Scan(&name)
		applied[name] = true
	}

	expected := []string{
		"000_base_schema",
		"001_backup_eligible",
		"002_user_language",
		"003_user_currency",
		"004_yield_frequency",
		"005_payout_frequency",
		"006_indexes",
		"007_audit_log",
		"008_encrypt_account_fields",
		"009_encrypt_recurring_amount",
	}

	for _, want := range expected {
		if !applied[want] {
			t.Errorf("migration %q not recorded in schema_migrations", want)
		}
	}
}

func TestMigrationIdempotency(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	var count1 int
	DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count1)
	if count1 == 0 {
		t.Fatal("no migrations recorded after Init")
	}

	// Second run — all migrations already applied, count must not grow.
	if err := runMigrations(""); err != nil {
		t.Fatalf("runMigrations (second run): %v", err)
	}

	var count2 int
	DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count2)
	if count1 != count2 {
		t.Errorf("not idempotent: first=%d second=%d", count1, count2)
	}
}

func TestSchemaTablesExist(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	tables := []string{"users", "accounts", "recurring_operations", "authenticators", "audit_log"}
	for _, table := range tables {
		var name string
		err := DB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}
