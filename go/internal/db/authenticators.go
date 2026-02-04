package db

// CreateAuthenticator crée une nouvelle passkey
func CreateAuthenticator(credentialID, publicKey string, counter int, deviceType string, backedUp, backupEligible bool, transports string, userID int64) error {
	_, err := DB.Exec(`
		INSERT INTO authenticators (credential_id, credential_public_key, counter, credential_device_type, credential_backed_up, backup_eligible, transports, user_id, name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, credentialID, publicKey, counter, deviceType, backedUp, backupEligible, transports, userID, "Passkey")

	return err
}

// GetAuthenticatorsByUserID récupère toutes les passkeys d'un utilisateur
func GetAuthenticatorsByUserID(userID int64) ([]Authenticator, error) {
	rows, err := DB.Query(`
		SELECT id, credential_id, credential_public_key, counter, credential_device_type,
		       credential_backed_up, backup_eligible, transports, user_id, name
		FROM authenticators WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var auths []Authenticator
	for rows.Next() {
		var a Authenticator
		err := rows.Scan(
			&a.ID, &a.CredentialID, &a.CredentialPublicKey, &a.Counter,
			&a.CredentialDeviceType, &a.CredentialBackedUp, &a.BackupEligible,
			&a.Transports, &a.UserID, &a.Name,
		)
		if err != nil {
			return nil, err
		}
		auths = append(auths, a)
	}

	return auths, rows.Err()
}

// GetAuthenticatorByCredentialID récupère une passkey par son credential ID
func GetAuthenticatorByCredentialID(credentialID string) (*Authenticator, error) {
	var a Authenticator
	err := DB.QueryRow(`
		SELECT id, credential_id, credential_public_key, counter, credential_device_type,
		       credential_backed_up, backup_eligible, transports, user_id, name
		FROM authenticators WHERE credential_id = ?
	`, credentialID).Scan(
		&a.ID, &a.CredentialID, &a.CredentialPublicKey, &a.Counter,
		&a.CredentialDeviceType, &a.CredentialBackedUp, &a.BackupEligible,
		&a.Transports, &a.UserID, &a.Name,
	)

	if err != nil {
		return nil, err
	}

	return &a, nil
}

// UpdateAuthenticatorCounter met à jour le compteur d'une passkey
func UpdateAuthenticatorCounter(credentialID string, counter int) error {
	_, err := DB.Exec(`
		UPDATE authenticators SET counter = ? WHERE credential_id = ?
	`, counter, credentialID)

	return err
}

// DeleteAuthenticator supprime une passkey
func DeleteAuthenticator(id, userID int64) error {
	_, err := DB.Exec(`
		DELETE FROM authenticators WHERE id = ? AND user_id = ?
	`, id, userID)

	return err
}

// RenameAuthenticator renomme une passkey
func RenameAuthenticator(id, userID int64, name string) error {
	_, err := DB.Exec(`
		UPDATE authenticators SET name = ? WHERE id = ? AND user_id = ?
	`, name, id, userID)

	return err
}
