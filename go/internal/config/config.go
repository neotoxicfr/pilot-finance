// Package config gère la configuration de l'application
package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// readFileFunc est un hook pour lire les fichiers de secrets (testabilité).
var readFileFunc = os.ReadFile

// ResolveEnv retourne la valeur pour key, en privilégiant KEY_FILE (lecture fichier) sur KEY (env).
// Si KEY_FILE est défini, le fichier est lu et trimé. En cas d'erreur ou fichier vide,
// retombe sur os.Getenv(KEY) avec un warning dans les logs.
func ResolveEnv(key string) string {
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		data, err := readFileFunc(filePath)
		if err != nil {
			slog.Warn("impossible de lire le fichier secret", "key", key+"_FILE", "path", filePath, "err", err)
		} else {
			val := strings.TrimSpace(string(data))
			if val == "" {
				slog.Warn("fichier secret vide", "key", key+"_FILE", "path", filePath)
			} else {
				slog.Info("secret chargé depuis fichier", "key", key+"_FILE")
				return val
			}
		}
	}
	return os.Getenv(key)
}

func resolveEnvWithDefault(key, defaultValue string) string {
	if val := ResolveEnv(key); val != "" {
		return val
	}
	return defaultValue
}

// Config contient toute la configuration de l'application
type Config struct {
	// Serveur
	Host string
	Port string

	// Base de données
	DatabaseURL string

	// Sécurité
	AuthSecret     string
	EncryptionKey  string
	BlindIndexKey  string

	// Fonctionnalités
	AllowRegister bool

	// SMTP (optionnel — activé automatiquement si SMTP_HOST est défini)
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

// Load charge la configuration depuis les variables d'environnement
func Load() (*Config, error) {
	cfg := &Config{
		Host:          getEnv("HOST", "localhost"),
		Port:          getEnv("PORT", "3000"),
		DatabaseURL:   resolveEnvWithDefault("DATABASE_URL", "file:./data/pilot.db"),
		AuthSecret:    ResolveEnv("AUTH_SECRET"),
		EncryptionKey: ResolveEnv("ENCRYPTION_KEY"),
		BlindIndexKey: ResolveEnv("BLIND_INDEX_KEY"),
		AllowRegister: getEnv("ALLOW_REGISTER", "false") == "true",
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      getEnv("SMTP_PORT", "587"),
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      ResolveEnv("SMTP_PASS"),
		SMTPFrom:      os.Getenv("SMTP_FROM"),
	}

	// Validation des clés critiques
	if cfg.AuthSecret == "" {
		return nil, fmt.Errorf("AUTH_SECRET requis")
	}
	if len(cfg.AuthSecret) < 32 {
		return nil, fmt.Errorf("AUTH_SECRET trop court (min 32 caractères)")
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY requis")
	}
	if _, err := hex.DecodeString(cfg.EncryptionKey); err != nil || len(cfg.EncryptionKey) != 64 {
		return nil, fmt.Errorf("ENCRYPTION_KEY doit faire 64 caractères hex valides (32 bytes)")
	}
	if cfg.BlindIndexKey == "" {
		return nil, fmt.Errorf("BLIND_INDEX_KEY requis")
	}
	if _, err := hex.DecodeString(cfg.BlindIndexKey); err != nil || len(cfg.BlindIndexKey) != 64 {
		return nil, fmt.Errorf("BLIND_INDEX_KEY doit faire 64 caractères hex valides (32 bytes)")
	}

	// Validation SMTP si configuré
	if cfg.SMTPHost != "" {
		if cfg.SMTPUser == "" || cfg.SMTPPass == "" || cfg.SMTPFrom == "" {
			return nil, fmt.Errorf("SMTP_HOST défini mais configuration SMTP incomplète (USER/PASS/FROM)")
		}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
