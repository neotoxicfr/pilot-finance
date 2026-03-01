package db

import (
	"testing"

	"pilot-finance/internal/crypto"
)

func TestGetUserByID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	user, err := GetUserByID(userID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil {
		t.Fatal("user should not be nil")
	}
	if user.ID != userID {
		t.Errorf("ID: want %d, got %d", userID, user.ID)
	}
	if user.Role != "user" {
		t.Errorf("role: want user, got %q", user.Role)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	user, err := GetUserByID(999999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != nil {
		t.Error("should return nil for missing user")
	}
}

func TestGetUserByBlindIndex(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	bi := crypto.ComputeBlindIndex("test@example.com")
	user, err := GetUserByBlindIndex(bi)
	if err != nil {
		t.Fatalf("GetUserByBlindIndex: %v", err)
	}
	if user == nil {
		t.Fatal("user should not be nil")
	}
	if user.ID != userID {
		t.Errorf("ID: want %d, got %d", userID, user.ID)
	}
}

func TestGetUserAuthData(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	sv, emailEnc, err := GetUserAuthData(userID)
	if err != nil {
		t.Fatalf("GetUserAuthData: %v", err)
	}
	if sv == 0 {
		t.Error("session_version should be > 0")
	}
	if emailEnc == "" {
		t.Error("email_encrypted should not be empty")
	}
}

func TestGetSessionVersion(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	sv, err := GetSessionVersion(userID)
	if err != nil {
		t.Fatalf("GetSessionVersion: %v", err)
	}
	if sv == 0 {
		t.Error("session_version should be > 0")
	}
}

func TestCountUsers(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	count, err := CountUsers()
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0 users initially, got %d", count)
	}

	createTestUser(t)
	count, _ = CountUsers()
	if count != 1 {
		t.Errorf("want 1 user after create, got %d", count)
	}
}

func TestUpdateUserPreferences(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	if err := UpdateUserPreferences(userID, "en", "USD"); err != nil {
		t.Fatalf("UpdateUserPreferences: %v", err)
	}

	user, _ := GetUserByID(userID)
	if user.Language != "en" {
		t.Errorf("language: want en, got %q", user.Language)
	}
	if user.Currency != "USD" {
		t.Errorf("currency: want USD, got %q", user.Currency)
	}
}

func TestUpdatePassword(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	sv1, _, _ := GetUserAuthData(userID)

	if err := UpdatePassword(userID, "newhash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	sv2, _, _ := GetUserAuthData(userID)
	if sv2 != sv1+1 {
		t.Errorf("session_version should increment: want %d, got %d", sv1+1, sv2)
	}
}

func TestGetAllUsers(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	users, err := GetAllUsers()
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("want 0 initially, got %d", len(users))
	}

	createTestUser(t)
	users, _ = GetAllUsers()
	if len(users) != 1 {
		t.Errorf("want 1 after create, got %d", len(users))
	}
}

func TestDeleteUserAndData_SingleUser(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	if err := DeleteUserAndData(userID); err != nil {
		t.Fatalf("DeleteUserAndData: %v", err)
	}

	user, _ := GetUserByID(userID)
	if user != nil {
		t.Error("user should be nil after delete")
	}
}

func TestUpdateLoginAttempts(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	if err := UpdateLoginAttempts(userID, 3, nil); err != nil {
		t.Fatalf("UpdateLoginAttempts: %v", err)
	}

	user, _ := GetUserByID(userID)
	if user.FailedLoginAttempts != 3 {
		t.Errorf("failed_login_attempts: want 3, got %d", user.FailedLoginAttempts)
	}
}

func TestEnableDisableMFA(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	if err := EnableMFA(userID, "encryptedsecret"); err != nil {
		t.Fatalf("EnableMFA: %v", err)
	}

	user, _ := GetUserByID(userID)
	if !user.MFAEnabled {
		t.Error("MFA should be enabled")
	}

	if err := DisableMFA(userID); err != nil {
		t.Fatalf("DisableMFA: %v", err)
	}

	user, _ = GetUserByID(userID)
	if user.MFAEnabled {
		t.Error("MFA should be disabled")
	}
}

func TestVerifyEmailByToken(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	emailEnc, _ := crypto.Encrypt("verify@example.com")
	emailBI := crypto.ComputeBlindIndex("verify@example.com")
	userID, _ := CreateUser(emailEnc, emailBI, "hash", "user")

	_, err := DB.Exec(`UPDATE users SET email_verified=0, verification_token=? WHERE id=?`, "mytoken123", userID)
	if err != nil {
		t.Fatalf("set verification_token: %v", err)
	}

	if err := VerifyEmailByToken("mytoken123"); err != nil {
		t.Fatalf("VerifyEmailByToken: %v", err)
	}

	user, _ := GetUserByID(userID)
	if !user.EmailVerified {
		t.Error("email should be verified")
	}
}

func TestVerifyEmailByTokenInvalid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	if err := VerifyEmailByToken("nonexistent-token"); err == nil {
		t.Error("should return error for invalid token")
	}
}
