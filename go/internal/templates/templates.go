package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"math"
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
var FuncMap = template.FuncMap{
	"formatMoney":        formatMoney,
	"formatMoneyCompact": formatMoneyCompact,
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

// toFloat64 converts numeric types for template use.
// int64 is treated as centimes (divided by 100).
// float64 and int are used as-is.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int64:
		return float64(n) / 100.0
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

// formatMoney formate un montant avec la devise donnée
func formatMoney(amount interface{}, currency string) string {
	f := toFloat64(amount)
	if currency == "" {
		currency = "EUR"
	}
	decimals := 0
	if f != float64(int64(f)) {
		decimals = 2
	}

	if decimals == 0 {
		return fmt.Sprintf("%s %s", formatWithSpaces(int64(f)), currency)
	}
	return fmt.Sprintf("%s %s", formatFloat(f), currency)
}

// formatMoneyCompact formate un montant en notation compacte (k, M) avec devise.
//
// Séparateur décimal : la virgule, comme formatFloat/formatMoney. Les deux
// formateurs Go divergeaient (« +2 800,50 EUR » au-dessus de « +2.8k EUR/an »
// sur la même carte, audit S-26) ; le point se lisait en outre comme un
// séparateur de milliers en français. La notation reste figée en français,
// comme le reste de la couche monétaire serveur (voir S-23 pour la locale).
//
// Ce formateur a un miroir JS strict dans go/static/js/charts.js
// (compactMoney) : toute modification de tiers, d'arrondi ou de séparateur
// doit être répercutée dans les deux.
func formatMoneyCompact(amount interface{}, currency string) string {
	f := toFloat64(amount)
	if currency == "" {
		currency = "EUR"
	}
	if f < 0 {
		return "-" + formatMoneyCompact(-f, currency)
	}
	if f >= 1000000 {
		return fmt.Sprintf("%sM %s", oneDecimal(f/1000000), currency)
	}
	if f >= 10000 {
		return fmt.Sprintf("%.0fk %s", f/1000, currency)
	}
	if f >= 1000 {
		return fmt.Sprintf("%sk %s", oneDecimal(f/1000), currency)
	}
	return fmt.Sprintf("%.0f %s", f, currency)
}

// oneDecimal formate une valeur avec une décimale et la virgule française,
// cohérent avec formatFloat (audit S-26).
func oneDecimal(f float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", f), ".", ",", 1)
}

// formatBalance formate un solde pour l'input
func formatBalance(amount interface{}) string {
	f := toFloat64(amount)
	if f == float64(int64(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}

func formatWithSpaces(n int64) string {
	if n < 0 {
		return "-" + formatWithSpaces(-n)
	}

	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	for i, c := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteRune(' ')
		}
		result.WriteRune(c)
	}
	return result.String()
}

func formatFloat(v interface{}) string {
	f := toFloat64(v)
	// Separer partie entiere et decimale
	negative := math.Signbit(f)
	absVal := math.Abs(f)
	intPart := int64(absVal)
	decPart := int(math.Round((absVal - float64(intPart)) * 100))

	// Handle rounding overflow (e.g., 99.999... -> decPart=100)
	if decPart >= 100 {
		intPart++
		decPart = 0
	}

	// Formater avec separateurs de milliers
	intStr := formatWithSpaces(intPart)

	if negative {
		return fmt.Sprintf("-%s,%02d", intStr, decPart)
	}
	return fmt.Sprintf("%s,%02d", intStr, decPart)
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

// Fonctions arithmetiques
func mult(a, b interface{}) float64 { return toFloat64(a) * toFloat64(b) }
func add(a, b interface{}) float64  { return toFloat64(a) + toFloat64(b) }
func sub(a, b interface{}) float64  { return toFloat64(a) - toFloat64(b) }

// Fonctions de comparaison
func ge(a, b interface{}) bool      { return toFloat64(a) >= toFloat64(b) }
func gt(a, b interface{}) bool      { return toFloat64(a) > toFloat64(b) }
func eqFunc(a, b interface{}) bool  { return a == b }
func neFunc(a, b interface{}) bool  { return a != b }

// Fonction valeur absolue
func absFunc(a interface{}) float64 {
	f := toFloat64(a)
	if f < 0 {
		return -f
	}
	return f
}
