package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pilot-finance/internal/crypto"

	_ "modernc.org/sqlite"
)

var DB *sql.DB
var bgDone chan struct{}
var bgMu sync.Mutex
var bgWg sync.WaitGroup
var dbInitOnce sync.Once
var dbInitErr error

// Config contient la configuration de la base de données
type Config struct {
	Path string
}

// ResetForTest resets the db package state so Init can be called again.
// ONLY for use in tests.
func ResetForTest() {
	dbInitOnce = sync.Once{}
	dbInitErr = nil
}

// Init initialise la connexion à la base de données.
// Protected by sync.Once to prevent double-init and goroutine leaks.
func Init(cfg Config) error {
	dbInitOnce.Do(func() {
		dbInitErr = initDB(cfg)
	})
	return dbInitErr
}

// initDB performs the actual database initialization (called via sync.Once).
func initDB(cfg Config) error {
	// S'assurer que le dossier existe
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("création dossier DB: %w", err)
	}

	// PRAGMAs via DSN ensure they apply to every connection in the pool
	// (foreign_keys, busy_timeout, cache_size, temp_store are per-connection).
	dsn := cfg.Path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(10000)&_pragma=temp_store(MEMORY)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=mmap_size(268435456)"

	var err error
	DB, err = sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("ouverture DB: %w", err)
	}

	// Pool de connexions
	DB.SetMaxOpenConns(10)
	DB.SetMaxIdleConns(10)
	DB.SetConnMaxLifetime(time.Hour)

	// Test de connexion
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("ping DB: %w", err)
	}

	// Migrations automatiques versionnées
	if err := runMigrations(cfg.Path); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// WAL checkpoint au démarrage
	if _, err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		slog.Warn("WAL checkpoint", "err", err)
	}

	// Backup rotatif au démarrage (max 3 fichiers, même volume)
	rotateBackups(cfg.Path)

	// VACUUM + backup périodique toutes les 24h
	dbPath := cfg.Path
	bgMu.Lock()
	bgDone = make(chan struct{})
	done := bgDone
	bgMu.Unlock()
	bgWg.Add(1)
	go func() {
		defer bgWg.Done()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
					slog.Warn("WAL checkpoint périodique", "err", err)
				}
				rotateBackups(dbPath)
			case <-done:
				return
			}
		}
	}()

	slog.Info("base de données connectée", "path", cfg.Path)
	return nil
}

// rotateBackups crée un backup et garde les 3 derniers (.backup.1 = plus récent)
func rotateBackups(dbPath string) {
	// Rotation : .backup.3 supprimé, .backup.2 → .3, .backup.1 → .2, nouveau → .1
	os.Remove(dbPath + ".backup.3")
	os.Rename(dbPath+".backup.2", dbPath+".backup.3")
	os.Rename(dbPath+".backup.1", dbPath+".backup.2")

	backupPath := dbPath + ".backup.1"
	if _, err := DB.Exec("VACUUM INTO ?", backupPath); err != nil {
		slog.Warn("backup automatique", "err", err)
	} else {
		slog.Info("backup créé", "path", backupPath)
	}
}

// migration représente une migration de schéma nommée et idempotente.
// SQL : instruction DDL unique. Run : migration Go complexe (chiffrement in-place, etc.)
type migration struct {
	Name string
	SQL  string
	Run  func(*sql.DB) error
}

// runMigrations exécute les migrations de schéma de manière versionnée.
// Chaque migration est enregistrée dans schema_migrations ; les migrations déjà
// appliquées sont ignorées. dbPath est utilisé pour le backup avant migrations Go.
// Retourne une erreur fatale si l'état des migrations ne peut être lu ou si une
// migration échoue avec une erreur non idempotente : l'appelant doit alors
// interrompre le démarrage plutôt que de servir une base au schéma incertain.
func runMigrations(dbPath string) error {
	if _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("schema_migrations: création impossible: %w", err)
	}

	migrations := []migration{
		{Name: "000_base_schema", Run: func(d *sql.DB) error {
			for _, stmt := range []string{
				`CREATE TABLE IF NOT EXISTS users (
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
				`CREATE TABLE IF NOT EXISTS accounts (
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
					yield_frequency TEXT NOT NULL DEFAULT 'MONTHLY',
					payout_frequency TEXT NOT NULL DEFAULT 'MONTHLY',
					last_yield_date INTEGER,
					reinvestment_rate TEXT DEFAULT '100',
					target_account_id INTEGER
				)`,
				`CREATE TABLE IF NOT EXISTS recurring_operations (
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
				`CREATE TABLE IF NOT EXISTS authenticators (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					credential_id TEXT NOT NULL UNIQUE,
					credential_public_key TEXT NOT NULL,
					counter INTEGER DEFAULT 0,
					credential_device_type TEXT,
					credential_backed_up INTEGER DEFAULT 0,
					backup_eligible INTEGER DEFAULT 0,
					transports TEXT,
					user_id INTEGER NOT NULL,
					name TEXT
				)`,
			} {
				if _, err := d.Exec(stmt); err != nil {
					return err
				}
			}
			return nil
		}},
		{Name: "001_backup_eligible", SQL: `ALTER TABLE authenticators ADD COLUMN backup_eligible INTEGER DEFAULT 0`},
		{Name: "002_user_language", SQL: `ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'fr'`},
		{Name: "003_user_currency", SQL: `ALTER TABLE users ADD COLUMN currency TEXT NOT NULL DEFAULT 'EUR'`},
		{Name: "004_yield_frequency", SQL: `ALTER TABLE accounts ADD COLUMN yield_frequency TEXT NOT NULL DEFAULT 'MONTHLY'`},
		{Name: "005_payout_frequency", SQL: `ALTER TABLE accounts ADD COLUMN payout_frequency TEXT NOT NULL DEFAULT 'MONTHLY'`},
		{Name: "006_indexes", Run: func(d *sql.DB) error {
			for _, idx := range []string{
				`CREATE INDEX IF NOT EXISTS idx_accounts_user_id       ON accounts(user_id, position)`,
				`CREATE INDEX IF NOT EXISTS idx_recurring_user_id      ON recurring_operations(user_id, day_of_month)`,
				`CREATE INDEX IF NOT EXISTS idx_authenticators_user_id ON authenticators(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_authenticators_cred_id ON authenticators(credential_id)`,
				`CREATE INDEX IF NOT EXISTS idx_users_blind            ON users(email_blind_index)`,
				`CREATE INDEX IF NOT EXISTS idx_users_reset_token      ON users(reset_token)`,
				`CREATE INDEX IF NOT EXISTS idx_users_verif_token      ON users(verification_token)`,
			} {
				if _, err := d.Exec(idx); err != nil {
					return err
				}
			}
			return nil
		}},
		{Name: "007_audit_log", SQL: `CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			action TEXT NOT NULL,
			ip TEXT,
			user_agent TEXT,
			created_at INTEGER NOT NULL
		)`},
		{Name: "008_encrypt_account_fields", Run: func(d *sql.DB) error {
			// Backup avant chiffrement via VACUUM INTO (copie propre incluant le WAL)
			backupPath := dbPath + ".bak"
			if _, err := d.Exec("VACUUM INTO ?", backupPath); err != nil {
				return fmt.Errorf("migration 008: backup impossible (arrêt par sécurité): %w", err)
			}
			slog.Info("migration 008: backup créé", "path", backupPath)

			rows, err := d.Query(`SELECT id, balance, yield_min, yield_max, reinvestment_rate FROM accounts`)
			if err != nil {
				return err
			}
			defer rows.Close()

			type accRow struct {
				id          int64
				balance     string
				yieldMin    string
				yieldMax    string
				reinvestRate string
			}
			var toUpdate []accRow
			for rows.Next() {
				var r accRow
				if err := rows.Scan(&r.id, &r.balance, &r.yieldMin, &r.yieldMax, &r.reinvestRate); err != nil {
					return err
				}
				toUpdate = append(toUpdate, r)
			}
			rows.Close()

			for _, r := range toUpdate {
				balEnc := encryptIfPlain(r.balance, func(s string) (string, error) {
					f, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return s, nil
					}
					return crypto.EncryptFloat(f)
				})
				ymEnc := encryptIfPlain(r.yieldMin, func(s string) (string, error) {
					f, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return s, nil
					}
					return crypto.EncryptFloat(f)
				})
				yxEnc := encryptIfPlain(r.yieldMax, func(s string) (string, error) {
					f, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return s, nil
					}
					return crypto.EncryptFloat(f)
				})
				rrEnc := encryptIfPlain(r.reinvestRate, func(s string) (string, error) {
					n, err := strconv.Atoi(s)
					if err != nil {
						return s, nil
					}
					return crypto.EncryptInt(n)
				})
				if _, err := d.Exec(`UPDATE accounts SET balance=?, yield_min=?, yield_max=?, reinvestment_rate=? WHERE id=?`,
					balEnc, ymEnc, yxEnc, rrEnc, r.id); err != nil {
					return fmt.Errorf("update account id=%d: %w", r.id, err)
				}
			}
			return nil
		}},
		{Name: "009_encrypt_recurring_amount", Run: func(d *sql.DB) error {
			rows, err := d.Query(`SELECT id, amount FROM recurring_operations`)
			if err != nil {
				return err
			}
			defer rows.Close()

			type recRow struct {
				id     int64
				amount string
			}
			var toUpdate []recRow
			for rows.Next() {
				var r recRow
				if err := rows.Scan(&r.id, &r.amount); err != nil {
					return err
				}
				toUpdate = append(toUpdate, r)
			}
			rows.Close()

			for _, r := range toUpdate {
				amtEnc := encryptIfPlain(r.amount, func(s string) (string, error) {
					f, err := strconv.ParseFloat(s, 64)
					if err != nil {
						return s, nil
					}
					return crypto.EncryptFloat(f)
				})
				if _, err := d.Exec(`UPDATE recurring_operations SET amount=? WHERE id=?`, amtEnc, r.id); err != nil {
					return fmt.Errorf("update recurring id=%d: %w", r.id, err)
				}
			}
			return nil
		}},
		{Name: "010_audit_log_indexes", Run: func(d *sql.DB) error {
			for _, idx := range []string{
				`CREATE INDEX IF NOT EXISTS idx_audit_user_id ON audit_log(user_id, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_log(created_at DESC)`,
			} {
				if _, err := d.Exec(idx); err != nil {
					return err
				}
			}
			return nil
		}},
		{Name: "011_encrypt_audit_ip_ua", Run: func(d *sql.DB) error {
			rows, err := d.Query(`SELECT id, COALESCE(ip, ''), COALESCE(user_agent, '') FROM audit_log`)
			if err != nil {
				return err
			}
			defer rows.Close()

			type auditRow struct {
				id        int64
				ip        string
				userAgent string
			}
			var toUpdate []auditRow
			for rows.Next() {
				var r auditRow
				if err := rows.Scan(&r.id, &r.ip, &r.userAgent); err != nil {
					return err
				}
				toUpdate = append(toUpdate, r)
			}
			rows.Close()

			for _, r := range toUpdate {
				ipEnc := encryptIfPlain(r.ip, func(s string) (string, error) {
					return crypto.Encrypt(s)
				})
				uaEnc := encryptIfPlain(r.userAgent, func(s string) (string, error) {
					return crypto.Encrypt(s)
				})
				if _, err := d.Exec(`UPDATE audit_log SET ip=?, user_agent=? WHERE id=?`,
					ipEnc, uaEnc, r.id); err != nil {
					return fmt.Errorf("update audit id=%d: %w", r.id, err)
				}
			}
			return nil
		}},
		{Name: "012_fix_audit_ua_encryption", Run: func(d *sql.DB) error {
			// Migration 011 utilisait l'ancien encryptIfPlain qui considérait les user agents
			// contenant ":" comme déjà chiffrés. Cette migration re-passe avec la détection corrigée.
			rows, err := d.Query(`SELECT id, COALESCE(ip, ''), COALESCE(user_agent, '') FROM audit_log`)
			if err != nil {
				return err
			}
			defer rows.Close()

			type auditRow struct {
				id        int64
				ip        string
				userAgent string
			}
			var toUpdate []auditRow
			for rows.Next() {
				var r auditRow
				if err := rows.Scan(&r.id, &r.ip, &r.userAgent); err != nil {
					return err
				}
				toUpdate = append(toUpdate, r)
			}
			rows.Close()

			for _, r := range toUpdate {
				ipEnc := encryptIfPlain(r.ip, func(s string) (string, error) {
					return crypto.Encrypt(s)
				})
				uaEnc := encryptIfPlain(r.userAgent, func(s string) (string, error) {
					return crypto.Encrypt(s)
				})
				if ipEnc != r.ip || uaEnc != r.userAgent {
					if _, err := d.Exec(`UPDATE audit_log SET ip=?, user_agent=? WHERE id=?`,
						ipEnc, uaEnc, r.id); err != nil {
						return fmt.Errorf("update audit id=%d: %w", r.id, err)
					}
				}
			}
			return nil
		}},
		{Name: "013_add_foreign_keys", Run: func(d *sql.DB) error {
			stmts := []string{
				`CREATE TABLE accounts_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					balance TEXT,
					color TEXT,
					position INTEGER DEFAULT 0,
					updated_at INTEGER,
					is_yield_active INTEGER DEFAULT 0,
					yield_type TEXT DEFAULT 'FIXED',
					yield_min TEXT DEFAULT '0',
					yield_max TEXT DEFAULT '0',
					yield_frequency TEXT NOT NULL DEFAULT 'MONTHLY',
					payout_frequency TEXT NOT NULL DEFAULT 'MONTHLY',
					last_yield_date INTEGER,
					reinvestment_rate TEXT DEFAULT '100',
					target_account_id INTEGER REFERENCES accounts_new(id) ON DELETE SET NULL
				)`,
				`INSERT INTO accounts_new SELECT * FROM accounts`,
				`DROP TABLE accounts`,
				`ALTER TABLE accounts_new RENAME TO accounts`,
				`CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts(user_id, position)`,

				`CREATE TABLE recurring_operations_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
					to_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
					amount TEXT,
					description TEXT,
					day_of_month INTEGER DEFAULT 1,
					last_run_date INTEGER,
					is_active INTEGER DEFAULT 1
				)`,
				`INSERT INTO recurring_operations_new SELECT * FROM recurring_operations`,
				`DROP TABLE recurring_operations`,
				`ALTER TABLE recurring_operations_new RENAME TO recurring_operations`,
				`CREATE INDEX IF NOT EXISTS idx_recurring_user_id ON recurring_operations(user_id, day_of_month)`,

				`CREATE TABLE authenticators_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					credential_id TEXT NOT NULL UNIQUE,
					credential_public_key TEXT NOT NULL,
					counter INTEGER DEFAULT 0,
					credential_device_type TEXT,
					credential_backed_up INTEGER DEFAULT 0,
					backup_eligible INTEGER DEFAULT 0,
					transports TEXT,
					user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					name TEXT
				)`,
				`INSERT INTO authenticators_new SELECT * FROM authenticators`,
				`DROP TABLE authenticators`,
				`ALTER TABLE authenticators_new RENAME TO authenticators`,
				`CREATE INDEX IF NOT EXISTS idx_authenticators_user_id ON authenticators(user_id)`,
				`CREATE INDEX IF NOT EXISTS idx_authenticators_cred_id ON authenticators(credential_id)`,
			}
			for _, stmt := range stmts {
				if _, err := d.Exec(stmt); err != nil {
					return fmt.Errorf("013_add_foreign_keys: %w", err)
				}
			}
			return nil
		}},
	}

	// Charger en une seule requête la liste des migrations déjà appliquées
	// (perf M3 : évite N round-trips DB au démarrage). Un échec de lecture est
	// fatal : on ne doit pas repartir d'un appliedSet vide et risquer de
	// re-rejouer des migrations destructrices (013 DROP/RENAME) sur une base
	// déjà migrée.
	appliedSet := make(map[string]bool)
	rows, err := DB.Query(`SELECT name FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("migration: lecture schema_migrations échouée: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("migration: scan schema_migrations échoué: %w", err)
		}
		appliedSet[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("migration: itération schema_migrations échouée: %w", err)
	}
	rows.Close()

	for _, m := range migrations {
		if appliedSet[m.Name] {
			continue
		}

		var applyErr error
		if m.Run != nil {
			applyErr = m.Run(DB)
		} else {
			_, applyErr = DB.Exec(m.SQL)
		}

		if applyErr != nil {
			// Les erreurs idempotentes ("already exists" / "duplicate column name")
			// signifient que l'objet existe déjà : on enregistre la migration comme
			// appliquée. Toute autre erreur est fatale — un enregistrement silencieux
			// laisserait la base dans un état incohérent (cf. migration 013 partielle).
			msg := applyErr.Error()
			if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "duplicate column name") {
				return fmt.Errorf("migration %q échouée: %w", m.Name, applyErr)
			}
		}

		if _, err := DB.Exec(`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.Name, time.Now().Unix()); err != nil {
			return fmt.Errorf("schema_migrations: enregistrement de %q impossible: %w", m.Name, err)
		}
		slog.Info("migration appliquée", "name", m.Name)
	}

	// Auto-test : vérifier que toutes les migrations attendues sont bien appliquées
	verifyMigrations(migrations)
	return nil
}

// verifyMigrations vérifie que toutes les migrations attendues sont présentes dans schema_migrations.
func verifyMigrations(expected []migration) {
	rows, err := DB.Query(`SELECT name FROM schema_migrations`)
	if err != nil {
		slog.Error("vérification migrations: impossible de lire schema_migrations", "err", err)
		return
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		applied[name] = true
	}

	var missing []string
	for _, m := range expected {
		if !applied[m.Name] {
			missing = append(missing, m.Name)
		}
	}

	if len(missing) > 0 {
		slog.Error("SCHEMA INCOHÉRENT: migrations manquantes", "missing", missing, "applied", len(applied), "expected", len(expected))
	} else {
		slog.Info("schéma DB vérifié", "migrations", len(applied))
	}
}

// isEncrypted vérifie si une valeur est au format chiffré AES-256-GCM (IV_HEX:TAG_HEX:CIPHERTEXT_HEX).
// Exactement 3 parties hexadécimales, IV de 24 chars hex (12 bytes), TAG de 32 chars hex (16 bytes).
func isEncrypted(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return false
	}
	// IV = 12 bytes = 24 hex chars
	if len(parts[0]) != 24 {
		return false
	}
	// TAG = 16 bytes = 32 hex chars
	if len(parts[1]) != 32 {
		return false
	}
	return true
}

// encryptIfPlain chiffre une valeur si elle n'est pas déjà au format AES-256-GCM.
// fn reçoit la valeur brute et retourne la valeur chiffrée.
func encryptIfPlain(raw string, fn func(string) (string, error)) string {
	if isEncrypted(raw) {
		return raw // déjà chiffré
	}
	enc, err := fn(raw)
	if err != nil {
		slog.Warn("encryptIfPlain: encryption failed, keeping plaintext", "err", err, "len", len(raw))
		return raw
	}
	return enc
}

// Close ferme la connexion à la base de données et arrête la goroutine de maintenance.
func Close() error {
	bgMu.Lock()
	ch := bgDone
	bgDone = nil
	bgMu.Unlock()
	if ch != nil {
		close(ch)
		bgWg.Wait()
	}
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// scanUser scanne une ligne SQL dans un User (helper partagé)
func scanUser(row *sql.Row) (*User, error) {
	var user User
	var createdAt, lockUntil, resetTokenExpiry sql.NullInt64
	var verificationToken, resetToken, mfaSecret sql.NullString

	err := row.Scan(
		&user.ID, &user.EmailEncrypted, &user.EmailBlindIndex, &user.Password, &user.Role,
		&createdAt, &user.EmailVerified, &verificationToken, &resetToken,
		&resetTokenExpiry, &user.MFAEnabled, &mfaSecret, &user.FailedLoginAttempts,
		&lockUntil, &user.SessionVersion, &user.Language, &user.Currency,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		user.CreatedAt = time.Unix(createdAt.Int64, 0)
	}
	if lockUntil.Valid {
		t := time.Unix(lockUntil.Int64, 0)
		user.LockUntil = &t
	}
	if resetTokenExpiry.Valid {
		t := time.Unix(resetTokenExpiry.Int64, 0)
		user.ResetTokenExpiry = &t
	}
	if verificationToken.Valid {
		user.VerificationToken = &verificationToken.String
	}
	if resetToken.Valid {
		user.ResetToken = &resetToken.String
	}
	if mfaSecret.Valid {
		user.MFASecret = &mfaSecret.String
	}
	return &user, nil
}

const userSelectCols = `
	SELECT id, email_encrypted, email_blind_index, password, role,
	       created_at, email_verified, verification_token, reset_token,
	       reset_token_expiry, mfa_enabled, mfa_secret, failed_login_attempts,
	       lock_until, session_version, language, currency
	FROM users`

// GetUserByBlindIndex récupère un utilisateur par son email blind index
func GetUserByBlindIndex(blindIndex string) (*User, error) {
	return scanUser(DB.QueryRow(userSelectCols+` WHERE email_blind_index = ?`, blindIndex))
}

// GetUserByID récupère un utilisateur par son ID
func GetUserByID(id int64) (*User, error) {
	return scanUser(DB.QueryRow(userSelectCols+` WHERE id = ?`, id))
}


// GetUserAuthData récupère session_version, email chiffré et flag de vérification email
// en une seule requête. Utilisé par RequireAuth pour peupler le contexte sans stocker
// l'email dans le JWT.
func GetUserAuthData(id int64) (sessionVersion int, emailEncrypted string, emailVerified bool, err error) {
	err = DB.QueryRow(`SELECT session_version, email_encrypted, email_verified FROM users WHERE id = ?`, id).
		Scan(&sessionVersion, &emailEncrypted, &emailVerified)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	return
}

// GetAccountsByUserID récupère tous les comptes d'un utilisateur.
// Le déchiffrement des champs numériques est effectué directement dans la boucle
// de scan (le déchiffrement AES-GCM est négligeable face à l'I/O DB).
func GetAccountsByUserID(userID int64) ([]Account, error) {
	rows, err := DB.Query(`
		SELECT id, user_id, name, balance, COALESCE(color, ''), position, updated_at,
		       is_yield_active, yield_type, yield_min, yield_max,
		       yield_frequency, payout_frequency, last_yield_date,
		       reinvestment_rate, target_account_id
		FROM accounts WHERE user_id = ? ORDER BY position ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var acc Account
		var balanceRaw, yieldMinRaw, yieldMaxRaw, reinvestRaw string
		var updatedAt, lastYieldDate sql.NullInt64
		var targetAccountID sql.NullInt64
		var yieldType, yieldFreq, payoutFreq sql.NullString

		if err := rows.Scan(
			&acc.ID, &acc.UserID, &acc.Name, &balanceRaw, &acc.Color, &acc.Position,
			&updatedAt, &acc.IsYieldActive, &yieldType, &yieldMinRaw, &yieldMaxRaw,
			&yieldFreq, &payoutFreq, &lastYieldDate, &reinvestRaw, &targetAccountID,
		); err != nil {
			return nil, err
		}
		if updatedAt.Valid {
			acc.UpdatedAt = time.Unix(updatedAt.Int64, 0)
		}
		if lastYieldDate.Valid {
			t := time.Unix(lastYieldDate.Int64, 0)
			acc.LastYieldDate = &t
		}
		if targetAccountID.Valid {
			acc.TargetAccountID = &targetAccountID.Int64
		}
		if yieldType.Valid {
			acc.YieldType = yieldType.String
		}
		if yieldFreq.Valid {
			acc.YieldFrequency = yieldFreq.String
		}
		if payoutFreq.Valid {
			acc.PayoutFrequency = payoutFreq.String
		}

		if acc.Balance, err = crypto.DecryptCents(balanceRaw); err != nil {
			return nil, fmt.Errorf("decrypt balance id=%d: %w", acc.ID, err)
		}
		if acc.YieldMin, err = crypto.DecryptFloat(yieldMinRaw); err != nil {
			return nil, fmt.Errorf("decrypt yield_min id=%d: %w", acc.ID, err)
		}
		if acc.YieldMax, err = crypto.DecryptFloat(yieldMaxRaw); err != nil {
			return nil, fmt.Errorf("decrypt yield_max id=%d: %w", acc.ID, err)
		}
		if acc.ReinvestmentRate, err = crypto.DecryptInt(reinvestRaw); err != nil {
			return nil, fmt.Errorf("decrypt reinvestment_rate id=%d: %w", acc.ID, err)
		}

		accounts = append(accounts, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

// GetRecurringByUserID récupère toutes les opérations récurrentes d'un utilisateur.
// Le déchiffrement du montant est effectué directement dans la boucle de scan.
func GetRecurringByUserID(userID int64) ([]RecurringOperation, error) {
	rows, err := DB.Query(`
		SELECT id, user_id, account_id, to_account_id, amount, COALESCE(description, ''),
		       day_of_month, last_run_date, is_active
		FROM recurring_operations WHERE user_id = ? ORDER BY day_of_month ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ops []RecurringOperation
	for rows.Next() {
		var op RecurringOperation
		var amountRaw string
		var toAccountID sql.NullInt64
		var lastRunDate sql.NullInt64

		if err := rows.Scan(
			&op.ID, &op.UserID, &op.AccountID, &toAccountID, &amountRaw,
			&op.Description, &op.DayOfMonth, &lastRunDate, &op.IsActive,
		); err != nil {
			return nil, err
		}
		if toAccountID.Valid {
			op.ToAccountID = &toAccountID.Int64
		}
		if lastRunDate.Valid {
			t := time.Unix(lastRunDate.Int64, 0)
			op.LastRunDate = &t
		}
		if op.Amount, err = crypto.DecryptCents(amountRaw); err != nil {
			return nil, err
		}

		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ops, nil
}

// CountUsers retourne le nombre total d'utilisateurs
func CountUsers() (int, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
