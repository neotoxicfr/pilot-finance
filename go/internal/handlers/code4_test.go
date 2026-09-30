package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// CODE-4 : l'échec d'enregistrement de la langue détectée n'empêche pas
// l'inscription (il est seulement journalisé).
func TestHandleRegister_PrefsErrorNonBlocking(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	orig := hookUpdateUserPrefs
	hookUpdateUserPrefs = func(int64, string, string) error { return errTest }
	t.Cleanup(func() { hookUpdateUserPrefs = orig })

	req := post("/register", url.Values{
		"email":           {"code4@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	})
	req.Header.Set("Accept-Language", "en-US")
	rr := httptest.NewRecorder()
	HandleRegister(rr, req)
	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Errorf("want registration to succeed, got %d (%s)", rr.Code, rr.Body.String())
	}
}
