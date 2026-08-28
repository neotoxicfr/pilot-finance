package db

import (
	"fmt"
	"time"

	"pilot-finance/internal/crypto"
)

// CreateUser crée un nouvel utilisateur (email non vérifié par défaut).
// L'email reste non bloquant : un bandeau persistant + relance dans Settings
// permet à l'utilisateur de vérifier ultérieurement.
func CreateUser(emailEncrypted, emailBlindIndex, password, role string) (int64, error) {
	result, err := DB.Exec(`
		INSERT INTO users (email_encrypted, email_blind_index, password, role, created_at, email_verified, session_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, emailEncrypted, emailBlindIndex, password, role, time.Now().Unix(), false, 1)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// CreateUserAtomic crée un utilisateur en assignant le rôle ADMIN si et
// seulement s'il n'existe pas encore d'admin, sinon USER. La logique se fait
// en une seule transaction SQL pour éviter la fenêtre TOCTOU entre
// CountUsers() et CreateUser() (L2 fix : deux inscriptions concurrentes ne
// peuvent plus créer deux admins).
//
// Retourne l'ID créé et le rôle effectivement assigné.
func CreateUserAtomic(emailEncrypted, emailBlindIndex, password string) (int64, string, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()

	// COUNT dans la même transaction → sérialise l'attribution ADMIN.
	//
	// Correction du commentaire d'origine (audit S-33), qui affirmait à tort que
	// « BEGIN IMMEDIATE/EXCLUSIVE est pris implicitement au premier write » :
	// SQLite ouvre un BEGIN DEFERRED et ne prend le verrou d'écriture qu'au
	// premier write, ce qui expose la séquence lecture→écriture ci-dessous à un
	// SQLITE_BUSY_SNAPSHOT que busy_timeout ne rattrape pas. La sérialisation
	// réelle vient de `_txlock=immediate` posé dans le DSN (sqlite.go) : le
	// verrou d'écriture est pris dès DB.Begin(), donc deux inscriptions
	// concurrentes s'attendent au lieu de courir.
	var adminCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'ADMIN'`).Scan(&adminCount); err != nil {
		return 0, "", err
	}

	role := "USER"
	if adminCount == 0 {
		role = "ADMIN"
	}

	res, err := tx.Exec(`
		INSERT INTO users (email_encrypted, email_blind_index, password, role, created_at, email_verified, session_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, emailEncrypted, emailBlindIndex, password, role, time.Now().Unix(), false, 1)
	if err != nil {
		return 0, "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}

	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return id, role, nil
}

// GetSessionVersion récupère la session_version d'un utilisateur en une requête
// légère. Utilisé par le middleware d'authentification pour valider rapidement
// qu'un JWT n'a pas été invalidé (logout, changement de mot de passe, MFA).
func GetSessionVersion(id int64) (int, error) {
	var sessionVersion int
	err := DB.QueryRow(`SELECT session_version FROM users WHERE id = ?`, id).Scan(&sessionVersion)
	return sessionVersion, err
}

// UpdateLoginAttempts met à jour les tentatives de connexion
func UpdateLoginAttempts(userID int64, attempts int, lockUntil *time.Time) error {
	var lockTime *int64
	if lockUntil != nil {
		t := lockUntil.Unix()
		lockTime = &t
	}

	_, err := DB.Exec(`
		UPDATE users SET failed_login_attempts = ?, lock_until = ?
		WHERE id = ?
	`, attempts, lockTime, userID)

	return err
}

// UpdatePasswordHash met à jour le hash bcrypt sans invalider les sessions (rehash silencieux)
func UpdatePasswordHash(userID int64, hashedPassword string) error {
	_, err := DB.Exec(`UPDATE users SET password = ? WHERE id = ?`, hashedPassword, userID)
	return err
}

// IncrementSessionVersion incrémente le compteur de version de session de
// l'utilisateur, invalidant tous les JWT émis avec une version antérieure.
// Utilisé au logout pour empêcher la réutilisation d'un JWT exfiltré (XSS bypass,
// malware) jusqu'à son expiration naturelle (24h).
func IncrementSessionVersion(userID int64) error {
	_, err := DB.Exec(`UPDATE users SET session_version = session_version + 1 WHERE id = ?`, userID)
	return err
}

// UpdatePassword met à jour le mot de passe et invalide les sessions
func UpdatePassword(userID int64, hashedPassword string) error {
	_, err := DB.Exec(`
		UPDATE users SET password = ?, session_version = session_version + 1
		WHERE id = ?
	`, hashedPassword, userID)

	return err
}

// UpdatePasswordAndClearResetToken applique le nouveau hash, incrémente la
// session_version (invalide les JWT existants) ET efface le reset_token dans
// la même transaction. Évite la fenêtre où le token resterait réutilisable
// si le ClearResetToken échouait après un UpdatePassword réussi.
func UpdatePasswordAndClearResetToken(userID int64, hashedPassword string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		UPDATE users
		SET password = ?,
		    session_version = session_version + 1,
		    reset_token = NULL,
		    reset_token_expiry = NULL
		WHERE id = ?
	`, hashedPassword, userID); err != nil {
		return err
	}

	return tx.Commit()
}

// EnableMFA active le 2FA pour un utilisateur
func EnableMFA(userID int64, encryptedSecret string) error {
	_, err := DB.Exec(`
		UPDATE users SET mfa_enabled = 1, mfa_secret = ?, session_version = session_version + 1
		WHERE id = ?
	`, encryptedSecret, userID)

	return err
}

// DisableMFA désactive le 2FA
func DisableMFA(userID int64) error {
	_, err := DB.Exec(`
		UPDATE users SET mfa_enabled = 0, mfa_secret = NULL, session_version = session_version + 1
		WHERE id = ?
	`, userID)

	return err
}

// SetResetToken définit le token de réinitialisation de mot de passe
func SetResetToken(userID int64, hashedToken string, expiry time.Time) error {
	_, err := DB.Exec(`
		UPDATE users SET reset_token = ?, reset_token_expiry = ?
		WHERE id = ?
	`, hashedToken, expiry.Unix(), userID)

	return err
}

// GetUserByResetToken récupère un utilisateur par son reset token
func GetUserByResetToken(hashedToken string) (*User, error) {
	return scanUser(DB.QueryRow(userSelectCols+` WHERE reset_token = ? AND reset_token_expiry > ?`, hashedToken, time.Now().Unix()))
}

// ClearResetToken efface le reset token
func ClearResetToken(userID int64) error {
	_, err := DB.Exec(`
		UPDATE users SET reset_token = NULL, reset_token_expiry = NULL
		WHERE id = ?
	`, userID)

	return err
}

// UpdateUserPreferences met à jour la langue et la devise de l'utilisateur
func UpdateUserPreferences(userID int64, language, currency string) error {
	_, err := DB.Exec(`
		UPDATE users SET language = ?, currency = ? WHERE id = ?
	`, language, currency, userID)
	return err
}

// DeleteUserAndData supprime un utilisateur et toutes ses données en cascade (GDPR).
// Audit logs are anonymized (not deleted) to preserve the audit trail.
func DeleteUserAndData(userID int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Anonymize audit logs: encrypt placeholder to match other audit entries
	deletedVal, _ := crypto.Encrypt("deleted-user")
	if _, err := tx.Exec(
		`UPDATE audit_log SET ip = ?, user_agent = ? WHERE user_id = ?`,
		deletedVal, deletedVal, userID,
	); err != nil {
		return err
	}

	for _, q := range []string{
		`DELETE FROM recurring_operations WHERE user_id = ?`,
		`DELETE FROM accounts WHERE user_id = ?`,
		`DELETE FROM authenticators WHERE user_id = ?`,
		`DELETE FROM users WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// VerifyEmailByToken vérifie l'email avec le token
func VerifyEmailByToken(hashedToken string) error {
	result, err := DB.Exec(`
		UPDATE users SET email_verified = 1, verification_token = NULL
		WHERE verification_token = ?
	`, hashedToken)

	if err != nil {
		return err
	}

	rows, err2 := result.RowsAffected()
	if err2 != nil {
		return fmt.Errorf("RowsAffected: %w", err2)
	}
	if rows == 0 {
		return ErrTokenInvalid
	}

	return nil
}

// SetVerificationToken stocke le hash SHA-256 du token de vérification d'email.
// Le token brut est envoyé par email ; seule sa version hashée transite par la DB.
func SetVerificationToken(userID int64, hashedToken string) error {
	_, err := DB.Exec(`UPDATE users SET verification_token = ? WHERE id = ?`, hashedToken, userID)
	return err
}

// GetUserByVerificationToken récupère l'utilisateur dont le verification_token correspond.
// Retourne (nil, nil) si aucun utilisateur ne correspond.
func GetUserByVerificationToken(hashedToken string) (*User, error) {
	return scanUser(DB.QueryRow(userSelectCols+` WHERE verification_token = ?`, hashedToken))
}

// MarkEmailVerified marque l'email comme vérifié et efface le token (idempotent).
func MarkEmailVerified(userID int64) error {
	_, err := DB.Exec(`UPDATE users SET email_verified = 1, verification_token = NULL WHERE id = ?`, userID)
	return err
}

// GetAllUsers récupère tous les utilisateurs (admin)
func GetAllUsers() ([]User, error) {
	rows, err := DB.Query(`
		SELECT id, email_encrypted, email_blind_index, password, role,
		       created_at, email_verified, mfa_enabled, session_version
		FROM users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		var createdAt int64
		err := rows.Scan(
			&user.ID, &user.EmailEncrypted, &user.EmailBlindIndex, &user.Password, &user.Role,
			&createdAt, &user.EmailVerified, &user.MFAEnabled, &user.SessionVersion,
		)
		if err != nil {
			return nil, err
		}
		user.CreatedAt = time.Unix(createdAt, 0)
		users = append(users, user)
	}

	return users, rows.Err()
}
