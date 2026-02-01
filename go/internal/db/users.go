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
	var user User
	var createdAt, lockUntil, resetTokenExpiry int64
	var verificationToken, resetToken, mfaSecret *string

	err := DB.QueryRow(`
		SELECT id, email_encrypted, email_blind_index, password, role,
		       created_at, email_verified, verification_token, reset_token,
		       reset_token_expiry, mfa_enabled, mfa_secret, failed_login_attempts,
		       lock_until, session_version
		FROM users
		WHERE reset_token = ? AND reset_token_expiry > ?
	`, hashedToken, time.Now().Unix()).Scan(
		&user.ID, &user.EmailEncrypted, &user.EmailBlindIndex, &user.Password, &user.Role,
		&createdAt, &user.EmailVerified, &verificationToken, &resetToken,
		&resetTokenExpiry, &user.MFAEnabled, &mfaSecret, &user.FailedLoginAttempts,
		&lockUntil, &user.SessionVersion,
	)

	if err != nil {
		return nil, err
	}

	user.CreatedAt = time.Unix(createdAt, 0)
	if lockUntil > 0 {
		t := time.Unix(lockUntil, 0)
		user.LockUntil = &t
	}
	if resetTokenExpiry > 0 {
		t := time.Unix(resetTokenExpiry, 0)
		user.ResetTokenExpiry = &t
	}
	user.VerificationToken = verificationToken
	user.ResetToken = resetToken
	user.MFASecret = mfaSecret

	return &user, nil
}

// ClearResetToken efface le reset token
func ClearResetToken(userID int64) error {
	_, err := DB.Exec(`
		UPDATE users SET reset_token = NULL, reset_token_expiry = NULL
		WHERE id = ?
	`, userID)

	return err
}

// DeleteUser supprime un utilisateur
func DeleteUser(userID int64) error {
	_, err := DB.Exec(`DELETE FROM users WHERE id = ?`, userID)
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
