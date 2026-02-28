package db

import (
	"testing"
	"time"
)

func TestCreateAndGetAuthenticators(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	err := CreateAuthenticator("cred-id-1", "pubkey-data", 0, "platform", false, false, "internal", userID)
	if err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	auths, err := GetAuthenticatorsByUserID(userID)
	if err != nil {
		t.Fatalf("GetAuthenticatorsByUserID: %v", err)
	}
	if len(auths) != 1 {
		t.Fatalf("want 1 authenticator, got %d", len(auths))
	}
	if auths[0].CredentialID != "cred-id-1" {
		t.Errorf("credential_id: want cred-id-1, got %q", auths[0].CredentialID)
	}
}

func TestGetAuthenticatorByCredentialID(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAuthenticator("cred-unique", "pubkey", 5, "cross-platform", true, true, "usb", userID)

	a, err := GetAuthenticatorByCredentialID("cred-unique")
	if err != nil {
		t.Fatalf("GetAuthenticatorByCredentialID: %v", err)
	}
	if a.Counter != 5 {
		t.Errorf("counter: want 5, got %d", a.Counter)
	}
	if a.UserID != userID {
		t.Errorf("user_id: want %d, got %d", userID, a.UserID)
	}
}

func TestUpdateAuthenticatorCounter(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAuthenticator("cred-counter", "pk", 0, "platform", false, false, "internal", userID)

	if err := UpdateAuthenticatorCounter("cred-counter", 42); err != nil {
		t.Fatalf("UpdateAuthenticatorCounter: %v", err)
	}

	a, _ := GetAuthenticatorByCredentialID("cred-counter")
	if a.Counter != 42 {
		t.Errorf("counter: want 42, got %d", a.Counter)
	}
}

func TestDeleteAuthenticator(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAuthenticator("cred-del", "pk", 0, "platform", false, false, "internal", userID)
	auths, _ := GetAuthenticatorsByUserID(userID)
	id := auths[0].ID

	if err := DeleteAuthenticator(id, userID); err != nil {
		t.Fatalf("DeleteAuthenticator: %v", err)
	}

	auths, _ = GetAuthenticatorsByUserID(userID)
	if len(auths) != 0 {
		t.Errorf("want 0 after delete, got %d", len(auths))
	}
}

func TestRenameAuthenticator(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	CreateAuthenticator("cred-rename", "pk", 0, "platform", false, false, "internal", userID)
	auths, _ := GetAuthenticatorsByUserID(userID)
	id := auths[0].ID

	if err := RenameAuthenticator(id, userID, "MyYubiKey"); err != nil {
		t.Fatalf("RenameAuthenticator: %v", err)
	}

	auths, _ = GetAuthenticatorsByUserID(userID)
	if auths[0].Name != "MyYubiKey" {
		t.Errorf("name: want MyYubiKey, got %q", auths[0].Name)
	}
}

func TestUpdatePasswordHash(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	if err := UpdatePasswordHash(userID, "newhash"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}

	user, _ := GetUserByID(userID)
	if user.Password != "newhash" {
		t.Errorf("password: want newhash, got %q", user.Password)
	}
}

func TestSetAndClearResetToken(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	expiry := time.Now().Add(time.Hour)
	if err := SetResetToken(userID, "hashedtoken123", expiry); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}

	user, _ := GetUserByID(userID)
	if user.ResetToken == nil {
		t.Fatal("reset_token should be set")
	}

	if err := ClearResetToken(userID); err != nil {
		t.Fatalf("ClearResetToken: %v", err)
	}

	user, _ = GetUserByID(userID)
	if user.ResetToken != nil {
		t.Error("reset_token should be nil after clear")
	}
}

func TestGetUserByResetToken(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	expiry := time.Now().Add(time.Hour)
	if err := SetResetToken(userID, "validhash456", expiry); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}

	user, err := GetUserByResetToken("validhash456")
	if err != nil {
		t.Fatalf("GetUserByResetToken: %v", err)
	}
	if user == nil {
		t.Fatal("user should not be nil")
	}
	if user.ID != userID {
		t.Errorf("user ID: want %d, got %d", userID, user.ID)
	}
}
