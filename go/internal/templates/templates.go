package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// pageTemplates stocke un template combiné (base + components + page) pour chaque page
var pageTemplates = make(map[string]*template.Template)

// globFn et osReadFile sont injectables pour les tests (couvrent les branches d'erreur de Init).
var (
	globFn     = filepath.Glob
	osReadFile = os.ReadFile
)

// FuncMap contient les fonctions personnalisees pour les templates
//
// UNITÉ MONÉTAIRE (audit S-40) — deux formateurs TYPÉS, jamais un seul générique :
//
//	formatCents        montant stocké, int64 centimes (db.Account.Balance,
//	                   db.RecurringOperation.Amount, buildRecurringData)
//	formatUnits        montant calculé, float64 en unité de devise (les totaux
//	                   agrégés du résumé mensuel, qui alimentent aussi mult/add)
//	formatUnitsCompact idem en notation compacte (k, M)
//
// L'ancien formatMoney(interface{}) déduisait l'unité du TYPE DYNAMIQUE
// (int64 ⇒ centimes, float64 ⇒ unités) : brancher la mauvaise source sur la
// même clé de template affichait un montant faux ×100 sans le moindre signal.
// Les signatures typées déplacent la décision côté appelant, et une erreur de
// câblage devient une erreur d'exécution du template au lieu d'un chiffre faux.
// Les noms disparus (formatMoney, formatMoneyCompact) font en outre échouer le
// PARSING au démarrage — « function not defined » — si un template les utilise
// encore.
var FuncMap = template.FuncMap{
	"formatCents":        formatCents,
	"formatUnits":        formatUnits,
	"formatUnitsCompact": formatUnitsCompact,
	"formatBalance":      formatBalance,
	"dict":               dict,
	"or":                 orFunc,
	"json":               toJSON,
	"mult":               mult,
	"add":                add,
	"sub":                sub,
	"ge":                 ge,
	"gt":                 gt,
	"eq":                 eqFunc,
	"ne":                 neFunc,
	"abs":                absFunc,
	"replace":            replaceFunc,
}

// Init charge tous les templates depuis le dossier templates
func Init(templatesDir string) error {
	// Trouver tous les fichiers de base (layouts + components)
	baseFiles := []string{}

	layoutFiles, err := globFn(filepath.Join(templatesDir, "layouts", "*.html"))
	if err != nil {
		return err
	}
	baseFiles = append(baseFiles, layoutFiles...)

	componentFiles, err := globFn(filepath.Join(templatesDir, "components", "*.html"))
	if err != nil {
		return err
	}
	baseFiles = append(baseFiles, componentFiles...)

	// Trouver toutes les pages
	pageFiles, err := globFn(filepath.Join(templatesDir, "pages", "*.html"))
	if err != nil {
		return err
	}

	// Lire chaque fichier de base une seule fois (au lieu d'une fois par page).
	// On conserve l'ordre des fichiers via baseFiles pour un parsing déterministe.
	type baseTemplate struct {
		name    string
		content string
	}
	baseContents := make([]baseTemplate, 0, len(baseFiles))
	for _, baseFile := range baseFiles {
		content, err := osReadFile(baseFile)
		if err != nil {
			return fmt.Errorf("erreur lecture %s: %v", baseFile, err)
		}
		baseContents = append(baseContents, baseTemplate{
			name:    filepath.Base(baseFile),
			content: string(content),
		})
	}

	// Pour chaque page, créer un template combiné
	for _, pageFile := range pageFiles {
		pageName := filepath.Base(pageFile)

		// Créer un nouveau template avec les fonctions
		tmpl := template.New("").Funcs(FuncMap)

		// Parser tous les fichiers de base depuis le cache
		for _, base := range baseContents {
			if _, err := tmpl.New(base.name).Parse(base.content); err != nil {
				return fmt.Errorf("erreur parsing %s: %v", base.name, err)
			}
		}

		// Parser la page (qui définit le bloc "content")
		pageContent, err := osReadFile(pageFile)
		if err != nil {
			return fmt.Errorf("erreur lecture %s: %v", pageFile, err)
		}

		// Parser le contenu de la page dans le template
		_, err = tmpl.New(pageName).Parse(string(pageContent))
		if err != nil {
			return fmt.Errorf("erreur parsing %s: %v", pageName, err)
		}

		pageTemplates[pageName] = tmpl
	}

	return nil
}

// Render affiche un template avec les donnees fournies
// Il exécute base.html qui inclut automatiquement le bloc "content" de la page
func Render(w io.Writer, name string, data interface{}) error {
	tmpl, ok := pageTemplates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}

	// Exécuter base.html qui va inclure {{template "content" .}}
	return tmpl.ExecuteTemplate(w, "base.html", data)
}

// RenderPartial rend un bloc template sans le wrapper base.html
// Utilisé pour les requêtes HTMX qui ne veulent qu'une partie de la page
func RenderPartial(w io.Writer, pageName, blockName string, data interface{}) error {
	tmpl, ok := pageTemplates[pageName]
	if !ok {
		return fmt.Errorf("template %s not found", pageName)
	}

	// Exécuter directement le bloc demandé
	return tmpl.ExecuteTemplate(w, blockName, data)
}

// toNumber convertit une valeur de template en float64 SANS interpréter
// d'unité (audit S-40).
//
// L'ancienne toFloat64 divisait les int64 par 100 « parce qu'un int64 est un
// montant en centimes » : la conversion arithmétique portait donc une
// sémantique monétaire, alors qu'elle sert aussi à comparer un numéro de page
// ou un compteur de lignes. Ici 12345 vaut 12345, quel que soit son type ; la
// conversion centimes → unités appartient au seul formatCents.
//
// Un type non numérique retourne une erreur au lieu de 0 : les fonctions de
// template propagent l'erreur, l'exécution s'arrête et le handler renvoie une
// 500 tracée. Un `return 0` silencieux transformait un mauvais câblage en
// « 0 EUR » parfaitement crédible à l'écran.
func toNumber(v interface{}) (float64, error) {
	switch n := v.(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("valeur numérique attendue, reçu %T (%v)", v, v)
	}
}

// twoNumbers convertit les deux opérandes d'une fonction binaire.
func twoNumbers(a, b interface{}) (float64, float64, error) {
	x, err := toNumber(a)
	if err != nil {
		return 0, 0, err
	}
	y, err := toNumber(b)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// moneySeparators retourne les séparateurs de milliers et de décimales de la
// locale (audit S-23).
//
// Le serveur figeait la notation française tandis que PILOT_FMT.currency, côté
// navigateur, passe par Intl et localise : un même écran pouvait afficher
// « 1 234,56 EUR » rendu par le serveur à côté de « 1,234.56 EUR » rendu par
// le JS. Seuls les séparateurs varient — la devise reste suffixée dans les deux
// langues, comme partout dans l'interface.
func moneySeparators(locale string) (thousands, decimal string) {
	if strings.HasPrefix(locale, "en") {
		return ",", "."
	}
	return " ", ","
}

// currencyDecimals retourne le nombre de décimales d'une devise. Le yen ne
// subdivise pas : afficher « 1 234,00 JPY » est faux (audit S-23).
func currencyDecimals(currency string) int {
	if currency == "JPY" {
		return 0
	}
	return 2
}

// formatCents formate un montant STOCKÉ, exprimé en centimes (int64), unité
// canonique de l'application (voir CLAUDE.md « Monetary amounts: int64
// centimes »). C'est le formateur des montants qui viennent de la base :
// db.Account.Balance, db.RecurringOperation.Amount, et la liste unifiée des
// opérations récurrentes construite par buildRecurringData.
//
// Le paramètre est typé int64 : passer un float64 est refusé par l'exécution du
// template, ce qui rend impossible la confusion ×100 de l'audit S-40.
func formatCents(cents int64, currency string, locale string) string {
	return formatUnits(float64(cents)/100.0, currency, locale)
}

// formatUnits formate un montant CALCULÉ, déjà exprimé dans l'unité de la
// devise (float64) : les totaux agrégés du résumé mensuel, qui alimentent aussi
// mult/add côté template et ne peuvent donc pas rester entiers.
func formatUnits(amount float64, currency string, locale string) string {
	f := amount
	if currency == "" {
		currency = "EUR"
	}
	thousands, decimal := moneySeparators(locale)

	// Une devise sans subdivision n'affiche jamais de décimales, même sur un
	// montant non entier (arrondi à l'unité).
	if currencyDecimals(currency) == 0 {
		return fmt.Sprintf("%s %s", groupDigits(int64(math.Round(f)), thousands), currency)
	}
	if f == float64(int64(f)) {
		return fmt.Sprintf("%s %s", groupDigits(int64(f), thousands), currency)
	}
	return fmt.Sprintf("%s %s", formatDecimal(f, thousands, decimal), currency)
}

// formatUnitsCompact formate un montant en notation compacte (k, M) avec devise.
// Comme formatUnits, l'argument est déjà dans l'unité de la devise (float64) :
// tous ses appels viennent d'un calcul de template (mult/add sur les totaux).
//
// Séparateur décimal : la virgule, comme formatFloat/formatUnits. Les deux
// formateurs Go divergeaient (« +2 800,50 EUR » au-dessus de « +2.8k EUR/an »
// sur la même carte, audit S-26) ; le point se lisait en outre comme un
// séparateur de milliers en français. La notation reste figée en français,
// comme le reste de la couche monétaire serveur (voir S-23 pour la locale).
//
// Ce formateur a un miroir JS strict dans go/static/js/charts.js
// (compactMoney) : toute modification de tiers, d'arrondi ou de séparateur
// doit être répercutée dans les deux.
func formatUnitsCompact(amount float64, currency string, locale string) string {
	f := amount
	if currency == "" {
		currency = "EUR"
	}
	if f < 0 {
		return "-" + formatUnitsCompact(-f, currency, locale)
	}
	_, decimal := moneySeparators(locale)
	if f >= 1000000 {
		return fmt.Sprintf("%sM %s", oneDecimal(f/1000000, decimal), currency)
	}
	if f >= 10000 {
		return fmt.Sprintf("%.0fk %s", f/1000, currency)
	}
	if f >= 1000 {
		return fmt.Sprintf("%sk %s", oneDecimal(f/1000, decimal), currency)
	}
	return fmt.Sprintf("%.0f %s", f, currency)
}

// oneDecimal formate une valeur avec une décimale et la virgule française,
// cohérent avec formatFloat (audit S-26).
func oneDecimal(f float64, decimal string) string {
	return strings.Replace(fmt.Sprintf("%.1f", f), ".", decimal, 1)
}

// groupDigits insère le séparateur de milliers de la locale dans un entier.
// Remplace formatWithSpaces, qui figeait l'espace (audit S-23).
func groupDigits(n int64, thousands string) string {
	if n < 0 {
		return "-" + groupDigits(-n, thousands)
	}
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}
	var b strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			b.WriteString(thousands)
		}
		b.WriteRune(c)
	}
	return b.String()
}

// formatDecimal rend un montant à deux décimales avec les séparateurs de la
// locale, en gérant le débordement d'arrondi (99,999… → 100,00).
func formatDecimal(f float64, thousands, decimal string) string {
	negative := math.Signbit(f)
	absVal := math.Abs(f)
	intPart := int64(absVal)
	decPart := int(math.Round((absVal - float64(intPart)) * 100))
	if decPart >= 100 {
		intPart++
		decPart = 0
	}
	s := fmt.Sprintf("%s%s%02d", groupDigits(intPart, thousands), decimal, decPart)
	if negative {
		return "-" + s
	}
	return s
}

// formatBalance formate un solde stocké (int64 centimes) pour l'input éditable
// de la ligne de compte, sans devise ni séparateur de milliers — la valeur est
// re-postée telle quelle. Typé int64 pour la même raison que formatCents.
func formatBalance(cents int64) string {
	f := float64(cents) / 100.0
	if f == float64(int64(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}

// replaceFunc substitue un marqueur dans une chaîne traduite, comme le fait
// déjà le JS sur les mêmes clés (« {n} », « {list} »). La valeur peut être un
// nombre : elle est rendue via %v.
func replaceFunc(s, old string, value interface{}) string {
	return strings.ReplaceAll(s, old, fmt.Sprintf("%v", value))
}

// dict cree un dictionnaire pour passer des parametres aux templates
func dict(values ...interface{}) map[string]interface{} {
	if len(values)%2 != 0 {
		return nil
	}

	d := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			continue
		}
		d[key] = values[i+1]
	}
	return d
}

// orFunc retourne la première valeur "truthy" (variadic).
// Sont considérés comme falsy : nil, "", false, et les zéros numériques
// (int, int64, float64). Un type switch est nécessaire car une comparaison
// directe `v != 0` compare contre une constante int et ne détecte donc pas
// int64(0) ni float64(0).
func orFunc(values ...interface{}) interface{} {
	for _, v := range values {
		if isTruthy(v) {
			return v
		}
	}
	if len(values) > 0 {
		return values[len(values)-1]
	}
	return nil
}

// isTruthy retourne false pour nil, "", false et les zéros numériques.
func isTruthy(v interface{}) bool {
	switch n := v.(type) {
	case nil:
		return false
	case string:
		return n != ""
	case bool:
		return n
	case int:
		return n != 0
	case int64:
		return n != 0
	case float64:
		return n != 0
	default:
		return true
	}
}

// toJSON convertit une valeur en JSON (SetEscapeHTML explicite pour clarifier l'intention)
func toJSON(v interface{}) template.JS {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		return template.JS("null")
	}
	return template.JS(strings.TrimSuffix(buf.String(), "\n"))
}

// Fonctions arithmetiques.
//
// Elles opèrent sur des NOMBRES, pas sur des montants : elles ne convertissent
// aucune unité et rendent le résultat dans l'unité des opérandes (audit S-40).
// Mélanger des centimes et des unités de devise dans un même calcul reste donc
// une erreur d'appelant — mais elle ne peut plus se produire silencieusement au
// moment du FORMATAGE, qui exige désormais un type par unité.
//
// La seconde valeur de retour interrompt l'exécution du template sur un
// opérande non numérique.
func mult(a, b interface{}) (float64, error) {
	x, y, err := twoNumbers(a, b)
	if err != nil {
		return 0, err
	}
	return x * y, nil
}

func add(a, b interface{}) (float64, error) {
	x, y, err := twoNumbers(a, b)
	if err != nil {
		return 0, err
	}
	return x + y, nil
}

func sub(a, b interface{}) (float64, error) {
	x, y, err := twoNumbers(a, b)
	if err != nil {
		return 0, err
	}
	return x - y, nil
}

// Fonctions de comparaison
func ge(a, b interface{}) (bool, error) {
	x, y, err := twoNumbers(a, b)
	if err != nil {
		return false, err
	}
	return x >= y, nil
}

func gt(a, b interface{}) (bool, error) {
	x, y, err := twoNumbers(a, b)
	if err != nil {
		return false, err
	}
	return x > y, nil
}

func eqFunc(a, b interface{}) bool { return a == b }
func neFunc(a, b interface{}) bool { return a != b }

// Fonction valeur absolue
func absFunc(a interface{}) (float64, error) {
	f, err := toNumber(a)
	if err != nil {
		return 0, err
	}
	if f < 0 {
		return -f, nil
	}
	return f, nil
}
