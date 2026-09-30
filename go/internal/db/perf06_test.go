package db

import "testing"

// PERF-06 : un id répété n'est écrit qu'une fois, à sa première position.
func TestReorderAccounts_Dedup(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "A", 10000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "B", 20000, "#fff", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	id0, id1 := accounts[0].ID, accounts[1].ID

	if err := ReorderAccounts(userID, []int64{id1, id1, id0}); err != nil {
		t.Fatalf("ReorderAccounts: %v", err)
	}
	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].ID != id1 || accounts[0].Position != 0 || accounts[1].Position != 1 {
		t.Errorf("ordre inattendu : %+v", accounts)
	}
}
