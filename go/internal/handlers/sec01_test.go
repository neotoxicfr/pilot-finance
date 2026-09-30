package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
	"pilot-finance/internal/ratelimit"
)

// SEC-01 : l'étape 2FA est bornée par compte (pas seulement par IP) et
// chaque échec est audité.

func sec01Attempt(t *testing.T, pending, code string, ip int) *httptest.ResponseRecorder {
	t.Helper()
	req := post("/login", url.Values{"twoFactorCode": {code}})
	req.AddCookie(&http.Cookie{Name: "pending_2fa", Value: pending})
	req.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1234", ip/250, ip%250+1)
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)
	return rr
}

func TestHandleLogin_2FA_AccountLimiterAcrossIPs(t *testing.T) {
	setupHandlerTest(t)
	uid, _ := newMFAUser(t, "sec01@example.com")
	actions := captureAudit(t)
	pending, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}

	// 10 essais depuis 10 IP distinctes : tous refusés (401) et audités.
	for i := 0; i < 10; i++ {
		if rr := sec01Attempt(t, pending, "000000", i); rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i, rr.Code)
		}
	}
	if len(*actions) != 10 {
		t.Errorf("want 10 audit entries, got %d", len(*actions))
	}
	for _, a := range *actions {
		if a != db.AuditLoginFail {
			t.Errorf("want LOGIN_FAIL, got %s", a)
		}
	}
	// 11e essai depuis une IP neuve : bloqué par le limiteur par compte.
	if rr := sec01Attempt(t, pending, "000000", 42); rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 from twoFactorAccount, got %d", rr.Code)
	}
}

func TestHandleLogin_2FA_SuccessResetsAccountLimiter(t *testing.T) {
	setupHandlerTest(t)
	uid, secret := newMFAUser(t, "sec01ok@example.com")
	pending, err := auth.GeneratePending2FAToken(uid)
	if err != nil {
		t.Fatalf("GeneratePending2FAToken: %v", err)
	}
	sec01Attempt(t, pending, "000000", 1)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if rr := sec01Attempt(t, pending, code, 2); rr.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", rr.Code)
	}
	if res := ratelimit.Check(strconv.FormatInt(uid, 10), "twoFactorAccount"); res.Remaining != 9 {
		t.Errorf("want twoFactorAccount reset (remaining 9), got %d", res.Remaining)
	}
}
