package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pilot-finance/internal/crypto"
	"golang.org/x/sync/errgroup"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// Config contient la configuration de la base de données
type Config struct {
	Path string
}

// Init initialise la connexion à la base de données
func Init(cfg Config) error {
	// S'assurer que le dossier existe
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("création dossier DB: %w", err)
	}

	var err error
	DB, err = sql.Open("sqlite", cfg.Path)
	if err != nil {
		return fmt.Errorf("ouverture DB: %w", err)
	}

	// Configuration SQLite pour performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=10000",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA mmap_size=268435456",
	}

	for _, pragma := range pragmas {
		if _, err := DB.Exec(pragma); err != nil {
			slog.Warn("pragma failed", "pragma", pragma, "err", err)
		}
	}

	// Pool de connexions
	DB.SetMaxOpenConns(4) // WAL mode : 1 writer + N readers concurrents
	DB.SetMaxIdleConns(1)
	DB.SetConnMaxLifetime(time.Hour)

	// Test de connexion
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("ping DB: %w", err)
	}

	// Migrations automatiques versionnées
	runMigrations(cfg.Path)

	// VACUUM périodique au démarrage pour compacter la DB
	if _, err := DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		slog.Warn("WAL checkpoint", "err", err)
	}

	// Backup automatique quotidien (même volume, pas de mount supplémentaire)
	backupPath := cfg.Path + ".backup"
	if _, err := DB.Exec("VACUUM INTO ?", backupPath); err != nil {
		slog.Warn("backup automatique", "err", err)
	} else {
		slog.Info("backup créé", "path", backupPath)
	}

	slog.Info("base de données connectée", "path", cfg.Path)
	return nil
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
func runMigrations(dbPath string) {
	if _, err := DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		slog.Error("schema_migrations: création impossible", "err", err)
		return
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
	}

	for _, m := range migrations {
		var count int
		DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, m.Name).Scan(&count)
		if count > 0 {
			continue
		}

		var applyErr error
		if m.Run != nil {
			applyErr = m.Run(DB)
		} else {
			_, applyErr = DB.Exec(m.SQL)
		}

		if applyErr != nil {
			msg := applyErr.Error()
			if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "duplicate column name") {
				slog.Warn("migration échouée", "name", m.Name, "err", applyErr)
				continue
			}
		}

		if _, err := DB.Exec(`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`, m.Name, time.Now().Unix()); err != nil {
			slog.Warn("schema_migrations: enregistrement impossible", "name", m.Name, "err", err)
		} else {
			slog.Info("migration appliquée", "name", m.Name)
		}
	}
}

// encryptIfPlain chiffre une valeur si elle n'est pas déjà chiffrée (pas de ":").
// fn reçoit la valeur brute et retourne la valeur chiffrée.
func encryptIfPlain(raw string, fn func(string) (string, error)) string {
	if strings.Contains(raw, ":") {
		return raw // déjà chiffré
	}
	enc, err := fn(raw)
	if err != nil {
		return raw
	}
	return enc
}

// Close ferme la connexion à la base de données
func Close() error {
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

// GetSessionVersion récupère uniquement la session_version d'un utilisateur (requête allégée)
func GetSessionVersion(id int64) (int, error) {
	var sv int
	err := DB.QueryRow(`SELECT session_version FROM users WHERE id = ?`, id).Scan(&sv)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return sv, err
}

// GetUserAuthData récupère session_version et email chiffré en une seule requête.
// Utilisé par RequireAuth pour peupler le contexte sans stocker l'email dans le JWT.
func GetUserAuthData(id int64) (sessionVersion int, emailEncrypted string, err error) {
	err = DB.QueryRow(`SELECT session_version, email_encrypted FROM users WHERE id = ?`, id).
		Scan(&sessionVersion, &emailEncrypted)
	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	return
}

// rawAccount contient les données brutes d'un compte avant déchiffrement.
type rawAccount struct {
	acc          Account
	balanceRaw   string
	yieldMinRaw  string
	yieldMaxRaw  string
	reinvestRaw  string
}

// decryptAccountRow déchiffre les champs numériques d'un rawAccount vers un Account.
func decryptAccountRow(dst *Account, raw rawAccount) error {
	*dst = raw.acc
	var err error
	if dst.Balance, err = crypto.DecryptFloat(raw.balanceRaw); err != nil {
		return fmt.Errorf("decrypt balance id=%d: %w", dst.ID, err)
	}
	if dst.YieldMin, err = crypto.DecryptFloat(raw.yieldMinRaw); err != nil {
		return fmt.Errorf("decrypt yield_min id=%d: %w", dst.ID, err)
	}
	if dst.YieldMax, err = crypto.DecryptFloat(raw.yieldMaxRaw); err != nil {
		return fmt.Errorf("decrypt yield_max id=%d: %w", dst.ID, err)
	}
	if dst.ReinvestmentRate, err = crypto.DecryptInt(raw.reinvestRaw); err != nil {
		return fmt.Errorf("decrypt reinvestment_rate id=%d: %w", dst.ID, err)
	}
	return nil
}

// GetAccountsByUserID récupère tous les comptes d'un utilisateur.
// Le déchiffrement des champs numériques est parallélisé via errgroup.
func GetAccountsByUserID(userID int64) ([]Account, error) {
	rows, err := DB.Query(`
		SELECT id, user_id, name, balance, color, position, updated_at,
		       is_yield_active, yield_type, yield_min, yield_max,
		       yield_frequency, payout_frequency, last_yield_date,
		       reinvestment_rate, target_account_id
		FROM accounts WHERE user_id = ? ORDER BY position ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Passe 1 : scan séquentiel des lignes SQL (driver non thread-safe)
	var raws []rawAccount
	for rows.Next() {
		var r rawAccount
		var updatedAt, lastYieldDate sql.NullInt64
		var targetAccountID sql.NullInt64
		var yieldType, yieldFreq, payoutFreq sql.NullString

		if err := rows.Scan(
			&r.acc.ID, &r.acc.UserID, &r.acc.Name, &r.balanceRaw, &r.acc.Color, &r.acc.Position,
			&updatedAt, &r.acc.IsYieldActive, &yieldType, &r.yieldMinRaw, &r.yieldMaxRaw,
			&yieldFreq, &payoutFreq, &lastYieldDate, &r.reinvestRaw, &targetAccountID,
		); err != nil {
			return nil, err
		}
		if updatedAt.Valid {
			r.acc.UpdatedAt = time.Unix(updatedAt.Int64, 0)
		}
		if lastYieldDate.Valid {
			t := time.Unix(lastYieldDate.Int64, 0)
			r.acc.LastYieldDate = &t
		}
		if targetAccountID.Valid {
			r.acc.TargetAccountID = &targetAccountID.Int64
		}
		if yieldType.Valid {
			r.acc.YieldType = yieldType.String
		}
		if yieldFreq.Valid {
			r.acc.YieldFrequency = yieldFreq.String
		}
		if payoutFreq.Valid {
			r.acc.PayoutFrequency = payoutFreq.String
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Passe 2 : déchiffrement parallèle des champs numériques
	accounts := make([]Account, len(raws))
	g := new(errgroup.Group)
	for i := range raws {
		i := i
		g.Go(func() error {
			return decryptAccountRow(&accounts[i], raws[i])
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return accounts, nil
}

// rawRecurring contient les données brutes d'une opération récurrente avant déchiffrement.
type rawRecurring struct {
	op        RecurringOperation
	amountRaw string
}

// GetRecurringByUserID récupère toutes les opérations récurrentes d'un utilisateur.
// Le déchiffrement du montant est parallélisé via errgroup.
func GetRecurringByUserID(userID int64) ([]RecurringOperation, error) {
	rows, err := DB.Query(`
		SELECT id, user_id, account_id, to_account_id, amount, description,
		       day_of_month, last_run_date, is_active
		FROM recurring_operations WHERE user_id = ? ORDER BY day_of_month ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Passe 1 : scan séquentiel
	var raws []rawRecurring
	for rows.Next() {
		var r rawRecurring
		var toAccountID sql.NullInt64
		var lastRunDate sql.NullInt64

		if err := rows.Scan(
			&r.op.ID, &r.op.UserID, &r.op.AccountID, &toAccountID, &r.amountRaw,
			&r.op.Description, &r.op.DayOfMonth, &lastRunDate, &r.op.IsActive,
		); err != nil {
			return nil, err
		}
		if toAccountID.Valid {
			r.op.ToAccountID = &toAccountID.Int64
		}
		if lastRunDate.Valid {
			t := time.Unix(lastRunDate.Int64, 0)
			r.op.LastRunDate = &t
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Passe 2 : déchiffrement parallèle des montants
	ops := make([]RecurringOperation, len(raws))
	g := new(errgroup.Group)
	for i := range raws {
		i := i
		g.Go(func() error {
			ops[i] = raws[i].op
			var err error
			ops[i].Amount, err = crypto.DecryptFloat(raws[i].amountRaw)
			return err
		})
	}
	if err := g.Wait(); err != nil {
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
