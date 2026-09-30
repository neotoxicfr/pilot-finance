package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"pilot-finance/internal/db"
	"pilot-finance/internal/ratelimit"
)

// SEC-03 : verifyCurrentPassword — limiteur par compte, audit d'échec, format de réponse.

func sec03User(t *testing.T) *db.User {
	t.Helper()
	uid := newUser(t, "sec03@example.com", "ValidP@ss1!", "USER")
	u, err := db.GetUserByID(uid)
	if err != nil || u == nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	return u
}

func captureAudit(t *testing.T) *[]string {
	t.Helper()
	var actions []string
	orig := hookLogAudit
	hookLogAudit = func(_ int64, action, _, _ string) { actions = append(actions, action) }
	t.Cleanup(func() { hookLogAudit = orig })
	return &actions
}

func TestVerifyCurrentPassword_WrongAuditsAndText(t *testing.T) {
	setupHandlerTest(t)
	u := sec03User(t)
	actions := captureAudit(t)

	rr := httptest.NewRecorder()
	if verifyCurrentPassword(rr, post("/x", url.Values{}), u, "bad") {
		t.Fatal("want false on wrong password")
	}
	if rr.Code != http.StatusUnauthorized || rr.Header().Get("X-Error-Code") != ErrAuthInvalid {
		t.Errorf("want 401 AUTH_INVALID, got %d %q", rr.Code, rr.Header().Get("X-Error-Code"))
	}
	if strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		t.Error("form request must get a text error")
	}
	if len(*actions) != 1 || (*actions)[0] != db.AuditLoginFail {
		t.Errorf("want one LOGIN_FAIL audit, got %v", *actions)
	}
}

func TestVerifyCurrentPassword_RateLimitedJSON(t *testing.T) {
	setupHandlerTest(t)
	u := sec03User(t)

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	// 5 échecs autorisés, le 6e est bloqué même avec le BON mot de passe.
	for i := 0; i < 5; i++ {
		verifyCurrentPassword(httptest.NewRecorder(), req, u, "bad")
	}
	rr := httptest.NewRecorder()
	if verifyCurrentPassword(rr, req, u, "ValidP@ss1!") {
		t.Fatal("want false when rate limited")
	}
	if rr.Code != http.StatusTooManyRequests || !strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json") {
		t.Errorf("want 429 JSON, got %d %q", rr.Code, rr.Header().Get("Content-Type"))
	}
	if strings.Contains(rr.Body.String(), "{n}") {
		t.Errorf("placeholder not replaced: %s", rr.Body.String())
	}
}

func TestVerifyCurrentPassword_SuccessResetsLimiter(t *testing.T) {
	setupHandlerTest(t)
	u := sec03User(t)

	for i := 0; i < 4; i++ {
		verifyCurrentPassword(httptest.NewRecorder(), post("/x", url.Values{}), u, "bad")
	}
	if !verifyCurrentPassword(httptest.NewRecorder(), post("/x", url.Values{}), u, "ValidP@ss1!") {
		t.Fatal("want true on correct password")
	}
	// Compteur remis à zéro : un nouveau cycle complet est permis.
	if res := ratelimit.Check(strconv.FormatInt(u.ID, 10), "reauth"); res.Remaining != 4 {
		t.Errorf("want limiter reset (remaining 4), got %d", res.Remaining)
	}
}
