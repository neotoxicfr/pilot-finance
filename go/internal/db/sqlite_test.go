package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pilot-finance/internal/crypto"
)

// TestEncryptIfPlainAlreadyEncrypted covers the isEncrypted(raw) true branch.
func TestEncryptIfPlainAlreadyEncrypted(t *testing.T) {
	// Valid AES-256-GCM format: 24-char hex IV : 32-char hex TAG : hex ciphertext
	raw := "0123456789abcdef01234567:0123456789abcdef0123456789abcdef:aabbccdd"
	result, err := encryptIfPlain(raw, func(s string) (string, error) {
		return "should-not-be-called", nil
	})
	if err != nil {
		t.Fatalf("already encrypted: unexpected error %v", err)
	}
	if result != raw {
		t.Errorf("already encrypted: want %q, got %q", raw, result)
	}
}

// TestEncryptIfPlainPlaintext covers the normal encryption path.
func TestEncryptIfPlainPlaintext(t *testing.T) {
	result, err := encryptIfPlain("1234.56", func(s string) (string, error) {
		return "encrypted-value", nil
	})
	if err != nil {
		t.Fatalf("plain value: unexpected error %v", err)
	}
	if result != "encrypted-value" {
		t.Errorf("plain value: want 'encrypted-value', got %q", result)
	}
}

// TestEncryptIfPlainFnError couvre le durcissement de l'audit S-24 : un
// chiffrement raté ne doit PLUS retomber silencieusement sur le plaintext
// (ce qui laissait des données en clair dans une base déclarée migrée), mais
// remonter l'erreur pour que la migration appelante interrompe le démarrage.
func TestEncryptIfPlainFnError(t *testing.T) {
	sentinel := errors.New("encrypt failed")
	result, err := encryptIfPlain("bad", func(s string) (string, error) {
		return "", sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("fn error: want sentinel error, got %v", err)
	}
	if result != "" {
		t.Errorf("fn error: want empty result (fail closed), got %q", result)
	}
}

// TestIsEncrypted covers the isEncrypted helper function.
func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name string
		input string
		want bool
	}{
		{"valid encrypted", "0123456789abcdef01234567:0123456789abcdef0123456789abcdef:aabb", true},
		{"plain IP", "192.168.1.1", false},
		{"user agent with colon", "Mozilla/5.0 (X11; rv:128.0)", false},
		{"empty string", "", false},
		{"two parts", "abc:def", false},
		{"four parts", "a:b:c:d", false},
		{"wrong IV length", "0123:0123456789abcdef0123456789abcdef:aabb", false},
		{"wrong TAG length", "0123456789abcdef01234567:0123:aabb", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEncrypted(tt.input); got != tt.want {
				t.Errorf("isEncrypted(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestEncryptIfPlainUserAgentWithColon ensures user agents containing ":" are encrypted.
func TestEncryptIfPlainUserAgentWithColon(t *testing.T) {
	ua := "Mozilla/5.0 (X11; rv:128.0) Gecko/20100101 Firefox/128.0"
	result, err := encryptIfPlain(ua, func(s string) (string, error) {
		return "encrypted-ua", nil
	})
	if err != nil {
		t.Fatalf("UA with colon: unexpected error %v", err)
	}
	if result != "encrypted-ua" {
		t.Errorf("UA with colon should be encrypted, got %q", result)
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


// TestGetUserAuthDataNotFound covers the sql.ErrNoRows path in GetUserAuthData.
func TestGetUserAuthDataNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sv, email, verified, err := GetUserAuthData(999999)
	if err != nil {
		t.Fatalf("expected nil error for missing user, got: %v", err)
	}
	if sv != 0 || email != "" || verified {
		t.Errorf("want zero values for missing user, got sv=%d email=%q verified=%v", sv, email, verified)
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
	FlushAuditLog() // M6 : LogAudit est async

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

// --- GetAccountsByUserID inline decryption error branches ---
//
// These tests insert an account row directly, then corrupt one encrypted
// column at a time so that the matching DecryptX call inside the scan loop
// fails. They replace the former decryptAccountRow unit tests after the
// pass-2 errgroup decryption was folded into the scan loop.

// insertAccountRaw inserts a single account row with the provided already-encoded
// column values, bypassing the encrypting CreateAccountWithYield helper so that a
// corrupted ciphertext can be planted for error-path coverage.
func insertAccountRaw(t *testing.T, userID int64, balance, yieldMin, yieldMax, reinvest string) {
	t.Helper()
	_, err := DB.Exec(`
		INSERT INTO accounts (user_id, name, balance, color, position, updated_at, is_yield_active, yield_type, yield_min, yield_max, reinvestment_rate, target_account_id, payout_frequency)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, "acc", balance, "#fff", 0, time.Now().Unix(), false, "FIXED", yieldMin, yieldMax, reinvest, nil, "MONTHLY")
	if err != nil {
		t.Fatalf("insertAccountRaw: %v", err)
	}
}

func TestGetAccountsByUserID_BalanceDecryptError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	insertAccountRaw(t, userID, "CORRUPTED-BALANCE",
		encryptTestFloat(t, 0), encryptTestFloat(t, 0), encryptTestInt(t, 0))

	if _, err := GetAccountsByUserID(userID); err == nil {
		t.Error("want error for corrupted balance")
	}
}

func TestGetAccountsByUserID_YieldMinDecryptError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	insertAccountRaw(t, userID, encryptTestCents(t, 100),
		"CORRUPTED-YIELD-MIN", encryptTestFloat(t, 0), encryptTestInt(t, 0))

	if _, err := GetAccountsByUserID(userID); err == nil {
		t.Error("want error for corrupted yieldMin")
	}
}

func TestGetAccountsByUserID_YieldMaxDecryptError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	insertAccountRaw(t, userID, encryptTestCents(t, 100),
		encryptTestFloat(t, 1.5), "CORRUPTED-YIELD-MAX", encryptTestInt(t, 0))

	if _, err := GetAccountsByUserID(userID); err == nil {
		t.Error("want error for corrupted yieldMax")
	}
}

func TestGetAccountsByUserID_ReinvestmentRateDecryptError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	insertAccountRaw(t, userID, encryptTestCents(t, 100),
		encryptTestFloat(t, 1.5), encryptTestFloat(t, 3.0), "CORRUPTED-REINVEST")

	if _, err := GetAccountsByUserID(userID); err == nil {
		t.Error("want error for corrupted reinvestmentRate")
	}
}

// TestGetRecurringByUserID_AmountDecryptError covers the inline amount decrypt
// error branch in GetRecurringByUserID.
func TestGetRecurringByUserID_AmountDecryptError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	// La contrainte FK sur account_id est appliquée (modernc/sqlite ≥ 1.54) :
	// référencer un compte réel plutôt qu'un ID arbitraire.
	insertAccountRaw(t, userID, encryptTestCents(t, 100),
		encryptTestFloat(t, 0), encryptTestFloat(t, 0), encryptTestInt(t, 0))
	accs, err := GetAccountsByUserID(userID)
	if err != nil || len(accs) == 0 {
		t.Fatalf("GetAccountsByUserID: err=%v len=%d", err, len(accs))
	}

	_, err = DB.Exec(`
		INSERT INTO recurring_operations (user_id, account_id, to_account_id, description, amount, day_of_month, is_active)
		VALUES (?, ?, ?, ?, ?, ?, 1)
	`, userID, accs[0].ID, nil, "desc", "CORRUPTED-AMOUNT", 1)
	if err != nil {
		t.Fatalf("insert recurring raw: %v", err)
	}

	if _, err := GetRecurringByUserID(userID); err == nil {
		t.Error("want error for corrupted recurring amount")
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

// encryptTestInt is a helper that encrypts an int for DB error-path tests.
func encryptTestInt(t *testing.T, n int) string {
	t.Helper()
	s, err := crypto.EncryptInt(n)
	if err != nil {
		t.Fatalf("encryptTestInt(%v): %v", n, err)
	}
	return s
}

// encryptTestCents is a helper that encrypts a centime amount for DB error-path tests.
func encryptTestCents(t *testing.T, cents int64) string {
	t.Helper()
	s, err := crypto.EncryptCents(cents)
	if err != nil {
		t.Fatalf("encryptTestCents(%v): %v", cents, err)
	}
	return s
}

// TestCheckDirWritable couvre la sonde d'inscriptibilité ajoutée après la bascule
// en utilisateur non privilégié (audit S-34) : un volume hérité d'une version
// antérieure appartient à root et faisait sortir le serveur en boucle sur une
// erreur SQLite opaque.
func TestCheckDirWritable(t *testing.T) {
	t.Run("dossier inscriptible", func(t *testing.T) {
		if err := checkDirWritable(t.TempDir()); err != nil {
			t.Errorf("un TempDir doit être inscriptible, got %v", err)
		}
	})
	t.Run("dossier inexistant", func(t *testing.T) {
		if err := checkDirWritable(filepath.Join(t.TempDir(), "absent")); err == nil {
			t.Error("un dossier inexistant doit remonter une erreur")
		}
	})
}

// TestNewDataDirPermError vérifie que le message porte l'uid effectif et la
// commande de correction — c'est tout l'intérêt du message.
func TestNewDataDirPermError(t *testing.T) {
	cause := errors.New("permission denied")
	err := newDataDirPermError("/data", cause)
	msg := err.Error()
	// Toujours présents, quelle que soit la plateforme.
	for _, want := range []string{"/data", "permission denied"} {
		if !strings.Contains(msg, want) {
			t.Errorf("le message doit contenir %q, got: %s", want, msg)
		}
	}
	if !errors.Is(err, cause) {
		t.Error("la cause doit rester enveloppée (errors.Is)")
	}
	// La commande de correction n'est affichée que là où un uid POSIX existe :
	// sur Windows os.Getuid() vaut -1 et « chown -R -1:-1 » n'aurait aucun sens.
	if os.Getuid() >= 0 {
		for _, want := range []string{"chown -R", "docker-compose.yml"} {
			if !strings.Contains(msg, want) {
				t.Errorf("sur plateforme POSIX le message doit contenir %q, got: %s", want, msg)
			}
		}
	} else if strings.Contains(msg, "chown") {
		t.Errorf("sans uid POSIX, aucune commande chown ne doit être suggérée: %s", msg)
	}
}
