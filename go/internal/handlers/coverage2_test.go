package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/ratelimit"
)

// ── helpers ─────────────────────────────────────────────────────────────────

// postBody builds a POST request with a raw body (no form encoding).
func postBody(path string, body []byte, ct string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	return req
}

// ── accounts.go ─────────────────────────────────────────────────────────────

// accounts.go:30 — r.ParseForm() error → 400 (multipart with bad boundary)
func TestCreateAccount_ParseFormError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pfe@example.com", "ValidP@ss1!", "USER")

	// multipart/form-data with a broken boundary forces ParseForm to fail
	req := injectUser(
		postBody("/accounts", []byte("--bad\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\nTest\r\n--bad--"),
			"multipart/form-data; boundary=bad"),
		mu(uid, "USER"),
	)
	// Sabotage the body so ParseMultipartForm fails
	req.Header.Set("Content-Type", "multipart/form-data; boundary=")
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (ParseForm error), got %d", rr.Code)
	}
}

// accounts.go:55 — hookEncryptStr error → 500
func TestCreateAccount_EncryptError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "enc_err@example.com", "ValidP@ss1!", "USER")

	orig := hookEncryptStr
	hookEncryptStr = func(s string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookEncryptStr = orig })

	req := injectUser(post("/accounts", url.Values{
		"name":    {"TestAccount"},
		"balance": {"0"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (encrypt error), got %d", rr.Code)
	}
}

// accounts.go:133+138 — ParseInt OK then hookUpdateAccountWithYield error → 500
func TestCreateAccount_UpdateError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "upd_err@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookUpdateAccountWithYield
	hookUpdateAccountWithYield = func(id, userID int64, name string, balance int64, color string, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
		return errTest
	}
	t.Cleanup(func() { hookUpdateAccountWithYield = orig })

	req := injectUser(post("/accounts", url.Values{
		"id":      {intStr(accID)},
		"name":    {"Updated"},
		"balance": {"100"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (update error), got %d", rr.Code)
	}
}

// accounts.go:152 — hookCreateAccountWithYield error → 500
func TestCreateAccount_CreateError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "cre_err@example.com", "ValidP@ss1!", "USER")

	orig := hookCreateAccountWithYield
	hookCreateAccountWithYield = func(userID int64, name string, balance int64, color string, position int, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
		return errTest
	}
	t.Cleanup(func() { hookCreateAccountWithYield = orig })

	req := injectUser(post("/accounts", url.Values{
		"name":    {"NewAccount"},
		"balance": {"500"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (create error), got %d", rr.Code)
	}
}

// accounts.go:179 — hookDeleteAccount error → 500
func TestDeleteAccount_DBError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delacc_err@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookDeleteAccount
	hookDeleteAccount = func(id, userID int64) error { return errTest }
	t.Cleanup(func() { hookDeleteAccount = orig })

	req := injectUser(withParam(httptest.NewRequest(http.MethodDelete, "/accounts/"+intStr(accID), nil), "id", intStr(accID)), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DeleteAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (delete DB error), got %d", rr.Code)
	}
}

// accounts.go:205 — UpdateBalance ParseForm error → 400
func TestUpdateBalance_ParseFormError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "ub_pfe@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(
		withParam(
			postBody("/accounts/"+intStr(accID)+"/balance", []byte(""), "multipart/form-data; boundary="),
			"id", intStr(accID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (UpdateBalance ParseForm error), got %d", rr.Code)
	}
}

// accounts.go:218 — hookUpdateAccountBalance error → 500
func TestUpdateBalance_DBError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "ub_dberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookUpdateAccountBalance
	hookUpdateAccountBalance = func(id, userID int64, balance int64) error { return errTest }
	t.Cleanup(func() { hookUpdateAccountBalance = orig })

	req := injectUser(
		withParam(
			post("/accounts/"+intStr(accID)+"/balance", url.Values{"balance": {"999"}}),
			"id", intStr(accID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (UpdateBalance DB error), got %d", rr.Code)
	}
}

// accounts.go:249 — hookGetAccountsByUserID error in MoveAccount → 500
func TestMoveAccount_GetAccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "move_get_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(userID int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(
		withParam(
			httptest.NewRequest(http.MethodPut, "/accounts/1/move?direction=up", nil),
			"id", "1",
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (MoveAccount get accounts error), got %d", rr.Code)
	}
}

// accounts.go:285 — hookSwapAccountPositions error → 500
func TestMoveAccount_SwapError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "swap_err@example.com", "ValidP@ss1!", "USER")
	acc1 := createAcc(t, uid)
	createAcc(t, uid) // second account so swap is valid

	orig := hookSwapAccountPositions
	hookSwapAccountPositions = func(id1, id2, userID int64) error { return errTest }
	t.Cleanup(func() { hookSwapAccountPositions = orig })

	req := injectUser(
		withParam(
			httptest.NewRequest(http.MethodPut, "/accounts/"+intStr(acc1)+"/move?direction=down", nil),
			"id", intStr(acc1),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (swap positions error), got %d", rr.Code)
	}
}

// accounts.go:310 — invalid JSON body in ReorderAccounts → 400
func TestReorderAccounts_InvalidJSON(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "reorder_json@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		postBody("/accounts/reorder", []byte("{not valid json}"), "application/json"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid JSON), got %d", rr.Code)
	}
}

// accounts.go:323 — hookGetAccountsByUserID error in renderAccountsList (non-fatal, logs)
// We exercise this by failing the hook AFTER a successful CreateAccount path has run.
// The handler should still return 200-range HTML (it just logs the error).
func TestRenderAccountsList_GetAccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "ral_get_err@example.com", "ValidP@ss1!", "USER")

	// hookGetAccountsByUserID is called in renderAccountsList only
	// (CreateAccount uses hookCountAccountsByUserID for position).
	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(userID int64) ([]db.Account, error) {
		return nil, errTest
	}
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(post("/accounts", url.Values{
		"name":    {"RenderErrAcc"},
		"balance": {"100"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("renderAccountsList accounts error should return 500, got %d", rr.Code)
	}
}

// accounts.go:327 — hookGetRecurringByUserID error in renderAccountsList (non-fatal)
func TestRenderAccountsList_GetRecurringError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "ral_rec_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(userID int64) ([]db.RecurringOperation, error) { return nil, errTest }
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(post("/accounts", url.Values{
		"name":    {"RecErrAcc"},
		"balance": {"100"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("renderAccountsList recurring error should return 500, got %d", rr.Code)
	}
}

// ── admin.go ────────────────────────────────────────────────────────────────

// admin.go:30 — hookGetAuditLog error → 500
func TestAuditPage_GetAuditLogError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "audit_err@example.com", "AdminP@ss1!", "ADMIN")

	orig := hookGetAuditLog
	hookGetAuditLog = func(page, limit int) ([]db.AuditEntry, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAuditLog = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (GetAuditLog error), got %d", rr.Code)
	}
}

// admin.go:37 — hookCountAuditLog error (non-critical, continues)
func TestAuditPage_CountAuditLogError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "audit_cnt_err@example.com", "AdminP@ss1!", "ADMIN")

	orig := hookCountAuditLog
	hookCountAuditLog = func() (int, error) { return 0, errTest }
	t.Cleanup(func() { hookCountAuditLog = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	// CountAuditLog error is non-critical (just logs), page renders 200
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (CountAuditLog error non-fatal), got %d", rr.Code)
	}
}

// admin.go — hookGetAllUsers error (non-critical, continues with ID fallback)
func TestAuditPage_GetAllUsersError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "audit_allusers_err@example.com", "AdminP@ss1!", "ADMIN")

	orig := hookGetAllUsers
	hookGetAllUsers = func() ([]db.User, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAllUsers = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (GetAllUsers error non-fatal), got %d", rr.Code)
	}
}

// ── auth.go ──────────────────────────────────────────────────────────────────

// auth.go:51 — rate limit exceeded → 429
func TestHandleLogin_RateLimited(t *testing.T) {
	setupHandlerTest(t)
	ratelimit.StopAll()

	newUser(t, "ratelim@example.com", "ValidP@ss1!", "USER")

	orig := hookRateLimitCheck
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		if action == "login" {
			return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
		}
		return ratelimit.Result{Allowed: true, Remaining: 10}
	}
	t.Cleanup(func() { hookRateLimitCheck = orig })

	req := post("/login", url.Values{
		"email":    {"ratelim@example.com"},
		"password": {"WrongP@ss!"},
	})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", rr.Code)
	}
}

// auth.go — per-account rate limit (loginAccount) → 429
func TestHandleLogin_AccountRateLimited(t *testing.T) {
	setupHandlerTest(t)
	ratelimit.StopAll()

	newUser(t, "acctrl@example.com", "ValidP@ss1!", "USER")

	// Mock hookRateLimitCheck to block on loginAccount but allow login IP
	orig := hookRateLimitCheck
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		if action == "loginAccount" {
			return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
		}
		return orig(identifier, action)
	}
	t.Cleanup(func() { hookRateLimitCheck = orig })

	req := post("/login", url.Values{
		"email":    {"acctrl@example.com"},
		"password": {"ValidP@ss1!"},
	})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
}

// auth.go:120 — hookGetUserByBlindIndex error → 500
func TestHandleLogin_GetUserByBlindIndexError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGetUserByBlindIndex
	hookGetUserByBlindIndex = func(blindIndex string) (*db.User, error) { return nil, errTest }
	t.Cleanup(func() { hookGetUserByBlindIndex = orig })

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"anyuser@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (blind index lookup error), got %d", rr.Code)
	}
}

// auth.go:182 — hookGenerateToken error after successful login → 500
func TestHandleLogin_GenerateTokenError(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "gen_tok_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGenerateToken
	hookGenerateToken = func(userID int64, role, lang, currency string, sv int) (string, error) {
		return "", errTest
	}
	t.Cleanup(func() { hookGenerateToken = orig })

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"gen_tok_err@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (GenerateToken error), got %d", rr.Code)
	}
}

// auth.go:161 — hookGeneratePending2FAToken error when MFA enabled → 500
func TestHandleLogin_GeneratePending2FATokenError(t *testing.T) {
	setupHandlerTest(t)
	newMFAUser(t, "mfa_pend_err@example.com")

	orig := hookGeneratePending2FAToken
	hookGeneratePending2FAToken = func(userID int64) (string, error) { return "", errTest }
	t.Cleanup(func() { hookGeneratePending2FAToken = orig })

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"mfa_pend_err@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (GeneratePending2FAToken error), got %d", rr.Code)
	}
}

// auth.go:63 — hookValidatePending2FAToken error → 401
func TestHandleLogin_2FA_ValidatePendingError(t *testing.T) {
	setupHandlerTest(t)
	uid, _ := newMFAUser(t, "mfa_val_err@example.com")

	orig := hookValidatePending2FAToken
	hookValidatePending2FAToken = func(token string) (int64, error) { return 0, errTest }
	t.Cleanup(func() { hookValidatePending2FAToken = orig })

	// Generate a valid pending token to get past the cookie check
	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {"123456"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (ValidatePending2FAToken error), got %d", rr.Code)
	}
}

// auth.go:70 — hookGetUserByID error in 2FA path → 401
func TestHandleLogin_2FA_GetUserError(t *testing.T) {
	setupHandlerTest(t)
	uid, _ := newMFAUser(t, "mfa_getuser_err@example.com")

	orig := hookGetUserByID
	hookGetUserByID = func(id int64) (*db.User, error) { return nil, errTest }
	t.Cleanup(func() { hookGetUserByID = orig })

	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {"123456"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (GetUserByID error), got %d", rr.Code)
	}
}

// auth.go:77 — hookDecryptStr error for MFA secret → 500
func TestHandleLogin_2FA_DecryptError(t *testing.T) {
	setupHandlerTest(t)
	uid, _ := newMFAUser(t, "mfa_dec_err@example.com")

	orig := hookDecryptStr
	hookDecryptStr = func(s string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookDecryptStr = orig })

	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {"123456"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (Decrypt MFA secret error), got %d", rr.Code)
	}
}

// auth.go:91 — hookGenerateToken error in 2FA path → 500
func TestHandleLogin_2FA_GenerateTokenError(t *testing.T) {
	setupHandlerTest(t)
	uid, secret := newMFAUser(t, "mfa_gentok_err@example.com")

	origTok := hookGenerateToken
	hookGenerateToken = func(userID int64, role, lang, currency string, sv int) (string, error) {
		return "", errTest
	}
	t.Cleanup(func() { hookGenerateToken = origTok })

	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {code}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (GenerateToken error in 2FA), got %d", rr.Code)
	}
}

// auth.go:289 — hookGenerateToken error in HandleRegister → 500
func TestHandleRegister_GenerateTokenError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGenerateToken
	hookGenerateToken = func(userID int64, role, lang, currency string, sv int) (string, error) {
		return "", errTest
	}
	t.Cleanup(func() { hookGenerateToken = orig })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"reg_tok_err@pilot.test"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (GenerateToken error in register), got %d", rr.Code)
	}
}

// ── dashboard.go ─────────────────────────────────────────────────────────────

// dashboard.go — hookGetRecurringByUserID error → 500 (audit FIN-11 : projection
// amputée de ses récurrentes = donnée critique, on échoue bruyamment).
func TestDashboardAPI_RecurringError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "dash_rec_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(userID int64) ([]db.RecurringOperation, error) { return nil, errTest }
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/dashboard", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DashboardAPI(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (recurring error fatal), got %d", rr.Code)
	}
}

// dashboard.go:52-59 — pieData path with account having balance > 0
func TestDashboardAPI_WithPositiveBalance(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "dash_pie@example.com", "ValidP@ss1!", "USER")

	// Create an account with balance > 0 so the pieData path is hit
	enc, _ := crypto.Encrypt("Livret A")
	if err := db.CreateAccountWithYield(uid, enc, 5000.0, "#3b82f6", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY"); err != nil {
		t.Fatalf("CreateAccountWithYield: %v", err)
	}

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/dashboard", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DashboardAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pieData, ok := resp["pieData"].([]interface{})
	if !ok || len(pieData) == 0 {
		t.Error("expected pieData to have at least one entry for positive-balance account")
	}
	// accountColors path (dashboard.go:77)
	if _, ok := resp["accounts"]; !ok {
		t.Error("response missing 'accounts'")
	}
}

// ── handlers.go ──────────────────────────────────────────────────────────────

// handlers.go:40+55 — DB ping error → 503 degraded
func TestHealthCheck_DBError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookPingDB
	hookPingDB = func(ctx context.Context) error { return errTest }
	t.Cleanup(func() { hookPingDB = orig })

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	HealthCheck(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 (DB ping error), got %d", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "degraded" {
		t.Errorf("want status=degraded, got %v", resp["status"])
	}
	if resp["database"] != "error" {
		t.Errorf("want database=error, got %v", resp["database"])
	}
}

// ── mfa.go ───────────────────────────────────────────────────────────────────

// mfa.go:35 — hookGenerateTOTPSecret error → 500
func TestMFASetup_SecretError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfa_setup_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGenerateTOTPSecret
	hookGenerateTOTPSecret = func() (string, error) { return "", errTest }
	t.Cleanup(func() { hookGenerateTOTPSecret = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/mfa/setup", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	MFASetup(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (GenerateTOTPSecret error), got %d", rr.Code)
	}
}

// ── pages.go ─────────────────────────────────────────────────────────────────

// pages.go — hookGetAccountsByUserID error in AccountsPage → 500 (audit FIN-17 :
// donnée critique, pas de liste vide trompeuse).
func TestAccountsPage_GetAccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "accpage_acc_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(userID int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (accounts error fatal), got %d", rr.Code)
	}
}

// pages.go — hookGetRecurringByUserID error in AccountsPage → 500 (audit FIN-17).
func TestAccountsPage_GetRecurringError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "accpage_rec_err@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(userID int64) ([]db.RecurringOperation, error) { return nil, errTest }
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (recurring error fatal), got %d", rr.Code)
	}
}

// ── recurring.go ─────────────────────────────────────────────────────────────

// renderRecurringTable: hookGetRecurringByUserID error (non-fatal, logs)
func TestRenderRecurringTable_RecurringError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rrt_rec_err@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(userID int64) ([]db.RecurringOperation, error) { return nil, errTest }
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	// DeleteRecurring calls renderRecurringTable at the end; use a real rec first
	enc, _ := crypto.Encrypt("test rec")
	_ = db.CreateRecurring(uid, accID, nil, enc, 10.0, 1)
	recs, _ := db.GetRecurringByUserID(uid)
	if len(recs) == 0 {
		t.Fatal("need at least one recurring")
	}
	recID := recs[0].ID

	// Now set up hook to fail so renderRecurringTable gets the error
	hookGetRecurringByUserID = func(userID int64) ([]db.RecurringOperation, error) { return nil, errTest }

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/"+intStr(recID), nil), "id", intStr(recID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("renderRecurringTable recurring error should return 500, got %d", rr.Code)
	}
}

// renderRecurringTable: hookGetAccountsByUserID error (non-fatal, logs)
func TestRenderRecurringTable_AccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rrt_acc_err@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	enc, _ := crypto.Encrypt("test rec 2")
	_ = db.CreateRecurring(uid, accID, nil, enc, 10.0, 1)
	recs, _ := db.GetRecurringByUserID(uid)
	if len(recs) == 0 {
		t.Fatal("need at least one recurring")
	}
	recID := recs[0].ID

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(userID int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/"+intStr(recID), nil), "id", intStr(recID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("renderRecurringTable accounts error should return 500, got %d", rr.Code)
	}
}

// renderRecurringTable: monthly yield payout branch (not YEARLY)
func TestRenderRecurringTable_MonthlyYield(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rrt_monthly_yield@example.com", "ValidP@ss1!", "USER")

	// Create target account (for yield payouts)
	encTarget, _ := crypto.Encrypt("Target Account")
	if err := db.CreateAccountWithYield(uid, encTarget, 0, "#ef4444", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY"); err != nil {
		t.Fatalf("CreateAccountWithYield target: %v", err)
	}
	accs, _ := db.GetAccountsByUserID(uid)
	targetID := accs[0].ID

	// Create yield account with active MONTHLY yield pointing to target
	encYield, _ := crypto.Encrypt("Yield Account")
	if err := db.CreateAccountWithYield(uid, encYield, 100000, "#3b82f6", 1, true, "FIXED", 5.0, 5.0, 0, &targetID, "MONTHLY"); err != nil {
		t.Fatalf("CreateAccountWithYield yield: %v", err)
	}
	accs, _ = db.GetAccountsByUserID(uid)
	yieldAccID := accs[1].ID

	// Create a recurring so we can delete it and trigger renderRecurringTable
	encRec, _ := crypto.Encrypt("Loyer")
	_ = db.CreateRecurring(uid, yieldAccID, nil, encRec, -50000, 1)
	recs, _ := db.GetRecurringByUserID(uid)
	if len(recs) == 0 {
		t.Fatal("need at least one recurring")
	}

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/"+intStr(recs[0].ID), nil), "id", intStr(recs[0].ID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	// Response should contain summary-card OOB
	if !strings.Contains(rr.Body.String(), "summary-card") {
		t.Error("response should contain OOB summary-card")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// intStr converts int64 to string.
func intStr(n int64) string {
	return strconv.FormatInt(n, 10)
}
