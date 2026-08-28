package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// --- LoginPage ---

func TestLoginPage_OK(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	LoginPage(rr, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q", ct)
	}
}

// --- LoginSubmit ---

func TestLoginSubmit_DelegatesHandleLogin(t *testing.T) {
	setupHandlerTest(t)

	// Missing fields → 400 (same behavior as HandleLogin)
	rr := httptest.NewRecorder()
	LoginSubmit(rr, post("/login", url.Values{"email": {""}, "password": {""}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// --- RegisterPage ---

func TestRegisterPage_RegisterDisabled_Redirects(t *testing.T) {
	setupHandlerTest(t)
	// Un utilisateur existe déjà : le bootstrap du premier compte ne s'applique
	// plus, donc ALLOW_REGISTER absent referme aussi le GET (audit S-08).
	newUser(t, "already@example.com", "ValidP@ss1!", "ADMIN")

	rr := httptest.NewRecorder()
	RegisterPage(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location: want /login, got %q", loc)
	}
}

func TestRegisterPage_RegisterEnabled_OK(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	rr := httptest.NewRecorder()
	RegisterPage(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- RegisterSubmit ---

func TestRegisterSubmit_DelegatesHandleRegister(t *testing.T) {
	setupHandlerTest(t)

	// Missing fields → 400
	rr := httptest.NewRecorder()
	RegisterSubmit(rr, post("/register", url.Values{"email": {""}, "password": {""}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// --- Logout ---

func TestLogout_WithUser_Redirects(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "logout@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodPost, "/logout", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	Logout(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location: want /login, got %q", loc)
	}
}

func TestLogout_NoUser_Redirects(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	Logout(rr, httptest.NewRequest(http.MethodPost, "/logout", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

// Logout : si IncrementSessionVersion échoue, on log et on continue (303 quand même).
func TestLogout_IncrementSessionVersionError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "logoutsv@example.com", "ValidP@ss1!", "USER")

	orig := hookIncrementSessionVersion
	hookIncrementSessionVersion = func(int64) error { return errTest }
	t.Cleanup(func() { hookIncrementSessionVersion = orig })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/logout", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	Logout(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 (logout best-effort), got %d", rr.Code)
	}
}

// --- Dashboard ---

func TestDashboard_NoUser_Redirects(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	Dashboard(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

func TestDashboard_WithUser_OK(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "dash2@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q", ct)
	}
}

// --- AccountsPage ---

func TestAccountsPage_NoUser_Redirects(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	AccountsPage(rr, httptest.NewRequest(http.MethodGet, "/accounts", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

func TestAccountsPage_WithUser_OK(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "accpage@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- SettingsPage ---

func TestSettingsPage_NoUser_Redirects(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	SettingsPage(rr, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

func TestSettingsPage_WithUser_OK(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "settings@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestSettingsPage_Admin_OK(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "settings_admin@example.com", "ValidP@ss1!", "ADMIN")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("admin settings: want 200, got %d", rr.Code)
	}
}

// --- VerifyEmailPage ---

func TestVerifyEmailPage_NoToken(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, httptest.NewRequest(http.MethodGet, "/verify-email", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Jeton manquant") {
		t.Error("response should mention missing token")
	}
}

func TestVerifyEmailPage_InvalidToken(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, httptest.NewRequest(http.MethodGet, "/verify-email?token=invalidtoken", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- PrivacyPage ---

func TestPrivacyPage_NoUser_OK(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	PrivacyPage(rr, httptest.NewRequest(http.MethodGet, "/privacy", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q", ct)
	}
}

func TestPrivacyPage_WithUser_OK(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "privacy@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/privacy", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PrivacyPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- ForgotPasswordPage ---

func TestForgotPasswordPage_OK(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ForgotPasswordPage(rr, httptest.NewRequest(http.MethodGet, "/forgot-password", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- ForgotPasswordSubmit ---

func TestForgotPasswordSubmit_MailDisabled(t *testing.T) {
	setupHandlerTest(t)
	// SMTP_HOST not set → mail disabled

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"anyone@example.com"}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (mail disabled), got %d", rr.Code)
	}
}

// --- ResetPasswordPage ---

func TestResetPasswordPage_NoToken(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResetPasswordPage(rr, httptest.NewRequest(http.MethodGet, "/reset-password", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 redirect, got %d", rr.Code)
	}
}

func TestResetPasswordPage_InvalidToken(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResetPasswordPage(rr, httptest.NewRequest(http.MethodGet, "/reset-password?token=invalid", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestResetPasswordPage_ValidToken(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resetpage@example.com", "ValidP@ss1!", "USER")

	rawToken := "testrawtoken123"
	hashed := crypto.HashToken(rawToken)
	expiry := time.Now().Add(1 * time.Hour)
	if err := db.SetResetToken(uid, hashed, expiry); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}

	rr := httptest.NewRecorder()
	ResetPasswordPage(rr, httptest.NewRequest(http.MethodGet, "/reset-password?token="+rawToken, nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- ResetPasswordSubmit ---

func TestResetPasswordSubmit_MissingFields(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{"token": {""}, "password": {""}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestResetPasswordSubmit_PasswordMismatch(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{
		"token":           {"sometoken"},
		"password":        {"ValidP@ss1!"},
		"confirmPassword": {"DifferentP@ss1!"},
	}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (renders error), got %d", rr.Code)
	}
}

func TestResetPasswordSubmit_WeakPassword(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{
		"token":           {"sometoken"},
		"password":        {"weak"},
		"confirmPassword": {"weak"},
	}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (renders error), got %d", rr.Code)
	}
}

func TestResetPasswordSubmit_InvalidToken(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{
		"token":           {"invalidtoken"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (renders expired error), got %d", rr.Code)
	}
}

func TestResetPasswordSubmit_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resetsubmit@example.com", "ValidP@ss1!", "USER")

	rawToken := "validresettoken456"
	hashed := crypto.HashToken(rawToken)
	expiry := time.Now().Add(1 * time.Hour)
	if err := db.SetResetToken(uid, hashed, expiry); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{
		"token":           {rawToken},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "reset=success") {
		t.Errorf("Location: want ?reset=success, got %q", loc)
	}
}

// --- baseData locale fallback ---

func TestDashboard_UnknownLanguage_FallsBackToFrFR(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "langunknown@example.com", "ValidP@ss1!", "USER")

	// Language "de" is not in localeMap → baseData falls back to "fr-FR"
	user := &middleware.User{ID: uid, Role: "USER", Language: "de", Currency: "EUR", SessionVersion: 1}
	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), user)
	rr := httptest.NewRecorder()
	Dashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}
