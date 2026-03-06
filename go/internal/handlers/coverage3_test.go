package handlers

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/go-webauthn/webauthn/webauthn"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
)

// ── accounts.go ─────────────────────────────────────────────────────────────

// accounts.go:69-70 — color=="" → default #3b82f6 applied
func TestCreateAccount_EmptyColor(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "emptycolor@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":    {"TestNoColor"},
		"balance": {"0"},
		// no "color" field → empty string → default
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (default color), got %d: %s", rr.Code, rr.Body.String())
	}
}

// accounts.go:132-133 — idStr!="" but ParseInt fails → 400
func TestCreateAccount_InvalidIDStr(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "invid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"id":      {"bad"},
		"name":    {"Test"},
		"balance": {"0"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (bad ID), got %d", rr.Code)
	}
}

// accounts.go:145-147 — hookGetAccountsByUserID fails in creation path → slog.Warn
func TestCreateAccount_PositionLookupError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "poslookup@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(id int64) ([]db.Account, error) {
		return nil, errTest2
	}
	defer func() { hookGetAccountsByUserID = orig }()

	req := injectUser(post("/accounts", url.Values{
		"name":    {"TestAccount"},
		"balance": {"0"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	// position lookup error is non-fatal; creation proceeds
	if rr.Code == http.StatusBadRequest {
		t.Errorf("unexpected 400: %s", rr.Body.String())
	}
}

// accounts.go:309-312 — hookReorderAccounts fails → 500
func TestReorderAccounts_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "reorder_dberr@example.com", "ValidP@ss1!", "USER")

	orig := hookReorderAccounts
	hookReorderAccounts = func(userID int64, ids []int64) error {
		return errTest2
	}
	defer func() { hookReorderAccounts = orig }()

	req := injectUser(
		postBody("/accounts/reorder", []byte(`{"ids":[1,2,3]}`), "application/json"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── mfa.go ──────────────────────────────────────────────────────────────────

// mfa.go:35-38 — hookQREncode fails → 500
func TestMFASetup_QREncodeError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "mfaqr_err@example.com", "ValidP@ss1!", "USER")

	orig := hookQREncode
	hookQREncode = func(content string, level qrcode.RecoveryLevel, size int) ([]byte, error) {
		return nil, errTest2
	}
	defer func() { hookQREncode = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/mfa/setup", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	MFASetup(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── pages.go ─────────────────────────────────────────────────────────────────

// pages.go:51-52 — hookRender fails for login.html → 500
func TestLoginPage_RenderError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest2
	}
	defer func() { hookRender = orig }()

	rr := httptest.NewRecorder()
	LoginPage(rr, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:99-102 — hookGetRecurringByUserID warning in Dashboard → 200
func TestDashboard_RecurringWarning(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "dash_recwarn@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(id int64) ([]db.RecurringOperation, error) {
		return nil, errTest2
	}
	defer func() { hookGetRecurringByUserID = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (recurring error is non-fatal), got %d: %s", rr.Code, rr.Body.String())
	}
}

// pages.go:110-117, 128-133 — accounts with positive balance → pie chart + accountColors loops
func TestDashboard_WithAccounts(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "dash_accs@example.com", "ValidP@ss1!", "USER")
	_ = createAcc(t, uid) // balance=1000, covers pieData loop (acc.Balance > 0)

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// pages.go:149-151 — hookRender fails for dashboard.html → 500
func TestDashboard_RenderError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "dash_renderrr@example.com", "ValidP@ss1!", "USER")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest2
	}
	defer func() { hookRender = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:214-216 — hookRender fails for accounts.html → 500
func TestAccountsPage_RenderError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "accs_renderrr@example.com", "ValidP@ss1!", "USER")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest2
	}
	defer func() { hookRender = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:258-260 — admin loop: crypto.Decrypt fails on corrupt email → continue
func TestSettingsPage_AdminDecryptError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "admindc@example.com", "AdminP@ss1!", "ADMIN")

	orig := hookGetAllUsers
	hookGetAllUsers = func() ([]db.User, error) {
		// Return a user with non-AES-GCM email → crypto.Decrypt will fail
		return []db.User{{ID: 999, EmailEncrypted: "not-valid-aes-gcm-data", Role: "USER"}}, nil
	}
	defer func() { hookGetAllUsers = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (admin page with decrypt error skipped), got %d: %s", rr.Code, rr.Body.String())
	}
}

// pages.go:272-274 — hookRender fails for settings.html → 500 (USER role, skips admin block)
func TestSettingsPage_RenderError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "settingsrnd@example.com", "ValidP@ss1!", "USER")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest2
	}
	defer func() { hookRender = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:301-302 — hookVerifyEmailByToken returns non-ErrTokenInvalid → "Erreur serveur."
func TestVerifyEmailPage_ServerError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	orig := hookVerifyEmailByToken
	hookVerifyEmailByToken = func(s string) error {
		return errTest2
	}
	defer func() { hookVerifyEmailByToken = orig }()

	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// pages.go:309-312 — hookVerifyEmailByToken returns nil → Success=true
func TestVerifyEmailPage_Success(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	orig := hookVerifyEmailByToken
	hookVerifyEmailByToken = func(s string) error {
		return nil
	}
	defer func() { hookVerifyEmailByToken = orig }()

	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// pages.go:324-326 — hookRender fails for privacy.html → 500
func TestPrivacyPage_RenderError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest2
	}
	defer func() { hookRender = orig }()

	rr := httptest.NewRecorder()
	PrivacyPage(rr, httptest.NewRequest(http.MethodGet, "/privacy", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── passkey.go ───────────────────────────────────────────────────────────────

// passkey.go:52-66 — credential loop body when user has existing authenticators
func TestPasskeyRegistrationStart_WithCreds(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test"); err != nil {
		t.Fatalf("InitWebAuthn: %v", err)
	}
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pkregwithcreds@example.com", "ValidP@ss1!", "USER")

	rawCred := []byte("test-cred-for-reg-start-123")
	credIDBase64 := base64.StdEncoding.EncodeToString(rawCred)
	pubKeyBase64 := base64.StdEncoding.EncodeToString([]byte("some-pubkey"))
	if err := db.CreateAuthenticator(credIDBase64, pubKeyBase64, 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (with existing creds), got %d: %s", rr.Code, rr.Body.String())
	}
}

// passkey.go:261-264 — hookDeleteAuthenticator fails → 500
func TestDeletePasskey_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "delpkerr@example.com", "ValidP@ss1!", "USER")

	if err := db.CreateAuthenticator("cred-del-err", "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}
	auths, _ := db.GetAuthenticatorsByUserID(uid)
	authID := auths[0].ID

	orig := hookDeleteAuthenticator
	hookDeleteAuthenticator = func(id, userID int64) error {
		return errTest2
	}
	defer func() { hookDeleteAuthenticator = orig }()

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/api/passkey/"+intStr(authID), nil), "id", intStr(authID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeletePasskey(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// passkey.go:295-298 — hookRenameAuthenticator fails → 500
func TestRenamePasskey_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "rnpkerr@example.com", "ValidP@ss1!", "USER")

	if err := db.CreateAuthenticator("cred-ren-err", "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}
	auths, _ := db.GetAuthenticatorsByUserID(uid)
	authID := auths[0].ID

	orig := hookRenameAuthenticator
	hookRenameAuthenticator = func(id, userID int64, name string) error {
		return errTest2
	}
	defer func() { hookRenameAuthenticator = orig }()

	body := bytes.NewBufferString(`{"name":"test"}`)
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/"+intStr(authID)+"/rename", body), "id", intStr(authID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// passkey.go:182-184 — closure: authenticator found but hookGetUserByID fails → 401
func TestPasskeyLoginFinish_ClosureUserError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pkclosure@example.com", "ValidP@ss1!", "USER")

	// Create a real authenticator so the closure finds it
	rawCredID := []byte("closure-user-err-cred-12345")
	credIDBase64 := base64.StdEncoding.EncodeToString(rawCredID)
	if err := db.CreateAuthenticator(credIDBase64, "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	// dbGetAuthByCredIDFn stays real (finds the authenticator)
	// hookGetUserByID → fail (covers closure line 182-184)
	origGetUser := hookGetUserByID
	defer func() { hookGetUserByID = origGetUser }()
	hookGetUserByID = func(id int64) (*db.User, error) {
		return nil, errTest2
	}

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		// Call the closure with the real credID → authenticator found → user lookup fails
		_, err := h(rawCredID, nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest2
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

// passkey.go: closure — hookGetAuthByCredentialID returns (nil, nil) → error "authenticator not found"
func TestPasskeyLoginFinish_ClosureAuthNil(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	origGetAuth := hookGetAuthByCredentialID
	defer func() { hookGetAuthByCredentialID = origGetAuth }()
	hookGetAuthByCredentialID = func(id string) (*db.Authenticator, error) {
		return nil, nil // not found, no error
	}

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		_, err := h([]byte("nonexistent"), nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest2
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (auth nil), got %d", rr.Code)
	}
}

// passkey.go: closure — authenticator found but hookGetUserByID returns (nil, nil) → error "user not found"
func TestPasskeyLoginFinish_ClosureUserNil(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pkclosusernil@example.com", "ValidP@ss1!", "USER")

	rawCredID := []byte("closure-user-nil-cred-67890")
	credIDBase64 := base64.StdEncoding.EncodeToString(rawCredID)
	if err := db.CreateAuthenticator(credIDBase64, "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	origGetUser := hookGetUserByID
	defer func() { hookGetUserByID = origGetUser }()
	hookGetUserByID = func(id int64) (*db.User, error) {
		return nil, nil // user not found, no error
	}

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		_, err := h(rawCredID, nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest2
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (user nil), got %d", rr.Code)
	}
}

// ── password_reset.go ────────────────────────────────────────────────────────

// password_reset.go:39-42 — rate limit exhausted → 429
func TestForgotPasswordSubmit_RateLimited(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	// Register mail.Init() cleanup BEFORE t.Setenv so it runs AFTER env is restored
	t.Cleanup(func() { mail.Init() }) //nolint:errcheck
	t.Setenv("SMTP_HOST", "localhost")
	mail.Init() //nolint:errcheck

	// Exhaust the rate limit (max 3 attempts for "forgotPassword")
	for i := 0; i < 3; i++ {
		req := post("/forgot-password", url.Values{"email": {"unknown@example.com"}})
		ForgotPasswordSubmit(httptest.NewRecorder(), req)
	}

	// 4th request should be rate limited
	req := post("/forgot-password", url.Values{"email": {"unknown@example.com"}})
	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 (rate limited), got %d", rr.Code)
	}
}

// password_reset.go:53-62 — user not found → render success page (don't reveal existence)
func TestForgotPasswordSubmit_UserNotFound(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	t.Cleanup(func() { mail.Init() }) //nolint:errcheck
	t.Setenv("SMTP_HOST", "localhost")
	mail.Init() //nolint:errcheck

	req := post("/forgot-password", url.Values{"email": {"notexist@example.com"}})
	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (user not found silently), got %d", rr.Code)
	}
}

// ── recurring.go ─────────────────────────────────────────────────────────────

// recurring.go:24-26 — r.ParseForm() fails → 400
func TestCreateRecurring_ParseFormError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crec_parse@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		postBody("/recurring", []byte("body"), "multipart/form-data; boundary="),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (ParseForm error), got %d", rr.Code)
	}
}

// recurring.go:43-46 — hookEncryptStr fails → 500
func TestCreateRecurring_EncryptError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crec_enc@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookEncryptStr
	hookEncryptStr = func(s string) (string, error) { return "", errTest2 }
	defer func() { hookEncryptStr = orig }()

	req := injectUser(post("/recurring", url.Values{
		"description": {"Test"},
		"amount":      {"100"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {intStr(accID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (encrypt error), got %d", rr.Code)
	}
}

// recurring.go:83-87 — idStr!="" but ParseInt fails → 400
func TestCreateRecurring_InvalidIDStr(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crec_badid@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(post("/recurring", url.Values{
		"id":          {"bad"},
		"description": {"Test"},
		"amount":      {"100"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {intStr(accID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (bad ID str), got %d", rr.Code)
	}
}

// recurring.go:88-92 — hookUpdateRecurring fails (update path) → 500
func TestCreateRecurring_UpdateDBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crec_updberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		return errTest2
	}
	defer func() { hookUpdateRecurring = orig }()

	req := injectUser(post("/recurring", url.Values{
		"id":          {intStr(recID)},
		"description": {"Test"},
		"amount":      {"100"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {intStr(accID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (update DB error), got %d", rr.Code)
	}
}

// recurring.go:95-99 — hookCreateRecurring fails (create path) → 500
func TestCreateRecurring_CreateDBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crec_createrr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookCreateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		return errTest2
	}
	defer func() { hookCreateRecurring = orig }()

	req := injectUser(post("/recurring", url.Values{
		"description": {"Test"},
		"amount":      {"100"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {intStr(accID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (create DB error), got %d", rr.Code)
	}
}

// recurring.go:121-123 — r.ParseForm() fails in UpdateRecurring → 400
func TestUpdateRecurring_ParseFormError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrec_parse@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(
			postBody("/recurring/1", []byte("body"), "multipart/form-data; boundary="),
			"id", "1",
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (ParseForm error), got %d", rr.Code)
	}
}

// recurring.go:133-137 — hookEncryptStr fails in UpdateRecurring → 500
func TestUpdateRecurring_EncryptError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrec_enc@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookEncryptStr
	hookEncryptStr = func(s string) (string, error) { return "", errTest2 }
	defer func() { hookEncryptStr = orig }()

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Test"},
				"amount":      {"100"},
				"dayOfMonth":  {"1"},
				"type":        {"income"},
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (encrypt error), got %d", rr.Code)
	}
}

// recurring.go:159-162 — hookUpdateRecurring fails in UpdateRecurring → 500
func TestUpdateRecurring_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrec_dberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		return errTest2
	}
	defer func() { hookUpdateRecurring = orig }()

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Test"},
				"amount":      {"100"},
				"dayOfMonth":  {"1"},
				"type":        {"income"},
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (update DB error), got %d", rr.Code)
	}
}

// recurring.go: UpdateRecurring — day out of range → clamped to 1
func TestUpdateRecurring_DayOutOfRange(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrec_day@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Test day"},
				"amount":      {"100"},
				"dayOfMonth":  {"0"},
				"type":        {"income"},
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// recurring.go: UpdateRecurring — income with negative amount → flipped to positive
func TestUpdateRecurring_IncomeNegative(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrec_incneg@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Salary"},
				"amount":      {"-500"},
				"dayOfMonth":  {"15"},
				"type":        {"income"},
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// recurring.go:183-187 — hookDeleteRecurring fails → 500
func TestDeleteRecurring_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "delrec_dberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookDeleteRecurring
	hookDeleteRecurring = func(id, userID int64) error {
		return errTest2
	}
	defer func() { hookDeleteRecurring = orig }()

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/"+intStr(recID), nil), "id", intStr(recID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (delete DB error), got %d", rr.Code)
	}
}

// ── settings.go ──────────────────────────────────────────────────────────────

// settings.go:24-26 — r.ParseForm() fails in ChangePassword → 400
func TestChangePassword_ParseFormError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "chpwd_parse@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		postBody("/settings/password", []byte("body"), "multipart/form-data; boundary="),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (ParseForm error), got %d", rr.Code)
	}
}

// settings.go:88-90 — r.ParseForm() fails in UpdatePreferences → 400
func TestUpdatePreferences_ParseFormError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updpref_parse@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		postBody("/settings/preferences", []byte("body"), "multipart/form-data; boundary="),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdatePreferences(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (ParseForm error), got %d", rr.Code)
	}
}

// settings.go:145-148 — hookGetRecurringByUserID fails in ExportData → 500
func TestExportData_RecurringError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "exportrec@example.com", "ExportP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(id int64) ([]db.RecurringOperation, error) {
		return nil, errTest2
	}
	defer func() { hookGetRecurringByUserID = orig }()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (recurring error), got %d", rr.Code)
	}
}

// ── error.go ─────────────────────────────────────────────────────────────────

func TestNotFound_Renders(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	rr := httptest.NewRecorder()
	NotFound(rr, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestMethodNotAllowed_Renders(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	rr := httptest.NewRecorder()
	MethodNotAllowed(rr, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

func TestInternalServerError_Renders(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	rr := httptest.NewRecorder()
	InternalServerError(rr, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── pages.go — LegalPage ──────────────────────────────────────────────────────

func TestLegalPage_RenderError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest2
	}
	defer func() { hookRender = orig }()

	rr := httptest.NewRecorder()
	LegalPage(rr, httptest.NewRequest(http.MethodGet, "/legal", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}
