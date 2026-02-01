package crypto

import (
	"testing"
)

// Clés de test (NE PAS utiliser en production)
const (
	testEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testBlindIndexKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
)

func TestInit(t *testing.T) {
	err := Init(testEncryptionKey, testBlindIndexKey)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	tests := []string{
		"Hello, World!",
		"test@example.com",
		"Données sensibles avec accénts",
		"12345.67",
		"",
	}

	for _, plaintext := range tests {
		encrypted, err := Encrypt(plaintext)
		if err != nil {
			t.Errorf("Encrypt(%q) failed: %v", plaintext, err)
			continue
		}

		decrypted, err := Decrypt(encrypted)
		if err != nil {
			t.Errorf("Decrypt failed: %v", err)
			continue
		}

		if decrypted != plaintext {
			t.Errorf("Decrypt(Encrypt(%q)) = %q, want %q", plaintext, decrypted, plaintext)
		}
	}
}

func TestDecryptNodeJSFormat(t *testing.T) {
	// Ce test sera utilisé avec des données réelles chiffrées par Node.js
	// Pour valider la compatibilité entre les deux implémentations

	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	// Format: IV:TAG:CIPHERTEXT
	// Les données de test doivent être générées par le code Node.js actuel
	t.Skip("Test à activer avec données Node.js réelles")
}

func TestComputeBlindIndex(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	// Le même input doit toujours produire le même index
	input := "test@example.com"
	index1 := ComputeBlindIndex(input)
	index2 := ComputeBlindIndex(input)

	if index1 != index2 {
		t.Errorf("ComputeBlindIndex not deterministic: %s != %s", index1, index2)
	}

	// Des inputs différents doivent produire des index différents
	index3 := ComputeBlindIndex("autre@example.com")
	if index1 == index3 {
		t.Error("Different inputs produced same blind index")
	}
}

func TestHashToken(t *testing.T) {
	token := "abc123xyz"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	if hash1 != hash2 {
		t.Error("HashToken not deterministic")
	}

	// Vérifier la longueur (SHA256 = 64 caractères hex)
	if len(hash1) != 64 {
		t.Errorf("HashToken length = %d, want 64", len(hash1))
	}
}

func TestHashPassword(t *testing.T) {
	password := "SecureP@ss123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if !VerifyPassword(password, hash) {
		t.Error("VerifyPassword failed for correct password")
	}

	if VerifyPassword("wrongpassword", hash) {
		t.Error("VerifyPassword succeeded for wrong password")
	}
}

func TestDecryptNonEncrypted(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	// Les données non chiffrées doivent être retournées telles quelles
	plaintext := "not encrypted"
	result, err := Decrypt(plaintext)
	if err != nil {
		t.Errorf("Decrypt failed for non-encrypted: %v", err)
	}
	if result != plaintext {
		t.Errorf("Decrypt(%q) = %q, want %q", plaintext, result, plaintext)
	}
}
