// Package i18n gère les traductions de l'application
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// translations contient toutes les traductions chargées, indexées par langue.
//
// Contrat de concurrence : Load() doit être appelé exactement une fois au
// démarrage, AVANT que le serveur HTTP n'accepte des requêtes. Une fois Load()
// terminé, la map n'est plus jamais mutée ; T() et Map() ne font que la lire.
// Cette séquence "load-once-before-serve" rend les lectures concurrentes sûres
// sans verrou. Ne pas appeler Load() en cours de service.
var translations = map[string]map[string]string{}

// readFileFn est injectable pour les tests (couvre la branche d'erreur os.ReadFile).
var readFileFn = os.ReadFile

// Load charge les fichiers de traduction depuis le dossier locales.
// Voir le contrat de concurrence documenté sur translations.
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

	// La langue de fallback "fr" doit être chargée et non vide, sinon T()
	// renverrait systématiquement les clés brutes pour les traductions
	// manquantes — révélateur d'un déploiement incomplet (locales/fr.json
	// absent ou vide).
	if len(translations["fr"]) == 0 {
		return fmt.Errorf("i18n: langue de fallback 'fr' absente ou vide dans %s", localesDir)
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

// Map retourne la map complète pour une langue (pour injection dans les templates).
// html/template ne mute jamais les données, donc pas besoin de copie.
func Map(lang string) map[string]string {
	src := translations[lang]
	if src == nil {
		src = translations["fr"]
	}
	if src == nil {
		return map[string]string{}
	}
	return src
}
