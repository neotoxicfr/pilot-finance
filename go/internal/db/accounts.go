package db

import "time"

// CreateAccount cree un nouveau compte
func CreateAccount(userID int64, name string, balance float64, color string, position int) error {
	_, err := DB.Exec(`
		INSERT INTO accounts (user_id, name, balance, color, position, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, name, balance, color, position, time.Now().Unix())
	return err
}

// UpdateAccount met a jour un compte
func UpdateAccount(id, userID int64, name string, balance float64, color string) error {
	_, err := DB.Exec(`
		UPDATE accounts SET name = ?, balance = ?, color = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, name, balance, color, time.Now().Unix(), id, userID)
	return err
}

// UpdateAccountBalance met a jour uniquement le solde d'un compte
func UpdateAccountBalance(id, userID int64, balance float64) error {
	_, err := DB.Exec(`
		UPDATE accounts SET balance = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, balance, time.Now().Unix(), id, userID)
	return err
}

// DeleteAccount supprime un compte
func DeleteAccount(id, userID int64) error {
	_, err := DB.Exec(`DELETE FROM accounts WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// SwapAccountPositions echange les positions de deux comptes
func SwapAccountPositions(id1, id2, userID int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pos1, pos2 int
	err = tx.QueryRow("SELECT position FROM accounts WHERE id = ? AND user_id = ?", id1, userID).Scan(&pos1)
	if err != nil {
		return err
	}
	err = tx.QueryRow("SELECT position FROM accounts WHERE id = ? AND user_id = ?", id2, userID).Scan(&pos2)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE accounts SET position = ? WHERE id = ?", pos2, id1)
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE accounts SET position = ? WHERE id = ?", pos1, id2)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// CreateRecurring cree une operation recurrente
func CreateRecurring(userID, accountID int64, toAccountID *int64, description string, amount float64, dayOfMonth int) error {
	_, err := DB.Exec(`
		INSERT INTO recurring_operations (user_id, account_id, to_account_id, description, amount, day_of_month, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, userID, accountID, toAccountID, description, amount, dayOfMonth)
	return err
}

// UpdateRecurring met a jour une operation recurrente
func UpdateRecurring(id, userID int64, description string, amount float64, dayOfMonth int, toAccountID *int64) error {
	_, err := DB.Exec(`
		UPDATE recurring_operations SET description = ?, amount = ?, day_of_month = ?, to_account_id = ?
		WHERE id = ? AND user_id = ?
	`, description, amount, dayOfMonth, toAccountID, id, userID)
	return err
}

// DeleteRecurring supprime une operation recurrente
func DeleteRecurring(id, userID int64) error {
	_, err := DB.Exec(`DELETE FROM recurring_operations WHERE id = ? AND user_id = ?`, id, userID)
	return err
}
