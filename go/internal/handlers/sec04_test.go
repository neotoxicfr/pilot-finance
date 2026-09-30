package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// SEC-04 : setup/enable refusés quand le 2FA est déjà actif (pas de
// ré-enrôlement silencieux depuis une session volée).

func TestMFASetupEnable_RejectWhenAlreadyEnabled(t *testing.T) {
	setupHandlerTest(t)
	uid, secret := newMFAUser(t, "sec04@example.com")

	rr := httptest.NewRecorder()
	MFASetup(rr, injectUser(httptest.NewRequest(http.MethodGet, "/settings/mfa/setup", nil), mu(uid, "USER")))
	if rr.Code != http.StatusConflict || rr.Header().Get("X-Error-Code") != ErrConflict {
		t.Errorf("setup: want 409 CONFLICT, got %d %q", rr.Code, rr.Header().Get("X-Error-Code"))
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "mfa_setup" {
			t.Error("setup must not issue a mfa_setup cookie when 2FA is active")
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/mfa/enable", strings.NewReader(`{"code":"000000"}`))
	req.AddCookie(mfaSetupCookie(t, uid, secret))
	rr = httptest.NewRecorder()
	MFAEnable(rr, injectUser(req, mu(uid, "USER")))
	if rr.Code != http.StatusConflict {
		t.Errorf("enable: want 409, got %d", rr.Code)
	}
}

func TestMFASetup_UnknownUser(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	MFASetup(rr, injectUser(httptest.NewRequest(http.MethodGet, "/settings/mfa/setup", nil), mu(999999, "USER")))
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}
