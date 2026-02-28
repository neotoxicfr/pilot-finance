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

func TestEncryptDecryptFloat(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	values := []float64{0, 1.23, -456.789, 100000.50, 0.001}
	for _, v := range values {
		enc, err := EncryptFloat(v)
		if err != nil {
			t.Errorf("EncryptFloat(%v): %v", v, err)
			continue
		}
		got, err := DecryptFloat(enc)
		if err != nil {
			t.Errorf("DecryptFloat failed for %v: %v", v, err)
			continue
		}
		if got != v {
			t.Errorf("round-trip float: want %v, got %v", v, got)
		}
	}
}

func TestDecryptFloatPlaintext(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Legacy plaintext value stored without encryption
	got, err := DecryptFloat("1234.56")
	if err != nil {
		t.Fatalf("DecryptFloat plaintext: %v", err)
	}
	if got != 1234.56 {
		t.Errorf("want 1234.56, got %v", got)
	}
}

func TestEncryptDecryptInt(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	values := []int{0, 1, 50, 100, -10}
	for _, v := range values {
		enc, err := EncryptInt(v)
		if err != nil {
			t.Errorf("EncryptInt(%v): %v", v, err)
			continue
		}
		got, err := DecryptInt(enc)
		if err != nil {
			t.Errorf("DecryptInt failed for %v: %v", v, err)
			continue
		}
		if got != v {
			t.Errorf("round-trip int: want %v, got %v", v, got)
		}
	}
}

func TestDecryptIntPlaintext(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	got, err := DecryptInt("42")
	if err != nil {
		t.Fatalf("DecryptInt plaintext: %v", err)
	}
	if got != 42 {
		t.Errorf("want 42, got %v", got)
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		pwd string
		ok  bool
	}{
		{"short", false},
		{"alllowercase1!", false},     // no uppercase
		{"ALLUPPERCASE1!", false},     // no lowercase
		{"NoDigitHereAt!", false},     // no digit
		{"NoSpecialChar12", false},    // no special char
		{"ValidP@ssw0rd1!", true},
		{"Another$ecure1!", true},
	}
	for _, tt := range tests {
		err := ValidatePassword(tt.pwd)
		if tt.ok && err != nil {
			t.Errorf("ValidatePassword(%q) = %v, want nil", tt.pwd, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("ValidatePassword(%q) = nil, want error", tt.pwd)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	// bcrypt cost 12 — should not need rehash
	hash, err := HashPassword("TestP@ss123!")
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(hash) {
		t.Error("fresh hash at cost 12 should not need rehash")
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
