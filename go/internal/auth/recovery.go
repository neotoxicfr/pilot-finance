// Package auth - codes de récupération 2FA (audit S-22)
package auth

import (
	"crypto/rand"
	"strings"
)

const (
	// RecoveryCodeCount est le nombre de codes générés à chaque lot. Un lot
	// remplace intégralement le précédent.
	RecoveryCodeCount = 10

	// Format d'un code : trois groupes de quatre caractères séparés par un
	// tiret (« K7QM-3XZP-9BRT »). 12 caractères tirés d'un alphabet de 32
	// symboles, soit 60 bits d'entropie — sans commune mesure avec un mot de
	// passe choisi par un humain, ce qui justifie le choix de hachage côté DB
	// (voir handlers/mfa.go).
	recoveryGroups   = 3
	recoveryGroupLen = 4

	// recoveryAlphabet écarte les caractères ambigus à la lecture ou à la
	// recopie manuelle : ni « 0 » ni « O », ni « 1 » ni « I ». Le « L »
	// subsiste, mais son seul confusable (« 1 »/« I ») ayant disparu de
	// l'alphabet, il n'est plus ambigu.
	//
	// Sa taille vaut exactement 32, soit un diviseur de 256 : `octet % 32` est
	// donc uniformément distribué et ne demande aucun rejet d'échantillon.
	recoveryAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	// recoveryMaxInput borne la normalisation d'une saisie utilisateur : au
	// delà, l'entrée ne peut de toute façon pas être un code valide.
	recoveryMaxInput = 64
)

// RecoveryCodeLen est la longueur d'un code une fois normalisé (tirets ôtés).
const RecoveryCodeLen = recoveryGroups * recoveryGroupLen

// recoveryRandRead est injectable pour les tests (couvre la branche d'erreur rand.Read).
var recoveryRandRead = rand.Read

// GenerateRecoveryCodes tire RecoveryCodeCount codes de récupération à usage
// unique depuis crypto/rand. Les codes sont retournés EN CLAIR : c'est la seule
// et unique occasion de les montrer à l'utilisateur, seul leur hash est stocké.
func GenerateRecoveryCodes() ([]string, error) {
	buf := make([]byte, RecoveryCodeLen)
	codes := make([]string, 0, RecoveryCodeCount)

	for i := 0; i < RecoveryCodeCount; i++ {
		if _, err := recoveryRandRead(buf); err != nil {
			return nil, err
		}

		var b strings.Builder
		b.Grow(RecoveryCodeLen + recoveryGroups - 1)
		for j, v := range buf {
			if j > 0 && j%recoveryGroupLen == 0 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
		}
		codes = append(codes, b.String())
	}

	return codes, nil
}

// NormalizeRecoveryCode ramène une saisie utilisateur à sa forme canonique :
// majuscules, sans tiret ni espace, sans aucun caractère hors alphabet.
//
// Retourne "" si le résultat n'a pas la longueur attendue. L'appelant s'en sert
// comme d'un filtre : un code TOTP à 6 chiffres, ou un champ vide, ne déclenche
// alors aucune requête de consommation en base.
func NormalizeRecoveryCode(input string) string {
	if len(input) > recoveryMaxInput {
		return ""
	}

	var b strings.Builder
	b.Grow(RecoveryCodeLen)
	for _, r := range strings.ToUpper(input) {
		if r > 127 || !strings.ContainsRune(recoveryAlphabet, r) {
			continue
		}
		if b.Len() == RecoveryCodeLen {
			// Un caractère valide de trop : la saisie ne peut pas être un code.
			return ""
		}
		b.WriteRune(r)
	}

	if b.Len() != RecoveryCodeLen {
		return ""
	}
	return b.String()
}
