package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	// Migrations automatiques
	runMigrations()

	slog.Info("base de données connectée", "path", cfg.Path)
	return nil
}

// runMigrations exécute les migrations de schéma
func runMigrations() {
	migrations := []string{
		// Ajouter backup_eligible aux authenticators (pour go-webauthn)
		`ALTER TABLE authenticators ADD COLUMN backup_eligible INTEGER DEFAULT 0`,
		// Multi-langue et multi-devise par utilisateur
		`ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'fr'`,
		`ALTER TABLE users ADD COLUMN currency TEXT NOT NULL DEFAULT 'EUR'`,
		// Périodicité du rendement et des versements
		`ALTER TABLE accounts ADD COLUMN yield_frequency TEXT NOT NULL DEFAULT 'MONTHLY'`,
		`ALTER TABLE accounts ADD COLUMN payout_frequency TEXT NOT NULL DEFAULT 'MONTHLY'`,
		// Index de performance
		`CREATE INDEX IF NOT EXISTS idx_accounts_user_id       ON accounts(user_id, position)`,
		`CREATE INDEX IF NOT EXISTS idx_recurring_user_id      ON recurring_operations(user_id, day_of_month)`,
		`CREATE INDEX IF NOT EXISTS idx_authenticators_user_id ON authenticators(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_authenticators_cred_id ON authenticators(credential_id)`,
		`CREATE INDEX IF NOT EXISTS idx_users_blind            ON users(email_blind_index)`,
		`CREATE INDEX IF NOT EXISTS idx_users_reset_token      ON users(reset_token)`,
		`CREATE INDEX IF NOT EXISTS idx_users_verif_token      ON users(verification_token)`,
	}

	for _, migration := range migrations {
		_, err := DB.Exec(migration)
		if err != nil {
			msg := err.Error()
			if !strings.Contains(msg, "already exists") && !strings.Contains(msg, "duplicate column name") {
				slog.Warn("migration error", "err", err, "sql", migration[:min(80, len(migration))])
			}
		}
	}
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

// GetAccountsByUserID récupère tous les comptes d'un utilisateur
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

	var accounts []Account
	for rows.Next() {
		var acc Account
		var updatedAt, lastYieldDate sql.NullInt64
		var targetAccountID sql.NullInt64
		var yieldType, yieldFreq, payoutFreq sql.NullString

		err := rows.Scan(
			&acc.ID, &acc.UserID, &acc.Name, &acc.Balance, &acc.Color, &acc.Position,
			&updatedAt, &acc.IsYieldActive, &yieldType, &acc.YieldMin, &acc.YieldMax,
			&yieldFreq, &payoutFreq, &lastYieldDate, &acc.ReinvestmentRate, &targetAccountID,
		)
		if err != nil {
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

		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}

// GetRecurringByUserID récupère toutes les opérations récurrentes d'un utilisateur
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

	var ops []RecurringOperation
	for rows.Next() {
		var op RecurringOperation
		var toAccountID sql.NullInt64
		var lastRunDate sql.NullInt64

		err := rows.Scan(
			&op.ID, &op.UserID, &op.AccountID, &toAccountID, &op.Amount,
			&op.Description, &op.DayOfMonth, &lastRunDate, &op.IsActive,
		)
		if err != nil {
			return nil, err
		}

		if toAccountID.Valid {
			op.ToAccountID = &toAccountID.Int64
		}
		if lastRunDate.Valid {
			t := time.Unix(lastRunDate.Int64, 0)
			op.LastRunDate = &t
		}

		ops = append(ops, op)
	}

	return ops, rows.Err()
}

// CountUsers retourne le nombre total d'utilisateurs
func CountUsers() (int, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
