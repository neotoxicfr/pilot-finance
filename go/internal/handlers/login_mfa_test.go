package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

// --- HandleLogin : method not allowed ---

func TestHandleLogin_GET_MethodNotAllowed(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	HandleLogin(rr, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

// --- HandleRegister : method not allowed ---

func TestHandleRegister_GET_MethodNotAllowed(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	HandleRegister(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

// --- HandleLogin : 2FA second step — invalid pending token ---

func TestHandleLogin_2FA_InvalidPendingToken(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	// Cookie pending_2fa avec valeur invalide + code quelconque
	req := post("/login", url.Values{"twoFactorCode": {"123456"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: "this.is.garbage"})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (invalid pending token), got %d", rr.Code)
	}
}

// newMFAUser crée un utilisateur avec MFA activé et renvoie (userID, rawSecret).
func newMFAUser(t *testing.T, email string) (int64, string) {
	t.Helper()
	uid := newUser(t, email, "ValidP@ss1!", "USER")
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	encSecret, err := crypto.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt secret: %v", err)
	}
	if err := db.EnableMFA(uid, encSecret); err != nil {
		t.Fatalf("EnableMFA: %v", err)
	}
	return uid, secret
}

// --- HandleLogin : MFA activé, premier step (password OK) → rendu formulaire 2FA ---

func TestHandleLogin_MFAEnabled_ShowsForm(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	newMFAUser(t, "mfaform@example.com")

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"mfaform@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (2FA form), got %d (body: %s)", rr.Code, rr.Body.String())
	}
	// Un cookie pending_2fa doit être posé
	cookies := rr.Result().Cookies()
	var hasPending bool
	for _, c := range cookies {
		if c.Name == "pending_2fa" {
			hasPending = true
		}
	}
	if !hasPending {
		t.Error("want pending_2fa cookie set")
	}
}

// --- HandleLogin : 2FA second step — code TOTP invalide ---

func TestHandleLogin_2FA_WrongCode(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid, _ := newMFAUser(t, "mfa2wrong@example.com")

	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {"000000"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (wrong TOTP), got %d", rr.Code)
	}
}

// --- HandleLogin : 2FA second step — succès complet ---

func TestHandleLogin_2FA_Success(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid, secret := newMFAUser(t, "mfa2ok@example.com")

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
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 redirect after 2FA, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("Location: want /, got %q", loc)
	}
}

// --- UpdatePreferences : langue/devise invalides → fallback silencieux ---

func TestUpdatePreferences_InvalidLang_FallsBack(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "prefsinvalid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/settings/preferences", url.Values{
		"language": {"de"},  // non supporté → fallback "fr"
		"currency": {"RUB"}, // non supporté → fallback "EUR"
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	UpdatePreferences(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (fallback), got %d", rr.Code)
	}
}
