package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// pageTemplates stocke un template combiné (base + components + page) pour chaque page
var pageTemplates = make(map[string]*template.Template)

// FuncMap contient les fonctions personnalisees pour les templates
var FuncMap = template.FuncMap{
	"formatMoney":   formatMoney,
	"formatBalance": formatBalance,
	"dict":          dict,
	"or":            orFunc,
	"json":          toJSON,
	"mult":          mult,
	"add":           add,
	"sub":           sub,
	"ge":            ge,
	"gt":            gt,
	"eq":            eqFunc,
	"ne":            neFunc,
}

// Init charge tous les templates depuis le dossier templates
func Init(templatesDir string) error {
	// Trouver tous les fichiers de base (layouts + components)
	baseFiles := []string{}

	layoutFiles, err := filepath.Glob(filepath.Join(templatesDir, "layouts", "*.html"))
	if err != nil {
		return err
	}
	baseFiles = append(baseFiles, layoutFiles...)

	componentFiles, err := filepath.Glob(filepath.Join(templatesDir, "components", "*.html"))
	if err != nil {
		return err
	}
	baseFiles = append(baseFiles, componentFiles...)

	// Trouver toutes les pages
	pageFiles, err := filepath.Glob(filepath.Join(templatesDir, "pages", "*.html"))
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
			content, err := os.ReadFile(baseFile)
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
		pageContent, err := os.ReadFile(pageFile)
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

// formatMoney formate un montant en euros
func formatMoney(amount float64) string {
	decimals := 0
	if amount != float64(int64(amount)) {
		decimals = 2
	}

	if decimals == 0 {
		return fmt.Sprintf("%s EUR", formatWithSpaces(int64(amount)))
	}
	return fmt.Sprintf("%s EUR", formatFloat(amount))
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
	return strings.Replace(fmt.Sprintf("%.2f", f), ".", ",", 1)
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

// orFunc retourne la premiere valeur non-nulle
func orFunc(a, b interface{}) interface{} {
	if a == nil || a == "" || a == 0 || a == false {
		return b
	}
	return a
}

// toJSON convertit une valeur en JSON
func toJSON(v interface{}) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return template.JS("null")
	}
	return template.JS(b)
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
