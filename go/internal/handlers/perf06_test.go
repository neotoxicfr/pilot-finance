package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// PERF-06 : plus d'ids que de comptes → 400 sans toucher la base.
func TestReorderAccounts_TooManyIDs(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "reorder_many@example.com", "ValidP@ss1!", "USER")
	createAcc(t, uid)

	orig := hookReorderAccounts
	hookReorderAccounts = func(int64, []int64) error {
		t.Error("hookReorderAccounts ne doit pas être appelé")
		return nil
	}
	t.Cleanup(func() { hookReorderAccounts = orig })

	req := injectUser(postBody("/accounts/reorder", []byte(`{"ids":[1,2]}`), "application/json"), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestReorderAccounts_CountError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "reorder_counterr@example.com", "ValidP@ss1!", "USER")

	orig := hookCountAccountsByUserID
	hookCountAccountsByUserID = func(int64) (int, error) { return 0, errTest }
	t.Cleanup(func() { hookCountAccountsByUserID = orig })

	req := injectUser(postBody("/accounts/reorder", []byte(`{"ids":[1]}`), "application/json"), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}
