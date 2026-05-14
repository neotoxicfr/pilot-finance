package db

import "testing"

func TestLogAndGetAuditLog(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "127.0.0.1", "go-test/1.0")
	LogAudit(userID, AuditLogout, "127.0.0.1", "go-test/1.0")
	FlushAuditLog() // M6 : LogAudit est async

	entries, err := GetAuditLog(1, 50)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	// Both actions should be present
	actions := map[string]bool{entries[0].Action: true, entries[1].Action: true}
	if !actions[AuditLoginSuccess] || !actions[AuditLogout] {
		t.Errorf("expected both LOGIN_SUCCESS and LOGOUT, got %v", actions)
	}
}

func TestCountAuditLog(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	count, err := CountAuditLog()
	if err != nil {
		t.Fatalf("CountAuditLog: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0, got %d", count)
	}

	LogAudit(userID, AuditAccountCreate, "192.168.1.1", "Mozilla/5.0")
	LogAudit(userID, AuditAccountDelete, "192.168.1.1", "Mozilla/5.0")
	FlushAuditLog() // M6 : LogAudit est async

	count, _ = CountAuditLog()
	if count != 2 {
		t.Errorf("want 2, got %d", count)
	}
}

func TestAuditLogPagination(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	for i := 0; i < 5; i++ {
		LogAudit(userID, AuditLoginSuccess, "127.0.0.1", "agent")
	}
	FlushAuditLog() // M6 : LogAudit est async

	page1, err := GetAuditLog(1, 3)
	if err != nil {
		t.Fatalf("GetAuditLog page 1: %v", err)
	}
	if len(page1) != 3 {
		t.Errorf("page 1: want 3, got %d", len(page1))
	}

	page2, err := GetAuditLog(2, 3)
	if err != nil {
		t.Fatalf("GetAuditLog page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page 2: want 2, got %d", len(page2))
	}
}

func TestSwapAccountPositions(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "A", 100, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	CreateAccountWithYield(userID, "B", 200, "#fff", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")

	accounts, _ := GetAccountsByUserID(userID)
	id0, id1 := accounts[0].ID, accounts[1].ID

	if err := SwapAccountPositions(id0, id1, userID); err != nil {
		t.Fatalf("SwapAccountPositions: %v", err)
	}

	accounts, _ = GetAccountsByUserID(userID)
	if accounts[0].ID != id1 {
		t.Errorf("after swap, first account should be B (id=%d), got id=%d", id1, accounts[0].ID)
	}
}

func TestDeleteRecurring(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAccountWithYield(userID, "Acc", 1000, "#000", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	accounts, _ := GetAccountsByUserID(userID)
	accID := accounts[0].ID

	CreateRecurring(userID, accID, nil, "Salary", 3000, 1)
	recs, _ := GetRecurringByUserID(userID)
	recID := recs[0].ID

	if err := DeleteRecurring(recID, userID); err != nil {
		t.Fatalf("DeleteRecurring: %v", err)
	}

	recs, _ = GetRecurringByUserID(userID)
	if len(recs) != 0 {
		t.Errorf("want 0 after delete, got %d", len(recs))
	}
}
