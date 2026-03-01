package crypto

import (
	"testing"
)

func init() {
	_ = Init(testEncryptionKey, testBlindIndexKey)
}

// FuzzEncryptDecrypt vérifie que Decrypt(Encrypt(x)) == x pour toute entrée
func FuzzEncryptDecrypt(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("Livret A — 22 950,00 €")
	f.Add("\x00\xff\xfe")

	f.Fuzz(func(t *testing.T, input string) {
		encrypted, err := Encrypt(input)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", input, err)
		}
		decrypted, err := Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt failed for input %q: %v", input, err)
		}
		if decrypted != input {
			t.Errorf("roundtrip failed: got %q, want %q", decrypted, input)
		}
	})
}

// FuzzEncryptDecryptFloat vérifie le roundtrip pour les flottants
func FuzzEncryptDecryptFloat(f *testing.F) {
	f.Add(0.0)
	f.Add(22950.50)
	f.Add(-100.0)
	f.Add(1e18)

	f.Fuzz(func(t *testing.T, input float64) {
		encrypted, err := EncryptFloat(input)
		if err != nil {
			t.Fatalf("EncryptFloat(%v): %v", input, err)
		}
		decrypted, err := DecryptFloat(encrypted)
		if err != nil {
			t.Fatalf("DecryptFloat failed for %v: %v", input, err)
		}
		if decrypted != input {
			t.Errorf("roundtrip failed: got %v, want %v", decrypted, input)
		}
	})
}

// FuzzValidatePassword vérifie que ValidatePassword ne panique jamais
func FuzzValidatePassword(f *testing.F) {
	f.Add("short")
	f.Add("ValidP@ssw0rd!")
	f.Add("")
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	f.Fuzz(func(t *testing.T, password string) {
		// Ne doit jamais paniquer
		_ = ValidatePassword(password)
	})
}
