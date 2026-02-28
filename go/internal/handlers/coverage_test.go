package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

// errTest est une erreur factice pour les tests d'injection de dépendances.
var errTest = errors.New("injected test error")

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

// --- UpdateBalance : ID invalide ---

func TestUpdateBalance_InvalidID(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "updbalidx@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(post("/accounts/abc/balance", url.Values{"balance": {"1000"}}), "id", "abc"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid id), got %d", rr.Code)
	}
}

// --- ChangePassword : utilisateur introuvable en DB ---

func TestChangePassword_UserNotInDB(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	// Injecter un utilisateur avec un ID qui n'existe pas en base
	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {"OldP@ss1!"},
		"newPassword":     {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	}), mu(99999, "USER")) // ID inexistant
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404 (user not in db), got %d", rr.Code)
	}
}

// --- renderAccountsList : compte avec rendement YEARLY ---
// Couvre la branche : payout.PayoutFrequency=="YEARLY" dans renderAccountsList et AccountsPage
// Conditions : IsYieldActive && ReinvestmentRate < 100 && TargetAccountID != nil

func TestCreateAccount_YieldYearly_CoversPayoutBranch(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "yldyrly@example.com", "ValidP@ss1!", "USER")

	// 1. Créer un compte cible (destination des intérêts)
	targetID := createAcc(t, uid)

	// 2. Créer le compte source avec rendement YEARLY, reinvestment=0% → payout distribué
	req1 := injectUser(post("/accounts", url.Values{
		"name":             {"PEA"},
		"balance":          {"10000"},
		"color":            {"#3b82f6"},
		"isYieldActive":    {"on"},
		"yieldType":        {"FIXED"},
		"yieldMin":         {"5.0"},
		"yieldMax":         {"5.0"},
		"reinvestmentRate": {"0"},
		"targetAccountId":  {strconv.FormatInt(targetID, 10)},
		"payoutFrequency":  {"YEARLY"},
	}), mu(uid, "USER"))
	rr1 := httptest.NewRecorder()
	CreateAccount(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("create yield account: want 200, got %d (body: %s)", rr1.Code, rr1.Body.String())
	}

	// 3. Appeler AccountsPage pour couvrir la branche YEARLY dans renderAccountsList
	pageReq := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rrPage := httptest.NewRecorder()
	AccountsPage(rrPage, pageReq)
	if rrPage.Code != http.StatusOK {
		t.Errorf("AccountsPage with YEARLY yield: want 200, got %d", rrPage.Code)
	}
}

// --- AccountsPage avec données riches (YEARLY yield + récurrente dépense) ---

func TestAccountsPage_WithRichData(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "richaccp@example.com", "ValidP@ss1!", "USER")

	// Compte cible (destination des intérêts non réinvestis)
	targetID := createAcc(t, uid)

	// Compte avec rendement YEARLY, reinvestment=0%, targetAccount → génère payout YEARLY
	accReq := injectUser(post("/accounts", url.Values{
		"name":             {"Obligations"},
		"balance":          {"5000"},
		"color":            {"#10b981"},
		"isYieldActive":    {"on"},
		"yieldType":        {"FIXED"},
		"yieldMin":         {"3.0"},
		"yieldMax":         {"3.0"},
		"reinvestmentRate": {"0"},
		"targetAccountId":  {strconv.FormatInt(targetID, 10)},
		"payoutFrequency":  {"YEARLY"},
	}), mu(uid, "USER"))
	rrAcc := httptest.NewRecorder()
	CreateAccount(rrAcc, accReq)

	// Ajouter une récurrente dépense pour couvrir la branche monthlyExpenses
	req2 := injectUser(post("/recurring", url.Values{
		"description": {"Abonnement"},
		"amount":      {"50"},
		"dayOfMonth":  {"5"},
		"type":        {"expense"},
		"accountId":   {strconv.FormatInt(targetID, 10)},
	}), mu(uid, "USER"))
	rr2 := httptest.NewRecorder()
	CreateRecurring(rr2, req2)

	// Accéder à AccountsPage — doit rendre 200
	pageReq := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, pageReq)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
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

// --- ChangePassword : nil user (401) ---

func TestChangePassword_Unauthorized(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	ChangePassword(rr, post("/settings/password", url.Values{
		"currentPassword": {"OldP@ss1!"},
		"newPassword":     {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

// --- ExportData : avec données accounts + recurrings ---

func TestExportData_WithData(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "exportdata@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	createRec(t, uid, accID)

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition: got %q", cd)
	}
	var export map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&export); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if _, ok := export["accounts"]; !ok {
		t.Error("export should contain 'accounts'")
	}
}

// --- renderAccountsList : branches MONTHLY payout + recurrings loop ---
// Couvre: else branch (payout MONTHLY), rec.ToAccountID!=nil, rec.Amount>0, rec.Amount<0

func TestRenderAccountsList_MonthlyAndRecurrings(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "rendlist@example.com", "ValidP@ss1!", "USER")

	// Compte cible pour les intérêts distribués
	targetID := createAcc(t, uid)

	// Compte source avec rendement MONTHLY, reinvestment=0%, targetAccountId=targetID
	// → génère un payout MONTHLY dans renderAccountsList (via CreateAccount)
	req1 := injectUser(post("/accounts", url.Values{
		"name":             {"Livret A"},
		"balance":          {"5000"},
		"color":            {"#3b82f6"},
		"isYieldActive":    {"on"},
		"yieldType":        {"FIXED"},
		"yieldMin":         {"3.0"},
		"yieldMax":         {"3.0"},
		"reinvestmentRate": {"0"},
		"targetAccountId":  {strconv.FormatInt(targetID, 10)},
		// payoutFrequency absent → default MONTHLY
	}), mu(uid, "USER"))
	rr1 := httptest.NewRecorder()
	CreateAccount(rr1, req1) // triggers renderAccountsList with MONTHLY payout

	// Récupérer l'ID du compte yield créé
	accs, _ := db.GetAccountsByUserID(uid)
	var sourceID int64
	for _, a := range accs {
		if a.ID != targetID {
			sourceID = a.ID
		}
	}

	// Créer récurrentes pour couvrir les branches du loop dans renderAccountsList
	toID := targetID
	// 1. Virement interne → rec.ToAccountID != nil → continue
	db.CreateRecurring(uid, sourceID, &toID, encStr(t, "Virement"), 300.0, 1)
	// 2. Revenu positif → rec.Amount > 0 → monthlyIncome +=
	db.CreateRecurring(uid, sourceID, nil, encStr(t, "Salaire"), 2000.0, 1)
	// 3. Dépense négative → rec.Amount < 0 → monthlyExpenses +=
	db.CreateRecurring(uid, sourceID, nil, encStr(t, "Loyer"), -1000.0, 1)

	// Déclencher renderAccountsList via UpdateBalance (avec les récurrentes déjà créées)
	idStr := strconv.FormatInt(targetID, 10)
	req2 := injectUser(
		withParam(post("/accounts/"+idStr+"/balance", url.Values{"balance": {"5500"}}), "id", idStr),
		mu(uid, "USER"),
	)
	rr2 := httptest.NewRecorder()
	UpdateBalance(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("UpdateBalance: want 200, got %d", rr2.Code)
	}
}

// --- AuditPage : entrée orpheline (user_id inexistant) ---
// Couvre emailCache[e.UserID] = strconv.FormatInt(e.UserID, 10) (fallback)

func TestAuditPage_OrphanEntry(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "auditorphan@example.com", "ValidP@ss1!", "ADMIN")

	// Entrée d'audit pour un user inexistant → emailCache ne trouve pas → fallback ID string
	db.LogAudit(99999, db.AuditLoginSuccess, "10.0.0.1", "test-agent")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- HandleLogin : NeedsRehash (bcrypt cost < 12) ---
// Couvre la branche if crypto.NeedsRehash(user.Password) { ... }

func TestHandleLogin_NeedsRehash(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	email := "rehash@example.com"
	password := "ValidP@ss1!"

	// Créer un utilisateur avec un hash bcrypt cost 4 (< 12 → NeedsRehash=true)
	lowCostHash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	encEmail, _ := crypto.Encrypt(email)
	blind := crypto.ComputeBlindIndex(email)
	db.CreateUser(encEmail, blind, string(lowCostHash), "USER")

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {email},
		"password": {password},
	}))
	// Le login doit réussir (303 redirect ou 200 HTMX)
	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Errorf("want 303 or 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- HandleRegister : méthode GET → 405 ---

func TestHandleRegister_GetMethod(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	HandleRegister(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

// --- HandleRegister : second user → role USER ---

func TestHandleRegister_SecondUser(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	newUser(t, "first@example.com", "ValidP@ss1!", "ADMIN") // premier user direct DB

	// Deuxième inscription via HandleRegister → isFirstUser=false → role=USER
	req := post("/register", url.Values{
		"email":           {"second@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	})
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	HandleRegister(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("second user register: want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- ChangePassword : champs manquants → 400 ---

func TestChangePassword_EmptyFields(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pwdempty@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {""},
		"newPassword":     {""},
		"confirmPassword": {""},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (empty fields), got %d", rr.Code)
	}
}

// --- ChangePassword : mots de passe ne correspondent pas → 400 ---

func TestChangePassword_Mismatch(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pwdmismatch@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {"ValidP@ss1!"},
		"newPassword":     {"NewP@ssw0rd!"},
		"confirmPassword": {"DifferentP@ssw0rd!"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (mismatch), got %d", rr.Code)
	}
}

// --- ChangePassword : nouveau mot de passe trop faible → 400 ---

func TestChangePassword_WeakPassword(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pwdweak@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {"ValidP@ss1!"},
		"newPassword":     {"weak"},
		"confirmPassword": {"weak"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (weak password), got %d", rr.Code)
	}
}

// --- UpdatePreferences : langue/devise invalides → fallback silencieux ---

func TestUpdatePreferences_InvalidLangCurrency(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "prefsinvalid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/settings/preferences", url.Values{
		"language": {"es"},     // invalide → fallback "fr"
		"currency": {"XBT"},   // invalide → fallback "EUR"
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	UpdatePreferences(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (with fallback), got %d", rr.Code)
	}
}

// --- ExportData : utilisateur introuvable en DB → 404 ---

func TestExportData_UserNotFound(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(99999, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404 (user not found), got %d", rr.Code)
	}
}

// --- VerifyEmailPage : token vide → rendu avec erreur ---

func TestVerifyEmailPage_EmptyToken(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	VerifyEmailPage(rr, httptest.NewRequest(http.MethodGet, "/verify-email", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (rendered with error), got %d", rr.Code)
	}
}

// ============================================================
// Hook-based error path tests — override function vars to inject errors
// ============================================================

// --- HandleRegister : rate limit → 429 ---

func TestHandleRegister_RateLimit(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

	// 3 tentatives valides (MaxAttempts=3) pour remplir le compteur
	for i := 0; i < 3; i++ {
		req := post("/register", url.Values{
			"email":           {strings.Replace("iter@example.com", "iter", strconv.Itoa(i), 1)},
			"password":        {"weak"}, // échoue validation mais après le check ratelimit
			"confirmPassword": {"weak"},
		})
		rr := httptest.NewRecorder()
		HandleRegister(rr, req)
	}

	// 4ème tentative → rate limited
	req := post("/register", url.Values{
		"email":           {"limited@example.com"},
		"password":        {"weak"},
		"confirmPassword": {"weak"},
	})
	rr := httptest.NewRecorder()
	HandleRegister(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("want 429 (rate limited), got %d", rr.Code)
	}
}

// --- HandleRegister : db.CountUsers error → 500 ---

func TestHandleRegister_CountUsersError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	orig := hookCountUsers
	hookCountUsers = func() (int, error) { return 0, errTest }
	t.Cleanup(func() { hookCountUsers = orig })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"ok@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- HandleRegister : db.GetUserByBlindIndex error → 500 ---

func TestHandleRegister_BlindIndexError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	orig := hookGetUserByBlindIndex
	hookGetUserByBlindIndex = func(string) (*db.User, error) { return nil, errTest }
	t.Cleanup(func() { hookGetUserByBlindIndex = orig })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"ok@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- HandleRegister : crypto.HashPassword error → 500 ---

func TestHandleRegister_HashPasswordError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	orig := hookHashPassword
	hookHashPassword = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookHashPassword = orig })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"ok@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- HandleRegister : crypto.Encrypt error → 500 ---

func TestHandleRegister_EncryptError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	orig := hookEncryptStr
	hookEncryptStr = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookEncryptStr = orig })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"ok@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- HandleRegister : db.CreateUser error → 500 ---

func TestHandleRegister_CreateUserError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	orig := hookCreateUser
	hookCreateUser = func(string, string, string, string) (int64, error) { return 0, errTest }
	t.Cleanup(func() { hookCreateUser = orig })

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"ok@example.com"},
		"password":        {"ValidP@ssw0rd!"},
		"confirmPassword": {"ValidP@ssw0rd!"},
	}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- ChangePassword : crypto.HashPassword error → 500 ---

func TestChangePassword_HashError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pwdhash@example.com", "ValidP@ss1!", "USER")

	orig := hookHashPassword
	hookHashPassword = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookHashPassword = orig })

	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {"ValidP@ss1!"},
		"newPassword":     {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- ChangePassword : db.UpdatePassword error → 500 ---

func TestChangePassword_UpdatePasswordError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pwdupderr@example.com", "ValidP@ss1!", "USER")

	orig := hookUpdatePassword
	hookUpdatePassword = func(int64, string) error { return errTest }
	t.Cleanup(func() { hookUpdatePassword = orig })

	req := injectUser(post("/settings/password", url.Values{
		"currentPassword": {"ValidP@ss1!"},
		"newPassword":     {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- UpdatePreferences : db.UpdateUserPreferences error → 500 ---

func TestUpdatePreferences_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "prefsdberr@example.com", "ValidP@ss1!", "USER")

	orig := hookUpdateUserPrefs
	hookUpdateUserPrefs = func(int64, string, string) error { return errTest }
	t.Cleanup(func() { hookUpdateUserPrefs = orig })

	req := injectUser(post("/settings/preferences", url.Values{
		"language": {"fr"},
		"currency": {"EUR"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	UpdatePreferences(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- UpdatePreferences : auth.GenerateToken error → 500 ---

func TestUpdatePreferences_TokenError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "prefstokenerr@example.com", "ValidP@ss1!", "USER")

	orig := hookGenerateToken
	hookGenerateToken = func(int64, string, string, string, int) (string, error) { return "", errTest }
	t.Cleanup(func() { hookGenerateToken = orig })

	req := injectUser(post("/settings/preferences", url.Values{
		"language": {"fr"},
		"currency": {"EUR"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	UpdatePreferences(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- AccountsAPI : db.GetAccountsByUserID error → 500 ---

func TestAccountsAPI_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "accapidberr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsAPI(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- RecurringAPI : db.GetRecurringByUserID error → 500 ---

func TestRecurringAPI_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "recapidberr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(int64) ([]db.RecurringOperation, error) { return nil, errTest }
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/recurring", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	RecurringAPI(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- DashboardAPI : db.GetAccountsByUserID error → 500 ---

func TestDashboardAPI_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "dashdberr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/dashboard", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DashboardAPI(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- MFASetup : auth.GenerateTOTPSecret error → 500 ---

func TestMFASetup_TOTPSecretError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "mfasetuperr@example.com", "ValidP@ss1!", "USER")

	orig := hookGenerateTOTPSecret
	hookGenerateTOTPSecret = func() (string, error) { return "", errTest }
	t.Cleanup(func() { hookGenerateTOTPSecret = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/api/mfa/setup", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	MFASetup(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- MFAEnable : crypto.Encrypt error → JSON error ---

func TestMFAEnable_EncryptError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "mfaencerr@example.com", "ValidP@ss1!", "USER")

	secret, _ := hookGenerateTOTPSecret()
	code, _ := totp.GenerateCode(secret, time.Now())

	orig := hookEncryptStr
	hookEncryptStr = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookEncryptStr = orig })

	body, _ := json.Marshal(map[string]string{"secret": secret, "code": code})
	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/mfa/enable", bytes.NewReader(body)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MFAEnable(rr, req)
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("want error in response, got none")
	}
}

// --- MFAEnable : db.EnableMFA error → JSON error ---

func TestMFAEnable_EnableMFAError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "mfaenablerr@example.com", "ValidP@ss1!", "USER")

	secret, _ := hookGenerateTOTPSecret()
	code2, _ := totp.GenerateCode(secret, time.Now())

	orig := hookEnableMFA
	hookEnableMFA = func(int64, string) error { return errTest }
	t.Cleanup(func() { hookEnableMFA = orig })

	body, _ := json.Marshal(map[string]string{"secret": secret, "code": code2})
	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/mfa/enable", bytes.NewReader(body)),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	MFAEnable(rr, req)
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("want error in response, got none")
	}
}

// --- MFADisable : db.DisableMFA error → 500 ---

func TestMFADisable_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "mfadisdberr@example.com", "ValidP@ss1!", "USER")

	orig := hookDisableMFA
	hookDisableMFA = func(int64) error { return errTest }
	t.Cleanup(func() { hookDisableMFA = orig })

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/mfa/disable", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	MFADisable(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- Dashboard : hookGetAccountsByUserID error → 500 ---

func TestDashboard_GetAccountsError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "dashdberr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	Dashboard(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- ExportData : hookGetAccountsByUserID error → 500 ---

func TestExportData_AccountsDBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "exportaccerr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAccountsByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- ExportData : hookGetRecurringByUserID error → 500 ---

func TestExportData_RecurringDBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "exportrecerr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetRecurringByUserID
	hookGetRecurringByUserID = func(int64) ([]db.RecurringOperation, error) { return nil, errTest }
	t.Cleanup(func() { hookGetRecurringByUserID = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- DeleteSelfAccount : hookDeleteUserAndData error → 500 ---

func TestDeleteSelfAccount_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "delselfErr@example.com", "ValidP@ss1!", "USER")

	orig := hookDeleteUserAndData
	hookDeleteUserAndData = func(int64) error { return errTest }
	t.Cleanup(func() { hookDeleteUserAndData = orig })

	req := injectUser(httptest.NewRequest(http.MethodDelete, "/settings/account", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- DeleteUser (admin) : hookDeleteUser error → 500 ---

func TestDeleteUser_DBError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	adminUID := newUser(t, "admindelUserErr@example.com", "ValidP@ss1!", "ADMIN")
	targetUID := newUser(t, "targetUserErr@example.com", "ValidP@ss1!", "USER")

	orig := hookDeleteUserAndData
	hookDeleteUserAndData = func(int64) error { return errTest }
	t.Cleanup(func() { hookDeleteUserAndData = orig })

	req := withParam(
		injectUser(httptest.NewRequest(http.MethodDelete, "/admin/users/"+strconv.FormatInt(targetUID, 10), nil), mu(adminUID, "ADMIN")),
		"id", strconv.FormatInt(targetUID, 10),
	)
	rr := httptest.NewRecorder()
	DeleteUser(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- SettingsPage (admin) : hookGetAllUsers error → 500 ---

func TestSettingsPage_Admin_GetAllUsersError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "settingsadminerr@example.com", "ValidP@ss1!", "ADMIN")

	orig := hookGetAllUsers
	hookGetAllUsers = func() ([]db.User, error) { return nil, errTest }
	t.Cleanup(func() { hookGetAllUsers = orig })

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(uid, "ADMIN"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- CreateAccount: reinvestmentRate valide mais hors-limites (< 0) ---

func TestCreateAccount_ReinvestmentRateOutOfRange(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "reinvrange@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":             {"Savings"},
		"balance":          {"1000"},
		"color":            {"#3b82f6"},
		"reinvestmentRate": {"-5"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (reinvestmentRate < 0), got %d", rr.Code)
	}
}

// --- CreateAccount: targetAccountId non-numérique → parse error 400 ---

func TestCreateAccount_TargetIDParseError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "targetparse@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":            {"Savings"},
		"balance":         {"1000"},
		"color":           {"#3b82f6"},
		"targetAccountId": {"notanid"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (targetAccountId parse error), got %d", rr.Code)
	}
}

// --- CreateRecurring: dayOfMonth invalide → normalisé à 1, succès ---

func TestCreateRecurring_DayNormalization(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "daynorm@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(post("/recurring", url.Values{
		"description": {"Test"},
		"amount":      {"100"},
		"dayOfMonth":  {"0"},
		"type":        {"income"},
		"accountId":   {strconv.FormatInt(accID, 10)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (day normalized), got %d", rr.Code)
	}
}

// --- CreateRecurring: toAccountId non-numérique → 400 ---

func TestCreateRecurring_ToAccountIDParseError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "toaccparse@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement"},
		"amount":      {"500"},
		"dayOfMonth":  {"1"},
		"type":        {"transfer"},
		"accountId":   {strconv.FormatInt(accID, 10)},
		"toAccountId": {"notanid"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (toAccountId parse error), got %d", rr.Code)
	}
}

// --- AccountsPage: branches MONTHLY payout + income + virement interne ---

func TestAccountsPage_AllBranches(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "accpageall@example.com", "ValidP@ss1!", "USER")

	// Compte cible
	targetID := createAcc(t, uid)

	// Compte source : rendement MONTHLY, reinvestment=0%, targetAccount → payout MONTHLY
	enc, _ := crypto.Encrypt("Source Account")
	tgt := targetID
	db.CreateAccountWithYield(uid, enc, 10000, "#3b82f6", 1, true, "FIXED", 5.0, 5.0, 0, &tgt, "MONTHLY")

	accs, _ := db.GetAccountsByUserID(uid)
	var sourceID int64
	for _, a := range accs {
		if a.ID != targetID {
			sourceID = a.ID
		}
	}

	// Virement interne → rec.ToAccountID != nil → continue
	db.CreateRecurring(uid, sourceID, &targetID, encStr(t, "Virement"), 300.0, 1)
	// Revenu positif → rec.Amount > 0 → monthlyIncome
	db.CreateRecurring(uid, sourceID, nil, encStr(t, "Salaire"), 2000.0, 1)

	req := injectUser(httptest.NewRequest(http.MethodGet, "/accounts", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	AccountsPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- SettingsPage: admin avec email_encrypted corrompu → continue ---

func TestSettingsPage_Admin_CorruptedEmail(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	adminUID := newUser(t, "admincorrupt@example.com", "ValidP@ss1!", "ADMIN")
	corruptUID := newUser(t, "corrupt@example.com", "ValidP@ss1!", "USER")

	// Hex invalide → crypto.Decrypt échoue → slog.Warn + continue
	db.DB.Exec("UPDATE users SET email_encrypted = ? WHERE id = ?", "gg:gg:gg", corruptUID)

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings", nil), mu(adminUID, "ADMIN"))
	rr := httptest.NewRecorder()
	SettingsPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (corrupt user skipped), got %d", rr.Code)
	}
}

// --- ResetPasswordSubmit: hookHashPassword error → 500 ---

func TestResetPasswordSubmit_HashError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "resethash@example.com", "ValidP@ss1!", "USER")

	rawToken := "aabbccdd112233440000000000000000aabbccdd1122334400000000"
	hashedToken := crypto.HashToken(rawToken)
	db.SetResetToken(uid, hashedToken, time.Now().Add(time.Hour))

	orig := hookHashPassword
	hookHashPassword = func(string) (string, error) { return "", errTest }
	t.Cleanup(func() { hookHashPassword = orig })

	req := post("/reset-password", url.Values{
		"token":           {rawToken},
		"password":        {"NewValidP@ssw0rd!"},
		"confirmPassword": {"NewValidP@ssw0rd!"},
	})
	rr := httptest.NewRecorder()
	ResetPasswordSubmit(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (hash error), got %d", rr.Code)
	}
}

// --- ExportData: avec passkey → couvre le loop du slice passkeys ---

func TestExportData_WithPasskey(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "exportpk@example.com", "ValidP@ss1!", "USER")

	db.CreateAuthenticator("cred-test-id", "pubkey-data", 0, "platform", true, true, "internal", uid)

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ExportData(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	var export map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&export); err != nil {
		t.Fatalf("decode: %v", err)
	}
	passkeys, _ := export["passkeys"].([]interface{})
	if len(passkeys) == 0 {
		t.Error("export should contain at least one passkey")
	}
}

