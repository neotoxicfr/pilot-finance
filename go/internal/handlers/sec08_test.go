package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

// SEC-08 : une réinitialisation de mot de passe réussie est journalisée.
func TestResetPasswordSubmit_AuditsPasswordReset(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "sec08@example.com", "ValidP@ss1!", "USER")
	if err := db.SetResetToken(uid, crypto.HashToken("sec08token"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetResetToken: %v", err)
	}
	actions := captureAudit(t)

	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, post("/reset-password", url.Values{
		"token":           {"sec08token"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", rr.Code)
	}
	if len(*actions) != 1 || (*actions)[0] != db.AuditPasswordReset {
		t.Errorf("want one PASSWORD_RESET audit, got %v", *actions)
	}
}
