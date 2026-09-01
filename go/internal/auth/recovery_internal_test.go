package auth

import (
	"errors"
	"strings"
	"testing"
)

// TestGenerateRecoveryCodes vérifie le contrat de génération (audit S-22) :
// nombre, format, alphabet sans caractères ambigus, et unicité.
func TestGenerateRecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("nombre de codes : want %d, got %d", RecoveryCodeCount, len(codes))
	}

	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		// Format « XXXX-XXXX-XXXX »
		parts := strings.Split(c, "-")
		if len(parts) != recoveryGroups {
			t.Errorf("%q : want %d groupes, got %d", c, recoveryGroups, len(parts))
		}
		for _, p := range parts {
			if len(p) != recoveryGroupLen {
				t.Errorf("%q : groupe %q de longueur %d, want %d", c, p, len(p), recoveryGroupLen)
			}
		}
		// Aucun caractère ambigu : ni 0/O ni 1/I.
		for _, bad := range []string{"0", "O", "1", "I"} {
			if strings.Contains(c, bad) {
				t.Errorf("%q contient le caractère ambigu %q", c, bad)
			}
		}
		if seen[c] {
			t.Errorf("code dupliqué dans le lot : %q", c)
		}
		seen[c] = true
	}
}

// TestGenerateRecoveryCodes_RandError couvre la branche d'échec de crypto/rand :
// mieux vaut refuser d'activer le 2FA que livrer des codes prévisibles.
func TestGenerateRecoveryCodes_RandError(t *testing.T) {
	sentinel := errors.New("entropie indisponible")
	orig := recoveryRandRead
	recoveryRandRead = func([]byte) (int, error) { return 0, sentinel }
	t.Cleanup(func() { recoveryRandRead = orig })

	codes, err := GenerateRecoveryCodes()
	if !errors.Is(err, sentinel) {
		t.Errorf("want sentinel error, got %v", err)
	}
	if codes != nil {
		t.Errorf("aucun code ne doit être retourné en cas d'échec, got %v", codes)
	}
}

// TestNormalizeRecoveryCode couvre le rôle de FILTRE de la normalisation : une
// saisie qui n'a pas la forme d'un code doit renvoyer "" pour qu'aucune requête
// de consommation ne parte en base (un code TOTP à 6 chiffres, notamment).
func TestNormalizeRecoveryCode(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"forme canonique", "K7QM-3XZP-9BRT", "K7QM3XZP9BRT"},
		{"minuscules", "k7qm-3xzp-9brt", "K7QM3XZP9BRT"},
		{"sans tiret", "K7QM3XZP9BRT", "K7QM3XZP9BRT"},
		{"espaces parasites", " K7QM 3XZP 9BRT ", "K7QM3XZP9BRT"},
		{"code TOTP à 6 chiffres rejeté", "123456", ""},
		{"vide", "", ""},
		{"trop court", "K7QM-3XZP", ""},
		{"un caractère valide de trop", "K7QM-3XZP-9BRTA", ""},
		{"saisie démesurée", strings.Repeat("A", 100), ""},
		{"caractères non ASCII ignorés", "K7QM-3XZP-9BRTé", "K7QM3XZP9BRT"},
		{"caractères ambigus ignorés", "K7QM-3XZP-9BRT01", "K7QM3XZP9BRT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeRecoveryCode(tc.in); got != tc.want {
				t.Errorf("NormalizeRecoveryCode(%q): want %q, got %q", tc.in, tc.want, got)
			}
		})
	}
}
