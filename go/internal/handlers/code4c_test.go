package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"pilot-finance/internal/db"
)

// CODE-4 : un échec de lecture des comptes ne doit plus renvoyer des noms vides.
func TestRecurringAPI_AccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "recapi_accerr@example.com", "ValidP@ss1!", "USER")

	origRec := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(int64) ([]db.RecurringOperation, error) {
		return []db.RecurringOperation{{ID: 1, AccountID: 1}}, nil
	}
	t.Cleanup(func() { hookGetRecurringByUserID = origRec })
	origAcc := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = origAcc })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/recurring", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	RecurringAPI(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}
