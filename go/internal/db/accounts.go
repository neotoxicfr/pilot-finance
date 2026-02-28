package db

import (
	"fmt"
	"time"

	"pilot-finance/internal/crypto"
)

// CreateAccountWithYield cree un nouveau compte avec rendement
func CreateAccountWithYield(userID int64, name string, balance float64, color string, position int, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
	balEnc, err := crypto.EncryptFloat(balance)
	if err != nil {
		return fmt.Errorf("encrypt balance: %w", err)
	}
	ymEnc, err := crypto.EncryptFloat(yieldMin)
	if err != nil {
		return fmt.Errorf("encrypt yield_min: %w", err)
	}
	yxEnc, err := crypto.EncryptFloat(yieldMax)
	if err != nil {
		return fmt.Errorf("encrypt yield_max: %w", err)
	}
	rrEnc, err := crypto.EncryptInt(reinvestmentRate)
	if err != nil {
		return fmt.Errorf("encrypt reinvestment_rate: %w", err)
	}

	_, err = DB.Exec(`
		INSERT INTO accounts (user_id, name, balance, color, position, updated_at, is_yield_active, yield_type, yield_min, yield_max, reinvestment_rate, target_account_id, payout_frequency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, name, balEnc, color, position, time.Now().Unix(), isYieldActive, yieldType, ymEnc, yxEnc, rrEnc, targetAccountID, payoutFrequency)
	return err
}

// UpdateAccountWithYield met a jour un compte avec rendement
func UpdateAccountWithYield(id, userID int64, name string, balance float64, color string, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
	balEnc, err := crypto.EncryptFloat(balance)
	if err != nil {
		return fmt.Errorf("encrypt balance: %w", err)
	}
	ymEnc, err := crypto.EncryptFloat(yieldMin)
	if err != nil {
		return fmt.Errorf("encrypt yield_min: %w", err)
	}
	yxEnc, err := crypto.EncryptFloat(yieldMax)
	if err != nil {
		return fmt.Errorf("encrypt yield_max: %w", err)
	}
	rrEnc, err := crypto.EncryptInt(reinvestmentRate)
	if err != nil {
		return fmt.Errorf("encrypt reinvestment_rate: %w", err)
	}

	_, err = DB.Exec(`
		UPDATE accounts SET name = ?, balance = ?, color = ?, updated_at = ?,
		is_yield_active = ?, yield_type = ?, yield_min = ?, yield_max = ?, reinvestment_rate = ?, target_account_id = ?, payout_frequency = ?
		WHERE id = ? AND user_id = ?
	`, name, balEnc, color, time.Now().Unix(), isYieldActive, yieldType, ymEnc, yxEnc, rrEnc, targetAccountID, payoutFrequency, id, userID)
	return err
}

// UpdateAccountBalance met a jour uniquement le solde d'un compte
func UpdateAccountBalance(id, userID int64, balance float64) error {
	balEnc, err := crypto.EncryptFloat(balance)
	if err != nil {
		return fmt.Errorf("encrypt balance: %w", err)
	}
	_, err = DB.Exec(`
		UPDATE accounts SET balance = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, balEnc, time.Now().Unix(), id, userID)
	return err
}

// DeleteAccount supprime un compte et les opérations récurrentes associées
func DeleteAccount(id, userID int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM recurring_operations
		WHERE (account_id = ? OR to_account_id = ?) AND user_id = ?`,
		id, id, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM accounts WHERE id = ? AND user_id = ?`, id, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// AccountBelongsToUser vérifie qu'un compte appartient à l'utilisateur donné.
func AccountBelongsToUser(accountID, userID int64) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id = ? AND user_id = ?`, accountID, userID).Scan(&count)
	return count > 0, err
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

// ReorderAccounts met a jour la position de chaque compte selon l'ordre fourni
func ReorderAccounts(userID int64, ids []int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for pos, id := range ids {
		if _, err := tx.Exec(
			"UPDATE accounts SET position = ? WHERE id = ? AND user_id = ?",
			pos, id, userID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateRecurring cree une operation recurrente
func CreateRecurring(userID, accountID int64, toAccountID *int64, description string, amount float64, dayOfMonth int) error {
	amtEnc, err := crypto.EncryptFloat(amount)
	if err != nil {
		return fmt.Errorf("encrypt amount: %w", err)
	}
	_, err = DB.Exec(`
		INSERT INTO recurring_operations (user_id, account_id, to_account_id, description, amount, day_of_month, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, userID, accountID, toAccountID, description, amtEnc, dayOfMonth)
	return err
}

// UpdateRecurring met a jour une operation recurrente
func UpdateRecurring(id, userID int64, description string, amount float64, dayOfMonth int, toAccountID *int64) error {
	amtEnc, err := crypto.EncryptFloat(amount)
	if err != nil {
		return fmt.Errorf("encrypt amount: %w", err)
	}
	_, err = DB.Exec(`
		UPDATE recurring_operations SET description = ?, amount = ?, day_of_month = ?, to_account_id = ?
		WHERE id = ? AND user_id = ?
	`, description, amtEnc, dayOfMonth, toAccountID, id, userID)
	return err
}

// DeleteRecurring supprime une operation recurrente
func DeleteRecurring(id, userID int64) error {
	_, err := DB.Exec(`DELETE FROM recurring_operations WHERE id = ? AND user_id = ?`, id, userID)
	return err
}
