package db

import (
	"database/sql"
	"fmt"
	"time"

	"pilot-finance/internal/crypto"
)

// CreateAccountWithYield cree un nouveau compte avec rendement
func CreateAccountWithYield(userID int64, name string, balance int64, color string, position int, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
	balEnc, err := crypto.EncryptCents(balance)
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
func UpdateAccountWithYield(id, userID int64, name string, balance int64, color string, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
	balEnc, err := crypto.EncryptCents(balance)
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
func UpdateAccountBalance(id, userID int64, balance int64) error {
	balEnc, err := crypto.EncryptCents(balance)
	if err != nil {
		return fmt.Errorf("encrypt balance: %w", err)
	}
	_, err = DB.Exec(`
		UPDATE accounts SET balance = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, balEnc, time.Now().Unix(), id, userID)
	return err
}

// AccountBalanceUpdate : couple (id de compte, solde en centimes) pour l'import.
type AccountBalanceUpdate struct {
	ID    int64
	Cents int64
}

// UpdateAccountBalancesTx applique plusieurs mises à jour de solde dans une
// seule transaction : soit toutes réussissent, soit aucune n'est appliquée
// (import CSV atomique, audit FIN-9).
func UpdateAccountBalancesTx(userID int64, updates []AccountBalanceUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, u := range updates {
		balEnc, err := crypto.EncryptCents(u.Cents)
		if err != nil {
			return fmt.Errorf("encrypt balance: %w", err)
		}
		if _, err := tx.Exec(`
			UPDATE accounts SET balance = ?, updated_at = ?
			WHERE id = ? AND user_id = ?
		`, balEnc, now, u.ID, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// CountAccountsByUserID retourne le nombre de comptes d'un utilisateur (sans déchiffrement).
func CountAccountsByUserID(userID int64) (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM accounts WHERE user_id = ?`, userID).Scan(&count)
	return count, err
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
func CreateRecurring(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
	amtEnc, err := crypto.EncryptCents(amount)
	if err != nil {
		return fmt.Errorf("encrypt amount: %w", err)
	}
	_, err = DB.Exec(`
		INSERT INTO recurring_operations (user_id, account_id, to_account_id, description, amount, day_of_month, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, userID, accountID, toAccountID, description, amtEnc, dayOfMonth)
	return err
}

// UpdateRecurring met a jour une operation recurrente.
// accountID met à jour le compte source ; la valeur 0 (jamais un id valide,
// l'autoincrément démarre à 1) laisse le compte inchangé — permet au chemin PUT
// de ne pas toucher au compte tout en corrigeant le chemin d'édition POST qui,
// lui, le persiste (audit FIN-3).
// Retourne sql.ErrNoRows si l'opération n'existe pas ou n'appartient pas à l'utilisateur.
func UpdateRecurring(id, userID, accountID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
	amtEnc, err := crypto.EncryptCents(amount)
	if err != nil {
		return fmt.Errorf("encrypt amount: %w", err)
	}
	res, err := DB.Exec(`
		UPDATE recurring_operations
		SET account_id = COALESCE(NULLIF(?, 0), account_id),
		    description = ?, amount = ?, day_of_month = ?, to_account_id = ?
		WHERE id = ? AND user_id = ?
	`, accountID, description, amtEnc, dayOfMonth, toAccountID, id, userID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("RowsAffected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteRecurring supprime une operation recurrente.
// Retourne sql.ErrNoRows si l'opération n'existe pas ou n'appartient pas à l'utilisateur.
func DeleteRecurring(id, userID int64) error {
	res, err := DB.Exec(`DELETE FROM recurring_operations WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("RowsAffected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
