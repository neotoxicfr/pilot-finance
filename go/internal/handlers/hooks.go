package handlers

// hooks.go — function variables for dependency injection.
// These wrap package-level DB/auth/crypto calls so tests can override them
// to exercise error paths without requiring mock libraries.

import (
	"context"
	"crypto/rand"
	"io"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/go-webauthn/webauthn/protocol"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/ratelimit"
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
	hookRenderPartial            = templates.RenderPartial
	hookQREncode                 = qrcode.Encode
	hookReorderAccounts          = db.ReorderAccounts
	hookDeleteAuthenticator      = db.DeleteAuthenticator
	hookRenameAuthenticator      = db.RenameAuthenticator
	hookUpdateAuthCounter        = db.UpdateAuthenticatorCounter
	hookCreateRecurring          = db.CreateRecurring
	hookUpdateRecurring          = db.UpdateRecurring
	hookDeleteRecurring          = db.DeleteRecurring
	hookVerifyEmailByToken       = db.VerifyEmailByToken

	// --- db: audit, login, reset, passkeys ---
	hookLogAudit                 = db.LogAudit
	hookUpdateLoginAttempts      = db.UpdateLoginAttempts
	hookUpdatePasswordHash       = db.UpdatePasswordHash
	hookGetAuditLogByUserID      = db.GetAuditLogByUserID
	hookGetAuthenticatorsByUserID = db.GetAuthenticatorsByUserID
	hookSetResetToken            = db.SetResetToken
	hookGetUserByResetToken      = db.GetUserByResetToken
	hookClearResetToken          = db.ClearResetToken
	hookGetAuthByCredentialID    = db.GetAuthenticatorByCredentialID
	hookCreateAuthenticator      = db.CreateAuthenticator
	hookPingDB                   = func(ctx context.Context) error {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return db.DB.PingContext(pingCtx)
	}

	// --- crypto ---
	hookComputeBlindIndex = crypto.ComputeBlindIndex
	hookVerifyPassword    = crypto.VerifyPassword
	hookNeedsRehash       = crypto.NeedsRehash
	hookValidatePassword  = crypto.ValidatePassword
	hookHashToken         = crypto.HashToken

	// --- auth: TOTP, passkeys ---
	hookValidateTOTP      = auth.ValidateTOTP
	hookGenerateTOTPURI   = auth.GenerateTOTPURI
	hookBeginRegistration = auth.BeginRegistration
	hookFinishRegistration = auth.FinishRegistration
	hookBeginLogin         = auth.BeginLogin
	hookFinishLogin        = auth.FinishLogin
	hookParseCCR           = func(r *protocol.CredentialCreationResponse) (*protocol.ParsedCredentialCreationData, error) {
		return r.Parse()
	}

	// --- mail ---
	hookMailIsEnabled     = mail.IsEnabled
	hookSendPasswordReset = mail.SendPasswordReset

	// --- ratelimit ---
	hookRateLimitCheck = ratelimit.Check
	hookRateLimitReset = ratelimit.Reset

	// --- stdlib ---
	hookRandRead = rand.Read
)
