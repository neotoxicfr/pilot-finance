package db

import (
	"context"
	"testing"
	"time"

	"pilot-finance/internal/crypto"
)

// TestWriteAuditEntry_EncryptionFailureNeverStoresPlaintext couvre le
// durcissement de l'audit S-24 : si le chiffrement échoue, writeAuditEntry
// retombait sur la valeur EN CLAIR (fail-open), alors que le contrat annoncé
// est « IP et UserAgent chiffrés avant stockage ». Le repli doit être la chaîne
// vide, la trace (utilisateur/action/date) restant enregistrée.
func TestWriteAuditEntry_EncryptionFailureNeverStoresPlaintext(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	const plainIP = "203.0.113.7"
	const plainUA = "Mozilla/5.0 poste-de-l-utilisateur"

	// Draine les écritures audit asynchrones d'éventuels tests précédents avant
	// de toucher à l'état du paquet crypto (les workers de LogAudit survivent
	// aux tests : ils appelleraient crypto.Encrypt en concurrence du reset).
	FlushAuditLog()

	// Neutralise le paquet crypto : crypto.Encrypt échoue (ErrNotInitialized).
	crypto.ResetForTest()
	t.Cleanup(func() {
		crypto.ResetForTest()
		if err := crypto.Init(testEncKey, testBlindKey); err != nil {
			t.Errorf("crypto.Init (restauration): %v", err)
		}
	})

	writeAuditEntry(auditJob{
		userID:    userID,
		action:    AuditLoginFail,
		ip:        plainIP,
		userAgent: plainUA,
		createdAt: time.Now().Unix(),
	})

	var ip, ua string
	if err := DB.QueryRow(`SELECT COALESCE(ip, ''), COALESCE(user_agent, '')
		FROM audit_log WHERE user_id = ? ORDER BY id DESC LIMIT 1`, userID).Scan(&ip, &ua); err != nil {
		t.Fatalf("lecture de l'entrée d'audit: %v", err)
	}
	if ip == plainIP || ua == plainUA {
		t.Errorf("fail-open: donnée personnelle stockée en clair (ip=%q, user_agent=%q)", ip, ua)
	}
	if ip != "" || ua != "" {
		t.Errorf("repli attendu = chaîne vide, got ip=%q user_agent=%q", ip, ua)
	}
}

func TestGetAuditLogByUserID_ReturnsOnlyUserEntries(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "1.2.3.4", "agent/1.0")
	LogAudit(userID, AuditLogout, "1.2.3.4", "agent/1.0")
	// Different user — should not appear in results
	LogAudit(99999, AuditLoginFail, "5.6.7.8", "other/1.0")
	FlushAuditLog() // M6 : LogAudit est async

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

	// Insérer deux entrées avec des created_at explicites (pattern direct INSERT)
	// pour garantir un ordre strictement DESC sans dépendre d'un time.Sleep.
	now := time.Now().Unix()
	older := now - 60
	newer := now
	if _, err := DB.Exec(`INSERT INTO audit_log (user_id, action, ip, user_agent, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, AuditLoginSuccess, "1.2.3.4", "agent", older); err != nil {
		t.Fatalf("insert older entry: %v", err)
	}
	if _, err := DB.Exec(`INSERT INTO audit_log (user_id, action, ip, user_agent, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, AuditLogout, "1.2.3.4", "agent", newer); err != nil {
		t.Fatalf("insert newer entry: %v", err)
	}

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
	FlushAuditLog() // M6 : LogAudit est async

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
	FlushAuditLog() // M6 : LogAudit est async

	deleted, err := PurgeAuditLog(90)
	if err != nil {
		t.Fatalf("PurgeAuditLog: %v", err)
	}
	if deleted != 0 {
		t.Errorf("want 0 deleted, got %d", deleted)
	}
}

func TestLogAudit_EncryptsIPAndUserAgent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()
	userID := createTestUser(t)

	LogAudit(userID, AuditLoginSuccess, "192.168.1.42", "Mozilla/5.0 (X11; Linux)")
	FlushAuditLog() // M6 : LogAudit est async

	// Vérifier que les valeurs en BDD sont chiffrées (contiennent ":")
	var rawIP, rawUA string
	DB.QueryRow(`SELECT COALESCE(ip, ''), COALESCE(user_agent, '') FROM audit_log WHERE user_id = ?`, userID).Scan(&rawIP, &rawUA)
	if rawIP == "192.168.1.42" {
		t.Error("IP should be encrypted in DB, got plaintext")
	}
	if rawUA == "Mozilla/5.0 (X11; Linux)" {
		t.Error("UserAgent should be encrypted in DB, got plaintext")
	}

	// Vérifier que GetAuditLog déchiffre correctement
	entries, err := GetAuditLog(1, 50)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("want at least 1 entry")
	}
	if entries[0].IP != "192.168.1.42" {
		t.Errorf("IP: want '192.168.1.42', got %q", entries[0].IP)
	}
	if entries[0].UserAgent != "Mozilla/5.0 (X11; Linux)" {
		t.Errorf("UserAgent: want 'Mozilla/5.0 (X11; Linux)', got %q", entries[0].UserAgent)
	}

	// Vérifier que GetAuditLogByUserID déchiffre aussi
	byUser, err := GetAuditLogByUserID(userID)
	if err != nil {
		t.Fatalf("GetAuditLogByUserID: %v", err)
	}
	if len(byUser) == 0 {
		t.Fatal("want at least 1 entry by user")
	}
	if byUser[0].IP != "192.168.1.42" {
		t.Errorf("ByUser IP: want '192.168.1.42', got %q", byUser[0].IP)
	}
	if byUser[0].UserAgent != "Mozilla/5.0 (X11; Linux)" {
		t.Errorf("ByUser UserAgent: want 'Mozilla/5.0 (X11; Linux)', got %q", byUser[0].UserAgent)
	}
}

func TestStartAuditRotation_StopsOnCancel(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Observer l'arrêt de la goroutine via le hook de test plutôt qu'un sleep.
	stopped := make(chan struct{})
	auditRotationStopped = func() { close(stopped) }
	defer func() { auditRotationStopped = nil }()

	ctx, cancel := context.WithCancel(context.Background())
	StartAuditRotation(ctx)

	// Cancel immédiatement — la goroutine doit s'arrêter proprement et signaler.
	cancel()

	select {
	case <-stopped:
		// Arrêt propre confirmé.
	case <-time.After(2 * time.Second):
		t.Fatal("audit rotation goroutine did not stop within timeout after cancel")
	}
}
