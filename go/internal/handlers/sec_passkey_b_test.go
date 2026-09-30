package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
)

// pkPwdBody : corps JSON de /api/passkey/register/start avec le mot de passe
// des utilisateurs de test (SEC-03).
func pkPwdBody() io.Reader {
	return strings.NewReader(`{"password":"ValidP@ss1!"}`)
}

// SEC-03 : sans mot de passe (corps absent/vide) → 400, aucun challenge émis.
func TestSEC03_PasskeyStart_PasswordRequired(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "sec03pk-missing@example.com", "ValidP@ss1!", "USER")
	for _, body := range []string{"", `{"password":""}`} {
		req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", strings.NewReader(body)), mu(uid, "USER"))
		rr := httptest.NewRecorder()
		PasskeyRegistrationStart(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %q : want 400, got %d", body, rr.Code)
		}
		if len(rr.Result().Cookies()) != 0 {
			t.Errorf("body %q : challenge émis sans ré-auth", body)
		}
	}
}

// SEC-03 : mauvais mot de passe → 401, aucun challenge émis.
func TestSEC03_PasskeyStart_WrongPassword(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "sec03pk-wrong@example.com", "ValidP@ss1!", "USER")
	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", strings.NewReader(`{"password":"nope"}`)), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Error("challenge émis malgré un mauvais mot de passe")
	}
}

// SEC-03 (stub) : utilisateur introuvable en base → 401.
func TestSEC03_PasskeyStart_UserGone(t *testing.T) {
	setupHandlerTest(t)
	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", pkPwdBody()), mu(987654, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

// SEC-02 : CloneWarning levé par go-webauthn → connexion refusée et auditée.
func TestSEC02_PasskeyLogin_CloneWarningRejected(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "sec02clone@example.com", "ValidP@ss1!", "USER")

	orig := hookFinishLogin
	t.Cleanup(func() { hookFinishLogin = orig })
	hookFinishLogin = func(string, *http.Request, func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		return &auth.PasskeyUser{ID: uid}, &webauthn.Credential{Authenticator: webauthn.Authenticator{CloneWarning: true}}, nil
	}
	var audited []string
	origAudit := hookLogAudit
	t.Cleanup(func() { hookLogAudit = origAudit })
	hookLogAudit = func(_ int64, action, _, _ string) { audited = append(audited, action) }

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/login/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "tok"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session" && c.Value != "" {
			t.Error("session émise malgré CloneWarning")
		}
	}
	if len(audited) != 1 || audited[0] != db.AuditLoginFail {
		t.Errorf("audit attendu LOGIN_FAIL, got %v", audited)
	}
}
