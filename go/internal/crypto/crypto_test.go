package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
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

func TestInitInvalidEncKey(t *testing.T) {
	err := Init("not-valid-hex!!", testBlindIndexKey)
	if err == nil {
		t.Fatal("expected error for invalid encryption key hex")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got: %v", err)
	}
}

func TestInitEncKeyTooShort(t *testing.T) {
	// 16 hex chars = 8 bytes, not 32
	err := Init("0102030405060708090a0b0c0d0e0f10", testBlindIndexKey)
	if err == nil {
		t.Fatal("expected error for short encryption key")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got: %v", err)
	}
}

func TestInitInvalidBlindKey(t *testing.T) {
	err := Init(testEncryptionKey, "not-valid-hex!!")
	if err == nil {
		t.Fatal("expected error for invalid blind index key hex")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got: %v", err)
	}
}

func TestInitBlindKeyTooShort(t *testing.T) {
	// 8 hex chars = 4 bytes, not 32
	err := Init(testEncryptionKey, "01020304")
	if err == nil {
		t.Fatal("expected error for short blind key")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got: %v", err)
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
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	t.Skip("Test à activer avec données Node.js réelles")
}

func TestDecryptNonEncrypted(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	plaintext := "not encrypted"
	result, err := Decrypt(plaintext)
	if err != nil {
		t.Errorf("Decrypt failed for non-encrypted: %v", err)
	}
	if result != plaintext {
		t.Errorf("Decrypt(%q) = %q, want %q", plaintext, result, plaintext)
	}
}

func TestDecryptWrongPartCount(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// 2 parts → not 3 → return as-is, no error
	result, err := Decrypt("a:b")
	if err != nil {
		t.Fatalf("2-part string: unexpected error: %v", err)
	}
	if result != "a:b" {
		t.Errorf("want 'a:b', got %q", result)
	}
	// 4 parts → not 3 → return as-is, no error
	result, err = Decrypt("a:b:c:d")
	if err != nil {
		t.Fatalf("4-part string: unexpected error: %v", err)
	}
	if result != "a:b:c:d" {
		t.Errorf("want 'a:b:c:d', got %q", result)
	}
}

func TestDecryptBadHexIV(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// 'g' is not valid hex → IV decode fails → ErrDecryption
	_, err := Decrypt("gggggggggggggggggggggggg:aabbccddeeff001122334455aabbccdd:aabb")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("bad IV hex: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptBadHexAuthTag(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Valid 12-byte IV (24 hex chars), invalid auth tag
	_, err := Decrypt("0123456789abcdef01234567:gggggggggggggggggggggggggggggggg:aabb")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("bad authTag hex: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptBadHexCiphertext(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Valid 12-byte IV, valid 16-byte auth tag, invalid ciphertext
	_, err := Decrypt("0123456789abcdef01234567:aabbccddeeff001122334455aabbccdd:gg")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("bad ciphertext hex: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptZeroLengthIV(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Empty IV → len=0 → NewGCMWithNonceSize(block, 0) returns error (nonce cannot be zero length)
	_, err := Decrypt(":aabbccddeeff001122334455aabbccdd:aabb")
	if err == nil {
		t.Error("zero-length IV should return an error")
	}
}

func TestDecryptTamperedData(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("secret data to tamper")
	if err != nil {
		t.Fatal(err)
	}
	// Flip first byte of auth tag → GCM authentication fails → ErrDecryption
	parts := strings.Split(enc, ":")
	tag := []byte(parts[1])
	if tag[0] == '0' {
		tag[0] = '1'
	} else {
		tag[0] = '0'
	}
	parts[1] = string(tag)
	tampered := strings.Join(parts, ":")
	_, err = Decrypt(tampered)
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("tampered auth tag: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptLegacy16ByteNonce(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Build a ciphertext with 16-byte nonce (legacy Node.js format)
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	const plaintext = "legacy node data"
	ciphertextWithTag := gcm.Seal(nil, iv, []byte(plaintext), nil)
	ct := ciphertextWithTag[:len(ciphertextWithTag)-16]
	authTag := ciphertextWithTag[len(ciphertextWithTag)-16:]
	encrypted := fmt.Sprintf("%s:%s:%s",
		hex.EncodeToString(iv),
		hex.EncodeToString(authTag),
		hex.EncodeToString(ct),
	)
	result, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("legacy 16-byte nonce: %v", err)
	}
	if result != plaintext {
		t.Errorf("want %q, got %q", plaintext, result)
	}
}

func TestComputeBlindIndex(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}

	input := "test@example.com"
	index1 := ComputeBlindIndex(input)
	index2 := ComputeBlindIndex(input)

	if index1 != index2 {
		t.Errorf("ComputeBlindIndex not deterministic: %s != %s", index1, index2)
	}

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
	got, err := DecryptFloat("1234.56")
	if err != nil {
		t.Fatalf("DecryptFloat plaintext: %v", err)
	}
	if got != 1234.56 {
		t.Errorf("want 1234.56, got %v", got)
	}
}

func TestDecryptFloatError(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Bad hex IV → ErrDecryption propagated through DecryptFloat
	_, err := DecryptFloat("gg:aabbccddeeff001122334455aabbccdd:aabb")
	if err == nil {
		t.Error("bad encrypted float should return error")
	}
}

func TestDecryptFloatBadValue(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Plain text that is not a valid float → ParseFloat error
	_, err := DecryptFloat("notafloat")
	if err == nil {
		t.Error("non-float plaintext should return strconv error")
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

func TestDecryptIntError(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	_, err := DecryptInt("gg:aabbccddeeff001122334455aabbccdd:aabb")
	if err == nil {
		t.Error("bad encrypted int should return error")
	}
}

func TestDecryptIntBadValue(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	// Plain text that is not a valid int → Atoi error
	_, err := DecryptInt("notanint")
	if err == nil {
		t.Error("non-integer plaintext should return strconv error")
	}
}

func TestValidatePassword(t *testing.T) {
	// "Aa1!" repeated 18 times = exactly 72 bytes
	max72 := ""
	for i := 0; i < 18; i++ {
		max72 += "Aa1!"
	}
	tests := []struct {
		pwd string
		ok  bool
	}{
		{"short", false},
		{"alllowercase1!", false},  // no uppercase
		{"ALLUPPERCASE1!", false},  // no lowercase
		{"NoDigitHereAt!", false},  // no digit
		{"NoSpecialChar12", false}, // no special char
		{"ValidP@ssw0rd1!", true},  // classic special chars
		{"Another$ecure1!", true},  // dollar sign
		{"Valid-Pass_w0rd", true},  // dash and underscore as special
		{"MyP4ssWithTilde~", true}, // tilde as special
		{max72, true},             // exactly 72 bytes — OK
		{max72 + "x", false},      // 73 bytes — too long for bcrypt
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

func TestNeedsRehashLowCost(t *testing.T) {
	// cost 4 (below 12) → NeedsRehash must return true
	hash, err := bcrypt.GenerateFromPassword([]byte("testpassword"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if !NeedsRehash(string(hash)) {
		t.Error("cost-4 hash should need rehash")
	}
}

func TestNeedsRehashInvalidHash(t *testing.T) {
	// Invalid bcrypt hash → bcrypt.Cost returns error → NeedsRehash returns false
	if NeedsRehash("not-a-bcrypt-hash") {
		t.Error("invalid hash should return false")
	}
}

// --- error branches via hooks / key corruption ---

func TestEncrypt_NewCipherError(t *testing.T) {
	// Corrupt the key to trigger aes.NewCipher failure.
	orig := encryptionKey
	encryptionKey = []byte("bad-key") // wrong length
	defer func() { encryptionKey = orig }()

	_, err := Encrypt("test")
	if err == nil {
		t.Error("Encrypt: want error with bad key, got nil")
	}
}

func TestEncrypt_NewGCMError(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	orig := cipherNewGCMFn
	defer func() { cipherNewGCMFn = orig }()

	cipherNewGCMFn = func(_ cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("forced gcm error")
	}

	_, err := Encrypt("test")
	if err == nil || err.Error() != "forced gcm error" {
		t.Errorf("Encrypt: want 'forced gcm error', got %v", err)
	}
}

func TestEncrypt_RandReadError(t *testing.T) {
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatal(err)
	}
	orig := cryptoRandRead
	defer func() { cryptoRandRead = orig }()

	cryptoRandRead = func(_ []byte) (int, error) {
		return 0, errors.New("forced rand error")
	}

	_, err := Encrypt("test")
	if err == nil || err.Error() != "forced rand error" {
		t.Errorf("Encrypt: want 'forced rand error', got %v", err)
	}
}

func TestDecrypt_NewCipherError(t *testing.T) {
	// Corrupt the key to trigger aes.NewCipher failure in Decrypt.
	orig := encryptionKey
	encryptionKey = []byte("bad") // wrong length
	defer func() { encryptionKey = orig }()

	// Valid 3-part hex string: IV (12 bytes), authTag (16 bytes), ciphertext (1 byte)
	encrypted := "616161616161616161616161:61616161616161616161616161616161:61"
	_, err := Decrypt(encrypted)
	if err == nil {
		t.Error("Decrypt: want error with bad key, got nil")
	}
}

func TestHashPassword_Error(t *testing.T) {
	orig := bcryptGenerateFn
	defer func() { bcryptGenerateFn = orig }()

	bcryptGenerateFn = func(_ []byte, _ int) ([]byte, error) {
		return nil, errors.New("forced bcrypt error")
	}

	_, err := HashPassword("any-password")
	if err == nil || err.Error() != "forced bcrypt error" {
		t.Errorf("HashPassword: want 'forced bcrypt error', got %v", err)
	}
}
