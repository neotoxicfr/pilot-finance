package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"pilot-finance/internal/db"
)

// SEC-10 : e-mail saisi avec majuscules → compte indexé en minuscules trouvé.
func TestSEC10_LoginMixedCaseEmail(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "jean@example.com", "ValidP@ss1!", "USER")

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{"email": {" Jean@Example.com "}, "password": {"ValidP@ss1!"}}))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("got %d, want 303", rr.Code)
	}
}

// SEC-10 : compte hérité du front Node, indexé sous la casse brute → repli.
func TestSEC10_LoginLegacyMixedCaseAccount(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "Legacy@Example.com", "ValidP@ss1!", "USER")

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{"email": {"Legacy@Example.com"}, "password": {"ValidP@ss1!"}}))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("got %d, want 303", rr.Code)
	}
}

// SEC-10 : une erreur base sur la recherche de repli remonte en 500.
func TestSEC10_LoginFallbackLookupError(t *testing.T) {
	setupHandlerTest(t)
	orig := hookGetUserByBlindIndex
	t.Cleanup(func() { hookGetUserByBlindIndex = orig })
	calls := 0
	hookGetUserByBlindIndex = func(string) (*db.User, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return nil, errors.New("boom")
	}

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{"email": {"Someone@Example.com"}, "password": {"ValidP@ss1!"}}))
	if rr.Code != http.StatusInternalServerError || calls != 2 {
		t.Errorf("got %d after %d lookups, want 500 after 2", rr.Code, calls)
	}
}
