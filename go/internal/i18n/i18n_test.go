package i18n

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeLangFile creates a minimal JSON locale file in dir.
func writeLangFile(t *testing.T, dir, lang string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, lang+".json"), []byte(content), 0644); err != nil {
		t.Fatalf("writeLangFile: %v", err)
	}
}

// resetTranslations clears the global translations map between tests.
func resetTranslations() {
	for k := range translations {
		delete(translations, k)
	}
}

func TestLoad_ValidDir(t *testing.T) {
	resetTranslations()
	dir := t.TempDir()
	writeLangFile(t, dir, "fr", `{"hello":"Bonjour","world":"Monde"}`)
	writeLangFile(t, dir, "en", `{"hello":"Hello","world":"World"}`)

	if err := Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if translations["fr"]["hello"] != "Bonjour" {
		t.Errorf("fr.hello: want 'Bonjour', got %q", translations["fr"]["hello"])
	}
	if translations["en"]["hello"] != "Hello" {
		t.Errorf("en.hello: want 'Hello', got %q", translations["en"]["hello"])
	}
}

func TestLoad_InvalidDir(t *testing.T) {
	resetTranslations()
	err := Load("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("want error for nonexistent dir")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	resetTranslations()
	dir := t.TempDir()
	writeLangFile(t, dir, "fr", `{ not valid json }`)

	err := Load(dir)
	if err == nil {
		t.Error("want error for invalid JSON")
	}
}

func TestLoad_SkipsDirectories(t *testing.T) {
	resetTranslations()
	dir := t.TempDir()
	// Create a subdirectory inside locales — should be skipped
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	writeLangFile(t, dir, "fr", `{"key":"val"}`)

	if err := Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoad_SkipsNonJSONFiles(t *testing.T) {
	resetTranslations()
	dir := t.TempDir()
	// Create a .txt file — should be skipped
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}
	writeLangFile(t, dir, "fr", `{"key":"val"}`)

	if err := Load(dir); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestT_KnownKey(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{"greeting": "Bonjour"}
	translations["en"] = map[string]string{"greeting": "Hello"}

	if got := T("en", "greeting"); got != "Hello" {
		t.Errorf("T(en, greeting): want 'Hello', got %q", got)
	}
	if got := T("fr", "greeting"); got != "Bonjour" {
		t.Errorf("T(fr, greeting): want 'Bonjour', got %q", got)
	}
}

func TestT_FallbackToFr(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{"fallback_key": "FallbackFR"}
	// "en" does NOT have fallback_key

	if got := T("en", "fallback_key"); got != "FallbackFR" {
		t.Errorf("T(en, missing key) should fallback to fr: want 'FallbackFR', got %q", got)
	}
}

func TestT_UnknownLang_FallsBackToFr(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{"key": "ValFR"}

	if got := T("de", "key"); got != "ValFR" {
		t.Errorf("T(de, key) should fallback to fr: want 'ValFR', got %q", got)
	}
}

func TestT_KeyNotFound_ReturnsKey(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{}

	key := "totally.unknown.key"
	if got := T("fr", key); got != key {
		t.Errorf("T(fr, unknown): want key itself %q, got %q", key, got)
	}
}

func TestT_NoTranslations_ReturnsKey(t *testing.T) {
	resetTranslations()

	key := "some.key"
	if got := T("fr", key); got != key {
		t.Errorf("T with no translations: want key %q, got %q", key, got)
	}
}

func TestMap_KnownLang(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{"a": "1", "b": "2"}

	m := Map("fr")
	if m["a"] != "1" || m["b"] != "2" {
		t.Errorf("Map(fr): unexpected content: %v", m)
	}
}

func TestMap_FallsBackToFr(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{"x": "X"}

	m := Map("de")
	if m["x"] != "X" {
		t.Errorf("Map(de) should fallback to fr, got %v", m)
	}
}

func TestMap_ReturnsSource(t *testing.T) {
	resetTranslations()
	translations["fr"] = map[string]string{"key": "value"}

	m := Map("fr")
	// Map returns the source map directly (templates are read-only)
	if m["key"] != "value" {
		t.Errorf("Map: want %q, got %q", "value", m["key"])
	}
}

func TestMap_NoTranslations_ReturnsEmpty(t *testing.T) {
	resetTranslations()

	m := Map("fr")
	if len(m) != 0 {
		t.Errorf("Map with no translations: want empty map, got %v", m)
	}
}

// TestLoad_ReadFileError covers the os.ReadFile error branch via the readFileFn hook.
func TestLoad_ReadFileError(t *testing.T) {
	resetTranslations()

	orig := readFileFn
	defer func() { readFileFn = orig }()

	readFileFn = func(path string) ([]byte, error) {
		return nil, errors.New("read permission denied")
	}

	dir := t.TempDir()
	writeLangFile(t, dir, "fr", `{"key":"val"}`)

	err := Load(dir)
	if err == nil {
		t.Error("want error when readFileFn fails")
	}
}
