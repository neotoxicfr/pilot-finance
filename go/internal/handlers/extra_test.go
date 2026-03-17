package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp/totp"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// withParam injecte un paramètre chi dans le contexte de la requête.
func withParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// mu construit un middleware.User minimal pour injection.
func mu(id int64, role string) *middleware.User {
	return &middleware.User{
		ID: id, Role: role, Language: "fr", Currency: "EUR",
		Email: "test@example.com", SessionVersion: 1,
	}
}

// createAcc crée un compte de test et retourne son ID.
func createAcc(t *testing.T, userID int64) int64 {
	t.Helper()
	enc, _ := crypto.Encrypt("Test Account")
	if err := db.CreateAccountWithYield(userID, enc, 1000, "#3b82f6", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY"); err != nil {
		t.Fatalf("CreateAccountWithYield: %v", err)
	}
	accs, _ := db.GetAccountsByUserID(userID)
	if len(accs) == 0 {
		t.Fatal("no accounts after create")
	}
	return accs[len(accs)-1].ID
}

// createRec crée une récurrente de test et retourne son ID.
func createRec(t *testing.T, userID, accountID int64) int64 {
	t.Helper()
	enc, _ := crypto.Encrypt("Test Recurring")
	if err := db.CreateRecurring(userID, accountID, nil, enc, 100.0, 1); err != nil {
		t.Fatalf("CreateRecurring: %v", err)
	}
	recs, _ := db.GetRecurringByUserID(userID)
	if len(recs) == 0 {
		t.Fatal("no recurrings after create")
	}
	return recs[len(recs)-1].ID
}

// --- htmxRedirect ---

func TestHtmxRedirect_HTMX(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	htmxRedirect(rr, req, "/dashboard")
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("HX-Redirect"); got != "/dashboard" {
		t.Errorf("HX-Redirect: want /dashboard, got %q", got)
	}
}

// --- getClientIP ---

func TestGetClientIP_XForwardedFor(t *testing.T) {
	// When RemoteAddr is localhost, getClientIP falls back to X-Forwarded-For
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if ip := getClientIP(req); ip != "1.2.3.4" {
		t.Errorf("want 1.2.3.4, got %q", ip)
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	// When RemoteAddr is localhost, getClientIP falls back to X-Real-IP
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Real-IP", "9.10.11.12")
	if ip := getClientIP(req); ip != "9.10.11.12" {
		t.Errorf("want 9.10.11.12, got %q", ip)
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ip := getClientIP(req); ip == "" {
		t.Error("want non-empty RemoteAddr fallback")
	}
}

func TestGetClientIP_PrefersRemoteAddr(t *testing.T) {
	// getClientIP prefers RemoteAddr over headers when it's not localhost
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "9.10.11.12")
	if ip := getClientIP(req); ip != "10.0.0.1" {
		t.Errorf("want 10.0.0.1 (RemoteAddr), got %q", ip)
	}
}

// --- handleFailedLogin ---

func TestHandleFailedLogin_LockOnFifthFailure(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "lockuser@example.com", "ValidP@ss1!", "USER")

	// 4 failures déjà → newCount = 5 → verrouillage
	user := &db.User{ID: uid, FailedLoginAttempts: 4}
	handleFailedLogin(user)

	dbUser, err := db.GetUserByID(uid)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if dbUser.LockUntil == nil {
		t.Fatal("should be locked after 5th failure")
	}
	if dbUser.LockUntil.Before(time.Now()) {
		t.Error("lock should be in the future")
	}
}

// --- resetLoginAttempts ---

func TestResetLoginAttempts_ClearsCount(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "resetuser@example.com", "ValidP@ss1!", "USER")

	// Provoquer 2 échecs, puis reset
	handleFailedLogin(&db.User{ID: uid, FailedLoginAttempts: 2})
	resetLoginAttempts(uid)

	dbUser, err := db.GetUserByID(uid)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if dbUser.FailedLoginAttempts != 0 {
		t.Errorf("want 0 after reset, got %d", dbUser.FailedLoginAttempts)
	}
}

// --- HandleLogin HTMX ---

func TestHandleLogin_Success_HTMX(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "htmxlogin@example.com", "ValidP@ss1!", "USER")

	req := post("/login", url.Values{
		"email":    {"htmxlogin@example.com"},
		"password": {"ValidP@ss1!"},
	})
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	HandleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HTMX login: want 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("HX-Redirect"); got != "/" {
		t.Errorf("HX-Redirect: want /, got %q", got)
	}
}

// --- HandleRegister success ---

func TestHandleRegister_Success_HTMX(t *testing.T) {
	setupHandlerTest(t)

	req := post("/register", url.Values{
		"email":           {"first@pilot.test"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	})
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	HandleRegister(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("register HTMX: want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("HX-Redirect"); got != "/" {
		t.Errorf("HX-Redirect: want /, got %q", got)
	}
}

// --- DashboardAPI ---

func TestDashboardAPI_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	DashboardAPI(rr, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestDashboardAPI_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "dash@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/dashboard", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DashboardAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["accounts"]; !ok {
		t.Error("response missing 'accounts'")
	}
}

// --- AccountsAPI ---

func TestAccountsAPI_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	AccountsAPI(rr, httptest.NewRequest(http.MethodGet, "/api/accounts", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestAccountsAPI_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "accapi@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q", ct)
	}
}

// --- RecurringAPI ---

func TestRecurringAPI_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	RecurringAPI(rr, httptest.NewRequest(http.MethodGet, "/api/recurring", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestRecurringAPI_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "recapi@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/recurring", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	RecurringAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- DeleteAccount ---

func TestDeleteAccount_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(httptest.NewRequest(http.MethodDelete, "/accounts/1", nil), "id", "1")
	rr := httptest.NewRecorder()
	DeleteAccount(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestDeleteAccount_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delaccid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(withParam(httptest.NewRequest(http.MethodDelete, "/accounts/abc", nil), "id", "abc"), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DeleteAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestDeleteAccount_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delacc@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	idStr := strconv.FormatInt(accID, 10)
	req := injectUser(withParam(httptest.NewRequest(http.MethodDelete, "/accounts/"+idStr, nil), "id", idStr), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DeleteAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- UpdateBalance ---

func TestUpdateBalance_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(post("/accounts/1/balance", url.Values{"balance": {"1000"}}), "id", "1")
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestUpdateBalance_InvalidBalance(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updbalist@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	idStr := strconv.FormatInt(accID, 10)
	req := injectUser(
		withParam(post("/accounts/"+idStr+"/balance", url.Values{"balance": {"notanumber"}}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdateBalance_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updbal@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	idStr := strconv.FormatInt(accID, 10)
	req := injectUser(
		withParam(post("/accounts/"+idStr+"/balance", url.Values{"balance": {"2500"}}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- MoveAccount ---

func TestMoveAccount_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(httptest.NewRequest(http.MethodPost, "/accounts/1/move?direction=up", nil), "id", "1")
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestMoveAccount_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "moveaccid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPost, "/accounts/abc/move?direction=up", nil), "id", "abc"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestMoveAccount_InvalidDirection(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "moveaccdir@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPost, "/accounts/1/move?direction=sideways", nil), "id", "1"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestMoveAccount_NotFound(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "moveaccnf@example.com", "ValidP@ss1!", "USER")

	// Aucun compte créé → currentIdx restera -1
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPost, "/accounts/99999/move?direction=up", nil), "id", "99999"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestMoveAccount_AtBoundary(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "moveaccbnd@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	// Un seul compte à l'index 0 → move up → no-op, retourne la liste
	idStr := strconv.FormatInt(accID, 10)
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPost, "/accounts/"+idStr+"/move?direction=up", nil), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("boundary no-op: want 200, got %d", rr.Code)
	}
}

func TestMoveAccount_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "moveacc@example.com", "ValidP@ss1!", "USER")

	// Créer 2 comptes
	enc1, _ := crypto.Encrypt("Account A")
	enc2, _ := crypto.Encrypt("Account B")
	db.CreateAccountWithYield(uid, enc1, 100, "#3b82f6", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY")
	db.CreateAccountWithYield(uid, enc2, 200, "#10b981", 1, false, "FIXED", 0, 0, 100, nil, "MONTHLY")

	accs, _ := db.GetAccountsByUserID(uid)
	if len(accs) < 2 {
		t.Fatal("need 2 accounts")
	}
	firstID := strconv.FormatInt(accs[0].ID, 10)

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPost, "/accounts/"+firstID+"/move?direction=down", nil), "id", firstID),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MoveAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- ReorderAccounts ---

func TestReorderAccounts_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	body := bytes.NewBufferString(`{"ids":[1,2]}`)
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, httptest.NewRequest(http.MethodPost, "/accounts/reorder", body))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestReorderAccounts_InvalidBody(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "reorderbad@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/accounts/reorder", bytes.NewBufferString(`not-json`)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestReorderAccounts_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "reorder@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	body, _ := json.Marshal(map[string]interface{}{"ids": []int64{accID}})
	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/accounts/reorder", bytes.NewReader(body)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	ReorderAccounts(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", rr.Code)
	}
}

// --- CreateRecurring ---

func TestCreateRecurring_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	CreateRecurring(rr, post("/recurring", url.Values{"description": {"Test"}}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestCreateRecurring_MissingFields(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "crecrembad@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/recurring", url.Values{"description": {"Test"}}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCreateRecurring_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "crerec@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(post("/recurring", url.Values{
		"description": {"Salaire"},
		"amount":      {"3000"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {strconv.FormatInt(accID, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q", ct)
	}
}

func TestCreateRecurring_Update(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrec@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	req := injectUser(post("/recurring", url.Values{
		"id":          {strconv.FormatInt(recID, 10)},
		"description": {"Loyer"},
		"amount":      {"1200"},
		"dayOfMonth":  {"5"},
		"type":        {"expense"},
		"accountId":   {strconv.FormatInt(accID, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- UpdateRecurring ---

func TestUpdateRecurring_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(post("/recurring/1", url.Values{"amount": {"100"}}), "id", "1")
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestUpdateRecurring_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrecid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(post("/recurring/abc", url.Values{"amount": {"100"}, "description": {"T"}}), "id", "abc"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdateRecurring_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "updrecok@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	idStr := strconv.FormatInt(recID, 10)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Updated"},
			"amount":      {"500"},
			"dayOfMonth":  {"15"},
			"type":        {"income"},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- DeleteRecurring ---

func TestDeleteRecurring_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(httptest.NewRequest(http.MethodDelete, "/recurring/1", nil), "id", "1")
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestDeleteRecurring_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delrecid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/abc", nil), "id", "abc"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestDeleteRecurring_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delrec@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	idStr := strconv.FormatInt(recID, 10)
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/recurring/"+idStr, nil), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteRecurring(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- UpdatePreferences ---

func TestUpdatePreferences_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	UpdatePreferences(rr, post("/settings/preferences", url.Values{"language": {"en"}, "currency": {"USD"}}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestUpdatePreferences_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "prefs@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/settings/preferences", url.Values{
		"language": {"en"},
		"currency": {"USD"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	UpdatePreferences(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- DeleteUser ---

func TestDeleteUser_NonAdmin(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "nonAdmin@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/admin/users/1", nil), "id", "1"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeleteUser(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestDeleteUser_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "adminid@example.com", "ValidP@ss1!", "ADMIN")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/admin/users/abc", nil), "id", "abc"),
		mu(uid, "ADMIN"),
	)
	rr := httptest.NewRecorder()
	DeleteUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "adminnf@example.com", "ValidP@ss1!", "ADMIN")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/admin/users/99999", nil), "id", "99999"),
		mu(uid, "ADMIN"),
	)
	rr := httptest.NewRecorder()
	DeleteUser(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestDeleteUser_TargetIsAdmin(t *testing.T) {
	setupHandlerTest(t)
	callerUID := newUser(t, "caller@example.com", "ValidP@ss1!", "ADMIN")
	targetUID := newUser(t, "targetadmin@example.com", "ValidP@ss1!", "ADMIN")

	idStr := strconv.FormatInt(targetUID, 10)
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/admin/users/"+idStr, nil), "id", idStr),
		mu(callerUID, "ADMIN"),
	)
	rr := httptest.NewRecorder()
	DeleteUser(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestDeleteUser_Success(t *testing.T) {
	setupHandlerTest(t)
	adminUID := newUser(t, "admindel@example.com", "ValidP@ss1!", "ADMIN")
	targetUID := newUser(t, "userTarget@example.com", "ValidP@ss1!", "USER")

	idStr := strconv.FormatInt(targetUID, 10)
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/admin/users/"+idStr, nil), "id", idStr),
		mu(adminUID, "ADMIN"),
	)
	rr := httptest.NewRecorder()
	DeleteUser(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d", rr.Code)
	}
}

// --- MFASetup ---

func TestMFASetup_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	MFASetup(rr, httptest.NewRequest(http.MethodGet, "/api/mfa/setup", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestMFASetup_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfasetup@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/mfa/setup", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	MFASetup(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["secret"] == "" {
		t.Error("response should contain non-empty 'secret'")
	}
	if !strings.HasPrefix(resp["imageUrl"], "data:image/png;base64,") {
		t.Errorf("imageUrl should be a PNG data URI, got %q", resp["imageUrl"][:min(len(resp["imageUrl"]), 30)])
	}
}

// --- MFAEnable ---

func TestMFAEnable_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	body := bytes.NewBufferString(`{"secret":"TESTSECRET","code":"123456"}`)
	rr := httptest.NewRecorder()
	MFAEnable(rr, httptest.NewRequest(http.MethodPost, "/api/mfa/enable", body))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestMFAEnable_InvalidBody(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfabody@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/mfa/enable", bytes.NewBufferString(`not-json`)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MFAEnable(rr, req)

	// Retourne JSON avec "error"
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q", ct)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("response should contain 'error' field")
	}
}

func TestMFAEnable_InvalidCode(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfacode@example.com", "ValidP@ss1!", "USER")

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"secret": secret, "code": "000000"})
	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/mfa/enable", bytes.NewReader(body)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MFAEnable(rr, req)

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("invalid code should return error")
	}
}

func TestMFAEnable_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfaok@example.com", "ValidP@ss1!", "USER")

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"secret": secret, "code": code})
	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/mfa/enable", bytes.NewReader(body)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MFAEnable(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["success"] != true {
		t.Errorf("want success=true, got %v", resp["success"])
	}
}

// --- MFADisable ---

func TestMFADisable_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	MFADisable(rr, httptest.NewRequest(http.MethodPost, "/api/mfa/disable", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestMFADisable_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "mfadis@example.com", "ValidP@ss1!", "USER")

	// MFADisable now requires password re-verification
	req := injectUser(post("/api/mfa/disable", url.Values{
		"current_password": {"ValidP@ss1!"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	MFADisable(rr, req)

	// Redirige vers /settings
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/settings" {
		t.Errorf("Location: want /settings, got %q", loc)
	}
}

// --- ChangePassword success ---

func TestChangePassword_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pwdchange@example.com", "OldP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {"OldP@ss1!"},
		"newPassword":     {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- HandleRegister additional branches ---

func TestHandleRegister_EmptyFields(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {""},
		"password":        {""},
		"confirmPassword": {""},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestHandleRegister_DuplicateEmail(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")
	newUser(t, "dup@example.com", "ValidP@ss1!", "USER")

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"dup@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", rr.Code)
	}
}

// --- HandleLogin additional branches ---

func TestHandleLogin_AccountLocked(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "locked@example.com", "ValidP@ss1!", "USER")

	// Verrouiller le compte
	lockUntil := time.Now().Add(15 * time.Minute)
	db.UpdateLoginAttempts(uid, 0, &lockUntil)

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"locked@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 for locked account, got %d", rr.Code)
	}
}

func TestHandleLogin_Success_ResetsFailedAttempts(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "hadattempts@example.com", "ValidP@ss1!", "USER")

	// Simuler 2 échecs précédents
	db.UpdateLoginAttempts(uid, 2, nil)

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"hadattempts@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	// Doit réussir et réinitialiser les tentatives
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303, got %d", rr.Code)
	}
}

// --- LegalPage ---

func TestLegalPage_Anonymous(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	LegalPage(rr, httptest.NewRequest(http.MethodGet, "/legal", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestLegalPage_Authenticated(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "legal@example.com", "ValidP@ss1!", "USER")

	rr := httptest.NewRecorder()
	LegalPage(rr, injectUser(httptest.NewRequest(http.MethodGet, "/legal", nil), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- CreateRecurring IDOR checks ---

func TestCreateRecurring_AccountNotOwned(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "idor1@example.com", "ValidP@ss1!", "USER")
	uid2 := newUser(t, "idor2@example.com", "ValidP@ss1!", "USER")
	accID2 := createAcc(t, uid2) // compte de l'autre utilisateur

	req := injectUser(post("/recurring", url.Values{
		"description": {"Test"},
		"amount":      {"100"},
		"dayOfMonth":  {"1"},
		"type":        {"expense"},
		"accountId":   {strconv.FormatInt(accID2, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (IDOR), got %d", rr.Code)
	}
}

func TestCreateRecurring_ToAccountNotOwned(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "idor3@example.com", "ValidP@ss1!", "USER")
	uid2 := newUser(t, "idor4@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	accID2 := createAcc(t, uid2) // compte destinataire de l'autre utilisateur

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement"},
		"amount":      {"500"},
		"dayOfMonth":  {"15"},
		"type":        {"transfer"},
		"accountId":   {strconv.FormatInt(accID, 10)},
		"toAccountId": {strconv.FormatInt(accID2, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (IDOR toAccount), got %d", rr.Code)
	}
}

// --- CreateAccount IDOR check (targetAccountId) ---

func TestCreateAccount_TargetAccountNotOwned(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "idor5@example.com", "ValidP@ss1!", "USER")
	uid2 := newUser(t, "idor6@example.com", "ValidP@ss1!", "USER")
	accID2 := createAcc(t, uid2)

	req := injectUser(post("/accounts", url.Values{
		"name":            {"Livret A"},
		"balance":         {"1000"},
		"isYieldActive":   {"on"},
		"yieldType":       {"FIXED"},
		"yieldMin":        {"2"},
		"targetAccountId": {strconv.FormatInt(accID2, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (IDOR targetAccount), got %d", rr.Code)
	}
}
