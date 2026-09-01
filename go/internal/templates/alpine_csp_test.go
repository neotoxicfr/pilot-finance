package templates_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// --- Garde-fous sur les expressions Alpine (build CSP) ---
//
// L'application charge le build CSP d'Alpine, qui n'évalue pas du JavaScript
// mais un sous-ensemble d'expressions qu'il parse lui-même. Une expression hors
// de ce sous-ensemble ne provoque PAS une erreur locale et visible : elle
// interrompt la mise en place des directives, et les `x-show` du composant — ou
// de toute la page — restent alors figés sur leur valeur INITIALE. La page
// s'affiche, ne journalise rien, et cesse simplement de réagir.
//
// Trois formes ont réellement cassé cette application. Elles sont figées ici
// parce qu'aucune n'est détectable au build, au rendu, ni à la lecture du diff.
//
// Le contrôle ne porte QUE sur les attributs évalués au rendu (x-show, x-if,
// x-for, x-text, x-bind). Les gestionnaires d'événements (@… / x-on:…) sont
// évalués au déclenchement et acceptent des formes plus larges — `setError(
// $event.detail.xhr.responseText)` fonctionne depuis toujours.

var (
	// Attributs évalués au rendu, avec leur expression.
	evaluatedAttrRe = regexp.MustCompile(`(?:x-show|x-if|x-for|x-text|x-html|x-model|:[a-zA-Z-]+|x-bind:[a-zA-Z-]+)="([^"]*)"`)

	// x-for="item in a.b" — un chemin de propriété dans la clause « in ».
	xForRe       = regexp.MustCompile(`x-for="([^"]*)"`)
	xForNestedRe = regexp.MustCompile(`\bin\s+[A-Za-z_$][\w$]*\.[\w$.]+\s*$`)

	// f(a.b) — un chemin de propriété passé en ARGUMENT d'appel.
	callWithMemberArgRe = regexp.MustCompile(`[A-Za-z_$][\w$]*\s*\(\s*[A-Za-z_$][\w$]*\.[\w$.]+`)

	// Littéraux de chaîne, retirés avant de chercher un « ; » séparateur :
	// « toast('a; b') » n'est pas une expression à deux instructions.
	stringLiteralRe = regexp.MustCompile(`'[^']*'`)

	// Tous les attributs Alpine, gestionnaires compris, pour le contrôle
	// « plusieurs instructions » : c'est justement sur des @htmx:… que cette
	// forme a cassé (l'attribut entier est ignoré, silencieusement).
	anyAlpineAttrRe = regexp.MustCompile(`(?:@[a-zA-Z][\w:.-]*|x-on:[\w:.-]+|x-show|x-if|x-for|x-text|x-html|x-model|x-effect|:[a-zA-Z-]+)="([^"]*)"`)
)

type finding struct{ file, attr, why string }

func walkTemplates(t *testing.T, fn func(rel string, body string)) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatalf("chemin templates: %v", err)
	}
	var seen int
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		seen++
		fn(filepath.ToSlash(rel), string(body))
		return nil
	})
	if err != nil {
		t.Fatalf("parcours: %v", err)
	}
	if seen == 0 {
		t.Fatal("aucun template parcouru — le test ne teste rien")
	}
}

func report(t *testing.T, found []finding, rule string) {
	t.Helper()
	if len(found) == 0 {
		return
	}
	lines := make([]string, 0, len(found))
	for _, f := range found {
		lines = append(lines, f.file+" : "+f.attr)
	}
	sort.Strings(lines)
	t.Errorf("%s\n  %s", rule, strings.Join(lines, "\n  "))
}

// x-for="c in importPreview.changes" a fige les x-show de TOUTE la page de
// paramètres, y compris ceux de composants sans aucun rapport. Itérer sur un
// getter du composant (« c in previewChanges ») est la forme sûre.
func TestAlpineCSP_NoNestedPathInXFor(t *testing.T) {
	var found []finding
	walkTemplates(t, func(rel, body string) {
		for _, m := range xForRe.FindAllStringSubmatch(body, -1) {
			expr := strings.TrimSpace(m[1])
			if xForNestedRe.MatchString(expr) {
				found = append(found, finding{rel, `x-for="` + expr + `"`, ""})
			}
		}
	})
	report(t, found, "x-for ne doit pas itérer sur un chemin de propriété (build CSP) — exposer un getter sur le composant :")
}

// x-text="importAmount(c.from)" a fige les x-show du composant englobant.
// Pré-calculer la valeur dans un getter est la forme sûre.
func TestAlpineCSP_NoMemberPathAsCallArgument(t *testing.T) {
	var found []finding
	walkTemplates(t, func(rel, body string) {
		for _, m := range evaluatedAttrRe.FindAllStringSubmatch(body, -1) {
			expr := m[1]
			if strings.Contains(expr, "$event") || strings.Contains(expr, "$el") {
				continue // magies Alpine, réservées aux gestionnaires d'événements
			}
			if callWithMemberArgRe.MatchString(expr) {
				found = append(found, finding{rel, m[0], ""})
			}
		}
	})
	report(t, found, "un chemin de propriété ne doit pas être passé en argument d'appel dans un attribut évalué (build CSP) — pré-calculer dans un getter :")
}

// « loading = true; error = '' » leve « CSP Parser Error » et l'attribut est
// ignore EN ENTIER : l'etat de chargement ne s'activait jamais et le bouton
// « Annuler » du QR ne faisait rien. Une methode du composant est la forme sûre.
func TestAlpineCSP_NoMultiStatementExpression(t *testing.T) {
	var found []finding
	walkTemplates(t, func(rel, body string) {
		for _, m := range anyAlpineAttrRe.FindAllStringSubmatch(body, -1) {
			// Retirer les littéraux d'abord : un « ; » à l'intérieur d'une
			// chaîne ne sépare pas deux instructions.
			expr := stringLiteralRe.ReplaceAllString(m[1], "''")
			body, _, _ := strings.Cut(expr, "//") // commentaire de fin de ligne
			if i := strings.Index(body, ";"); i >= 0 && strings.TrimSpace(body[i+1:]) != "" {
				found = append(found, finding{rel, m[0], ""})
			}
		}
	})
	report(t, found, "un attribut Alpine ne doit pas contenir plusieurs instructions séparées par « ; » (build CSP) — en faire une méthode du composant :")
}
