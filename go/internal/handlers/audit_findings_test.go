package handlers

// audit_findings_test.go — non-régression des findings S-03, S-04, S-08 et S-31
// du giga-audit du 21 août 2026.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ── S-03 : souplesse des formats numériques sur les formulaires ──────────────

func TestParseRateFlexible(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    float64
		wantErr bool
	}{
		{"point", "2.5", 2.5, false},
		{"virgule", "2,5", 2.5, false},
		{"pourcent", "2,5 %", 2.5, false},
		{"espace_insecable", "1 2", 12, false},
		{"espace_fine", "1 2", 12, false},
		{"vide", "", 0, false},
		{"negatif", "-1,5", -1.5, false},
		{"invalide", "abc", 0, true},
		{"deux_virgules", "1,2,3", 0, true},
		{"hors_borne", "1000", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRateFlexible(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseRateFlexible(%q): err=%v, wantErr=%t", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseRateFlexible(%q): want %v, got %v", tc.in, tc.want, got)
			}
		})
	}
}

// S-03 : le formulaire de compte accepte désormais ce que l'import CSV accepte.
func TestCreateAccount_AcceptsFrenchNumberFormats(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s03_create@example.com", "ValidP@ss1!", "USER")
	createAcc(t, uid) // la liste rendue en fin de handler n'est pas vide

	var gotBalance int64
	var gotMin, gotMax float64
	orig := hookCreateAccountWithYield
	hookCreateAccountWithYield = func(userID int64, name string, balance int64, color string, position int, isYieldActive bool, yieldType string, yieldMin, yieldMax float64, reinvestmentRate int, targetAccountID *int64, payoutFrequency string) error {
		gotBalance = balance
		gotMin = yieldMin
		gotMax = yieldMax
		return nil
	}
	t.Cleanup(func() { hookCreateAccountWithYield = orig })

	req := injectUser(post("/accounts", url.Values{
		"name":          {"Livret A"},
		"balance":       {"1 500,50 €"},
		"color":         {"#3b82f6"},
		"isYieldActive": {"on"},
		"yieldType":     {"RANGE"},
		"yieldMin":      {"2,5 %"},
		"yieldMax":      {"3,75 %"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if gotBalance != 150050 {
		t.Errorf("balance: want 150050, got %d", gotBalance)
	}
	if gotMin != 2.5 || gotMax != 3.75 {
		t.Errorf("taux: want 2.5/3.75, got %v/%v", gotMin, gotMax)
	}
}

// S-03 : le montant ambigu « 1.234 » (point + 3 chiffres) reste refusé — la
// souplesse ne doit pas devenir de la devinette (cf. FIN-7).
func TestCreateAccount_AmbiguousBalanceStillRejected(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s03_ambig@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":    {"Ambigu"},
		"balance": {"1.234"},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (montant ambigu), got %d", rr.Code)
	}
}

// S-03 : les garde-fous v2.23.0 (NaN/±Inf/maxCents) restent actifs derrière
// parseCentsFlexible.
func TestCreateAccount_GuardsStillActiveThroughFlexibleParse(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s03_guards@example.com", "ValidP@ss1!", "USER")

	for _, bad := range []string{"NaN", "Inf", "-Inf", "1e300", "1e15"} {
		t.Run(bad, func(t *testing.T) {
			req := injectUser(post("/accounts", url.Values{
				"name":    {"Garde"},
				"balance": {bad},
			}), mu(uid, "USER"))
			rr := httptest.NewRecorder()
			CreateAccount(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("balance=%q: want 400, got %d", bad, rr.Code)
			}
		})
	}
}

// S-03 : UpdateBalance accepte la virgule décimale et les espaces.
func TestUpdateBalance_AcceptsFrenchNumberFormat(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s03_upd@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	var gotBalance int64
	orig := hookUpdateAccountBalance
	hookUpdateAccountBalance = func(id, userID int64, balance int64) error {
		gotBalance = balance
		return nil
	}
	t.Cleanup(func() { hookUpdateAccountBalance = orig })

	idStr := intStr(accID)
	req := injectUser(
		withParam(post("/accounts/"+idStr+"/balance", url.Values{
			"balance": {"1 234,56 €"},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if gotBalance != 123456 {
		t.Errorf("balance: want 123456, got %d", gotBalance)
	}
}

// S-03 : un solde vide reste refusé sur UpdateBalance (FIN-2, « vide ≠ 0 »).
func TestUpdateBalance_EmptyStillRejected(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s03_updempty@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	idStr := intStr(accID)
	req := injectUser(
		withParam(post("/accounts/"+idStr+"/balance", url.Values{
			"balance": {""},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateBalance(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (solde vide), got %d", rr.Code)
	}
}

// ── S-04 : virement sans destinataire / vers soi-même ────────────────────────

func TestCreateRecurring_TransferWithoutToAccount_Rejected(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s04_noto@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	called := false
	orig := hookCreateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		called = true
		return nil
	}
	t.Cleanup(func() { hookCreateRecurring = orig })

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement incomplet"},
		"amount":      {"500"},
		"dayOfMonth":  {"1"},
		"type":        {"transfer"},
		"accountId":   {intStr(accID)},
		"toAccountId": {""},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 (destinataire requis), got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "destinataire") {
		t.Errorf("message attendu explicite, got %q", rr.Body.String())
	}
	if called {
		t.Error("aucune écriture ne doit avoir lieu sur un virement sans destinataire")
	}
}

func TestCreateRecurring_TransferToSelf_Rejected(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s04_self@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)

	called := false
	orig := hookCreateRecurring
	hookCreateRecurring = func(userID, accountID int64, toAccountID *int64, description string, amount int64, dayOfMonth int) error {
		called = true
		return nil
	}
	t.Cleanup(func() { hookCreateRecurring = orig })

	req := injectUser(post("/recurring", url.Values{
		"description": {"Virement vers soi-même"},
		"amount":      {"500"},
		"dayOfMonth":  {"1"},
		"type":        {"transfer"},
		"accountId":   {intStr(accID)},
		"toAccountId": {intStr(accID)},
	}), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	CreateRecurring(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 (virement vers soi-même), got %d", rr.Code)
	}
	if called {
		t.Error("aucune écriture ne doit avoir lieu sur un virement vers soi-même")
	}
}

// S-04 : le refus s'applique aussi au chemin PUT, qui ne transmet pas
// accountId (seul le contrôle « destinataire requis » y est donc actif).
func TestUpdateRecurring_TransferWithoutToAccount_Rejected(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "s04_putnoto@example.com", "ValidP@ss1!", "USER")
	accID := createAcc(t, uid)
	recID := createRec(t, uid, accID)

	idStr := intStr(recID)
	req := injectUser(
		withParam(post("/recurring/"+idStr, url.Values{
			"description": {"Virement incomplet"},
			"amount":      {"500"},
			"dayOfMonth":  {"1"},
			"type":        {"transfer"},
		}), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	UpdateRecurring(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (destinataire requis), got %d", rr.Code)
	}
}

// ── S-08 : cohérence GET/POST de l'inscription ───────────────────────────────

// Base vide, ALLOW_REGISTER absent : le POST acceptait déjà (bootstrap du
// premier admin) ; le GET doit désormais l'accepter aussi au lieu de rediriger.
func TestRegisterPage_Bootstrap_EmptyDB_OK(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "")

	rr := httptest.NewRecorder()
	RegisterPage(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (bootstrap premier compte), got %d", rr.Code)
	}
}

// Symétrie : dès qu'un compte existe, GET et POST refusent tous les deux.
func TestRegistrationOpen_ClosesAfterFirstUser(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "")

	if !registrationOpen() {
		t.Fatal("base vide : l'inscription doit être ouverte pour le bootstrap")
	}
	newUser(t, "s08_first@example.com", "ValidP@ss1!", "ADMIN")
	if registrationOpen() {
		t.Error("un compte existe : l'inscription doit être refermée")
	}

	t.Setenv("ALLOW_REGISTER", "true")
	if !registrationOpen() {
		t.Error("ALLOW_REGISTER=true : l'inscription doit être ouverte")
	}
}

// Fail-closed : une erreur de comptage referme le GET comme le POST.
func TestRegisterPage_CountUsersError_Redirects(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "")

	orig := hookCountUsers
	hookCountUsers = func() (int, error) { return 0, errTest }
	t.Cleanup(func() { hookCountUsers = orig })

	rr := httptest.NewRecorder()
	RegisterPage(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusSeeOther {
		t.Errorf("want 303 (fail-closed), got %d", rr.Code)
	}
}

func TestRegisterPage_RenderError(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	orig := hookRender
	hookRender = func(w io.Writer, name string, data interface{}) error { return errTest }
	t.Cleanup(func() { hookRender = orig })

	rr := httptest.NewRecorder()
	RegisterPage(rr, httptest.NewRequest(http.MethodGet, "/register", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// ── S-31 : borne des taux ────────────────────────────────────────────────────

// maxRate ramené de 1000 % à 100 % : la borne inclusive reste acceptée, tout ce
// qui la dépasse est refusé.
func TestParseRate_MaxRateBound(t *testing.T) {
	if got, err := parseRate("100"); err != nil || got != 100 {
		t.Errorf("parseRate(\"100\"): got %v, %v", got, err)
	}
	if got, err := parseRate("-100"); err != nil || got != -100 {
		t.Errorf("parseRate(\"-100\"): got %v, %v", got, err)
	}
	for _, s := range []string{"100.01", "-100.01", "150", "1000"} {
		if _, err := parseRate(s); err == nil {
			t.Errorf("parseRate(%q): want error, got nil", s)
		}
	}
}
