package db

import "testing"

func TestGetAuditLogByUserID_ReturnsOnlyUserEntries(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "1.2.3.4", "agent/1.0")
	LogAudit(userID, AuditLogout, "1.2.3.4", "agent/1.0")
	// Different user — should not appear in results
	LogAudit(99999, AuditLoginFail, "5.6.7.8", "other/1.0")

	entries, err := GetAuditLogByUserID(userID)
	if err != nil {
		t.Fatalf("GetAuditLogByUserID: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries for userID %d, got %d", userID, len(entries))
	}
	for _, e := range entries {
		if e.UserID != userID {
			t.Errorf("entry has wrong userID: want %d, got %d", userID, e.UserID)
		}
	}
}

func TestGetAuditLogByUserID_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	entries, err := GetAuditLogByUserID(99999)
	if err != nil {
		t.Fatalf("GetAuditLogByUserID: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("want 0 entries for unknown user, got %d", len(entries))
	}
}

func TestGetAuditLogByUserID_OrderDesc(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "1.2.3.4", "agent")
	LogAudit(userID, AuditLogout, "1.2.3.4", "agent")

	entries, err := GetAuditLogByUserID(userID)
	if err != nil {
		t.Fatalf("GetAuditLogByUserID: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("want at least 2 entries, got %d", len(entries))
	}
	// Most recent first
	if entries[0].CreatedAt.Before(entries[1].CreatedAt) {
		t.Error("entries should be ordered DESC by created_at")
	}
}
