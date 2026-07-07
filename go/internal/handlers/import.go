package handlers

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strings"

	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// balanceUpdate est une ligne d'import validée : nom de compte + solde en centimes.
type balanceUpdate struct {
	name  string
	cents int64
}

var errUnexpectedFields = errors.New("champs excédentaires")

// parseCentsFlexible convertit un montant saisi humainement en centimes.
// Accepte point ou virgule décimale, séparateurs de milliers (espaces ou
// l'autre séparateur), et symboles monétaires usuels.
func parseCentsFlexible(s string) (int64, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', ' ', ' ', '€', '$', '£':
			return -1
		}
		return r
	}, s)
	if strings.Contains(s, ",") {
		if strings.Contains(s, ".") {
			// Les deux présents : le dernier est le séparateur décimal.
			if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
				s = strings.ReplaceAll(s, ".", "") // 1.234,56
				s = strings.Replace(s, ",", ".", 1)
			} else {
				s = strings.ReplaceAll(s, ",", "") // 1,234.56
			}
		} else {
			s = strings.Replace(s, ",", ".", 1) // 1234,56
		}
	}
	if s == "" {
		return 0, errors.New("montant vide")
	}
	return parseCents(s)
}

// parseBalancesCSV lit un CSV "nom,solde" (séparateur , ou ; détecté sur la
// première ligne, en-tête optionnel) et retourne les lignes valides plus les
// numéros des lignes rejetées.
func parseBalancesCSV(rd io.Reader) ([]balanceUpdate, []int) {
	data, err := io.ReadAll(io.LimitReader(rd, 1<<20))
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	firstLine, _, _ := strings.Cut(string(data), "\n")
	comma := ','
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		comma = ';'
	}
	cr := csv.NewReader(strings.NewReader(string(data)))
	cr.Comma = comma
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	var out []balanceUpdate
	var invalid []int
	line := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil || len(rec) < 2 {
			invalid = append(invalid, line)
			continue
		}
		name := strings.TrimSpace(rec[0])
		cents, perr := parseCentsFlexible(rec[1])
		// Champs excédentaires non vides (ex. décimale à virgule dans un CSV
		// séparé par des virgules) : rejeter plutôt que deviner le montant.
		for _, extra := range rec[2:] {
			if strings.TrimSpace(extra) != "" {
				perr = errUnexpectedFields
				break
			}
		}
		if perr != nil || name == "" {
			if line == 1 {
				continue // en-tête probable ("nom,solde") : ignorer silencieusement
			}
			invalid = append(invalid, line)
			continue
		}
		out = append(out, balanceUpdate{name: name, cents: cents})
	}
	return out, invalid
}

// ImportBalances met à jour les soldes en masse depuis un CSV "nom,solde".
// Le matching se fait sur le nom de compte déchiffré, insensible à la casse.
// Réponse JSON : {updated, unknown, ambiguous, invalid_lines}.
func ImportBalances(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientError(w, ErrAuthRequired, "Non authentifié", http.StatusUnauthorized)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		clientError(w, ErrValidation, "Fichier manquant", http.StatusBadRequest)
		return
	}
	defer file.Close()

	updates, invalidLines := parseBalancesCSV(file)
	if len(updates) == 0 && len(invalidLines) == 0 {
		clientError(w, ErrValidation, "CSV vide", http.StatusBadRequest)
		return
	}

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, "get accounts", err)
		return
	}
	// Index nom déchiffré (minuscules) → IDs ; un nom peut apparaître plusieurs
	// fois, auquel cas la ligne est signalée ambiguë plutôt que devinée.
	byName := make(map[string][]int64, len(accounts))
	for _, acc := range accounts {
		name, derr := hookDecryptStr(acc.Name)
		if derr != nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(name))
		byName[key] = append(byName[key], acc.ID)
	}

	updated := 0
	unknown := []string{}
	ambiguous := []string{}
	for _, u := range updates {
		ids := byName[strings.ToLower(u.name)]
		switch len(ids) {
		case 0:
			unknown = append(unknown, u.name)
		case 1:
			if err := hookUpdateAccountBalance(ids[0], user.ID, u.cents); err != nil {
				serverError(w, "import balance", err)
				return
			}
			updated++
		default:
			ambiguous = append(ambiguous, u.name)
		}
	}

	if updated > 0 {
		hookLogAudit(user.ID, db.AuditImportBalances, getClientIP(r), r.UserAgent())
	}

	if invalidLines == nil {
		invalidLines = []int{}
	}
	jsonSuccess(w, map[string]interface{}{
		"updated":       updated,
		"unknown":       unknown,
		"ambiguous":     ambiguous,
		"invalid_lines": invalidLines,
	})
}
