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

// --- HandleLogin : 2FA second step — invalid pending token ---

func TestHandleLogin_2FA_InvalidPendingToken(t *testing.T) {
	setupHandlerTest(t)

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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
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
	setupHandlerTest(t)
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

// --- HandleLogin : 2FA second step — MFASecret nil (DB inconsistency) ---

func TestHandleLogin_2FA_NilMFASecret(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfanil@example.com", "ValidP@ss1!", "USER")

	// Activer MFA sans secret (simule une incohérence DB)
	if err := db.EnableMFA(uid, ""); err != nil {
		t.Fatalf("EnableMFA: %v", err)
	}
	// Forcer MFASecret à nil via hook
	origGetUser := hookGetUserByID
	defer func() { hookGetUserByID = origGetUser }()
	hookGetUserByID = func(id int64) (*db.User, error) {
		u, err := origGetUser(id)
		if u != nil {
			u.MFASecret = nil // simuler DB inconsistency
		}
		return u, err
	}

	pendingToken, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	req := post("/login", url.Values{"twoFactorCode": {"123456"}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pendingToken})
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401 (nil MFASecret), got %d", rr.Code)
	}
}

// --- UpdatePreferences : langue/devise invalides → fallback silencieux ---

func TestUpdatePreferences_InvalidLang_FallsBack(t *testing.T) {
	setupHandlerTest(t)
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
