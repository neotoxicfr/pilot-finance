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

	// Pour chaque page, créer un template combiné
	for _, pageFile := range pageFiles {
		pageName := filepath.Base(pageFile)

		// Créer un nouveau template avec les fonctions
		tmpl := template.New("").Funcs(FuncMap)

		// Parser tous les fichiers de base
		for _, baseFile := range baseFiles {
			content, err := osReadFile(baseFile)
			if err != nil {
				return fmt.Errorf("erreur lecture %s: %v", baseFile, err)
			}
			baseName := filepath.Base(baseFile)
			_, err = tmpl.New(baseName).Parse(string(content))
			if err != nil {
				return fmt.Errorf("erreur parsing %s: %v", baseName, err)
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

// formatMoney formate un montant avec la devise donnée
func formatMoney(amount float64, currency string) string {
	if currency == "" {
		currency = "EUR"
	}
	decimals := 0
	if amount != float64(int64(amount)) {
		decimals = 2
	}

	if decimals == 0 {
		return fmt.Sprintf("%s %s", formatWithSpaces(int64(amount)), currency)
	}
	return fmt.Sprintf("%s %s", formatFloat(amount), currency)
}

// formatMoneyCompact formate un montant en notation compacte (k, M) avec devise
func formatMoneyCompact(amount float64, currency string) string {
	if currency == "" {
		currency = "EUR"
	}
	if amount < 0 {
		return "-" + formatMoneyCompact(-amount, currency)
	}
	if amount >= 1000000 {
		return fmt.Sprintf("%.1fM %s", amount/1000000, currency)
	}
	if amount >= 10000 {
		return fmt.Sprintf("%.0fk %s", amount/1000, currency)
	}
	if amount >= 1000 {
		return fmt.Sprintf("%.1fk %s", amount/1000, currency)
	}
	return fmt.Sprintf("%.0f %s", amount, currency)
}

// formatBalance formate un solde pour l'input
func formatBalance(amount float64) string {
	if amount == float64(int64(amount)) {
		return fmt.Sprintf("%.0f", amount)
	}
	return fmt.Sprintf("%.2f", amount)
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

func formatFloat(f float64) string {
	// Separer partie entiere et decimale
	intPart := int64(f)
	decPart := int(math.Round((f - float64(intPart)) * 100))
	if decPart < 0 {
		decPart = -decPart
	}

	// Formater avec separateurs de milliers
	intStr := formatWithSpaces(intPart)

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

// orFunc retourne la premiere valeur non-nulle (variadic)
func orFunc(values ...interface{}) interface{} {
	for _, v := range values {
		if v != nil && v != "" && v != 0 && v != false {
			return v
		}
	}
	if len(values) > 0 {
		return values[len(values)-1]
	}
	return nil
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
func mult(a, b float64) float64 { return a * b }
func add(a, b float64) float64  { return a + b }
func sub(a, b float64) float64  { return a - b }

// Fonctions de comparaison
func ge(a, b float64) bool         { return a >= b }
func gt(a, b float64) bool         { return a > b }
func eqFunc(a, b interface{}) bool { return a == b }
func neFunc(a, b interface{}) bool { return a != b }

// Fonction valeur absolue
func absFunc(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}
