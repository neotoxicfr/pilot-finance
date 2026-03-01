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
	"strconv"
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

// Hooks injectables pour les tests (permettent de couvrir les branches d'erreur impossibles en prod).
var (
	cipherNewGCMFn  = cipher.NewGCM
	cryptoRandRead  = rand.Read
	bcryptGenerateFn = bcrypt.GenerateFromPassword
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

	gcm, err := cipherNewGCMFn(block)
	if err != nil {
		return "", err
	}

	iv := make([]byte, ivLength)
	if _, err := cryptoRandRead(iv); err != nil {
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
// Supporte les IV de 12 bytes (standard) et 16 bytes (Node.js legacy)
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

	// Créer GCM avec la taille de nonce appropriée (12 ou 16 bytes)
	var gcm cipher.AEAD
	if len(iv) == 12 {
		gcm, err = cipher.NewGCM(block)
	} else {
		gcm, err = cipher.NewGCMWithNonceSize(block, len(iv))
	}
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

// NeedsRehash retourne true si le hash bcrypt utilise un coût inférieur à 12
func NeedsRehash(hash string) bool {
	cost, err := bcrypt.Cost([]byte(hash))
	return err == nil && cost < 12
}

// HashPassword génère un hash bcrypt du mot de passe
func HashPassword(password string) (string, error) {
	hash, err := bcryptGenerateFn([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword vérifie un mot de passe contre son hash bcrypt
func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// EncryptFloat chiffre un float64 après conversion en string.
// Idempotent : si la valeur contient déjà ":" (déjà chiffrée), elle est retournée telle quelle.
func EncryptFloat(f float64) (string, error) {
	return Encrypt(strconv.FormatFloat(f, 'f', -1, 64))
}

// DecryptFloat déchiffre une valeur chiffrée vers un float64.
// Si la valeur ne contient pas ":" (plaintext), elle est parsée directement.
func DecryptFloat(s string) (float64, error) {
	plain, err := Decrypt(s)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(plain, 64)
}

// EncryptInt chiffre un int après conversion en string.
func EncryptInt(i int) (string, error) {
	return Encrypt(strconv.Itoa(i))
}

// DecryptInt déchiffre une valeur chiffrée vers un int.
func DecryptInt(s string) (int, error) {
	plain, err := Decrypt(s)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(plain)
}

// Erreurs de validation du mot de passe — codes machine-readable pour i18n.
// Utiliser i18n.T(lang, "pwd_error."+err.Error()) pour obtenir le message traduit.
var (
	ErrPwdMinLength = errors.New("min_length")
	ErrPwdUppercase = errors.New("uppercase")
	ErrPwdLowercase = errors.New("lowercase")
	ErrPwdDigit     = errors.New("digit")
	ErrPwdSpecial   = errors.New("special")
)

// ValidatePassword vérifie que le mot de passe respecte les 5 critères.
// Returns nil si valide, sinon une erreur codée (ErrPwd*) à traduire via i18n.
func ValidatePassword(password string) error {
	if len(password) < 12 {
		return ErrPwdMinLength
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*(),.?:{}|", c):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrPwdUppercase
	}
	if !hasLower {
		return ErrPwdLowercase
	}
	if !hasDigit {
		return ErrPwdDigit
	}
	if !hasSpecial {
		return ErrPwdSpecial
	}

	return nil
}
