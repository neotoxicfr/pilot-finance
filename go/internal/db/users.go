package db

import (
	"time"
)

// CreateUser crée un nouvel utilisateur
func CreateUser(emailEncrypted, emailBlindIndex, password, role string) (int64, error) {
	result, err := DB.Exec(`
		INSERT INTO users (email_encrypted, email_blind_index, password, role, created_at, email_verified, session_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, emailEncrypted, emailBlindIndex, password, role, time.Now().Unix(), true, 1)

	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
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

// UpdatePassword met à jour le mot de passe et invalide les sessions
func UpdatePassword(userID int64, hashedPassword string) error {
	_, err := DB.Exec(`
		UPDATE users SET password = ?, session_version = session_version + 1
		WHERE id = ?
	`, hashedPassword, userID)

	return err
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
func DeleteUserAndData(userID int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, q := range []string{
		`DELETE FROM recurring_operations WHERE user_id = ?`,
		`DELETE FROM accounts WHERE user_id = ?`,
		`DELETE FROM authenticators WHERE user_id = ?`,
		`DELETE FROM audit_log WHERE user_id = ?`,
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

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTokenInvalid
	}

	return nil
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
