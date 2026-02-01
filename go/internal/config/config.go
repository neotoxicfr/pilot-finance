// Package config gère la configuration de l'application
package config

import (
	"fmt"
	"os"
)

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
	EnableMail    bool

	// SMTP (optionnel)
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
		DatabaseURL:   getEnv("DATABASE_URL", "file:./data/pilot.db"),
		AuthSecret:    os.Getenv("AUTH_SECRET"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		BlindIndexKey: os.Getenv("BLIND_INDEX_KEY"),
		AllowRegister: getEnv("ALLOW_REGISTER", "false") == "true",
		EnableMail:    getEnv("ENABLE_MAIL", "false") == "true",
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      getEnv("SMTP_PORT", "587"),
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
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
	if len(cfg.EncryptionKey) != 64 {
		return nil, fmt.Errorf("ENCRYPTION_KEY doit faire 64 caractères hex (32 bytes)")
	}
	if cfg.BlindIndexKey == "" {
		return nil, fmt.Errorf("BLIND_INDEX_KEY requis")
	}
	if len(cfg.BlindIndexKey) != 64 {
		return nil, fmt.Errorf("BLIND_INDEX_KEY doit faire 64 caractères hex (32 bytes)")
	}

	// Validation SMTP si mail activé
	if cfg.EnableMail {
		if cfg.SMTPHost == "" || cfg.SMTPUser == "" || cfg.SMTPPass == "" || cfg.SMTPFrom == "" {
			return nil, fmt.Errorf("ENABLE_MAIL=true mais configuration SMTP incomplète")
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
