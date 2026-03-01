package db

import (
	"context"
	"testing"
	"time"
)

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

func TestPurgeAuditLog_RemovesOldEntries(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	// Insérer une entrée ancienne (120 jours) directement
	oldTS := time.Now().AddDate(0, 0, -120).Unix()
	DB.Exec(`INSERT INTO audit_log (user_id, action, ip, user_agent, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, AuditLoginSuccess, "1.2.3.4", "agent", oldTS)

	// Insérer une entrée récente
	LogAudit(userID, AuditLogout, "1.2.3.4", "agent")

	count, _ := CountAuditLog()
	if count != 2 {
		t.Fatalf("want 2 entries before purge, got %d", count)
	}

	deleted, err := PurgeAuditLog(90)
	if err != nil {
		t.Fatalf("PurgeAuditLog: %v", err)
	}
	if deleted != 1 {
		t.Errorf("want 1 deleted, got %d", deleted)
	}

	count, _ = CountAuditLog()
	if count != 1 {
		t.Errorf("want 1 entry after purge, got %d", count)
	}
}

func TestPurgeAuditLog_NoOldEntries(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "1.2.3.4", "agent")

	deleted, err := PurgeAuditLog(90)
	if err != nil {
		t.Fatalf("PurgeAuditLog: %v", err)
	}
	if deleted != 0 {
		t.Errorf("want 0 deleted, got %d", deleted)
	}
}

func TestStartAuditRotation_StopsOnCancel(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	StartAuditRotation(ctx)

	// Cancel immédiatement — la goroutine doit s'arrêter proprement
	cancel()
	// Pas de deadlock ni panic = succès
	time.Sleep(10 * time.Millisecond)
}
