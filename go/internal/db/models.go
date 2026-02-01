// Package db contient les modèles et la connexion SQLite
package db

import "time"

// User représente un utilisateur
type User struct {
	ID                  int64      `json:"id"`
	EmailEncrypted      string     `json:"-"`
	EmailBlindIndex     string     `json:"-"`
	Password            string     `json:"-"`
	Role                string     `json:"role"`
	CreatedAt           time.Time  `json:"created_at"`
	EmailVerified       bool       `json:"email_verified"`
	VerificationToken   *string    `json:"-"`
	ResetToken          *string    `json:"-"`
	ResetTokenExpiry    *time.Time `json:"-"`
	MFAEnabled          bool       `json:"mfa_enabled"`
	MFASecret           *string    `json:"-"`
	FailedLoginAttempts int        `json:"-"`
	LockUntil           *time.Time `json:"-"`
	SessionVersion      int        `json:"session_version"`
}

// Account représente un compte bancaire/épargne
type Account struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"`
	Name             string     `json:"name"`              // Chiffré en BDD
	Balance          float64    `json:"balance"`
	Color            string     `json:"color"`
	Position         int        `json:"position"`
	UpdatedAt        time.Time  `json:"updated_at"`
	IsYieldActive    bool       `json:"is_yield_active"`
	YieldType        string     `json:"yield_type"`        // FIXED ou RANGE
	YieldMin         float64    `json:"yield_min"`
	YieldMax         float64    `json:"yield_max"`
	YieldFrequency   string     `json:"yield_frequency"`   // YEARLY, MONTHLY
	PayoutFrequency  string     `json:"payout_frequency"`  // MONTHLY, YEARLY
	LastYieldDate    *time.Time `json:"last_yield_date"`
	ReinvestmentRate int        `json:"reinvestment_rate"` // 0-100
	TargetAccountID  *int64     `json:"target_account_id"`
}

// Transaction représente une transaction
type Transaction struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	AccountID   int64     `json:"account_id"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"` // Chiffré en BDD
	Category    *string   `json:"category"`
	Date        time.Time `json:"date"`
	CreatedAt   time.Time `json:"created_at"`
}

// RecurringOperation représente une opération récurrente
type RecurringOperation struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	AccountID   int64      `json:"account_id"`
	ToAccountID *int64     `json:"to_account_id"`
	Amount      float64    `json:"amount"`
	Description string     `json:"description"` // Chiffré en BDD
	DayOfMonth  int        `json:"day_of_month"`
	LastRunDate *time.Time `json:"last_run_date"`
	IsActive    bool       `json:"is_active"`
}

// Authenticator représente une Passkey WebAuthn
type Authenticator struct {
	ID                   int64  `json:"id"`
	CredentialID         string `json:"credential_id"`
	CredentialPublicKey  string `json:"-"`
	Counter              int    `json:"counter"`
	CredentialDeviceType string `json:"credential_device_type"`
	CredentialBackedUp   bool   `json:"credential_backed_up"`
	Transports           string `json:"transports"`
	UserID               int64  `json:"user_id"`
	Name                 string `json:"name"`
}
