package handlers

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
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
var errAmbiguousAmount = errors.New("montant ambigu (séparateur de milliers ?)")

// ambiguousThousandsDot repère "1.234" : un point unique suivi d'exactement 3
// chiffres, sans virgule — indistinguable entre décimale (1,234) et milliers
// FR (1 234). On refuse plutôt que de deviner (audit FIN-7).
var ambiguousThousandsDot = regexp.MustCompile(`^-?\d{1,3}\.\d{3}$`)

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
	} else if ambiguousThousandsDot.MatchString(s) {
		return 0, errAmbiguousAmount
	}
	if s == "" {
		return 0, errors.New("montant vide")
	}
	return parseCents(s)
}

// parseRateFlexible convertit un taux saisi humainement en pourcentage.
// Même souplesse que parseCentsFlexible sur les saisies francophones (virgule
// décimale, espaces fine/insécable, symbole « % »), sans logique de séparateur
// de milliers : un taux tient sur trois chiffres, « 1.234 » n'y est jamais
// ambigu. Les garde-fous de parseRate (NaN/±Inf, magnitude ≤ maxRate) restent
// appliqués — audit S-03.
func parseRateFlexible(s string) (float64, error) {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', ' ', ' ', '%':
			return -1
		}
		return r
	}, s)
	s = strings.Replace(s, ",", ".", 1)
	return parseRate(s)
}

// parseBalancesCSV lit un CSV "nom,solde" (séparateur , ou ; détecté sur la
// première ligne, en-tête optionnel) et retourne les lignes valides plus les
// numéros des lignes rejetées.
func parseBalancesCSV(rd io.Reader) ([]balanceUpdate, []int) {
	data, err := io.ReadAll(io.LimitReader(rd, 1<<20))
	if err != nil || len(data) == 0 {
		return nil, nil
	}
	// Retirer le BOM UTF-8 (exports Excel/Notepad FR) : sinon la 1re ligne de
	// données d'un fichier sans en-tête serait préfixée d'un caractère
	// invisible et classée « inconnue » (audit FIN-8).
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	firstLine, _, _ := strings.Cut(string(data), "\n")
	comma := ','
	// Le ';' gagne dès qu'il est présent et au moins aussi fréquent que la
	// virgule : un CSV français "nom;1 234,56" a autant de ';' que de ','.
	if n := strings.Count(firstLine, ";"); n > 0 && n >= strings.Count(firstLine, ",") {
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
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		clientErrorT(w, r, ErrValidation, "error.file_missing", http.StatusBadRequest)
		return
	}
	defer file.Close()

	updates, invalidLines := parseBalancesCSV(file)
	if len(updates) == 0 && len(invalidLines) == 0 {
		clientErrorT(w, r, ErrValidation, "error.csv_empty", http.StatusBadRequest)
		return
	}

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, r, "get accounts", err)
		return
	}
	// Index nom déchiffré (minuscules) → IDs ; un nom peut apparaître plusieurs
	// fois, auquel cas la ligne est signalée ambiguë plutôt que devinée.
	byName := make(map[string][]int64, len(accounts))
	// Solde courant par ID : l'apercu doit montrer « de X vers Y », pas
	// seulement la valeur cible, sinon l'utilisateur ne peut pas juger de ce
	// que l'import va ecraser.
	currentByID := make(map[int64]int64, len(accounts))
	for _, acc := range accounts {
		currentByID[acc.ID] = acc.Balance
		name, derr := hookDecryptStr(acc.Name)
		if derr != nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(name))
		byName[key] = append(byName[key], acc.ID)
	}

	unknown := []string{}
	ambiguous := []string{}
	changes := []map[string]interface{}{}
	pairs := make([]db.AccountBalanceUpdate, 0, len(updates))
	for _, u := range updates {
		ids := byName[strings.ToLower(u.name)]
		switch len(ids) {
		case 0:
			unknown = append(unknown, u.name)
		case 1:
			pairs = append(pairs, db.AccountBalanceUpdate{ID: ids[0], Cents: u.cents})
			// Montants en centimes : unite canonique de l'application (S-40).
			// La conversion pour l'affichage appartient au client.
			changes = append(changes, map[string]interface{}{
				"name": u.name,
				"from": currentByID[ids[0]],
				"to":   u.cents,
			})
		default:
			ambiguous = append(ambiguous, u.name)
		}
	}

	// Mode simulation (audit S-21) : l'import ecrasait les soldes des le choix
	// du fichier, sans confirmation ni retour arriere. Le client demande donc
	// d'abord un dry-run, affiche l'apercu des changements, et ne repost sans
	// ce drapeau qu'apres confirmation explicite. Aucune ecriture, aucune
	// entree d'audit : une simulation ne doit rien laisser derriere elle.
	if r.FormValue("dry_run") != "" {
		writeImportResult(w, len(pairs), unknown, ambiguous, invalidLines, changes, true)
		return
	}

	// Application atomique : soit toutes les lignes valides sont écrites, soit
	// aucune — pas de mise à jour partielle sur erreur en cours (audit FIN-9).
	if len(pairs) > 0 {
		if err := hookUpdateAccountBalancesTx(user.ID, pairs); err != nil {
			serverError(w, r, "import balances", err)
			return
		}
		hookLogAudit(user.ID, db.AuditImportBalances, getClientIP(r), r.UserAgent())
	}
	writeImportResult(w, len(pairs), unknown, ambiguous, invalidLines, changes, false)
}

// writeImportResult serialise le resultat d'un import, reel ou simule. Les deux
// chemins partagent la MEME forme de reponse : l'apercu montre alors exactement
// ce que la confirmation appliquera, au lieu de deux calculs paralleles qui
// pourraient diverger.
func writeImportResult(w http.ResponseWriter, updated int, unknown, ambiguous []string, invalidLines []int, changes []map[string]interface{}, dryRun bool) {
	if invalidLines == nil {
		invalidLines = []int{}
	}
	jsonSuccess(w, map[string]interface{}{
		"updated":       updated,
		"unknown":       unknown,
		"ambiguous":     ambiguous,
		"invalid_lines": invalidLines,
		"changes":       changes,
		"dry_run":       dryRun,
	})
}

// ImportTemplate renvoie un CSV « nom,solde » pre-rempli avec les comptes de
// l'utilisateur et leurs soldes actuels (audit S-21).
//
// L'import se fait par correspondance EXACTE sur le nom de compte : un modele
// vide n'aiderait pas, puisque la difficulte n'est pas le format mais d'ecrire
// les noms tels qu'ils existent. Le fichier telecharge est donc directement
// reimportable, et il suffit d'en modifier les montants.
//
// Les soldes sont ecrits avec un point decimal et sans separateur de milliers :
// parseCentsFlexible accepte bien plus large, mais le modele doit rester le
// format le moins ambigu possible (cf. le refus de « 1.234 », audit FIN-7).
func ImportTemplate(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		clientErrorT(w, r, ErrAuthRequired, "error.auth_required", http.StatusUnauthorized)
		return
	}

	accounts, err := hookGetAccountsByUserID(user.ID)
	if err != nil {
		serverError(w, r, "get accounts", err)
		return
	}

	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // BOM UTF-8 : Excel lit les accents correctement
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"nom", "solde"})
	for _, acc := range accounts {
		name, derr := hookDecryptStr(acc.Name)
		if derr != nil {
			continue
		}
		_ = cw.Write([]string{name, formatCentsPlain(acc.Balance)})
	}
	cw.Flush()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pilot-soldes.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// formatCentsPlain rend un montant en centimes sous la forme la plus neutre
// possible : point decimal, aucun separateur de milliers, aucun symbole. C'est
// le format destine a une MACHINE (relecture par parseCentsFlexible), pas a
// l'affichage — celui-ci passe par templates.formatCents.
func formatCentsPlain(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}
