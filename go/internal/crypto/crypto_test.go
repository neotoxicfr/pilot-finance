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

// mustInit resets crypto state and initializes with valid test keys.
func mustInit(t *testing.T) {
	t.Helper()
	ResetForTest()
	if err := Init(testEncryptionKey, testBlindIndexKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
}

func TestInit(t *testing.T) {
	ResetForTest()
	err := Init(testEncryptionKey, testBlindIndexKey)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestInitInvalidEncKey(t *testing.T) {
	ResetForTest()
	err := Init("not-valid-hex!!", testBlindIndexKey)
	if err == nil {
		t.Fatal("expected error for invalid encryption key hex")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got: %v", err)
	}
}

func TestInitEncKeyTooShort(t *testing.T) {
	ResetForTest()
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
	ResetForTest()
	err := Init(testEncryptionKey, "not-valid-hex!!")
	if err == nil {
		t.Fatal("expected error for invalid blind index key hex")
	}
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("want ErrInvalidKey, got: %v", err)
	}
}

func TestInitBlindKeyTooShort(t *testing.T) {
	ResetForTest()
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
	mustInit(t)

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

func TestDecryptNonEncrypted(t *testing.T) {
	mustInit(t)

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
	mustInit(t)
	// A string that contains ':' but is not the 3-part format is malformed
	// ciphertext: it must fail closed with ErrDecryption (not echo the input).
	cases := []string{
		"a:b",     // 2 parts
		"a:b:c:d", // 4 parts
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			result, err := Decrypt(in)
			if !errors.Is(err, ErrDecryption) {
				t.Errorf("Decrypt(%q): want ErrDecryption, got result=%q err=%v", in, result, err)
			}
			if result != "" {
				t.Errorf("Decrypt(%q): want empty result, got %q", in, result)
			}
		})
	}
}

func TestDecryptBadHexIV(t *testing.T) {
	mustInit(t)
	// 'g' is not valid hex → IV decode fails → ErrDecryption
	_, err := Decrypt("gggggggggggggggggggggggg:aabbccddeeff001122334455aabbccdd:aabb")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("bad IV hex: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptBadHexAuthTag(t *testing.T) {
	mustInit(t)
	// Valid 12-byte IV (24 hex chars), invalid auth tag
	_, err := Decrypt("0123456789abcdef01234567:gggggggggggggggggggggggggggggggg:aabb")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("bad authTag hex: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptBadHexCiphertext(t *testing.T) {
	mustInit(t)
	// Valid 12-byte IV, valid 16-byte auth tag, invalid ciphertext
	_, err := Decrypt("0123456789abcdef01234567:aabbccddeeff001122334455aabbccdd:gg")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("bad ciphertext hex: want ErrDecryption, got: %v", err)
	}
}

func TestDecryptZeroLengthIV(t *testing.T) {
	mustInit(t)
	// Empty IV → len(iv)==0 → not an accepted nonce size (12 or 16) → ErrDecryption.
	_, err := Decrypt(":aabbccddeeff001122334455aabbccdd:aabb")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("zero-length IV: want ErrDecryption, got %v", err)
	}
}

func TestDecryptUnsupportedNonceSize(t *testing.T) {
	mustInit(t)
	// IV of an unsupported length (13 bytes = 26 hex chars) must be rejected with
	// ErrDecryption instead of being fed to NewGCMWithNonceSize with an
	// attacker-influenced nonce size.
	thirteenByteIV := strings.Repeat("ab", 13)
	_, err := Decrypt(thirteenByteIV + ":aabbccddeeff001122334455aabbccdd:aabb")
	if !errors.Is(err, ErrDecryption) {
		t.Errorf("13-byte IV: want ErrDecryption, got %v", err)
	}
}

func TestDecryptTamperedData(t *testing.T) {
	mustInit(t)
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
	mustInit(t)
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

// TestDecryptLegacy16ByteNonce_GCMError covers the error branch of the legacy
// 16-byte path: NewGCMWithNonceSize never fails for a valid block+size in prod,
// so inject a failing factory to exercise it (matches the package's hook pattern).
func TestDecryptLegacy16ByteNonce_GCMError(t *testing.T) {
	mustInit(t)
	orig := cipherNewGCMWithNonceSizeFn
	t.Cleanup(func() { cipherNewGCMWithNonceSizeFn = orig })
	cipherNewGCMWithNonceSizeFn = func(cipher.Block, int) (cipher.AEAD, error) {
		return nil, errors.New("forced gcm error")
	}
	// A well-formed entry with a 16-byte IV (32 hex chars) reaches the case 16 branch.
	iv := strings.Repeat("ab", 16)
	tag := strings.Repeat("cd", 16)
	ct := strings.Repeat("ef", 8)
	if _, err := Decrypt(iv + ":" + tag + ":" + ct); err == nil {
		t.Fatal("expected error from injected NewGCMWithNonceSize failure")
	}
}

func TestComputeBlindIndex(t *testing.T) {
	mustInit(t)

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
	mustInit(t)

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
	mustInit(t)
	got, err := DecryptFloat("1234.56")
	if err != nil {
		t.Fatalf("DecryptFloat plaintext: %v", err)
	}
	if got != 1234.56 {
		t.Errorf("want 1234.56, got %v", got)
	}
}

func TestDecryptFloatError(t *testing.T) {
	mustInit(t)
	// Bad hex IV → ErrDecryption propagated through DecryptFloat
	_, err := DecryptFloat("gg:aabbccddeeff001122334455aabbccdd:aabb")
	if err == nil {
		t.Error("bad encrypted float should return error")
	}
}

func TestDecryptFloatBadValue(t *testing.T) {
	mustInit(t)
	// Plain text that is not a valid float → ParseFloat error
	_, err := DecryptFloat("notafloat")
	if err == nil {
		t.Error("non-float plaintext should return strconv error")
	}
}

func TestEncryptDecryptInt(t *testing.T) {
	mustInit(t)

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
	mustInit(t)
	got, err := DecryptInt("42")
	if err != nil {
		t.Fatalf("DecryptInt plaintext: %v", err)
	}
	if got != 42 {
		t.Errorf("want 42, got %v", got)
	}
}

func TestDecryptIntError(t *testing.T) {
	mustInit(t)
	_, err := DecryptInt("gg:aabbccddeeff001122334455aabbccdd:aabb")
	if err == nil {
		t.Error("bad encrypted int should return error")
	}
}

func TestDecryptIntBadValue(t *testing.T) {
	mustInit(t)
	// Plain text that is not a valid int → Atoi error
	_, err := DecryptInt("notanint")
	if err == nil {
		t.Error("non-integer plaintext should return strconv error")
	}
}

// --- EncryptCents / DecryptCents ---

func TestEncryptDecryptCents(t *testing.T) {
	mustInit(t)

	values := []int64{0, 1, 100, 123456, -50000, 999999999}
	for _, v := range values {
		enc, err := EncryptCents(v)
		if err != nil {
			t.Errorf("EncryptCents(%d): %v", v, err)
			continue
		}
		got, err := DecryptCents(enc)
		if err != nil {
			t.Errorf("DecryptCents failed for %d: %v", v, err)
			continue
		}
		if got != v {
			t.Errorf("round-trip cents: want %d, got %d", v, got)
		}
	}
}

func TestDecryptCents_LegacyFloatFormat(t *testing.T) {
	mustInit(t)
	// Legacy format: stored as float string "1234.56" → should convert to 123456 cents
	enc, err := Encrypt("1234.56")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := DecryptCents(enc)
	if err != nil {
		t.Fatalf("DecryptCents legacy: %v", err)
	}
	if got != 123456 {
		t.Errorf("legacy float: want 123456, got %d", got)
	}
}

func TestDecryptCents_LegacyFloatPlaintext(t *testing.T) {
	mustInit(t)
	// Plaintext float (no colon separator) → Decrypt returns as-is, then parsed as float
	got, err := DecryptCents("99.99")
	if err != nil {
		t.Fatalf("DecryptCents plaintext float: %v", err)
	}
	if got != 9999 {
		t.Errorf("plaintext float: want 9999, got %d", got)
	}
}

func TestDecryptCents_PlaintextInt(t *testing.T) {
	mustInit(t)
	// Plaintext int string (no period, no colon) → parsed as int64
	got, err := DecryptCents("42")
	if err != nil {
		t.Fatalf("DecryptCents plaintext int: %v", err)
	}
	if got != 42 {
		t.Errorf("plaintext int: want 42, got %d", got)
	}
}

func TestDecryptCents_DecryptError(t *testing.T) {
	mustInit(t)
	// Bad hex → Decrypt returns ErrDecryption
	_, err := DecryptCents("gg:aabbccddeeff001122334455aabbccdd:aabb")
	if err == nil {
		t.Error("bad encrypted cents should return error")
	}
}

func TestDecryptCents_LegacyBadFloat(t *testing.T) {
	mustInit(t)
	// Encrypt a string containing "." but not a valid float → ParseFloat error
	enc, err := Encrypt("not.a.float")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = DecryptCents(enc)
	if err == nil {
		t.Error("bad legacy float should return error")
	}
}

func TestDecryptCents_BadIntString(t *testing.T) {
	mustInit(t)
	// Encrypt a non-numeric string without "." → ParseInt error
	enc, err := Encrypt("notanumber")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = DecryptCents(enc)
	if err == nil {
		t.Error("bad int string should return error")
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

func TestEncrypt_NilCipherBlock(t *testing.T) {
	ResetForTest()
	// cipherBlock is nil after ResetForTest (Init not called) : audit S-24, le
	// cas « non initialisé » remonte désormais ErrNotInitialized et non plus
	// ErrDecryption, pour être distinguable d'une donnée corrompue.
	_, err := Encrypt("test")
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("want ErrNotInitialized, got %v", err)
	}
	mustInit(t) // restore for subsequent tests
}

// TestDecrypt_NotInitialized couvre la garde « non initialisé » de Decrypt sur
// une entrée BIEN FORMÉE (3 parties, IV de 12 octets) : sans Init, elle doit
// remonter ErrNotInitialized et non ErrDecryption.
func TestDecrypt_NotInitialized(t *testing.T) {
	ResetForTest()
	iv := strings.Repeat("ab", 12)
	tag := strings.Repeat("cd", 16)
	ct := strings.Repeat("ef", 8)
	_, err := Decrypt(iv + ":" + tag + ":" + ct)
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("want ErrNotInitialized, got %v", err)
	}
	mustInit(t) // restore for subsequent tests
}

// TestComputeBlindIndexE_NotInitialized couvre la garde ajoutée par l'audit
// S-24 : hmac.New accepte une clé nil et produit un digest d'apparence valide.
func TestComputeBlindIndexE_NotInitialized(t *testing.T) {
	ResetForTest()
	idx, err := ComputeBlindIndexE("test@example.com")
	if !errors.Is(err, ErrNotInitialized) {
		t.Errorf("ComputeBlindIndexE: want ErrNotInitialized, got %v", err)
	}
	if idx != "" {
		t.Errorf("ComputeBlindIndexE: want empty index, got %q", idx)
	}
	// La façade historique échoue en mode fermé : chaîne vide, jamais un digest.
	if got := ComputeBlindIndex("test@example.com"); got != "" {
		t.Errorf("ComputeBlindIndex non initialisé: want %q, got %q", "", got)
	}
	mustInit(t) // restore for subsequent tests
}

// TestComputeBlindIndexE_Initialized vérifie que la variante avec erreur
// produit exactement le même index que la façade historique.
func TestComputeBlindIndexE_Initialized(t *testing.T) {
	mustInit(t)
	idx, err := ComputeBlindIndexE("test@example.com")
	if err != nil {
		t.Fatalf("ComputeBlindIndexE: %v", err)
	}
	if len(idx) != 64 {
		t.Errorf("blind index length = %d, want 64", len(idx))
	}
	if got := ComputeBlindIndex("test@example.com"); got != idx {
		t.Errorf("ComputeBlindIndex = %q, want %q", got, idx)
	}
}

// --- error branches via hooks / key corruption ---

func TestEncrypt_NilGCM(t *testing.T) {
	ResetForTest()
	mustInit(t)
	origGCM := gcmStandard
	gcmStandard = nil
	t.Cleanup(func() { gcmStandard = origGCM })

	_, err := Encrypt("test")
	if err == nil {
		t.Error("Encrypt: want error with nil gcmStandard, got nil")
	}
}

func TestInit_NewGCMError(t *testing.T) {
	ResetForTest()
	orig := cipherNewGCMFn
	cipherNewGCMFn = func(_ cipher.Block) (cipher.AEAD, error) {
		return nil, errors.New("forced gcm error")
	}
	t.Cleanup(func() { cipherNewGCMFn = orig })

	err := Init(testEncryptionKey, testBlindIndexKey)
	if err == nil || !strings.Contains(err.Error(), "forced gcm error") {
		t.Errorf("Init: want 'forced gcm error', got %v", err)
	}
}

func TestEncrypt_RandReadError(t *testing.T) {
	mustInit(t)
	orig := cryptoRandRead
	t.Cleanup(func() { cryptoRandRead = orig })

	cryptoRandRead = func(_ []byte) (int, error) {
		return 0, errors.New("forced rand error")
	}

	_, err := Encrypt("test")
	if err == nil || err.Error() != "forced rand error" {
		t.Errorf("Encrypt: want 'forced rand error', got %v", err)
	}
}

func TestDecrypt_NilGCM(t *testing.T) {
	ResetForTest()
	mustInit(t)
	encrypted, err := Encrypt("test")
	if err != nil {
		t.Fatal(err)
	}
	origGCM := gcmStandard
	gcmStandard = nil
	t.Cleanup(func() { gcmStandard = origGCM })

	_, err = Decrypt(encrypted)
	if err == nil {
		t.Error("Decrypt: want error with nil gcmStandard, got nil")
	}
}

func TestInit_AESNewCipherError(t *testing.T) {
	ResetForTest()

	orig := aesNewCipherFn
	t.Cleanup(func() { aesNewCipherFn = orig })
	aesNewCipherFn = func(key []byte) (cipher.Block, error) {
		return nil, errors.New("forced AES cipher error")
	}

	err := Init(testEncryptionKey, testBlindIndexKey)
	if err == nil {
		t.Error("Init: want error when AES cipher creation fails, got nil")
	}
	if !strings.Contains(err.Error(), "AES cipher init") {
		t.Errorf("Init: want 'AES cipher init' error, got: %v", err)
	}
}

func TestHashPassword_Error(t *testing.T) {
	orig := bcryptGenerateFn
	t.Cleanup(func() { bcryptGenerateFn = orig })

	bcryptGenerateFn = func(_ []byte, _ int) ([]byte, error) {
		return nil, errors.New("forced bcrypt error")
	}

	_, err := HashPassword("any-password")
	if err == nil || err.Error() != "forced bcrypt error" {
		t.Errorf("HashPassword: want 'forced bcrypt error', got %v", err)
	}
}
