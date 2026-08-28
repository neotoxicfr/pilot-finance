package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"pilot-finance/internal/crypto"
)

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

// --- Bases héritées de la version Node : ordre physique des colonnes différent ---

// setupLegacySchemaDB crée une base au schéma LEGACY (ordre physique des
// colonnes de la version Node, colonnes des migrations 001/004/005 ajoutées en
// FIN de table par ALTER) et branche la variable globale DB dessus, SANS table
// schema_migrations : toute la chaîne 000→013 va donc s'exécuter, exactement
// comme au premier démarrage de la version Go sur une base héritée.
//
// Reproduit le scénario de l'audit S-10 : avec `INSERT ... SELECT *`, le
// rebuild de la migration 013 écrivait les valeurs dans les mauvaises colonnes
// (NOT NULL / FOREIGN KEY constraint failed → boot loop permanent).
func setupLegacySchemaDB(t *testing.T) (string, func()) {
	t.Helper()
	crypto.ResetForTest()
	if err := crypto.Init(testEncKey, testBlindKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}

	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite", path+"?_txlock=immediate&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	prev := DB
	DB = conn
	cleanup := func() {
		conn.Close()
		DB = prev
	}

	stmts := []string{
		// users : colonnes complètes (le sujet du test est l'ordre des tables
		// reconstruites par 013, pas les colonnes manquantes du schéma v1).
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email_encrypted TEXT NOT NULL,
			email_blind_index TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			created_at INTEGER,
			email_verified INTEGER DEFAULT 0,
			verification_token TEXT,
			reset_token TEXT,
			reset_token_expiry INTEGER,
			mfa_enabled INTEGER DEFAULT 0,
			mfa_secret TEXT,
			failed_login_attempts INTEGER DEFAULT 0,
			lock_until INTEGER,
			session_version INTEGER DEFAULT 1,
			language TEXT NOT NULL DEFAULT 'fr',
			currency TEXT NOT NULL DEFAULT 'EUR'
		)`,
		// accounts : yield_frequency et payout_frequency EN FIN de table
		// (ajoutées par les ALTER 004/005), et non à leur position canonique.
		`CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			balance TEXT,
			color TEXT,
			position INTEGER DEFAULT 0,
			updated_at INTEGER,
			is_yield_active INTEGER DEFAULT 0,
			yield_type TEXT DEFAULT 'FIXED',
			yield_min TEXT DEFAULT '0',
			yield_max TEXT DEFAULT '0',
			last_yield_date INTEGER,
			reinvestment_rate TEXT DEFAULT '100',
			target_account_id INTEGER,
			yield_frequency TEXT NOT NULL DEFAULT 'MONTHLY',
			payout_frequency TEXT NOT NULL DEFAULT 'MONTHLY'
		)`,
		`CREATE TABLE recurring_operations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			to_account_id INTEGER,
			amount TEXT,
			description TEXT,
			day_of_month INTEGER DEFAULT 1,
			last_run_date INTEGER,
			is_active INTEGER DEFAULT 1
		)`,
		// authenticators : backup_eligible EN FIN de table (ALTER 001).
		`CREATE TABLE authenticators (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			credential_id TEXT NOT NULL UNIQUE,
			credential_public_key TEXT NOT NULL,
			counter INTEGER DEFAULT 0,
			credential_device_type TEXT,
			credential_backed_up INTEGER DEFAULT 0,
			transports TEXT,
			user_id INTEGER NOT NULL,
			name TEXT,
			backup_eligible INTEGER DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if _, err := conn.Exec(s); err != nil {
			cleanup()
			t.Fatalf("legacy DDL: %v", err)
		}
	}
	return path, cleanup
}

// seedLegacyData insère un jeu de données EN CLAIR (comme la version Node les
// stockait) et retourne l'id de l'utilisateur créé.
func seedLegacyData(t *testing.T) int64 {
	t.Helper()
	emailEnc, err := crypto.Encrypt("legacy@example.com")
	if err != nil {
		t.Fatalf("crypto.Encrypt: %v", err)
	}
	res, err := DB.Exec(`INSERT INTO users (email_encrypted, email_blind_index, password, role, created_at, email_verified, session_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		emailEnc, crypto.ComputeBlindIndex("legacy@example.com"), "hash", "ADMIN", 1700000000, 1, 1)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	accRes, err := DB.Exec(`INSERT INTO accounts
		(user_id, name, balance, color, position, updated_at, is_yield_active, yield_type,
		 yield_min, yield_max, last_yield_date, reinvestment_rate, target_account_id,
		 yield_frequency, payout_frequency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, "Livret A", "1500.50", "#ff0000", 0, 1700000000, 1, "FIXED",
		"2", "3", nil, "100", nil, "MONTHLY", "YEARLY")
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID, err := accRes.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	if _, err := DB.Exec(`INSERT INTO recurring_operations
		(user_id, account_id, to_account_id, amount, description, day_of_month, last_run_date, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, accID, nil, "250.75", "Loyer", 5, nil, 1); err != nil {
		t.Fatalf("insert recurring: %v", err)
	}

	if _, err := DB.Exec(`INSERT INTO authenticators
		(credential_id, credential_public_key, counter, credential_device_type,
		 credential_backed_up, transports, user_id, name, backup_eligible)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"cred-legacy-1", "public-key", 7, "platform", 1, "internal,hybrid", userID, "MacBook", 1); err != nil {
		t.Fatalf("insert authenticator: %v", err)
	}
	return userID
}

// TestMigrations_LegacyColumnOrder est le test de non-régression de l'audit
// S-10 : la chaîne complète de migrations doit réussir sur une base dont
// l'ordre physique des colonnes diffère, et chaque valeur doit atterrir dans SA
// colonne (pas dans la colonne voisine).
func TestMigrations_LegacyColumnOrder(t *testing.T) {
	path, cleanup := setupLegacySchemaDB(t)
	defer cleanup()

	userID := seedLegacyData(t)

	if err := runMigrations(path); err != nil {
		t.Fatalf("runMigrations sur schéma legacy: %v", err)
	}

	accounts, err := GetAccountsByUserID(userID)
	if err != nil {
		t.Fatalf("GetAccountsByUserID: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("want 1 account, got %d", len(accounts))
	}
	acc := accounts[0]
	if acc.Name != "Livret A" {
		t.Errorf("name = %q, want %q", acc.Name, "Livret A")
	}
	if acc.Balance != 150050 {
		t.Errorf("balance = %d cents, want 150050", acc.Balance)
	}
	if acc.YieldMin != 2 || acc.YieldMax != 3 {
		t.Errorf("yield_min/max = %v/%v, want 2/3", acc.YieldMin, acc.YieldMax)
	}
	if acc.ReinvestmentRate != 100 {
		t.Errorf("reinvestment_rate = %d, want 100", acc.ReinvestmentRate)
	}
	// La valeur discriminante : en INSERT positionnel, payout_frequency
	// recevait la valeur d'une autre colonne (ou NULL → NOT NULL failed).
	if acc.PayoutFrequency != "YEARLY" {
		t.Errorf("payout_frequency = %q, want %q", acc.PayoutFrequency, "YEARLY")
	}
	if acc.YieldFrequency != "MONTHLY" {
		t.Errorf("yield_frequency = %q, want %q", acc.YieldFrequency, "MONTHLY")
	}
	if acc.Color != "#ff0000" || acc.Position != 0 || !acc.IsYieldActive {
		t.Errorf("colonnes décalées: color=%q position=%d isYieldActive=%v", acc.Color, acc.Position, acc.IsYieldActive)
	}

	ops, err := GetRecurringByUserID(userID)
	if err != nil {
		t.Fatalf("GetRecurringByUserID: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("want 1 recurring op, got %d", len(ops))
	}
	if ops[0].Amount != 25075 {
		t.Errorf("amount = %d cents, want 25075", ops[0].Amount)
	}
	if ops[0].Description != "Loyer" || ops[0].DayOfMonth != 5 {
		t.Errorf("recurring décalée: description=%q day=%d", ops[0].Description, ops[0].DayOfMonth)
	}

	// authenticators : l'ordre legacy place backup_eligible en dernier, donc un
	// INSERT positionnel décalait transports → backup_eligible, user_id →
	// transports et name → user_id (FOREIGN KEY constraint failed).
	var credID, transports, name string
	var uid int64
	var backupEligible int
	if err := DB.QueryRow(`SELECT credential_id, transports, user_id, name, backup_eligible
		FROM authenticators WHERE credential_id = ?`, "cred-legacy-1").
		Scan(&credID, &transports, &uid, &name, &backupEligible); err != nil {
		t.Fatalf("select authenticator: %v", err)
	}
	if transports != "internal,hybrid" || uid != userID || name != "MacBook" || backupEligible != 1 {
		t.Errorf("authenticator décalé: transports=%q user_id=%d name=%q backup_eligible=%d",
			transports, uid, name, backupEligible)
	}

	// Les clés étrangères de la migration 013 doivent bien être en place.
	var accountsDDL string
	if err := DB.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='accounts'`).Scan(&accountsDDL); err != nil {
		t.Fatalf("select accounts DDL: %v", err)
	}
	if !strings.Contains(accountsDDL, "REFERENCES users(id)") {
		t.Errorf("accounts sans FK vers users après 013:\n%s", accountsDDL)
	}
}

// TestMigration008_BackupIsReplayable est le test de non-régression de l'audit
// S-11 : le backup de la migration 008 utilisait un nom FIXE (dbPath+".bak")
// que rien ne supprimait jamais, alors que VACUUM INTO refuse d'écrire sur un
// fichier existant. Tout rejeu de 008 (crash entre le succès et l'INSERT dans
// schema_migrations, ou suppression manuelle de la base) échouait alors
// définitivement : « migration 008: backup impossible (arrêt par sécurité) ».
func TestMigration008_BackupIsReplayable(t *testing.T) {
	path, cleanup := setupLegacySchemaDB(t)
	defer cleanup()

	seedLegacyData(t)

	if err := runMigrations(path); err != nil {
		t.Fatalf("runMigrations (1er passage): %v", err)
	}
	backups, err := filepath.Glob(path + ".pre008.*.bak")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(backups) == 0 {
		t.Fatalf("aucun backup pré-008 créé alors que accounts est non vide")
	}

	// Simule le crash entre le succès de 008 et son enregistrement.
	if _, err := DB.Exec(`DELETE FROM schema_migrations WHERE name = ?`, "008_encrypt_account_fields"); err != nil {
		t.Fatalf("delete schema_migrations: %v", err)
	}

	// Avant correctif : échec fatal sur « output file already exists ».
	if err := runMigrations(path); err != nil {
		t.Fatalf("runMigrations (rejeu de 008): %v", err)
	}
}

// TestMigration008_NoBackupOnEmptyDatabase : sur une base neuve, la table
// accounts est vide, donc il n'y a rien à convertir et aucun fichier de backup
// contenant des montants en clair ne doit être laissé à côté de la base
// (audit S-11).
func TestMigration008_NoBackupOnEmptyDatabase(t *testing.T) {
	crypto.ResetForTest()
	ResetForTest()
	if err := crypto.Init(testEncKey, testBlindKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
	path := filepath.Join(t.TempDir(), "fresh.db")
	if err := Init(Config{Path: path}); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	defer Close()

	matches, err := filepath.Glob(path + ".pre008.*.bak")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("backup 008 créé sur une base vide: %v", matches)
	}
	if legacy, _ := filepath.Glob(path + ".bak"); len(legacy) != 0 {
		t.Errorf("ancien backup à nom fixe recréé: %v", legacy)
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
