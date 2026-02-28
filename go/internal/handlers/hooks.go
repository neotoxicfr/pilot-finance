package handlers

// hooks.go — function variables for dependency injection.
// These wrap package-level DB/auth/crypto calls so tests can override them
// to exercise error paths without requiring mock libraries.

import (
	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

var (
	hookCountUsers           = db.CountUsers
	hookGetUserByBlindIndex  = db.GetUserByBlindIndex
	hookCreateUser           = db.CreateUser
	hookHashPassword         = crypto.HashPassword
	hookEncryptStr           = crypto.Encrypt
	hookUpdatePassword       = db.UpdatePassword
	hookUpdateUserPrefs      = db.UpdateUserPreferences
	hookGenerateToken        = auth.GenerateToken
	hookGetAccountsByUserID  = db.GetAccountsByUserID
	hookGetRecurringByUserID = db.GetRecurringByUserID
	hookGenerateTOTPSecret   = auth.GenerateTOTPSecret
	hookEnableMFA            = db.EnableMFA
	hookDisableMFA           = db.DisableMFA
	hookDeleteUserAndData    = db.DeleteUserAndData
	hookDeleteUser           = db.DeleteUser
	hookGetAllUsers          = db.GetAllUsers
)
