package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// --- ResendVerificationEmail ---

func TestResendVerificationEmail_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestResendVerificationEmail_MailDisabled(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resend_mailoff@example.com", "ValidP@ss1!", "USER")

	// hookMailIsEnabled returns false by default in tests (no SMTP_HOST)
	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (mail disabled), got %d", rr.Code)
	}
	if rr.Header().Get("X-Error-Code") != ErrDisabled {
		t.Errorf("want X-Error-Code=%s, got %q", ErrDisabled, rr.Header().Get("X-Error-Code"))
	}
}

func TestResendVerificationEmail_UserNotFound(t *testing.T) {
	setupHandlerTest(t)

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	origGet := hookGetUserByID
	hookGetUserByID = func(int64) (*db.User, error) { return nil, nil }
	t.Cleanup(func() { hookGetUserByID = origGet })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: 999, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestResendVerificationEmail_GetUserError(t *testing.T) {
	setupHandlerTest(t)

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	origGet := hookGetUserByID
	hookGetUserByID = func(int64) (*db.User, error) { return nil, errors.New("db down") }
	t.Cleanup(func() { hookGetUserByID = origGet })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: 1, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404 on DB error, got %d", rr.Code)
	}
}

func TestResendVerificationEmail_AlreadyVerified(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resend_verified@example.com", "ValidP@ss1!", "USER")

	// Mark email as verified directly in DB
	if err := db.MarkEmailVerified(uid); err != nil {
		t.Fatalf("MarkEmailVerified: %v", err)
	}

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (already verified), got %d", rr.Code)
	}
	if rr.Header().Get("X-Error-Code") != ErrConflict {
		t.Errorf("want X-Error-Code=%s, got %q", ErrConflict, rr.Header().Get("X-Error-Code"))
	}
}

func TestResendVerificationEmail_RateLimited(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resend_rl@example.com", "ValidP@ss1!", "USER")

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	origSend := hookSendVerification
	hookSendVerification = func(_, _, _, _ string) error { return nil }
	t.Cleanup(func() { hookSendVerification = origSend })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first send: want 200, got %d", rr.Code)
	}

	// Second attempt → rate limited (1/5min)
	req2 := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr2 := httptest.NewRecorder()
	ResendVerificationEmail(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second send: want 429, got %d", rr2.Code)
	}
}

func TestResendVerificationEmail_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resend_ok@example.com", "ValidP@ss1!", "USER")

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	called := false
	var capturedEmail, capturedLang string
	origSend := hookSendVerification
	hookSendVerification = func(to, _, _, lang string) error {
		called = true
		capturedEmail = to
		capturedLang = lang
		return nil
	}
	t.Cleanup(func() { hookSendVerification = origSend })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if !called {
		t.Error("hookSendVerification should have been called")
	}
	if capturedEmail != "resend_ok@example.com" {
		t.Errorf("captured email: want resend_ok@example.com, got %q", capturedEmail)
	}
	// dbUser.Language defaults to "fr"
	if capturedLang != "fr" {
		t.Errorf("captured lang: want fr (default), got %q", capturedLang)
	}
}

func TestResendVerificationEmail_DecryptError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resend_dec@example.com", "ValidP@ss1!", "USER")

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	origDec := hookDecryptStr
	hookDecryptStr = func(string) (string, error) { return "", errors.New("decrypt failed") }
	t.Cleanup(func() { hookDecryptStr = origDec })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (decrypt error), got %d", rr.Code)
	}
}

func TestResendVerificationEmail_SendFails(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resend_sendfail@example.com", "ValidP@ss1!", "USER")

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	origSend := hookSendVerification
	hookSendVerification = func(_, _, _, _ string) error { return errors.New("smtp down") }
	t.Cleanup(func() { hookSendVerification = origSend })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/verify-email/resend", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ResendVerificationEmail(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (send failure), got %d", rr.Code)
	}
}

// --- sendVerificationToken (covered indirectly via Resend; explicit error paths here) ---

func TestSendVerificationToken_RandReadError(t *testing.T) {
	setupHandlerTest(t)

	origRand := hookRandRead
	hookRandRead = func([]byte) (int, error) { return 0, errors.New("rng fail") }
	t.Cleanup(func() { hookRandRead = origRand })

	if err := sendVerificationToken(1, "x@example.com", "fr"); err == nil {
		t.Error("want error from rand fail")
	}
}

func TestSendVerificationToken_SetTokenError(t *testing.T) {
	setupHandlerTest(t)

	origSet := hookSetVerificationToken
	hookSetVerificationToken = func(int64, string) error { return errors.New("db fail") }
	t.Cleanup(func() { hookSetVerificationToken = origSet })

	if err := sendVerificationToken(1, "x@example.com", "fr"); err == nil {
		t.Error("want error from SetVerificationToken")
	}
}

func TestSendVerificationToken_DefaultsHostAndLang(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "tokendef@example.com", "ValidP@ss1!", "USER")

	called := false
	var hostCaptured, langCaptured string
	origSend := hookSendVerification
	hookSendVerification = func(_, _, host, lang string) error {
		called = true
		hostCaptured = host
		langCaptured = lang
		return nil
	}
	t.Cleanup(func() { hookSendVerification = origSend })

	if err := sendVerificationToken(uid, "x@example.com", ""); err != nil {
		t.Fatalf("sendVerificationToken: %v", err)
	}
	if !called {
		t.Fatal("hookSendVerification should be called")
	}
	if hostCaptured != "localhost:3000" {
		t.Errorf("default host: want localhost:3000, got %q", hostCaptured)
	}
	if langCaptured != "fr" {
		t.Errorf("default lang: want fr, got %q", langCaptured)
	}
}

func TestSendVerificationToken_HonorsHostEnv(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "tokenhost@example.com", "ValidP@ss1!", "USER")
	t.Setenv("HOST", "pilot.example.com")

	var hostCaptured string
	origSend := hookSendVerification
	hookSendVerification = func(_, _, host, _ string) error {
		hostCaptured = host
		return nil
	}
	t.Cleanup(func() { hookSendVerification = origSend })

	if err := sendVerificationToken(uid, "x@example.com", "en"); err != nil {
		t.Fatalf("sendVerificationToken: %v", err)
	}
	if hostCaptured != "pilot.example.com" {
		t.Errorf("HOST env: want pilot.example.com, got %q", hostCaptured)
	}
}

// --- HandleRegister sends verification email when SMTP enabled ---

func TestHandleRegister_TriggersVerificationEmail(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	called := false
	origSend := hookSendVerification
	hookSendVerification = func(_, _, _, _ string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { hookSendVerification = origSend })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", map[string][]string{
		"email":           {"verifme@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("register: want 303, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if !called {
		t.Error("verification email should be sent during registration")
	}
}

func TestHandleRegister_VerificationEmailFailureNonBlocking(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	origMail := hookMailIsEnabled
	hookMailIsEnabled = func() bool { return true }
	t.Cleanup(func() { hookMailIsEnabled = origMail })

	origSend := hookSendVerification
	hookSendVerification = func(_, _, _, _ string) error { return errors.New("smtp down") }
	t.Cleanup(func() { hookSendVerification = origSend })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", map[string][]string{
		"email":           {"verifail@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	// Even when email fails, registration must succeed
	if rr.Code != http.StatusSeeOther {
		t.Errorf("register: want 303 (non-blocking), got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- Banner rendering: SettingsPage exposes EmailVerified flag ---

func TestSettingsPage_BannerWhenUnverified(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "banner@example.com", "ValidP@ss1!", "USER")

	user := &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1, EmailVerified: false}
	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), user)
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "verify-email") {
		t.Error("settings should contain verify-email anchor for unverified user")
	}
}

func TestSettingsPage_NoBannerWhenVerified(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "noban@example.com", "ValidP@ss1!", "USER")
	if err := db.MarkEmailVerified(uid); err != nil {
		t.Fatalf("MarkEmailVerified: %v", err)
	}

	user := &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1, EmailVerified: true}
	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings?verified=1", nil), user)
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	// Verified user → banner above nav must NOT render
	if strings.Contains(body, "verify_email.banner_text") {
		t.Error("banner should not render for verified user")
	}
}
