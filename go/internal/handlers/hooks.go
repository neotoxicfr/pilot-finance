package handlers

// hooks.go — function variables for dependency injection.
// These wrap package-level DB/auth/crypto calls so tests can override them
// to exercise error paths without requiring mock libraries.

import (
	"io"

	qrcode "github.com/skip2/go-qrcode"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/templates"
)

var (
	hookCountUsers               = db.CountUsers
	hookGetUserByBlindIndex      = db.GetUserByBlindIndex
	hookCreateUser               = db.CreateUser
	hookHashPassword             = crypto.HashPassword
	hookEncryptStr               = crypto.Encrypt
	hookDecryptStr               = crypto.Decrypt
	hookUpdatePassword           = db.UpdatePassword
	hookUpdateUserPrefs          = db.UpdateUserPreferences
	hookGenerateToken            = auth.GenerateToken
	hookGetAccountsByUserID      = db.GetAccountsByUserID
	hookGetRecurringByUserID     = db.GetRecurringByUserID
	hookGenerateTOTPSecret       = auth.GenerateTOTPSecret
	hookEnableMFA                = db.EnableMFA
	hookDisableMFA               = db.DisableMFA
	hookDeleteUserAndData        = db.DeleteUserAndData
	hookGetAllUsers              = db.GetAllUsers
	hookGetUserByID              = db.GetUserByID
	hookValidatePending2FAToken  = auth.ValidatePending2FAToken
	hookGeneratePending2FAToken  = auth.GeneratePending2FAToken
	hookUpdateAccountWithYield   = db.UpdateAccountWithYield
	hookCreateAccountWithYield   = db.CreateAccountWithYield
	hookDeleteAccount            = db.DeleteAccount
	hookAccountBelongsToUser     = db.AccountBelongsToUser
	hookUpdateAccountBalance     = db.UpdateAccountBalance
	hookSwapAccountPositions     = db.SwapAccountPositions
	hookGetAuditLog              = db.GetAuditLog
	hookCountAuditLog            = db.CountAuditLog
	hookRender                   = func(w io.Writer, name string, data interface{}) error { return templates.Render(w, name, data) }
	hookQREncode                 = qrcode.Encode
	hookReorderAccounts          = db.ReorderAccounts
	hookDeleteAuthenticator      = db.DeleteAuthenticator
	hookRenameAuthenticator      = db.RenameAuthenticator
	hookUpdateAuthCounter        = db.UpdateAuthenticatorCounter
	hookCreateRecurring          = db.CreateRecurring
	hookUpdateRecurring          = db.UpdateRecurring
	hookDeleteRecurring          = db.DeleteRecurring
	hookVerifyEmailByToken       = db.VerifyEmailByToken
)
