// Package i18n gère les traductions de l'application
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var translations = map[string]map[string]string{}

// readFileFn est injectable pour les tests (couvre la branche d'erreur os.ReadFile).
var readFileFn = os.ReadFile

// Load charge les fichiers de traduction depuis le dossier locales
func Load(localesDir string) error {
	entries, err := os.ReadDir(localesDir)
	if err != nil {
		return fmt.Errorf("i18n: lecture dossier %s: %w", localesDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		lang := entry.Name()[:len(entry.Name())-5] // strip .json
		path := filepath.Join(localesDir, entry.Name())

		data, err := readFileFn(path)
		if err != nil {
			return fmt.Errorf("i18n: lecture %s: %w", path, err)
		}

		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("i18n: parsing %s: %w", path, err)
		}

		translations[lang] = m
	}

	return nil
}

// T retourne la traduction d'une clé pour une langue donnée.
// Fallback sur "fr" si la clé n'existe pas dans la langue demandée.
func T(lang, key string) string {
	if m, ok := translations[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	// Fallback français
	if m, ok := translations["fr"]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// Map retourne une copie de la map complète pour une langue (pour injection dans les templates).
// La copie empêche toute mutation accidentelle de la source.
func Map(lang string) map[string]string {
	src := translations[lang]
	if src == nil {
		src = translations["fr"]
	}
	if src == nil {
		return map[string]string{}
	}
	cp := make(map[string]string, len(src))
	for k, v := range src {
		cp[k] = v
	}
	return cp
}
