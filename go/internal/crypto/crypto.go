// Package crypto fournit les fonctions de chiffrement compatibles avec Node.js
// Format: IV_HEX:AUTH_TAG_HEX:CIPHERTEXT_HEX
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	ivLength      = 12 // GCM standard
	authTagLength = 16 // GCM standard
)

var (
	encryptionKey  []byte
	blindIndexKey  []byte
	ErrInvalidKey  = errors.New("clé invalide: 32 bytes requis")
	ErrDecryption  = errors.New("échec déchiffrement")
)

// Init initialise les clés de chiffrement
func Init(encKeyHex, blindKeyHex string) error {
	var err error

	encryptionKey, err = hex.DecodeString(encKeyHex)
	if err != nil || len(encryptionKey) != 32 {
		return fmt.Errorf("ENCRYPTION_KEY: %w", ErrInvalidKey)
	}

	blindIndexKey, err = hex.DecodeString(blindKeyHex)
	if err != nil || len(blindIndexKey) != 32 {
		return fmt.Errorf("BLIND_INDEX_KEY: %w", ErrInvalidKey)
	}

	return nil
}

// Encrypt chiffre un texte avec AES-256-GCM
// Format de sortie: IV_HEX:AUTH_TAG_HEX:CIPHERTEXT_HEX (compatible Node.js)
func Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	// GCM Seal ajoute le tag à la fin du ciphertext
	ciphertextWithTag := gcm.Seal(nil, iv, []byte(plaintext), nil)

	// Séparer ciphertext et authTag (les 16 derniers bytes sont le tag)
	ciphertext := ciphertextWithTag[:len(ciphertextWithTag)-authTagLength]
	authTag := ciphertextWithTag[len(ciphertextWithTag)-authTagLength:]

	// Format Node.js: IV:TAG:CIPHERTEXT
	return fmt.Sprintf("%s:%s:%s",
		hex.EncodeToString(iv),
		hex.EncodeToString(authTag),
		hex.EncodeToString(ciphertext),
	), nil
}

// Decrypt déchiffre un texte chiffré avec AES-256-GCM
// Accepte le format: IV_HEX:AUTH_TAG_HEX:CIPHERTEXT_HEX
func Decrypt(encrypted string) (string, error) {
	// Si pas de séparateur, retourner tel quel (données non chiffrées)
	if !strings.Contains(encrypted, ":") {
		return encrypted, nil
	}

	parts := strings.Split(encrypted, ":")
	if len(parts) != 3 {
		return encrypted, nil
	}

	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", ErrDecryption
	}

	authTag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", ErrDecryption
	}

	ciphertext, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", ErrDecryption
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Reconstituer ciphertext + tag pour GCM Open
	ciphertextWithTag := append(ciphertext, authTag...)

	plaintext, err := gcm.Open(nil, iv, ciphertextWithTag, nil)
	if err != nil {
		return "", ErrDecryption
	}

	return string(plaintext), nil
}

// ComputeBlindIndex calcule un index aveugle HMAC-SHA256
// Compatible avec Node.js: createHmac('sha256', key).update(input).digest('hex')
func ComputeBlindIndex(input string) string {
	h := hmac.New(sha256.New, blindIndexKey)
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}

// HashToken calcule un hash SHA256 simple
// Compatible avec Node.js: createHash('sha256').update(token).digest('hex')
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// HashPassword génère un hash bcrypt du mot de passe
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword vérifie un mot de passe contre son hash bcrypt
func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
