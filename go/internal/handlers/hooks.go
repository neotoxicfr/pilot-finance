package handlers

// hooks.go — function variables for dependency injection.
// These wrap package-level DB/auth/crypto calls so tests can override them
// to exercise error paths without requiring mock libraries.

import (
	"context"
	"crypto/rand"
	"image/png"
	"io"
	"time"

	"github.com/go-webauthn/webauthn/protocol"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/ratelimit"
	"pilot-finance/internal/templates"
)

var (
	hookCountUsers               = db.CountUsers
	hookGetUserByBlindIndex      = db.GetUserByBlindIndex
	hookCreateUserAtomic         = db.CreateUserAtomic
	hookHashPassword             = crypto.HashPassword
	hookEncryptStr               = crypto.Encrypt
	hookDecryptStr               = crypto.Decrypt
	hookUpdatePassword           = db.UpdatePassword
	hookIncrementSessionVersion  = db.IncrementSessionVersion
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
	hookGetSessionVersion        = db.GetSessionVersion
	hookValidatePending2FAToken  = auth.ValidatePending2FAToken
	hookGeneratePending2FAToken  = auth.GeneratePending2FAToken
	hookGenerateMFASetupToken    = auth.GenerateMFASetupToken
	hookValidateMFASetupToken    = auth.ValidateMFASetupToken
	hookUpdateAccountWithYield   = db.UpdateAccountWithYield
	hookCreateAccountWithYield   = db.CreateAccountWithYield
	hookDeleteAccount            = db.DeleteAccount
	hookAccountBelongsToUser     = db.AccountBelongsToUser
	hookCountAccountsByUserID    = db.CountAccountsByUserID
	hookUpdateAccountBalance     = db.UpdateAccountBalance
	hookUpdateAccountBalancesTx  = db.UpdateAccountBalancesTx
	hookSwapAccountPositions     = db.SwapAccountPositions
	hookGetAuditLog              = db.GetAuditLog
	hookCountAuditLog            = db.CountAuditLog
	hookRender                   = func(w io.Writer, name string, data interface{}) error { return templates.Render(w, name, data) }
	hookRenderPartial            = templates.RenderPartial
	hookQREncode                 = qrEncodePNG
	hookReorderAccounts          = db.ReorderAccounts
	hookDeleteAuthenticator      = db.DeleteAuthenticator
	hookRenameAuthenticator      = db.RenameAuthenticator
	hookUpdateAuthCounter        = db.UpdateAuthenticatorCounter
	hookCreateRecurring          = db.CreateRecurring
	hookUpdateRecurring          = db.UpdateRecurring
	hookDeleteRecurring          = db.DeleteRecurring
	hookSetVerificationToken     = db.SetVerificationToken
	hookGetUserByVerificationTok = db.GetUserByVerificationToken
	hookMarkEmailVerified        = db.MarkEmailVerified

	// --- db: audit, login, reset, passkeys ---
	hookLogAudit                    = db.LogAudit
	hookUpdateLoginAttempts         = db.UpdateLoginAttempts
	hookUpdatePasswordHash          = db.UpdatePasswordHash
	hookGetAuditLogByUserID         = db.GetAuditLogByUserID
	hookGetAuthenticatorsByUserID   = db.GetAuthenticatorsByUserID
	hookSetResetToken               = db.SetResetToken
	hookGetUserByResetToken         = db.GetUserByResetToken
	hookUpdatePasswordAndClearReset = db.UpdatePasswordAndClearResetToken
	hookGetAuthByCredentialID       = db.GetAuthenticatorByCredentialID
	hookCreateAuthenticator         = db.CreateAuthenticator
	hookPingDB                      = func(ctx context.Context) error {
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
	hookValidateTOTP       = auth.ValidateTOTP
	hookGenerateTOTPURI    = auth.GenerateTOTPURI
	hookBeginRegistration  = auth.BeginRegistration
	hookFinishRegistration = auth.FinishRegistration
	hookBeginLogin         = auth.BeginLogin
	hookFinishLogin        = auth.FinishLogin
	hookParseCCR           = func(r *protocol.CredentialCreationResponse) (*protocol.ParsedCredentialCreationData, error) {
		return r.Parse()
	}

	// --- mail ---
	hookMailIsEnabled     = mail.IsEnabled
	hookSendPasswordReset = mail.SendPasswordReset
	hookSendVerification  = mail.SendVerificationEmail

	// --- ratelimit ---
	hookRateLimitCheck = ratelimit.Check
	hookRateLimitReset = ratelimit.Reset

	// --- middleware ---
	hookInvalidateSessionCache = func(userID int64) { middleware.InvalidateSessionCache(userID) }

	// --- stdlib ---
	hookRandRead = rand.Read
	// Injectable pour couvrir la branche d'erreur d'encodage PNG de
	// qrEncodePNG, inatteignable avec un bytes.Buffer.
	hookPNGEncode = png.Encode
)
