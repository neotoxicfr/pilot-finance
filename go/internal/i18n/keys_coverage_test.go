package i18n_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Les templates lisent les libellés via `{{index .T "clé"}}`, où `.T` est une
// map. `index` sur une clé absente ne remonte PAS d'erreur : il rend la chaîne
// VIDE. Un libellé manquant produit donc un bouton sans texte, invisible à
// l'écran et parfaitement silencieux dans les logs — c'est exactement ainsi que
// toute l'interface des codes de récupération a pu passer une revue en
// paraissant fonctionner.
//
// Ce test relie les deux moitiés : toute clé référencée par un template doit
// exister dans CHAQUE locale, et les locales doivent rester à parité.

// Le motif exige `index` : sans lui il attraperait aussi le `"T" $.T` passé en
// argument d'un `dict`, dont le terme suivant n'est pas une clé de traduction.
var keyRefRe = regexp.MustCompile(`index\s+\$?\.T\s+"([^"]+)"`)

func repoPath(t *testing.T, rel ...string) string {
	t.Helper()
	parts := append([]string{"..", ".."}, rel...)
	p, err := filepath.Abs(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("chemin %v: %v", rel, err)
	}
	return p
}

func loadLocale(t *testing.T, lang string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(repoPath(t, "locales", lang+".json"))
	if err != nil {
		t.Fatalf("lecture locale %s: %v", lang, err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing locale %s: %v", lang, err)
	}
	return m
}

// collectTemplateKeys retourne, pour chaque clé i18n référencée, le fichier où
// elle apparaît en premier — de quoi pointer directement le coupable.
func collectTemplateKeys(t *testing.T) map[string]string {
	t.Helper()
	root := repoPath(t, "templates")
	keys := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		for _, m := range keyRefRe.FindAllSubmatch(body, -1) {
			key := string(m[1])
			if _, seen := keys[key]; !seen {
				keys[key] = filepath.ToSlash(rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("parcours des templates: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("aucune clé i18n trouvée dans les templates — le test ne teste rien")
	}
	return keys
}

func TestTemplateKeysExistInAllLocales(t *testing.T) {
	keys := collectTemplateKeys(t)

	for _, lang := range []string{"fr", "en"} {
		loc := loadLocale(t, lang)
		var missing []string
		for key, file := range keys {
			if _, ok := loc[key]; !ok {
				missing = append(missing, key+" ("+file+")")
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("locales/%s.json : %d clé(s) référencée(s) par un template mais absente(s) — elles s'afficheraient VIDES :\n  %s",
				lang, len(missing), strings.Join(missing, "\n  "))
		}
	}
}

func TestLocalesAreAtParity(t *testing.T) {
	fr := loadLocale(t, "fr")
	en := loadLocale(t, "en")

	var onlyFR, onlyEN []string
	for k := range fr {
		if _, ok := en[k]; !ok {
			onlyFR = append(onlyFR, k)
		}
	}
	for k := range en {
		if _, ok := fr[k]; !ok {
			onlyEN = append(onlyEN, k)
		}
	}
	sort.Strings(onlyFR)
	sort.Strings(onlyEN)

	if len(onlyFR) > 0 {
		t.Errorf("%d clé(s) présente(s) en fr mais pas en en :\n  %s", len(onlyFR), strings.Join(onlyFR, "\n  "))
	}
	if len(onlyEN) > 0 {
		t.Errorf("%d clé(s) présente(s) en en mais pas en fr :\n  %s", len(onlyEN), strings.Join(onlyEN, "\n  "))
	}
}

// Une valeur vide est indistinguable d'une clé manquante à l'écran : la refuser
// oblige à choisir explicitement un libellé plutôt qu'à en oublier un.
func TestNoEmptyTranslations(t *testing.T) {
	for _, lang := range []string{"fr", "en"} {
		loc := loadLocale(t, lang)
		var empty []string
		for k, v := range loc {
			if strings.TrimSpace(v) == "" {
				empty = append(empty, k)
			}
		}
		sort.Strings(empty)
		if len(empty) > 0 {
			t.Errorf("locales/%s.json : %d valeur(s) vide(s) :\n  %s", lang, len(empty), strings.Join(empty, "\n  "))
		}
	}
}
