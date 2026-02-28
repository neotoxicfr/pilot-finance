package config

import (
	"testing"
)

func TestLoad_Success(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

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
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

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
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
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
	t.Setenv("AUTH_SECRET", "")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	_, err := Load()
	if err == nil {
		t.Error("want error for missing AUTH_SECRET")
	}
}

func TestLoad_AuthSecretTooShort(t *testing.T) {
	t.Setenv("AUTH_SECRET", "short")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	_, err := Load()
	if err == nil {
		t.Error("want error for short AUTH_SECRET")
	}
}

func TestLoad_MissingEncryptionKey(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	_, err := Load()
	if err == nil {
		t.Error("want error for missing ENCRYPTION_KEY")
	}
}

func TestLoad_WrongEncryptionKeyLength(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef") // 16 hex chars, not 64
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	_, err := Load()
	if err == nil {
		t.Error("want error for wrong ENCRYPTION_KEY length")
	}
}

func TestLoad_MissingBlindIndexKey(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "")

	_, err := Load()
	if err == nil {
		t.Error("want error for missing BLIND_INDEX_KEY")
	}
}

func TestLoad_WrongBlindIndexKeyLength(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba98") // too short

	_, err := Load()
	if err == nil {
		t.Error("want error for wrong BLIND_INDEX_KEY length")
	}
}

func TestLoad_MailEnabled_MissingSMTP(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	t.Setenv("ENABLE_MAIL", "true")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_USER", "")
	t.Setenv("SMTP_PASS", "")
	t.Setenv("SMTP_FROM", "")

	_, err := Load()
	if err == nil {
		t.Error("want error for ENABLE_MAIL=true with incomplete SMTP config")
	}
}

func TestLoad_MailEnabled_FullSMTP(t *testing.T) {
	t.Setenv("AUTH_SECRET", "this-is-a-32-char-secret-minimum!!")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("BLIND_INDEX_KEY", "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	t.Setenv("ENABLE_MAIL", "true")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "user@example.com")
	t.Setenv("SMTP_PASS", "secret")
	t.Setenv("SMTP_FROM", "noreply@example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with full SMTP: %v", err)
	}
	if !cfg.EnableMail {
		t.Error("want EnableMail=true")
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
