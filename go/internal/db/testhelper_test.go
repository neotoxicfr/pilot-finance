package db

import (
	"testing"

	"pilot-finance/internal/crypto"
)

const (
	testEncKey   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testBlindKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

// setupTestDB initialise une base de données SQLite en fichier temporaire pour les tests.
// Retourne une fonction de nettoyage à appeler avec defer.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	crypto.ResetForTest()
	ResetForTest()
	if err := crypto.Init(testEncKey, testBlindKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
	dir := t.TempDir()
	if err := Init(Config{Path: dir + "/test.db"}); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	return func() { Close() }
}

// createTestUser crée un utilisateur de test et retourne son ID.
func createTestUser(t *testing.T) int64 {
	t.Helper()
	emailEnc, err := crypto.Encrypt("test@example.com")
	if err != nil {
		t.Fatalf("crypto.Encrypt: %v", err)
	}
	emailBI := crypto.ComputeBlindIndex("test@example.com")
	id, err := CreateUser(emailEnc, emailBI, "testhash", "user")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}
