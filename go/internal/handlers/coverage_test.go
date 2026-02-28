package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

// --- CreateAccount : branches de validation rendement ---

func TestCreateAccount_InvalidYieldMin(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "invymin@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":         {"Savings"},
		"balance":      {"1000"},
		"color":        {"#3b82f6"},
		"isYieldActive": {"on"},
		"yieldType":    {"FIXED"},
		"yieldMin":     {"notafloat"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid yieldMin), got %d", rr.Code)
	}
}

func TestCreateAccount_InvalidYieldMax(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "invymax@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":         {"Savings"},
		"balance":      {"1000"},
		"color":        {"#3b82f6"},
		"isYieldActive": {"on"},
		"yieldType":    {"RANGE"},
		"yieldMin":     {"1.0"},
		"yieldMax":     {"notafloat"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid yieldMax), got %d", rr.Code)
	}
}

func TestCreateAccount_InvalidReinvestmentRate(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "invreinv@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":             {"Savings"},
		"balance":          {"1000"},
		"color":            {"#3b82f6"},
		"reinvestmentRate": {"notanint"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid reinvestmentRate), got %d", rr.Code)
	}
}

func TestCreateAccount_RANGE_MinGtMax(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "rangeinv@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":          {"Savings"},
		"balance":       {"1000"},
		"color":         {"#3b82f6"},
		"isYieldActive": {"on"},
		"yieldType":     {"RANGE"},
		"yieldMin":      {"5.0"},
		"yieldMax":      {"2.0"}, // min > max → erreur
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (min > max), got %d", rr.Code)
	}
}

func TestCreateAccount_RANGE_NegativeMin(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "rangeneg@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":          {"Savings"},
		"balance":       {"1000"},
		"color":         {"#3b82f6"},
		"isYieldActive": {"on"},
		"yieldType":     {"RANGE"},
		"yieldMin":      {"-1.0"},
		"yieldMax":      {"2.0"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (negative yieldMin), got %d", rr.Code)
	}
}

func TestCreateAccount_Update(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updacc@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(post("/accounts", url.Values{
		"id":      {strconv.FormatInt(accID, 10)},
		"name":    {"Updated Name"},
		"balance": {"9999"},
		"color":   {"#10b981"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("update account: want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestCreateAccount_PayoutYearly(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "yearly@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":            {"Obligataire"},
		"balance":         {"5000"},
		"color":           {"#3b82f6"},
		"payoutFrequency": {"YEARLY"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- CreateRecurring : branches non couvertes ---

func TestCreateRecurring_InvalidAmount(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crecbadamt@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(post("/recurring", url.Values{
		"description": {"Bad"},
		"amount":      {"notafloat"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {strconv.FormatInt(accID, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid amount), got %d", rr.Code)
	}
}

func TestCreateRecurring_InvalidAccountID(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crecbadacc@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/recurring", url.Values{
		"description": {"Bad"},
		"amount":      {"100"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {"notanid"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid accountId), got %d", rr.Code)
	}
}

func TestCreateRecurring_WithToAccount(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crecto@example.com", "ValidP@ss1!", "USER")
	accID1 := createAcc(t, uid)
	accID2 := createAcc(t, uid) // compte destination

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement"},
		"amount":      {"500"},
		"dayOfMonth":  {"1"},
		"type":        {"income"},
		"accountId":   {strconv.FormatInt(accID1, 10)},
		"toAccountId": {strconv.FormatInt(accID2, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestCreateRecurring_IncomeNegativeAmount(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "crecincome@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	// income type avec montant négatif → doit être renégativé (en positif)
	req := injectUser(post("/recurring", url.Values{
		"description": {"Remboursement"},
		"amount":      {"-200"},
		"dayOfMonth":  {"10"},
		"type":        {"income"},
		"accountId":   {strconv.FormatInt(accID, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- UpdateRecurring : branches non couvertes ---

func TestUpdateRecurring_Expense(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrecexp@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	idStr := strconv.FormatInt(recID, 10)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Loyer"},
			"amount":      {"800"},
			"dayOfMonth":  {"1"},
			"type":        {"expense"}, // doit négativiser le montant
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestUpdateRecurring_WithToAccount(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrecto@example.com", "ValidP@ss1!", "USER")
	accID1 := createAcc(t, uid)
	accID2 := createAcc(t, uid)
	recID := createRec(t, uid, accID1)

	idStr := strconv.FormatInt(recID, 10)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Virement auto"},
			"amount":      {"300"},
			"dayOfMonth":  {"15"},
			"type":        {"income"},
			"toAccountId": {strconv.FormatInt(accID2, 10)},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestUpdateRecurring_InvalidAmount(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updrecbadamt@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	idStr := strconv.FormatInt(recID, 10)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Bad"},
			"amount":      {"notafloat"},
			"dayOfMonth":  {"1"},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// --- RecurringAPI : avec toAccountID ---

func TestRecurringAPI_WithTransfer(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "recapitr@example.com", "ValidP@ss1!", "USER")
	accID1 := createAcc(t, uid)
	accID2 := createAcc(t, uid)

	// Créer une récurrente avec toAccountID
	if err := db.CreateRecurring(uid, accID1, &accID2, encStr(t, "Virement"), 500.0, 1); err != nil {
		t.Fatalf("CreateRecurring with toAccount: %v", err)
	}

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/recurring", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	RecurringAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"toAccountName"`) {
		t.Error("response should contain toAccountName field")
	}
}

// encStr chiffre une chaîne avec crypto.Encrypt pour les appels directs en DB.
func encStr(t *testing.T, s string) string {
	t.Helper()
	enc, err := crypto.Encrypt(s)
	if err != nil {
		t.Fatalf("encStr: %v", err)
	}
	return enc
}

// --- AuditPage : avec entrées + résolution email ---

func TestAuditPage_WithEntries(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "auditwithen@example.com", "ValidP@ss1!", "ADMIN")

	// Créer quelques entrées d'audit
	db.LogAudit(uid, db.AuditLoginSuccess, "127.0.0.1", "test-agent")
	db.LogAudit(uid, db.AuditPasswordChange, "127.0.0.1", "test-agent")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	AuditPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestAuditPage_NilUser_Forbidden(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	AuditPage(rr, httptest.NewRequest(http.MethodGet, "/admin/audit", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rr.Code)
	}
}

func TestAuditPage_Pagination(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "auditpage2@example.com", "ValidP@ss1!", "ADMIN")

	req := injectUser(
		httptest.NewRequest(http.MethodGet, "/admin/audit?page=2", nil),
		mu(uid, "ADMIN"),
	)
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

// --- DashboardAPI : paramètre years ---

func TestDashboardAPI_WithYearsParam(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "dashyears@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		httptest.NewRequest(http.MethodGet, "/api/dashboard?years=10", nil),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DashboardAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}
