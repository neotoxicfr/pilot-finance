package handlers

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/go-webauthn/webauthn/webauthn"

	"golang.org/x/crypto/bcrypt"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/ratelimit"
)

// ── accounts.go ─────────────────────────────────────────────────────────────

// accounts.go:69-70 — color=="" → default #3b82f6 applied
func TestCreateAccount_EmptyColor(t *testing.T) {
	setupHandlerTest(t)
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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "poslookup@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(id int64) ([]db.Account, error) {
		return nil, errTest
	}
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

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
	setupHandlerTest(t)
	uid := newUser(t, "reorder_dberr@example.com", "ValidP@ss1!", "USER")

	orig := hookReorderAccounts
	hookReorderAccounts = func(userID int64, ids []int64) error {
		return errTest
	}
	t.Cleanup(func() { hookReorderAccounts = orig })

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
	setupHandlerTest(t)
	uid := newUser(t, "mfaqr_err@example.com", "ValidP@ss1!", "USER")

	orig := hookQREncode
	hookQREncode = func(content string, level qrcode.RecoveryLevel, size int) ([]byte, error) {
		return nil, errTest
	}
	t.Cleanup(func() { hookQREncode = orig })

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
	setupHandlerTest(t)

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest
	}
	t.Cleanup(func() { hookRender = orig })

	rr := httptest.NewRecorder()
	LoginPage(rr, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:99-102 — hookGetRecurringByUserID warning in Dashboard → 200
func TestDashboard_RecurringWarning(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "dash_recwarn@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(id int64) ([]db.RecurringOperation, error) {
		return nil, errTest
	}
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (recurring error is non-fatal), got %d: %s", rr.Code, rr.Body.String())
	}
}

// pages.go:110-117, 128-133 — accounts with positive balance → pie chart + accountColors loops
func TestDashboard_WithAccounts(t *testing.T) {
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "dash_renderrr@example.com", "ValidP@ss1!", "USER")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest
	}
	t.Cleanup(func() { hookRender = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:214-216 — hookRender fails for accounts.html → 500
func TestAccountsPage_RenderError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "accs_renderrr@example.com", "ValidP@ss1!", "USER")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest
	}
	t.Cleanup(func() { hookRender = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go:258-260 — admin loop: crypto.Decrypt fails on corrupt email → continue
func TestSettingsPage_AdminDecryptError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "admindc@example.com", "AdminP@ss1!", "ADMIN")

	orig := hookGetAllUsers
	hookGetAllUsers = func() ([]db.User, error) {
		// Return a user with non-AES-GCM email → crypto.Decrypt will fail
		return []db.User{{ID: 999, EmailEncrypted: "not-valid-aes-gcm-data", Role: "USER"}}, nil
	}
	t.Cleanup(func() { hookGetAllUsers = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (admin page with decrypt error skipped), got %d: %s", rr.Code, rr.Body.String())
	}
}

// pages.go:272-274 — hookRender fails for settings.html → 500 (USER role, skips admin block)
func TestSettingsPage_RenderError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "settingsrnd@example.com", "ValidP@ss1!", "USER")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest
	}
	t.Cleanup(func() { hookRender = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// pages.go — hookGetUserByVerificationTok returns DB error → "Erreur serveur."
func TestVerifyEmailPage_LookupError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGetUserByVerificationTok
	hookGetUserByVerificationTok = func(s string) (*db.User, error) {
		return nil, errTest
	}
	t.Cleanup(func() { hookGetUserByVerificationTok = orig })

	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// pages.go — hookGetUserByVerificationTok returns nil → token invalid
func TestVerifyEmailPage_TokenInvalid(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGetUserByVerificationTok
	hookGetUserByVerificationTok = func(s string) (*db.User, error) {
		return nil, nil
	}
	t.Cleanup(func() { hookGetUserByVerificationTok = orig })

	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Jeton invalide") {
		t.Errorf("want invalid token message, got: %s", rr.Body.String())
	}
}

// pages.go — MarkEmailVerified fails → server error rendered
func TestVerifyEmailPage_MarkError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGetUserByVerificationTok
	hookGetUserByVerificationTok = func(s string) (*db.User, error) {
		return &db.User{ID: 42}, nil
	}
	t.Cleanup(func() { hookGetUserByVerificationTok = orig })

	origMark := hookMarkEmailVerified
	hookMarkEmailVerified = func(int64) error { return errTest }
	t.Cleanup(func() { hookMarkEmailVerified = origMark })

	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// pages.go — verify success without logged-in user → renders success page
func TestVerifyEmailPage_SuccessNoUser(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGetUserByVerificationTok
	hookGetUserByVerificationTok = func(s string) (*db.User, error) {
		return &db.User{ID: 42}, nil
	}
	t.Cleanup(func() { hookGetUserByVerificationTok = orig })

	origMark := hookMarkEmailVerified
	hookMarkEmailVerified = func(int64) error { return nil }
	t.Cleanup(func() { hookMarkEmailVerified = origMark })

	req := httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil)
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// pages.go — verify success with logged-in user → 303 to /settings?verified=1
func TestVerifyEmailPage_SuccessLoggedIn_Redirects(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "verify_loggedin@example.com", "ValidP@ss1!", "USER")

	orig := hookGetUserByVerificationTok
	hookGetUserByVerificationTok = func(s string) (*db.User, error) {
		return &db.User{ID: uid}, nil
	}
	t.Cleanup(func() { hookGetUserByVerificationTok = orig })

	origMark := hookMarkEmailVerified
	hookMarkEmailVerified = func(int64) error { return nil }
	t.Cleanup(func() { hookMarkEmailVerified = origMark })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/verify-email?token=sometoken", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/settings?verified=1" {
		t.Errorf("Location: want /settings?verified=1, got %q", loc)
	}
}

// pages.go:324-326 — hookRender fails for privacy.html → 500
func TestPrivacyPage_RenderError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest
	}
	t.Cleanup(func() { hookRender = orig })

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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "delpkerr@example.com", "ValidP@ss1!", "USER")

	if err := db.CreateAuthenticator("cred-del-err", "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}
	auths, _ := db.GetAuthenticatorsByUserID(uid)
	authID := auths[0].ID

	orig := hookDeleteAuthenticator
	hookDeleteAuthenticator = func(id, userID int64) error {
		return errTest
	}
	t.Cleanup(func() { hookDeleteAuthenticator = orig })

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
	setupHandlerTest(t)
	uid := newUser(t, "rnpkerr@example.com", "ValidP@ss1!", "USER")

	if err := db.CreateAuthenticator("cred-ren-err", "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}
	auths, _ := db.GetAuthenticatorsByUserID(uid)
	authID := auths[0].ID

	orig := hookRenameAuthenticator
	hookRenameAuthenticator = func(id, userID int64, name string) error {
		return errTest
	}
	t.Cleanup(func() { hookRenameAuthenticator = orig })

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
	setupHandlerTest(t)
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
	t.Cleanup(func() { hookGetUserByID = origGetUser })
	hookGetUserByID = func(id int64) (*db.User, error) {
		return nil, errTest
	}

	origFinish := hookFinishLogin
	t.Cleanup(func() { hookFinishLogin = origFinish })
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		// Call the closure with the real credID → authenticator found → user lookup fails
		_, err := h(rawCredID, nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

// passkey.go: closure — hookGetAuthByCredentialID returns a real error → propagated (line 171-173).
// (GetAuthenticatorByCredentialID now returns (nil,nil) for not-found, so this real-error
// branch needs its own test to stay covered.)
func TestPasskeyLoginFinish_ClosureAuthError(t *testing.T) {
	setupHandlerTest(t)

	origGetAuth := hookGetAuthByCredentialID
	t.Cleanup(func() { hookGetAuthByCredentialID = origGetAuth })
	hookGetAuthByCredentialID = func(string) (*db.Authenticator, error) {
		return nil, errTest
	}

	origFinish := hookFinishLogin
	t.Cleanup(func() { hookFinishLogin = origFinish })
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		_, err := h([]byte("any-cred"), nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest
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
	setupHandlerTest(t)

	origGetAuth := hookGetAuthByCredentialID
	t.Cleanup(func() { hookGetAuthByCredentialID = origGetAuth })
	hookGetAuthByCredentialID = func(id string) (*db.Authenticator, error) {
		return nil, nil // not found, no error
	}

	origFinish := hookFinishLogin
	t.Cleanup(func() { hookFinishLogin = origFinish })
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		_, err := h([]byte("nonexistent"), nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest
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
	setupHandlerTest(t)
	uid := newUser(t, "pkclosusernil@example.com", "ValidP@ss1!", "USER")

	rawCredID := []byte("closure-user-nil-cred-67890")
	credIDBase64 := base64.StdEncoding.EncodeToString(rawCredID)
	if err := db.CreateAuthenticator(credIDBase64, "pubkey", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	origGetUser := hookGetUserByID
	t.Cleanup(func() { hookGetUserByID = origGetUser })
	hookGetUserByID = func(id int64) (*db.User, error) {
		return nil, nil // user not found, no error
	}

	origFinish := hookFinishLogin
	t.Cleanup(func() { hookFinishLogin = origFinish })
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		_, err := h(rawCredID, nil)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errTest
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
	setupHandlerTest(t)
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
	FlushForgotPassword() // M5 : drain les goroutines des 3 premiers appels
}

// password_reset.go:53-62 — user not found → render success page (don't reveal existence)
func TestForgotPasswordSubmit_UserNotFound(t *testing.T) {
	setupHandlerTest(t)
	t.Cleanup(func() { mail.Init() }) //nolint:errcheck
	t.Setenv("SMTP_HOST", "localhost")
	mail.Init() //nolint:errcheck

	req := post("/forgot-password", url.Values{"email": {"notexist@example.com"}})
	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (user not found silently), got %d", rr.Code)
	}
	FlushForgotPassword() // M5 : drain la goroutine background
}

// ── recurring.go ─────────────────────────────────────────────────────────────

// recurring.go:24-26 — r.ParseForm() fails → 400
func TestCreateRecurring_ParseFormError(t *testing.T) {
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "crec_enc@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookEncryptStr
	hookEncryptStr = func(s string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookEncryptStr = orig })

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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "crec_updberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		return errTest
	}
	t.Cleanup(func() { hookUpdateRecurring = orig })

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
	setupHandlerTest(t)
	uid := newUser(t, "crec_createrr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	orig := hookCreateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		return errTest
	}
	t.Cleanup(func() { hookCreateRecurring = orig })

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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "updrec_enc@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookEncryptStr
	hookEncryptStr = func(s string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookEncryptStr = orig })

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
	setupHandlerTest(t)
	uid := newUser(t, "updrec_dberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		return errTest
	}
	t.Cleanup(func() { hookUpdateRecurring = orig })

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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
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

// recurring.go: CreateRecurring — income with toAccountId in form is ignored
// (defends against form leak when select stays in DOM with x-show)
func TestCreateRecurring_IncomeIgnoresToAccountID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "crec_incignoreto@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	toID := createAcc(t, uid)

	var capturedToID *int64
	orig := hookCreateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		capturedToID = toAccountID
		return nil
	}
	t.Cleanup(func() { hookCreateRecurring = orig })

	req := injectUser(post("/recurring", url.Values{
		"description": {"Salary"},
		"amount":      {"500"},
		"dayOfMonth":  {"15"},
		"type":        {"income"},
		"accountId":   {intStr(accID)},
		"toAccountId": {intStr(toID)}, // valeur résiduelle envoyée par le formulaire
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if capturedToID != nil {
		t.Errorf("toAccountID should be nil for income, got %d", *capturedToID)
	}
}

// recurring.go: CreateRecurring — expense with toAccountId in form is ignored
func TestCreateRecurring_ExpenseIgnoresToAccountID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "crec_expignoreto@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	toID := createAcc(t, uid)

	var capturedToID *int64
	orig := hookCreateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		capturedToID = toAccountID
		return nil
	}
	t.Cleanup(func() { hookCreateRecurring = orig })

	req := injectUser(post("/recurring", url.Values{
		"description": {"Rent"},
		"amount":      {"500"},
		"dayOfMonth":  {"1"},
		"type":        {"expense"},
		"accountId":   {intStr(accID)},
		"toAccountId": {intStr(toID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if capturedToID != nil {
		t.Errorf("toAccountID should be nil for expense, got %d", *capturedToID)
	}
}

// recurring.go: UpdateRecurring — income with toAccountId in form is ignored
// (regression test for "salary becomes transfer when re-edited")
func TestUpdateRecurring_IncomeIgnoresToAccountID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrec_incignoreto@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	toID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	var capturedToID *int64
	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		capturedToID = toAccountID
		return nil
	}
	t.Cleanup(func() { hookUpdateRecurring = orig })

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Salary"},
				"amount":      {"500"},
				"dayOfMonth":  {"15"},
				"type":        {"income"},
				"toAccountId": {intStr(toID)}, // valeur résiduelle dans le formulaire
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if capturedToID != nil {
		t.Errorf("toAccountID should be nil for income, got %d", *capturedToID)
	}
}

// recurring.go: UpdateRecurring — transfer correctly preserves toAccountId
func TestUpdateRecurring_TransferKeepsToAccountID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrec_transferkeep@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	toID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	var capturedToID *int64
	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		capturedToID = toAccountID
		return nil
	}
	t.Cleanup(func() { hookUpdateRecurring = orig })

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {"Transfer"},
				"amount":      {"500"},
				"dayOfMonth":  {"1"},
				"type":        {"transfer"},
				"toAccountId": {intStr(toID)},
			}),
			"id", intStr(recID),
		),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if capturedToID == nil || *capturedToID != toID {
		t.Errorf("toAccountID should be %d for transfer, got %v", toID, capturedToID)
	}
}

// recurring.go — CreateRecurring : transfer happy path (couvre toAccountID = &id)
func TestCreateRecurring_TransferSuccess(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "crec_transfer_ok@example.com", "ValidP@ss1!", "USER")
	fromID := createAcc(t, uid)
	toID := createAcc(t, uid)

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement"},
		"amount":      {"500"},
		"dayOfMonth":  {"1"},
		"type":        {"transfer"},
		"accountId":   {intStr(fromID)},
		"toAccountId": {intStr(toID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// recurring.go — UpdateRecurring : sql.ErrNoRows → 404
func TestUpdateRecurring_NotFound(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrec_404@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		return sql.ErrNoRows
	}
	t.Cleanup(func() { hookUpdateRecurring = orig })

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
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// recurring.go — CreateRecurring (mode update via id) : sql.ErrNoRows → 404
func TestCreateRecurring_UpdateBranch_NotFound(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "createrec_404@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookUpdateRecurring
	hookUpdateRecurring = func(id, userID int64, description string, amount int64, dayOfMonth int, toAccountID *int64) error {
		return sql.ErrNoRows
	}
	t.Cleanup(func() { hookUpdateRecurring = orig })

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
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// recurring.go — DeleteRecurring : sql.ErrNoRows → 404
func TestDeleteRecurring_NotFound(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delrec_404@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookDeleteRecurring
	hookDeleteRecurring = func(id, userID int64) error {
		return sql.ErrNoRows
	}
	t.Cleanup(func() { hookDeleteRecurring = orig })

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/"+intStr(recID), nil), "id", intStr(recID)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

// recurring.go:183-187 — hookDeleteRecurring fails → 500
func TestDeleteRecurring_DBError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delrec_dberr@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	orig := hookDeleteRecurring
	hookDeleteRecurring = func(id, userID int64) error {
		return errTest
	}
	t.Cleanup(func() { hookDeleteRecurring = orig })

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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
	uid := newUser(t, "exportrec@example.com", "ExportP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(id int64) ([]db.RecurringOperation, error) {
		return nil, errTest
	}
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (recurring error), got %d", rr.Code)
	}
}

// ── error.go ─────────────────────────────────────────────────────────────────

func TestNotFound_Renders(t *testing.T) {
	setupHandlerTest(t)
	rr := httptest.NewRecorder()
	NotFound(rr, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestMethodNotAllowed_Renders(t *testing.T) {
	setupHandlerTest(t)
	rr := httptest.NewRecorder()
	MethodNotAllowed(rr, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
	if got := rr.Header().Get("X-Error-Code"); got != ErrMethodNotAllowed {
		t.Errorf("X-Error-Code: want %q, got %q", ErrMethodNotAllowed, got)
	}
}

func TestInternalServerError_Renders(t *testing.T) {
	setupHandlerTest(t)
	rr := httptest.NewRecorder()
	InternalServerError(rr, httptest.NewRequest(http.MethodGet, "/bad", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── pages.go — LegalPage ──────────────────────────────────────────────────────

func TestLegalPage_RenderError(t *testing.T) {
	setupHandlerTest(t)

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error {
		return errTest
	}
	t.Cleanup(func() { hookRender = orig })

	rr := httptest.NewRecorder()
	LegalPage(rr, httptest.NewRequest(http.MethodGet, "/legal", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── auth.go — ALLOW_REGISTER disabled ──────────────────────────────────────

func TestHandleRegister_Disabled_WithExistingUsers(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "existing@example.com", "ValidP@ss1!", "ADMIN")

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"new@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403 (register disabled), got %d", rr.Code)
	}
}

// ── auth.go — handleFailedLogin error log ──────────────────────────────────

func TestHandleLogin_FailedLoginAttemptsError(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "faillog@example.com", "ValidP@ss1!", "USER")

	orig := hookUpdateLoginAttempts
	hookUpdateLoginAttempts = func(int64, int, *time.Time) error { return errTest }
	t.Cleanup(func() { hookUpdateLoginAttempts = orig })

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"faillog@example.com"},
		"password": {"WrongPassword!"},
	}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

// ── auth.go — resetLoginAttempts error log ──────────────────────────────────

func TestHandleLogin_ResetAttemptsError(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "resetlog@example.com", "ValidP@ss1!", "USER")

	// First, trigger a failed login to increment FailedLoginAttempts > 0
	rr1 := httptest.NewRecorder()
	HandleLogin(rr1, post("/login", url.Values{
		"email":    {"resetlog@example.com"},
		"password": {"WrongPass1!"},
	}))

	// Now mock hookUpdateLoginAttempts to return error
	orig := hookUpdateLoginAttempts
	hookUpdateLoginAttempts = func(int64, int, *time.Time) error { return errTest }
	t.Cleanup(func() { hookUpdateLoginAttempts = orig })

	// Login with correct password → resetLoginAttempts is called (FailedLoginAttempts > 0)
	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"resetlog@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

// ── auth.go — rehash hookUpdatePasswordHash error ──────────────────────────

func TestHandleLogin_RehashUpdateError(t *testing.T) {
	setupHandlerTest(t)

	email := "rehashfail@example.com"
	password := "ValidP@ss1!"

	// Créer un user avec low-cost hash pour déclencher NeedsRehash
	lowCostHash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	encEmail, _ := crypto.Encrypt(email)
	blind := crypto.ComputeBlindIndex(email)
	db.CreateUser(encEmail, blind, string(lowCostHash), "USER") //nolint:errcheck

	orig := hookUpdatePasswordHash
	hookUpdatePasswordHash = func(int64, string) error { return errTest }
	t.Cleanup(func() { hookUpdatePasswordHash = orig })

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {email},
		"password": {password},
	}))
	// Login should still succeed despite rehash error
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

// ── helpers.go — decryptAccountNames error ──────────────────────────────────

func TestDecryptAccountNames_Error(t *testing.T) {
	orig := hookDecryptStr
	hookDecryptStr = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookDecryptStr = orig })

	accounts := []db.Account{{ID: 1, Name: "encrypted"}}
	decryptAccountNames(accounts)
	if accounts[0].Name != "???" {
		t.Errorf("want '???' placeholder, got %q", accounts[0].Name)
	}
}

// ── pages.go — SettingsPage error logs ──────────────────────────────────────

func TestSettingsPage_GetUserError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "setuser@example.com", "ValidP@ss1!", "USER")

	orig := hookGetUserByID
	hookGetUserByID = func(int64) (*db.User, error) { return nil, errTest }
	t.Cleanup(func() { hookGetUserByID = orig })

	rr := httptest.NewRecorder()
	SettingsPage(rr, injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (graceful), got %d", rr.Code)
	}
}

func TestSettingsPage_GetPasskeysError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "setpk@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAuthenticatorsByUserID
	hookGetAuthenticatorsByUserID = func(int64) ([]db.Authenticator, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAuthenticatorsByUserID = orig })

	rr := httptest.NewRecorder()
	SettingsPage(rr, injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (graceful), got %d", rr.Code)
	}
}

// ── passkey.go — PasskeyLoginFinish counter update error ────────────────────

func TestPasskeyLoginFinish_UpdateCounterError(t *testing.T) {
	setupHandlerTest(t)

	uid := newUser(t, "pkcounter@example.com", "ValidP@ss1!", "USER")

	origCounter := hookUpdateAuthCounter
	hookUpdateAuthCounter = func(string, int) error { return errTest }
	t.Cleanup(func() { hookUpdateAuthCounter = origCounter })

	origFinish := hookFinishLogin
	hookFinishLogin = func(string, *http.Request, func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		return &auth.PasskeyUser{ID: uid, Email: "pkcounter@example.com"}, &webauthn.Credential{ID: []byte("cred")}, nil
	}
	t.Cleanup(func() { hookFinishLogin = origFinish })

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/login/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummy"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── passkey.go — RenamePasskey validation branches ──────────────────────────

func TestRenamePasskey_EmptyName(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rnpkempty@example.com", "ValidP@ss1!", "USER")

	body, _ := json.Marshal(map[string]string{"name": "   "})
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/1/rename", bytes.NewReader(body)), "id", "1"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (empty name), got %d", rr.Code)
	}
}

func TestRenamePasskey_TooLongName(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rnpklong@example.com", "ValidP@ss1!", "USER")

	longName := make([]byte, 101)
	for i := range longName {
		longName[i] = 'a'
	}
	body, _ := json.Marshal(map[string]string{"name": string(longName)})
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/1/rename", bytes.NewReader(body)), "id", "1"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (name too long), got %d", rr.Code)
	}
}

// ── password_reset.go — atomic UpdatePasswordAndClearResetToken success path ──
// Vérifie qu'après un reset valide, le token est effacé et le mot de passe
// est mis à jour en une seule transaction (M4 fix).

func TestResetPasswordSubmit_AtomicSuccess(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "clrtok@example.com", "ValidP@ss1!", "USER")

	rawToken := "cleartoken-test-456"
	hashed := crypto.HashToken(rawToken)
	if err := db.SetResetToken(uid, hashed, time.Now().Add(1*time.Hour)); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{
		"token":           {rawToken},
		"password":        {"NewValidP@ss1!"},
		"confirmPassword": {"NewValidP@ss1!"},
	}))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}

	// Le token doit être effacé (lookup retourne nil) → atomic
	user, err := db.GetUserByResetToken(hashed)
	if err != nil {
		t.Fatalf("GetUserByResetToken: %v", err)
	}
	if user != nil {
		t.Error("reset token should be cleared after successful reset")
	}
}

// ── settings.go — ExportData error logs ─────────────────────────────────────

func TestExportData_DecryptEmailError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "expdec@example.com", "ValidP@ss1!", "USER")

	origDecrypt := hookDecryptStr
	hookDecryptStr = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookDecryptStr = origDecrypt })

	rr := httptest.NewRecorder()
	ExportData(rr, injectUser(httptest.NewRequest(http.MethodGet, "/api/export", nil), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestExportData_AuditLogError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "expaudit@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAuditLogByUserID
	hookGetAuditLogByUserID = func(int64) ([]db.AuditEntry, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAuditLogByUserID = orig })

	rr := httptest.NewRecorder()
	ExportData(rr, injectUser(httptest.NewRequest(http.MethodGet, "/api/export", nil), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestExportData_PasskeysError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "exppk@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAuthenticatorsByUserID
	hookGetAuthenticatorsByUserID = func(int64) ([]db.Authenticator, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAuthenticatorsByUserID = orig })

	rr := httptest.NewRecorder()
	ExportData(rr, injectUser(httptest.NewRequest(http.MethodGet, "/api/export", nil), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// ── helpers.go: parseFormAny DELETE io.ReadAll error ─────────────────────────

func TestParseFormAny_DeleteReadError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pfdelete_readerr@example.com", "ValidP@ss1!", "USER")

	req := httptest.NewRequest(http.MethodDelete, "/settings/account", &errReader{})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = injectUser(req, mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (parseFormAny ReadAll error), got %d", rr.Code)
	}
}

// ── settings.go: DeleteSelfAccount password verification ────────────────────

func TestDeleteSelfAccount_MissingPassword(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delself_nopass@example.com", "ValidP@ss1!", "USER")

	body := strings.NewReader("")
	req := injectUser(httptest.NewRequest(http.MethodDelete, "/settings/account", body), mu(uid, "USER"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (missing password), got %d", rr.Code)
	}
}

func TestDeleteSelfAccount_WrongPassword(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delself_wrongpw@example.com", "ValidP@ss1!", "USER")

	body := strings.NewReader("current_password=WrongP%40ss1!")
	req := injectUser(httptest.NewRequest(http.MethodDelete, "/settings/account", body), mu(uid, "USER"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (wrong password), got %d", rr.Code)
	}
}

func TestDeleteSelfAccount_ParseFormError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delself_parseerr@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		postBody("/settings/account", []byte("bad"), "multipart/form-data; boundary="),
		mu(uid, "USER"),
	)
	req.Method = http.MethodDelete
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (ParseForm error), got %d", rr.Code)
	}
}

func TestDeleteSelfAccount_UserNotFound(t *testing.T) {
	setupHandlerTest(t)

	orig := hookGetUserByID
	hookGetUserByID = func(int64) (*db.User, error) { return nil, nil }
	t.Cleanup(func() { hookGetUserByID = orig })

	body := strings.NewReader("current_password=AnyP%40ss1!")
	req := injectUser(httptest.NewRequest(http.MethodDelete, "/settings/account", body), mu(99999, "USER"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404 (user not found), got %d", rr.Code)
	}
}

// ── recurring.go: UpdateRecurring toAccountID ownership ─────────────────────

func TestUpdateRecurring_ToAccountNotOwned(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrecidor1@example.com", "ValidP@ss1!", "USER")
	uid2 := newUser(t, "updrecidor2@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	accID2 := createAcc(t, uid2)
	recID := createRec(t, uid, accID)

	// Note: toAccountId n'est validé que pour les virements (type=transfer).
	// Pour income/expense, il est ignoré quel que soit son contenu.
	idStr := intStr(recID)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Updated"},
			"amount":      {"500"},
			"dayOfMonth":  {"15"},
			"type":        {"transfer"},
			"toAccountId": {intStr(accID2)},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (IDOR toAccount in UpdateRecurring), got %d", rr.Code)
	}
	_ = uid2
}

// ── password_reset.go: ResetPasswordSubmit rate limit ────────────────────────

func TestResetPasswordSubmit_RateLimited(t *testing.T) {
	setupHandlerTest(t)

	orig := hookRateLimitCheck
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		if action == "resetPassword" {
			return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
		}
		return orig(identifier, action)
	}
	t.Cleanup(func() { hookRateLimitCheck = orig })

	req := post("/reset-password", url.Values{
		"token":           {"sometoken"},
		"password":        {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	})
	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 (rate limited), got %d", rr.Code)
	}
}

// ── passkey.go: PasskeyLoginFinish rate limit ────────────────────────────────

func TestPasskeyLoginFinish_RateLimited(t *testing.T) {
	setupHandlerTest(t)

	orig := hookRateLimitCheck
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		if action == "login" {
			return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
		}
		return orig(identifier, action)
	}
	t.Cleanup(func() { hookRateLimitCheck = orig })

	req := httptest.NewRequest(http.MethodPost, "/passkey/login/finish", nil)
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 (passkey login rate limited), got %d", rr.Code)
	}
}

// ── auth.go: HandleLogin 2FA rate limit ──────────────────────────────────────

func TestHandleLogin_2FA_RateLimited(t *testing.T) {
	setupHandlerTest(t)

	uid, _ := newMFAUser(t, "mfa_ratelim@example.com")

	orig := hookRateLimitCheck
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		if action == "twoFactor" {
			return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
		}
		return orig(identifier, action)
	}
	t.Cleanup(func() { hookRateLimitCheck = orig })

	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {"123456"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 (2FA rate limited), got %d", rr.Code)
	}
}

// ── accounts.go: CreateAccount hookCountAccountsByUserID error ───────────────

func TestCreateAccount_CountAccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "countaccerr@example.com", "ValidP@ss1!", "USER")

	orig := hookCountAccountsByUserID
	hookCountAccountsByUserID = func(userID int64) (int, error) { return 0, errTest }
	t.Cleanup(func() { hookCountAccountsByUserID = orig })

	req := injectUser(post("/accounts", url.Values{
		"name":    {"FallbackAcc"},
		"balance": {"100"},
		"color":   {"#3b82f6"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (account created despite count error), got %d", rr.Code)
	}
}

// ── recurring.go: UpdateRecurring invalid toAccountId ────────────────────────

func TestUpdateRecurring_InvalidToAccountID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrectoaccbad@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	idStr := strconv.FormatInt(recID, 10)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Updated"},
			"amount":      {"500"},
			"dayOfMonth":  {"15"},
			"type":        {"transfer"},
			"toAccountId": {"abc"},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid toAccountId), got %d", rr.Code)
	}
}

// ── helpers.go: parseFormAny url.ParseQuery error ────────────────────────────

func TestParseFormAny_InvalidBody(t *testing.T) {
	body := strings.NewReader("key=%zz")
	req := httptest.NewRequest(http.MethodDelete, "/test", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	err := parseFormAny(req)
	if err == nil {
		t.Error("want error for invalid percent-encoded body")
	}
}

// ── helpers.go: parseFormAny ParseForm error ────────────────────────────────

func TestParseFormAny_ParseFormError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = nil
	req.Form = nil
	// Force un Content-Type multipart invalide pour que ParseForm échoue
	req.Header.Set("Content-Type", "multipart/form-data") // pas de boundary → erreur
	err := parseFormAny(req)
	if err == nil {
		t.Error("want error from ParseForm with invalid multipart")
	}
}

// ── passkey.go: PasskeyLoginFinish account rate limit ───────────────────────

func TestPasskeyLoginFinish_AccountRateLimited(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pkacctrl@example.com", "ValidP@ss1!", "USER")

	rawCredID := []byte("testcred-acctrl-12345")
	credIDBase64 := base64.StdEncoding.EncodeToString(rawCredID)
	if err := db.CreateAuthenticator(credIDBase64, "pubkey-test", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	origFinish := hookFinishLogin
	t.Cleanup(func() { hookFinishLogin = origFinish })
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		user, err := h(rawCredID, nil)
		if err != nil || user == nil {
			return nil, nil, errTest
		}
		return user.(*auth.PasskeyUser), &webauthn.Credential{ID: rawCredID}, nil
	}

	// Block loginAccount but allow login IP
	origRL := hookRateLimitCheck
	t.Cleanup(func() { hookRateLimitCheck = origRL })
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		if action == "loginAccount" {
			return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
		}
		return ratelimit.Result{Allowed: true, Remaining: 10}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 (account rate limited), got %d", rr.Code)
	}
}

// ── recurring.go: CreateRecurring description too long ──────────────────────

func TestCreateRecurring_DescriptionTooLong(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "longdesc@example.com", "ValidP@ss1!", "USER")

	longDesc := strings.Repeat("a", 501)
	form := url.Values{
		"description": {longDesc},
		"amount":      {"100.00"},
		"dayOfMonth":  {"1"},
		"type":        {"EXPENSE"},
		"accountId":   {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = injectUser(req, mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (description too long), got %d", rr.Code)
	}
}

// recurring.go parseRecurringForm — UpdateRecurring now rejects an empty
// description (previously it could blank the field). → 400
func TestUpdateRecurring_EmptyDescription(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrec_empty@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {""},
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
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (empty description), got %d", rr.Code)
	}
}

// recurring.go parseRecurringForm — UpdateRecurring now rejects a description
// longer than 500 runes (previously unchecked on update). → 400
func TestUpdateRecurring_DescriptionTooLong(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrec_long@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	req := injectUser(
		withParam(
			post("/recurring/"+intStr(recID), url.Values{
				"description": {strings.Repeat("a", 501)},
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
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (description too long), got %d", rr.Code)
	}
}

// settings.go ChangePassword — hookGetSessionVersion error path: the password is
// still updated (200) but no new cookie is re-issued. Covers the svErr != nil branch.
func TestChangePassword_GetSessionVersionError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pwd_sverr@example.com", "OldP@ss1!", "USER")

	orig := hookGetSessionVersion
	hookGetSessionVersion = func(int64) (int, error) { return 0, errTest }
	t.Cleanup(func() { hookGetSessionVersion = orig })

	req := injectUser(post("/settings/password", url.Values{
		"current_password": {"OldP@ss1!"},
		"newPassword":      {"NewValidP@ssw0rd!"},
		"confirmPassword":  {"NewValidP@ssw0rd!"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (password changed despite session-version read error), got %d", rr.Code)
	}
}
