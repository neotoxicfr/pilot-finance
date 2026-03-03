package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// setupBaseEnv configure les 3 variables d'environnement obligatoires avec des valeurs valides.
func setupBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
}

func TestLoad_Success(t *testing.T) {
	setupBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	if cfg.AuthSecret == "" {
		t.Error("AuthSecret should be set")
	}
}

func TestLoad_WithDefaults(t *testing.T) {
	setupBaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "3000" {
		t.Errorf("default Port: want '3000', got %q", cfg.Port)
	}
	if cfg.Host != "localhost" {
		t.Errorf("default Host: want 'localhost', got %q", cfg.Host)
	}
}

func TestLoad_WithEnvOverrides(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("PORT", "8080")
	t.Setenv("HOST", "myapp.example.com")
	t.Setenv("ALLOW_REGISTER", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port: want '8080', got %q", cfg.Port)
	}
	if cfg.Host != "myapp.example.com" {
		t.Errorf("Host: want 'myapp.example.com', got %q", cfg.Host)
	}
	if !cfg.AllowRegister {
		t.Error("AllowRegister: want true")
	}
}

func TestLoad_MissingAuthSecret(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("AUTH_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Error("want error for missing AUTH_SECRET")
	}
}

func TestLoad_AuthSecretTooShort(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("AUTH_SECRET", "short")

	_, err := Load()
	if err == nil {
		t.Error("want error for short AUTH_SECRET")
	}
}

func TestLoad_MissingEncryptionKey(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("ENCRYPTION_KEY", "")

	_, err := Load()
	if err == nil {
		t.Error("want error for missing ENCRYPTION_KEY")
	}
}

func TestLoad_WrongEncryptionKeyLength(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef") // 16 hex chars, not 64

	_, err := Load()
	if err == nil {
		t.Error("want error for wrong ENCRYPTION_KEY length")
	}
}

func TestLoad_MissingBlindIndexKey(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("BLIND_INDEX_KEY", "")

	_, err := Load()
	if err == nil {
		t.Error("want error for missing BLIND_INDEX_KEY")
	}
}

func TestLoad_WrongBlindIndexKeyLength(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("BLIND_INDEX_KEY", "fedcba98") // too short

	_, err := Load()
	if err == nil {
		t.Error("want error for wrong BLIND_INDEX_KEY length")
	}
}

func TestLoad_SMTPHost_MissingCredentials(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASS", "")
	t.Setenv("SMTP_FROM", "")

	_, err := Load()
	if err == nil {
		t.Error("want error for SMTP_HOST with incomplete SMTP config")
	}
}

func TestLoad_SMTP_FullConfig(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "user@example.com")
	t.Setenv("SMTP_PASS", "secret")
	t.Setenv("SMTP_FROM", "noreply@example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with full SMTP: %v", err)
	}
	if cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("SMTPHost: want 'smtp.example.com', got %q", cfg.SMTPHost)
	}
}

// --- ResolveEnv ---

func TestResolveEnv_PlainEnvVar(t *testing.T) {
	t.Setenv("TEST_KEY", "plainvalue")
	got := ResolveEnv("TEST_KEY")
	if got != "plainvalue" {
		t.Errorf("want 'plainvalue', got %q", got)
	}
}

func TestResolveEnv_FileOverridesEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "envvalue")
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte("filevalue\n"), 0600)
	t.Setenv("TEST_KEY_FILE", path)
	got := ResolveEnv("TEST_KEY")
	if got != "filevalue" {
		t.Errorf("want 'filevalue', got %q", got)
	}
}

func TestResolveEnv_FileTrimmed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte("  trimmed  \n"), 0600)
	t.Setenv("TEST_KEY_FILE", path)
	t.Setenv("TEST_KEY", "")
	got := ResolveEnv("TEST_KEY")
	if got != "trimmed" {
		t.Errorf("want 'trimmed', got %q", got)
	}
}

func TestResolveEnv_FileMissing_FallsBackToEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "fallback")
	t.Setenv("TEST_KEY_FILE", "/nonexistent/path/secret.txt")
	got := ResolveEnv("TEST_KEY")
	if got != "fallback" {
		t.Errorf("want 'fallback', got %q", got)
	}
}

func TestResolveEnv_FileEmpty_FallsBackToEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte(""), 0600)
	t.Setenv("TEST_KEY_FILE", path)
	t.Setenv("TEST_KEY", "envfallback")
	got := ResolveEnv("TEST_KEY")
	if got != "envfallback" {
		t.Errorf("want 'envfallback', got %q", got)
	}
}

func TestResolveEnv_FileWhitespaceOnly_FallsBackToEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte("   \n"), 0600)
	t.Setenv("TEST_KEY_FILE", path)
	t.Setenv("TEST_KEY", "envfallback")
	got := ResolveEnv("TEST_KEY")
	if got != "envfallback" {
		t.Errorf("want 'envfallback', got %q", got)
	}
}

func TestResolveEnv_NoFileNoEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "")
	got := ResolveEnv("TEST_KEY")
	if got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestResolveEnv_ReadFileError_FallsBack(t *testing.T) {
	orig := readFileFunc
	defer func() { readFileFunc = orig }()
	readFileFunc = func(_ string) ([]byte, error) {
		return nil, fmt.Errorf("permission denied")
	}
	t.Setenv("TEST_KEY", "fallback")
	t.Setenv("TEST_KEY_FILE", "/some/path")
	got := ResolveEnv("TEST_KEY")
	if got != "fallback" {
		t.Errorf("want 'fallback', got %q", got)
	}
}

// --- resolveEnvWithDefault ---

func TestResolveEnvWithDefault_FileValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte("filevalue"), 0600)
	t.Setenv("TEST_DEF_FILE", path)
	t.Setenv("TEST_DEF", "")
	got := resolveEnvWithDefault("TEST_DEF", "default")
	if got != "filevalue" {
		t.Errorf("want 'filevalue', got %q", got)
	}
}

func TestResolveEnvWithDefault_EnvValue(t *testing.T) {
	t.Setenv("TEST_DEF", "envval")
	got := resolveEnvWithDefault("TEST_DEF", "default")
	if got != "envval" {
		t.Errorf("want 'envval', got %q", got)
	}
}

func TestResolveEnvWithDefault_FallsBackToDefault(t *testing.T) {
	t.Setenv("TEST_DEF", "")
	got := resolveEnvWithDefault("TEST_DEF", "mydefault")
	if got != "mydefault" {
		t.Errorf("want 'mydefault', got %q", got)
	}
}

// --- Load with _FILE ---

func TestLoad_AuthSecretFromFile(t *testing.T) {
	setupBaseEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "auth_secret")
	secret := "this-is-a-file-based-secret-minimum-32chars!!"
	os.WriteFile(path, []byte(secret+"\n"), 0600)
	t.Setenv("AUTH_SECRET_FILE", path)
	t.Setenv("AUTH_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuthSecret != secret {
		t.Errorf("AuthSecret: want from file, got %q", cfg.AuthSecret)
	}
}

func TestLoad_DatabaseURLFromFile(t *testing.T) {
	setupBaseEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "db_url")
	os.WriteFile(path, []byte("file:/secrets/pilot.db\n"), 0600)
	t.Setenv("DATABASE_URL_FILE", path)
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "file:/secrets/pilot.db" {
		t.Errorf("DatabaseURL: want 'file:/secrets/pilot.db', got %q", cfg.DatabaseURL)
	}
}

func TestLoad_SMTPPassFromFile(t *testing.T) {
	setupBaseEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "user@example.com")
	t.Setenv("SMTP_FROM", "noreply@example.com")

	dir := t.TempDir()
	path := filepath.Join(dir, "smtp_pass")
	os.WriteFile(path, []byte("file-smtp-secret"), 0600)
	t.Setenv("SMTP_PASS_FILE", path)
	t.Setenv("SMTP_PASS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SMTPPass != "file-smtp-secret" {
		t.Errorf("SMTPPass: want 'file-smtp-secret', got %q", cfg.SMTPPass)
	}
}

func TestLoad_EncryptionKeyFromFile(t *testing.T) {
	setupBaseEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "enc_key")
	key := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	os.WriteFile(path, []byte(key+"\n"), 0600)
	t.Setenv("ENCRYPTION_KEY_FILE", path)
	t.Setenv("ENCRYPTION_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.EncryptionKey != key {
		t.Errorf("EncryptionKey: want from file, got %q", cfg.EncryptionKey)
	}
}

func TestLoad_BlindIndexKeyFromFile(t *testing.T) {
	setupBaseEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "blind_key")
	key := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	os.WriteFile(path, []byte(key+"\n"), 0600)
	t.Setenv("BLIND_INDEX_KEY_FILE", path)
	t.Setenv("BLIND_INDEX_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BlindIndexKey != key {
		t.Errorf("BlindIndexKey: want from file, got %q", cfg.BlindIndexKey)
	}
}

// getEnv is covered indirectly by Load(), but test it directly too.
func TestGetEnv_WithValue(t *testing.T) {
	t.Setenv("TEST_GETENV_KEY", "myvalue")
	got := getEnv("TEST_GETENV_KEY", "default")
	if got != "myvalue" {
		t.Errorf("want 'myvalue', got %q", got)
	}
}

func TestGetEnv_WithDefault(t *testing.T) {
	t.Setenv("TEST_GETENV_KEY_MISSING", "")
	got := getEnv("TEST_GETENV_KEY_MISSING", "defaultval")
	if got != "defaultval" {
		t.Errorf("want 'defaultval', got %q", got)
	}
}
