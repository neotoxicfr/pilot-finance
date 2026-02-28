package db

// closed_db_test.go — error path coverage via a voluntarily closed *sql.DB.
//
// Pattern: setupTestDB + immediate Close() → DB still points to a *sql.DB whose
// connection pool is closed. All subsequent Exec/Begin/Query calls return an error,
// which lets us exercise error branches that are otherwise never reached in normal
// test flow.

import "testing"

// setupClosedDB creates a test DB, closes it, and returns the now-closed DB state.
// The caller must NOT call cleanup() again; the connection is already closed.
func setupClosedDB(t *testing.T) {
	t.Helper()
	cleanup := setupTestDB(t)
	cleanup() // close the DB immediately
	// DB still points to the closed *sql.DB object — all ops will fail
}

// --- accounts.go error paths ---

func TestCreateAccountWithYield_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := CreateAccountWithYield(1, "T", 100, "#fff", 0, false, "FIXED", 0, 0, 0, nil, "MONTHLY")
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestUpdateAccountWithYield_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := UpdateAccountWithYield(1, 1, "T", 100, "#fff", false, "FIXED", 0, 0, 0, nil, "MONTHLY")
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestUpdateAccountBalance_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := UpdateAccountBalance(1, 1, 100)
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestDeleteAccount_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := DeleteAccount(1, 1)
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestSwapAccountPositions_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := SwapAccountPositions(1, 2, 1)
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestReorderAccounts_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := ReorderAccounts(1, []int64{1, 2, 3})
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestCreateRecurring_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := CreateRecurring(1, 1, nil, "Test", 100, 15)
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestUpdateRecurring_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := UpdateRecurring(1, 1, "Test", 100, 15, nil)
	if err == nil {
		t.Error("want error with closed DB")
	}
}

// --- users.go error paths ---

func TestCreateUser_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	_, err := CreateUser("enc", "bi", "hash", "USER")
	if err == nil {
		t.Error("want error with closed DB")
	}
}

func TestDeleteUserAndData_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	err := DeleteUserAndData(1)
	if err == nil {
		t.Error("want error with closed DB")
	}
}

// --- audit.go error paths ---

func TestLogAudit_ClosedDB(t *testing.T) {
	setupClosedDB(t)
	// fire-and-forget: should NOT panic, just slog.Warn the error
	LogAudit(1, AuditLoginSuccess, "127.0.0.1", "test-agent")
}
