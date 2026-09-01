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
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	ivLength      = 12 // GCM standard
	authTagLength = 16 // GCM standard
)

var (
	encryptionKey  []byte
	blindIndexKey  []byte
	cipherBlock    cipher.Block // pre-computed AES cipher block
	gcmStandard    cipher.AEAD // pre-computed GCM for standard 12-byte nonce
	initOnce       sync.Once
	initErr        error
	ErrInvalidKey  = errors.New("clé invalide: 32 bytes requis")
	ErrDecryption  = errors.New("échec déchiffrement")
	// ErrNotInitialized distingue « paquet crypto non initialisé » (bug de
	// câblage : Init n'a pas été appelé ou a échoué) d'un « échec de
	// déchiffrement » (donnée corrompue / clé erronée). Auparavant les deux cas
	// remontaient ErrDecryption, ce qui rendait le premier indiscernable du
	// second dans les logs (audit S-24).
	ErrNotInitialized = errors.New("crypto non initialisé: Init() n'a pas été appelé")
)

// ResetForTest resets the crypto package state so Init can be called again.
// ONLY for use in tests.
func ResetForTest() {
	initOnce = sync.Once{}
	initErr = nil
	encryptionKey = nil
	blindIndexKey = nil
	cipherBlock = nil
	gcmStandard = nil
}

// Hooks injectables pour les tests (permettent de couvrir les branches d'erreur impossibles en prod).
var (
	aesNewCipherFn              = aes.NewCipher
	cipherNewGCMFn              = cipher.NewGCM
	cipherNewGCMWithNonceSizeFn = cipher.NewGCMWithNonceSize
	cryptoRandRead              = rand.Read
	bcryptGenerateFn            = bcrypt.GenerateFromPassword
)

// Init initialise les clés de chiffrement et pré-calcule le cipher block AES.
// Protégé par sync.Once : les appels suivants sont no-op et retournent le résultat initial.
func Init(encKeyHex, blindKeyHex string) error {
	initOnce.Do(func() {
		var err error

		encryptionKey, err = hex.DecodeString(encKeyHex)
		if err != nil || len(encryptionKey) != 32 {
			initErr = fmt.Errorf("ENCRYPTION_KEY: %w", ErrInvalidKey)
			return
		}

		blindIndexKey, err = hex.DecodeString(blindKeyHex)
		if err != nil || len(blindIndexKey) != 32 {
			initErr = fmt.Errorf("BLIND_INDEX_KEY: %w", ErrInvalidKey)
			return
		}

		// Pre-compute AES cipher block to avoid recreating per Encrypt/Decrypt call
		cipherBlock, err = aesNewCipherFn(encryptionKey)
		if err != nil {
			initErr = fmt.Errorf("AES cipher init: %w", err)
			return
		}

		// Pre-compute GCM for standard 12-byte nonce (avoids GF(2^128) table rebuild per call)
		gcmStandard, err = cipherNewGCMFn(cipherBlock)
		if err != nil {
			initErr = fmt.Errorf("GCM init: %w", err)
			return
		}
	})
	return initErr
}

// Encrypt chiffre un texte avec AES-256-GCM
// Format de sortie: IV_HEX:AUTH_TAG_HEX:CIPHERTEXT_HEX (compatible Node.js)
func Encrypt(plaintext string) (string, error) {
	if gcmStandard == nil {
		return "", ErrNotInitialized
	}
	gcm := gcmStandard

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
//
// CONTRAT (audit S-24) — Decrypt est volontairement TOLÉRANT sur un seul cas :
// une valeur SANS ':' est considérée comme du plaintext legacy et retournée
// telle quelle (avec un slog.Warn). Ce passthrough est LOAD-BEARING et ne peut
// pas être supprimé sans casser la lecture de bases réelles : sur une instance
// migrée depuis la version Node, accounts.name et
// recurring_operations.description ne sont chiffrés par AUCUNE migration
// (008 ne traite que balance/yield_min/yield_max/reinvestment_rate, 009 que
// recurring_operations.amount, 011/012 que audit_log). Ces colonnes restent
// donc en clair indéfiniment et transitent par Decrypt à chaque lecture ;
// échouer ici afficherait « ??? » à la place de tous les noms de comptes
// (handlers/helpers.go decryptAccountNames). Le durcissement fail-closed
// complet suppose d'abord une migration qui chiffre ces deux colonnes.
// Tous les AUTRES cas échouent fermés (ErrDecryption) : format à N != 3
// parties, hex invalide, taille d'IV non supportée, tag GCM invalide.
func Decrypt(encrypted string) (string, error) {
	// Si pas de séparateur, retourner tel quel (données non chiffrées) — voir
	// le contrat documenté ci-dessus : legacy Node uniquement.
	if !strings.Contains(encrypted, ":") {
		slog.Warn("crypto.Decrypt: value appears unencrypted, returning as plaintext (legacy Node data)", "len", len(encrypted))
		return encrypted, nil
	}

	parts := strings.Split(encrypted, ":")
	if len(parts) != 3 {
		// Has a ':' separator but not the expected 3-part format: this is malformed
		// ciphertext, not legacy plaintext. Fail closed instead of echoing the input.
		slog.Warn("crypto.Decrypt: malformed ciphertext (expected 3 parts)", "parts", len(parts))
		return "", ErrDecryption
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

	// Use pre-computed GCM for standard 12-byte nonce, fallback for legacy 16-byte
	if gcmStandard == nil {
		return "", ErrNotInitialized
	}
	var gcm cipher.AEAD
	switch len(iv) {
	case 12:
		gcm = gcmStandard
	case 16:
		// Legacy Node.js ciphertexts used a 16-byte GCM nonce. Only this documented
		// legacy size is accepted — any other length is rejected to avoid using an
		// attacker-influenced nonce size.
		gcm, err = cipherNewGCMWithNonceSizeFn(cipherBlock, len(iv))
		if err != nil {
			return "", err
		}
	default:
		return "", ErrDecryption
	}

	// Reconstituer ciphertext + tag pour GCM Open
	ciphertextWithTag := make([]byte, 0, len(ciphertext)+len(authTag))
	ciphertextWithTag = append(ciphertextWithTag, ciphertext...)
	ciphertextWithTag = append(ciphertextWithTag, authTag...)

	plaintext, err := gcm.Open(nil, iv, ciphertextWithTag, nil)
	if err != nil {
		return "", ErrDecryption
	}

	return string(plaintext), nil
}

// ComputeBlindIndexE calcule un index aveugle HMAC-SHA256 et échoue
// explicitement si le paquet n'est pas initialisé.
// Compatible avec Node.js: createHmac('sha256', key).update(input).digest('hex')
//
// Audit S-24 : hmac.New accepte une clé nil sans paniquer et produit un digest
// d'apparence parfaitement valide. Sans cette garde, un appel avant Init()
// écrivait un index aveugle calculé avec une clé vide — indétectable à l'œil,
// et incohérent avec tous les index calculés après Init. Encrypt et Decrypt
// avaient déjà cette garde ; l'asymétrie induisait en erreur.
// Préférer cette variante dans tout nouveau code.
func ComputeBlindIndexE(input string) (string, error) {
	if len(blindIndexKey) == 0 {
		return "", ErrNotInitialized
	}
	h := hmac.New(sha256.New, blindIndexKey)
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeBlindIndex conserve la signature historique (appelée via des hooks de
// test dans handlers/ et middleware/). Elle échoue en mode FERMÉ : si le paquet
// n'est pas initialisé, elle retourne la chaîne vide — jamais un digest
// plausible calculé avec une clé nulle. Une chaîne vide ne peut correspondre à
// aucun index aveugle stocké (toujours 64 caractères hex), donc une recherche
// ne renvoie aucune ligne au lieu de renvoyer une ligne arbitraire.
func ComputeBlindIndex(input string) string {
	idx, err := ComputeBlindIndexE(input)
	if err != nil {
		slog.Error("crypto.ComputeBlindIndex appelé avant Init: index vide retourné", "err", err)
		return ""
	}
	return idx
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
// ATTENTION : cette fonction n'est PAS idempotente (le commentaire antérieur
// l'affirmait à tort, audit S-24) — elle chiffre toujours. La détection
// « déjà chiffré » est assurée en amont par db.encryptIfPlain/isEncrypted, qui
// est le seul endroit à s'en charger.
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

// EncryptCents chiffre un montant en centimes.
func EncryptCents(cents int64) (string, error) {
	return Encrypt(strconv.FormatInt(cents, 10))
}

// DecryptCents déchiffre une valeur vers des centimes (int64).
// Gère le format legacy (float string "1234.56") et le nouveau format (cents string "123456").
//
// LIMITE CONNUE (audit FIN-15, étendue par S-32) : l'heuristique repose sur la
// présence d'un point décimal. Deux migrations ont chiffré des montants legacy
// en EUROS float via EncryptFloat, et strconv.FormatFloat n'émet pas de point
// pour une valeur entière :
//
//	migration 008 → accounts.balance                  (soldes)
//	migration 009 → recurring_operations.amount       (salaires, loyers, abos)
//
// Dans les DEUX cas, un montant entier comme 3000 € est stocké "3000" (sans
// point) et sera relu ici comme 3000 CENTIMES (30,00 €), soit ÷100. Les
// montants récurrents sont le cas le plus fréquent (un salaire ou un loyer est
// presque toujours un entier), donc PLUS exposé que les soldes.
//
// Ces cas ne concernent QUE les bases importées de l'ancienne version Node ;
// les bases créées par cette version stockent toujours des centimes explicites
// (EncryptCents), et une valeur relue est alors correcte par construction.
// En cas de migration depuis Node, AUDITER après migration À LA FOIS les soldes
// ET les opérations récurrentes (comparer au backup créé par la migration 008,
// qui précède 009 : « <base>.pre008.<timestamp>.bak » désormais, « <base>.bak »
// sur les instances migrées avant le correctif S-11). Non corrigeable rétroactivement
// sans ce backup : "3000" est indiscernable d'un montant de 30,00 €
// légitimement stocké en centimes.
//
// Correctif de fond (non appliqué : changerait le format stocké et casserait
// tout retour arrière de version sur une base réelle) : préfixer l'unité dans
// le plaintext chiffré ("c:" pour centimes) au lieu de la deviner.
func DecryptCents(s string) (int64, error) {
	plain, err := Decrypt(s)
	if err != nil {
		return 0, err
	}
	if strings.Contains(plain, ".") {
		// Legacy: stored as float string
		f, err := strconv.ParseFloat(plain, 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(f * 100)), nil
	}
	return strconv.ParseInt(plain, 10, 64)
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
	ErrPwdMaxLength = errors.New("max_length")
	ErrPwdUppercase = errors.New("uppercase")
	ErrPwdLowercase = errors.New("lowercase")
	ErrPwdDigit     = errors.New("digit")
	ErrPwdSpecial   = errors.New("special")
)

// ValidatePassword vérifie que le mot de passe respecte les critères.
// Returns nil si valide, sinon une erreur codée (ErrPwd*) à traduire via i18n.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return ErrPwdMinLength
	}
	// bcrypt tronque silencieusement au-delà de 72 octets
	if len(password) > 72 {
		return ErrPwdMaxLength
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
		default:
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
