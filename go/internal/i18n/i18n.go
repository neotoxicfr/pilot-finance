// Package i18n gère les traductions de l'application
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var translations = map[string]map[string]string{}

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

		data, err := os.ReadFile(path)
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

// Map retourne la map complète pour une langue (pour injection dans les templates)
func Map(lang string) map[string]string {
	if m, ok := translations[lang]; ok {
		return m
	}
	if m, ok := translations["fr"]; ok {
		return m
	}
	return map[string]string{}
}
