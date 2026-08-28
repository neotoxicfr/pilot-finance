package db

import (
	"fmt"
	"testing"
)

func TestCreateAndGetAccount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	err := CreateAccountWithYield(userID, "Savings", 500050, "#4CAF50", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	if err != nil {
		t.Fatalf("CreateAccountWithYield: %v", err)
	}

	accounts, err := GetAccountsByUserID(userID)
	if err != nil {
		t.Fatalf("GetAccountsByUserID: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("want 1 account, got %d", len(accounts))
	}
	if accounts[0].Balance != 500050 {
		t.Errorf("balance: want 500050, got %v", accounts[0].Balance)
	}
	if accounts[0].Name != "Savings" {
		t.Errorf("name: want Savings, got %q", accounts[0].Name)
	}
	if accounts[0].ReinvestmentRate != 100 {
		t.Errorf("reinvestment_rate: want 100, got %d", accounts[0].ReinvestmentRate)
	}
}

func TestCreateYieldAccount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	err := CreateAccountWithYield(userID, "PEA", 1000000, "#FF5722", 0, true, "RANGE", 3.5, 7.0, 50, nil, "MONTHLY")
	if err != nil {
		t.Fatalf("CreateAccountWithYield: %v", err)
	}

	accounts, _ := GetAccountsByUserID(userID)
	if len(accounts) != 1 {
		t.Fatalf("want 1 account, got %d", len(accounts))
	}
	a := accounts[0]
	if !a.IsYieldActive {
		t.Error("IsYieldActive should be true")
	}
	if a.YieldMin != 3.5 {
		t.Errorf("YieldMin: want 3.5, got %v", a.YieldMin)
	}
	if a.YieldMax != 7.0 {
		t.Errorf("YieldMax: want 7.0, got %v", a.YieldMax)
	}
	if a.ReinvestmentRate != 50 {
		t.Errorf("ReinvestmentRate: want 50, got %d", a.ReinvestmentRate)
	}
}

func TestUpdateAccountWithYield(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "Old", 100000, "#000", 0, true, "FIXED", 3.0, 3.0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id := accounts[0].ID

	err := UpdateAccountWithYield(id, userID, "New", 200000, "#fff", false, "FIXED", 0, 0, 0, nil, "MONTHLY")
	if err != nil {
		t.Fatalf("UpdateAccountWithYield: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].Balance != 200000 {
		t.Errorf("balance: want 200000, got %v", accounts[0].Balance)
	}
	if accounts[0].Name != "New" {
		t.Errorf("name: want New, got %q", accounts[0].Name)
	}
	if accounts[0].IsYieldActive {
		t.Error("IsYieldActive should be false after update")
	}
}

func TestUpdateAccountBalance(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "Acc", 10000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id := accounts[0].ID

	if err := UpdateAccountBalance(id, userID, 999999); err != nil {
		t.Fatalf("UpdateAccountBalance: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].Balance != 999999 {
		t.Errorf("balance: want 999999, got %v", accounts[0].Balance)
	}
}

// ── S-36 / FIN-9 : atomicité réelle de l'import CSV ─────────────────────────
//
// Les tests d'import côté handlers remplacent hookUpdateAccountBalancesTx par
// un stub qui n'écrit rien : ils ne pouvaient donc pas détecter la suppression
// de la transaction. Les deux tests ci-dessous exercent la vraie fonction.

// failUpdateTrigger installe un déclencheur SQLite qui fait échouer toute
// écriture sur le compte donné, et le retire en fin de test.
func failUpdateTrigger(t *testing.T, accountID int64) {
	t.Helper()
	// CREATE TRIGGER est du DDL : la valeur est interpolée (elle vient de la
	// base de test, pas d'une entrée utilisateur).
	stmt := fmt.Sprintf(`
		CREATE TRIGGER fail_update_%d BEFORE UPDATE ON accounts
		WHEN NEW.id = %d
		BEGIN SELECT RAISE(ABORT, 'echec simule'); END`, accountID, accountID)
	if _, err := DB.Exec(stmt); err != nil {
		t.Fatalf("CREATE TRIGGER: %v", err)
	}
	t.Cleanup(func() {
		DB.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS fail_update_%d`, accountID)) //nolint:errcheck
	})
}

// TestUpdateAccountBalancesTx_AppliesAll vérifie le cas nominal : toutes les
// lignes valides sont écrites en une passe.
func TestUpdateAccountBalancesTx_AppliesAll(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "A", 111, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "B", 222, "#111", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	idA, idB := accounts[0].ID, accounts[1].ID

	if err := UpdateAccountBalancesTx(userID, []AccountBalanceUpdate{
		{ID: idA, Cents: 1000},
		{ID: idB, Cents: 2000},
	}); err != nil {
		t.Fatalf("UpdateAccountBalancesTx: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	got := map[int64]int64{}
	for _, a := range accounts {
		got[a.ID] = a.Balance
	}
	if got[idA] != 1000 {
		t.Errorf("solde A : want 1000, got %d", got[idA])
	}
	if got[idB] != 2000 {
		t.Errorf("solde B : want 2000, got %d", got[idB])
	}
}

// TestUpdateAccountBalancesTx_RollbackOnFailure vérifie l'atomicité : quand la
// seconde écriture échoue, la première ne doit PAS subsister. Retirer la
// transaction (Begin/Commit/Rollback) au profit d'un DB.Exec direct laisserait
// le premier solde modifié et fait rougir ce test.
func TestUpdateAccountBalancesTx_RollbackOnFailure(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "A", 111, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "B", 222, "#111", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	idA, idB := accounts[0].ID, accounts[1].ID

	// La 2e ligne de l'import échouera ; la 1re a déjà été écrite dans la
	// transaction et doit donc être annulée.
	failUpdateTrigger(t, idB)

	err := UpdateAccountBalancesTx(userID, []AccountBalanceUpdate{
		{ID: idA, Cents: 1000},
		{ID: idB, Cents: 2000},
	})
	if err == nil {
		t.Fatal("UpdateAccountBalancesTx devrait échouer quand une écriture est refusée")
	}

	accounts, _ = GetAccountsByUserID(userID)
	got := map[int64]int64{}
	for _, a := range accounts {
		got[a.ID] = a.Balance
	}
	if got[idA] != 111 {
		t.Errorf("mise à jour partielle : le solde A devrait rester 111, got %d", got[idA])
	}
	if got[idB] != 222 {
		t.Errorf("le solde B devrait rester 222, got %d", got[idB])
	}
}

// TestUpdateAccountBalancesTx_Empty vérifie le court-circuit sur liste vide.
func TestUpdateAccountBalancesTx_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	if err := UpdateAccountBalancesTx(userID, nil); err != nil {
		t.Errorf("liste vide : want nil, got %v", err)
	}
}

// TestUpdateAccountBalancesTx_ForeignUser vérifie le cloisonnement : les
// soldes d'un autre utilisateur ne sont jamais touchés.
func TestUpdateAccountBalancesTx_ForeignUser(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "A", 111, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	idA := accounts[0].ID

	// userID+999 n'est pas le propriétaire : la clause WHERE user_id doit
	// neutraliser l'écriture (0 ligne affectée, pas d'erreur).
	if err := UpdateAccountBalancesTx(userID+999, []AccountBalanceUpdate{{ID: idA, Cents: 9999}}); err != nil {
		t.Fatalf("UpdateAccountBalancesTx: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].Balance != 111 {
		t.Errorf("solde d'autrui modifié : want 111, got %d", accounts[0].Balance)
	}
}

func TestDeleteAccount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "ToDelete", 50000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id := accounts[0].ID

	// Associated recurring should be cascade-deleted
	CreateRecurring(userID, id, nil, "Salary", 300000, 1)

	if err := DeleteAccount(id, userID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if len(accounts) != 0 {
		t.Errorf("want 0 accounts, got %d", len(accounts))
	}

	recurrings, _ := GetRecurringByUserID(userID)
	if len(recurrings) != 0 {
		t.Errorf("want 0 recurrings after account delete, got %d", len(recurrings))
	}
}

func TestCreateAndGetRecurring(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "Acc", 100000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	accID := accounts[0].ID

	if err := CreateRecurring(userID, accID, nil, "Rent", -120000, 5); err != nil {
		t.Fatalf("CreateRecurring: %v", err)
	}

	recs, err := GetRecurringByUserID(userID)
	if err != nil {
		t.Fatalf("GetRecurringByUserID: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 recurring, got %d", len(recs))
	}
	if recs[0].Amount != -120000 {
		t.Errorf("amount: want -120000, got %v", recs[0].Amount)
	}
	if recs[0].Description != "Rent" {
		t.Errorf("description: want Rent, got %q", recs[0].Description)
	}
	if recs[0].DayOfMonth != 5 {
		t.Errorf("day_of_month: want 5, got %d", recs[0].DayOfMonth)
	}
}

func TestUpdateRecurring(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "Acc", 100000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "Acc2", 100000, "#111", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	accID := accounts[0].ID
	accID2 := accounts[1].ID

	CreateRecurring(userID, accID, nil, "Old", 10000, 1)
	recs, _ := GetRecurringByUserID(userID)
	recID := recs[0].ID

	// accountID=0 : le compte source reste inchangé (chemin PUT).
	if err := UpdateRecurring(recID, userID, 0, "New", 20000, 15, nil); err != nil {
		t.Fatalf("UpdateRecurring: %v", err)
	}

	recs, _ = GetRecurringByUserID(userID)
	if recs[0].Amount != 20000 {
		t.Errorf("amount: want 20000, got %v", recs[0].Amount)
	}
	if recs[0].Description != "New" {
		t.Errorf("description: want New, got %q", recs[0].Description)
	}
	if recs[0].DayOfMonth != 15 {
		t.Errorf("day_of_month: want 15, got %d", recs[0].DayOfMonth)
	}
	if recs[0].AccountID != accID {
		t.Errorf("account inchangé attendu %d, got %d", accID, recs[0].AccountID)
	}

	// accountID>0 : le compte source est mis à jour (audit FIN-3).
	if err := UpdateRecurring(recID, userID, accID2, "New", 20000, 15, nil); err != nil {
		t.Fatalf("UpdateRecurring (change account): %v", err)
	}
	recs, _ = GetRecurringByUserID(userID)
	if recs[0].AccountID != accID2 {
		t.Errorf("account: want %d, got %d", accID2, recs[0].AccountID)
	}
}

func TestTransferRecurring(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "From", 500000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "To", 100000, "#fff", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	fromID := accounts[0].ID
	toID := accounts[1].ID

	if err := CreateRecurring(userID, fromID, &toID, "Transfer", 50000, 10); err != nil {
		t.Fatalf("CreateRecurring transfer: %v", err)
	}

	recs, _ := GetRecurringByUserID(userID)
	if len(recs) != 1 {
		t.Fatalf("want 1, got %d", len(recs))
	}
	if recs[0].ToAccountID == nil {
		t.Fatal("ToAccountID should not be nil")
	}
	if *recs[0].ToAccountID != toID {
		t.Errorf("ToAccountID: want %d, got %d", toID, *recs[0].ToAccountID)
	}
}

func TestDeleteUserAndData(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "Acc", 100000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	CreateRecurring(userID, accounts[0].ID, nil, "Sal", 200000, 1)
	LogAudit(userID, "TEST", "127.0.0.1", "go-test")
	FlushAuditLog() // M6 : LogAudit est async — synchroniser avant DeleteUserAndData

	if err := DeleteUserAndData(userID); err != nil {
		t.Fatalf("DeleteUserAndData: %v", err)
	}

	user, err := GetUserByID(userID)
	if err != nil || user != nil {
		t.Errorf("user should be deleted, got %v err=%v", user, err)
	}

	accs, _ := GetAccountsByUserID(userID)
	if len(accs) != 0 {
		t.Errorf("accounts should be deleted, got %d", len(accs))
	}
}

func TestReorderAccounts(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "A", 10000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "B", 20000, "#fff", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")

	accounts, _ := GetAccountsByUserID(userID)
	id0, id1 := accounts[0].ID, accounts[1].ID

	if err := ReorderAccounts(userID, []int64{id1, id0}); err != nil {
		t.Fatalf("ReorderAccounts: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].ID != id1 {
		t.Errorf("after reorder, first account should be B (id=%d), got id=%d", id1, accounts[0].ID)
	}
}
