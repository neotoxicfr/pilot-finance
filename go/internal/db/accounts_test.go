package db

import "testing"

func TestCreateAndGetAccount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	err := CreateAccountWithYield(userID, "Savings", 5000.50, "#4CAF50", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
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
	if accounts[0].Balance != 5000.50 {
		t.Errorf("balance: want 5000.50, got %v", accounts[0].Balance)
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

	err := CreateAccountWithYield(userID, "PEA", 10000, "#FF5722", 0, true, "RANGE", 3.5, 7.0, 50, nil, "MONTHLY")
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

	CreateAccountWithYield(userID, "Old", 1000, "#000", 0, true, "FIXED", 3.0, 3.0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id := accounts[0].ID

	err := UpdateAccountWithYield(id, userID, "New", 2000, "#fff", false, "FIXED", 0, 0, 0, nil, "MONTHLY")
	if err != nil {
		t.Fatalf("UpdateAccountWithYield: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].Balance != 2000 {
		t.Errorf("balance: want 2000, got %v", accounts[0].Balance)
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

	CreateAccountWithYield(userID, "Acc", 100, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id := accounts[0].ID

	if err := UpdateAccountBalance(id, userID, 9999.99); err != nil {
		t.Fatalf("UpdateAccountBalance: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].Balance != 9999.99 {
		t.Errorf("balance: want 9999.99, got %v", accounts[0].Balance)
	}
}

func TestDeleteAccount(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "ToDelete", 500, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id := accounts[0].ID

	// Associated recurring should be cascade-deleted
	CreateRecurring(userID, id, nil, "Salary", 3000, 1)

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

	CreateAccountWithYield(userID, "Acc", 1000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	accID := accounts[0].ID

	if err := CreateRecurring(userID, accID, nil, "Rent", -1200, 5); err != nil {
		t.Fatalf("CreateRecurring: %v", err)
	}

	recs, err := GetRecurringByUserID(userID)
	if err != nil {
		t.Fatalf("GetRecurringByUserID: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 recurring, got %d", len(recs))
	}
	if recs[0].Amount != -1200 {
		t.Errorf("amount: want -1200, got %v", recs[0].Amount)
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

	CreateAccountWithYield(userID, "Acc", 1000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	accID := accounts[0].ID

	CreateRecurring(userID, accID, nil, "Old", 100, 1)
	recs, _ := GetRecurringByUserID(userID)
	recID := recs[0].ID

	if err := UpdateRecurring(recID, userID, "New", 200, 15, nil); err != nil {
		t.Fatalf("UpdateRecurring: %v", err)
	}

	recs, _ = GetRecurringByUserID(userID)
	if recs[0].Amount != 200 {
		t.Errorf("amount: want 200, got %v", recs[0].Amount)
	}
	if recs[0].Description != "New" {
		t.Errorf("description: want New, got %q", recs[0].Description)
	}
	if recs[0].DayOfMonth != 15 {
		t.Errorf("day_of_month: want 15, got %d", recs[0].DayOfMonth)
	}
}

func TestTransferRecurring(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "From", 5000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "To", 1000, "#fff", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	fromID := accounts[0].ID
	toID := accounts[1].ID

	if err := CreateRecurring(userID, fromID, &toID, "Transfer", 500, 10); err != nil {
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

	CreateAccountWithYield(userID, "Acc", 1000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	CreateRecurring(userID, accounts[0].ID, nil, "Sal", 2000, 1)
	LogAudit(userID, "TEST", "127.0.0.1", "go-test")

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

	CreateAccountWithYield(userID, "A", 100, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "B", 200, "#fff", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")

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
