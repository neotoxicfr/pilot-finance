package db

import "time"

// Codes de récupération 2FA (audit S-22).
//
// La table ne contient QUE le hash du code (SHA-256, calculé par
// crypto.HashToken côté handlers) : un dump de la base ne permet pas de
// rejouer un code. Le hachage est délibérément déterministe et non salé — voir
// la justification détaillée dans handlers/mfa.go — ce qui autorise la
// consommation en un seul UPDATE indexé plutôt qu'en N comparaisons bcrypt.

// ReplaceRecoveryCodes remplace l'intégralité du lot de codes d'un utilisateur.
// Les anciens codes — utilisés ou non — sont supprimés dans la même
// transaction : une régénération invalide donc bien tout le lot précédent.
func ReplaceRecoveryCodes(userID int64, hashes []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM mfa_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return err
	}

	now := time.Now().Unix()
	for _, h := range hashes {
		if _, err := tx.Exec(
			`INSERT INTO mfa_recovery_codes (user_id, code_hash, created_at, used_at) VALUES (?, ?, ?, NULL)`,
			userID, h, now,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ConsumeRecoveryCode marque un code comme utilisé et retourne true s'il
// existait ET n'avait pas déjà servi.
//
// L'UPDATE conditionnel (`used_at IS NULL`) fait office de compare-and-swap :
// deux tentatives concurrentes sur le même code ne peuvent pas réussir toutes
// les deux, SQLite sérialisant les écritures. Le contrôle « à usage unique »
// ne dépend donc d'aucune lecture préalable côté application.
func ConsumeRecoveryCode(userID int64, hash string) (bool, error) {
	res, err := DB.Exec(
		`UPDATE mfa_recovery_codes SET used_at = ? WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`,
		time.Now().Unix(), userID, hash,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountUnusedRecoveryCodes compte les codes encore consommables.
func CountUnusedRecoveryCodes(userID int64) (int, error) {
	var n int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL`,
		userID,
	).Scan(&n)
	return n, err
}

// DeleteRecoveryCodes supprime tous les codes d'un utilisateur.
func DeleteRecoveryCodes(userID int64) error {
	_, err := DB.Exec(`DELETE FROM mfa_recovery_codes WHERE user_id = ?`, userID)
	return err
}
