package db

import (
	"errors"
	"testing"
	"time"

	"pilot-finance/internal/crypto"
)

// TestEncryptIfPlainAlreadyEncrypted covers the strings.Contains(raw, ":") true branch.
func TestEncryptIfPlainAlreadyEncrypted(t *testing.T) {
	raw := "iv:tag:cipher"
	result := encryptIfPlain(raw, func(s string) (string, error) {
		return "should-not-be-called", nil
	})
	if result != raw {
		t.Errorf("already encrypted: want %q, got %q", raw, result)
	}
}

// TestEncryptIfPlainPlaintext covers the normal encryption path.
func TestEncryptIfPlainPlaintext(t *testing.T) {
	result := encryptIfPlain("1234.56", func(s string) (string, error) {
		return "encrypted-value", nil
	})
	if result != "encrypted-value" {
		t.Errorf("plain value: want 'encrypted-value', got %q", result)
	}
}

// TestEncryptIfPlainFnError covers the fn error path → returns raw value.
func TestEncryptIfPlainFnError(t *testing.T) {
	result := encryptIfPlain("bad", func(s string) (string, error) {
		return "", errors.New("encrypt failed")
	})
	if result != "bad" {
		t.Errorf("fn error: want original 'bad', got %q", result)
	}
}

// TestCloseNilDB covers the DB==nil path in Close.
func TestCloseNilDB(t *testing.T) {
	// Save and nil out DB
	saved := DB
	DB = nil
	defer func() { DB = saved }()

	err := Close()
	if err != nil {
		t.Errorf("Close(nil DB) should return nil, got: %v", err)
	}
}

// TestGetSessionVersionNotFound covers the sql.ErrNoRows path in GetSessionVersion.
func TestGetSessionVersionNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sv, err := GetSessionVersion(999999)
	if err != nil {
		t.Fatalf("expected nil error for missing user, got: %v", err)
	}
	if sv != 0 {
		t.Errorf("want 0 for non-existent user, got %d", sv)
	}
}

// TestGetUserAuthDataNotFound covers the sql.ErrNoRows path in GetUserAuthData.
func TestGetUserAuthDataNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sv, email, err := GetUserAuthData(999999)
	if err != nil {
		t.Fatalf("expected nil error for missing user, got: %v", err)
	}
	if sv != 0 || email != "" {
		t.Errorf("want zero values for missing user, got sv=%d email=%q", sv, email)
	}
}

// TestUpdateLoginAttemptsWithLock covers the lockUntil != nil branch and
// subsequently GetUserByID → scanUser with lockUntil.Valid == true.
func TestUpdateLoginAttemptsWithLock(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	lockTime := time.Now().Add(15 * time.Minute)
	if err := UpdateLoginAttempts(userID, 5, &lockTime); err != nil {
		t.Fatalf("UpdateLoginAttempts with lock: %v", err)
	}

	user, err := GetUserByID(userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user.FailedLoginAttempts != 5 {
		t.Errorf("failed_login_attempts: want 5, got %d", user.FailedLoginAttempts)
	}
	if user.LockUntil == nil {
		t.Error("lock_until should not be nil after setting a lock")
	}
}

// TestGetAuditLogPageZero covers the page < 1 normalisation in GetAuditLog.
func TestGetAuditLogPageZero(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "127.0.0.1", "go-test")

	// page=0 should be treated as page=1
	entries, err := GetAuditLog(0, 50)
	if err != nil {
		t.Fatalf("GetAuditLog(0, 50): %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("want 1 entry, got %d", len(entries))
	}
}

// TestGetUserByResetTokenNotFound covers the sql.ErrNoRows path.
func TestGetUserByResetTokenNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	user, err := GetUserByResetToken("nonexistent-token-xyz")
	// scanUser returns (nil, nil) for sql.ErrNoRows — consistent with GetUserByID
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if user != nil {
		t.Error("user should be nil for non-existent reset token")
	}
}

// TestGetUserByResetTokenWithLock covers lockUntil.Valid && lockUntil.Int64 > 0 branch.
func TestGetUserByResetTokenWithLock(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	// Set a reset token
	expiry := time.Now().Add(time.Hour)
	if err := SetResetToken(userID, "tokenwithlocktest", expiry); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}
	// Also set a lock_until
	lockTime := time.Now().Add(10 * time.Minute)
	if err := UpdateLoginAttempts(userID, 3, &lockTime); err != nil {
		t.Fatalf("UpdateLoginAttempts: %v", err)
	}

	user, err := GetUserByResetToken("tokenwithlocktest")
	if err != nil {
		t.Fatalf("GetUserByResetToken: %v", err)
	}
	if user == nil {
		t.Fatal("user should not be nil")
	}
	if user.LockUntil == nil {
		t.Error("LockUntil should be set")
	}
}

// --- decryptAccountRow error branches ---

// TestDecryptAccountRow_BalanceError covers the early-return on balance decrypt failure.
func TestDecryptAccountRow_BalanceError(t *testing.T) {
	raw := rawAccount{
		acc:         Account{ID: 1},
		balanceRaw:  "CORRUPTED-BALANCE",
		yieldMinRaw: "0",
		yieldMaxRaw: "0",
		reinvestRaw: "0",
	}
	var dst Account
	if err := decryptAccountRow(&dst, raw); err == nil {
		t.Error("want error for corrupted balance")
	}
}

// TestDecryptAccountRow_YieldMinError covers the yieldMin decrypt error branch.
func TestDecryptAccountRow_YieldMinError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Use a real encrypted balance so that passes, but corrupt yieldMin
	validBalance := encryptTestFloat(t, 100.0)
	raw := rawAccount{
		acc:         Account{ID: 2},
		balanceRaw:  validBalance,
		yieldMinRaw: "CORRUPTED-YIELD-MIN",
		yieldMaxRaw: "0",
		reinvestRaw: "0",
	}
	var dst Account
	if err := decryptAccountRow(&dst, raw); err == nil {
		t.Error("want error for corrupted yieldMin")
	}
}

// TestDecryptAccountRow_YieldMaxError covers the yieldMax decrypt error branch.
func TestDecryptAccountRow_YieldMaxError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	validBalance := encryptTestFloat(t, 100.0)
	validYieldMin := encryptTestFloat(t, 1.5)
	raw := rawAccount{
		acc:         Account{ID: 3},
		balanceRaw:  validBalance,
		yieldMinRaw: validYieldMin,
		yieldMaxRaw: "CORRUPTED-YIELD-MAX",
		reinvestRaw: "0",
	}
	var dst Account
	if err := decryptAccountRow(&dst, raw); err == nil {
		t.Error("want error for corrupted yieldMax")
	}
}

// TestDecryptAccountRow_ReinvestmentRateError covers the reinvestmentRate decrypt error branch.
func TestDecryptAccountRow_ReinvestmentRateError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	validBalance := encryptTestFloat(t, 100.0)
	validYieldMin := encryptTestFloat(t, 1.5)
	validYieldMax := encryptTestFloat(t, 3.0)
	raw := rawAccount{
		acc:         Account{ID: 4},
		balanceRaw:  validBalance,
		yieldMinRaw: validYieldMin,
		yieldMaxRaw: validYieldMax,
		reinvestRaw: "CORRUPTED-REINVEST",
	}
	var dst Account
	if err := decryptAccountRow(&dst, raw); err == nil {
		t.Error("want error for corrupted reinvestmentRate")
	}
}

// encryptTestFloat is a helper that encrypts a float64 for DB error-path tests.
func encryptTestFloat(t *testing.T, f float64) string {
	t.Helper()
	s, err := crypto.EncryptFloat(f)
	if err != nil {
		t.Fatalf("encryptTestFloat(%v): %v", f, err)
	}
	return s
}
