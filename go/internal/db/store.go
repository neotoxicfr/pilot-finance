package db

import "time"

// Store définit le contrat de la couche de persistance.
// L'implémentation concrète utilise les fonctions package-level (SQLite),
// les handlers y accèdent via les hooks (DI) définis dans hooks.go.
type Store interface {
	// Users
	CountUsers() (int, error)
	CreateUser(emailEncrypted, emailBlindIndex, password, role string) (int64, error)
	GetUserByBlindIndex(blindIndex string) (*User, error)
	GetUserByID(id int64) (*User, error)
	GetSessionVersion(id int64) (int, error)
	GetUserAuthData(id int64) (sessionVersion int, emailEncrypted string, err error)
	GetUserByResetToken(hashedToken string) (*User, error)
	GetAllUsers() ([]User, error)
	UpdateLoginAttempts(userID int64, attempts int, lockUntil *time.Time) error
	UpdatePasswordHash(userID int64, hashedPassword string) error
	UpdatePassword(userID int64, hashedPassword string) error
	UpdateUserPreferences(userID int64, language, currency string) error
	SetResetToken(userID int64, hashedToken string, expiry time.Time) error
	ClearResetToken(userID int64) error
	DeleteUser(userID int64) error
	DeleteUserAndData(userID int64) error
	VerifyEmailByToken(hashedToken string) error

	// MFA
	EnableMFA(userID int64, encryptedSecret string) error
	DisableMFA(userID int64) error

	// Accounts
	CreateAccountWithYield(userID int64, name string, balance float64, color string, position int, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error
	UpdateAccountWithYield(id, userID int64, name string, balance float64, color string, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error
	UpdateAccountBalance(id, userID int64, balance float64) error
	DeleteAccount(id, userID int64) error
	AccountBelongsToUser(accountID, userID int64) (bool, error)
	SwapAccountPositions(id1, id2, userID int64) error
	ReorderAccounts(userID int64, ids []int64) error
	GetAccountsByUserID(userID int64) ([]Account, error)

	// Recurring
	CreateRecurring(userID, accountID int64, toAccountID *int64, description string, amount float64, dayOfMonth int) error
	UpdateRecurring(id, userID int64, description string, amount float64, dayOfMonth int, toAccountID *int64) error
	DeleteRecurring(id, userID int64) error
	GetRecurringByUserID(userID int64) ([]RecurringOperation, error)

	// Audit
	LogAudit(userID int64, action, ip, userAgent string)
	GetAuditLogByUserID(userID int64) ([]AuditEntry, error)
	GetAuditLog(page, limit int) ([]AuditEntry, error)
	CountAuditLog() (int, error)

	// Authenticators (Passkeys)
	CreateAuthenticator(credentialID, publicKey string, counter int, deviceType string, backedUp, backupEligible bool, transports string, userID int64) error
	GetAuthenticatorsByUserID(userID int64) ([]Authenticator, error)
	GetAuthenticatorByCredentialID(credentialID string) (*Authenticator, error)
	UpdateAuthenticatorCounter(credentialID string, counter int) error
	DeleteAuthenticator(id, userID int64) error
	RenameAuthenticator(id, userID int64, name string) error
}
