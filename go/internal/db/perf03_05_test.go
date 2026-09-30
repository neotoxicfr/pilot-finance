package db

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// PERF-03 : un User-Agent de 100 Ko ne doit pas être stocké tel quel.
func TestLogAudit_TruncatesUserAgent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginFail, "1.2.3.4", strings.Repeat("é", 50_000))
	FlushAuditLog()

	entries, err := GetAuditLogByUserID(userID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("GetAuditLogByUserID: %v (%d entrées)", err, len(entries))
	}
	ua := entries[0].UserAgent
	if len(ua) > auditUserAgentMax || !utf8.ValidString(ua) || ua == "" {
		t.Errorf("UA stocké : %d octets, UTF-8 valide=%v", len(ua), utf8.ValidString(ua))
	}
}

func TestTruncateUTF8(t *testing.T) {
	if got := truncateUTF8("abc", 5); got != "abc" {
		t.Errorf("chaîne courte modifiée : %q", got)
	}
	// "é" = 2 octets : couper à 3 ne doit pas laisser un octet orphelin.
	if got := truncateUTF8("ééé", 3); got != "é" {
		t.Errorf("coupe UTF-8 : %q", got)
	}
	if got := truncateUTF8("abcdef", 4); got != "abcd" {
		t.Errorf("coupe ASCII : %q", got)
	}
}

// PERF-05 : une rafale est écrite en lots par le worker unique, sans perte.
func TestLogAudit_BurstIsBatchedWithoutLoss(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	const n = 300 // > capacité de la file (256) et > auditBatchMax
	for range n {
		LogAudit(userID, AuditLoginSuccess, "1.2.3.4", "agent/1.0")
	}
	FlushAuditLog()

	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != n {
		t.Errorf("want %d entrées, got %d", n, count)
	}
}

func TestCollectAuditBatch(t *testing.T) {
	q := make(chan auditJob, auditBatchMax+10)
	for i := range auditBatchMax + 5 {
		q <- auditJob{userID: int64(i)}
	}
	if got := collectAuditBatch(q, auditJob{}); len(got) != auditBatchMax {
		t.Errorf("lot plein : want %d, got %d", auditBatchMax, len(got))
	}
	// Il reste 6 jobs : le lot s'arrête quand la file est vide.
	if got := collectAuditBatch(q, auditJob{}); len(got) != 7 {
		t.Errorf("file vidée : want 7, got %d", len(got))
	}
	q <- auditJob{}
	close(q)
	if got := collectAuditBatch(q, auditJob{}); len(got) != 2 {
		t.Errorf("file fermée : want 2, got %d", len(got))
	}
}

// Transaction impossible (base fermée) : le lot est abandonné sans panique.
func TestWriteAuditBatch_BeginError(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	FlushAuditLog()
	if err := DB.Close(); err != nil {
		t.Fatal(err)
	}
	writeAuditBatch([]auditJob{{userID: 1, action: AuditLogout}})
}
