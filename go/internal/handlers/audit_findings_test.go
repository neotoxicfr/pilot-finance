package handlers

// audit_findings_test.go — non-régression des findings S-03, S-04, S-08, S-31
// et FIN-14 du giga-audit du 21 août 2026.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/ratelimit"
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

// ── FIN-14 : messages d'erreur utilisateur traduits ──────────────────────────

// requestLang privilégie la préférence du compte, et retombe sur
// Accept-Language quand le contexte n'est pas authentifié (login, register,
// forgot/reset password : middleware.GetUser y renvoie nil).
func TestFIN14_RequestLang(t *testing.T) {
	cases := []struct {
		name   string
		accept string
		user   func() *middleware.User
		want   string
	}{
		{"compte_en", "fr-FR,fr;q=0.9", func() *middleware.User { u := mu(1, "USER"); u.Language = "en"; return u }, "en"},
		{"compte_fr", "en-US,en;q=0.9", func() *middleware.User { return mu(1, "USER") }, "fr"},
		{"compte_sans_langue", "fr-FR", func() *middleware.User { u := mu(1, "USER"); u.Language = ""; return u }, "fr"},
		{"anonyme_fr", "fr-FR,fr;q=0.9", nil, "fr"},
		{"anonyme_en", "en-US,en;q=0.9", nil, "en"},
		{"anonyme_sans_entete", "", nil, "en"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.accept != "" {
				req.Header.Set("Accept-Language", tc.accept)
			}
			if tc.user != nil {
				req = injectUser(req, tc.user())
			}
			if got := requestLang(req); got != tc.want {
				t.Errorf("requestLang: got %q, want %q", got, tc.want)
			}
		})
	}
}

// Chemin NON authentifié : le message suit la langue du navigateur, pas un
// français codé en dur. Le code machine-readable, lui, ne bouge pas.
func TestFIN14_UnauthenticatedErrorFollowsAcceptLanguage(t *testing.T) {
	setupHandlerTest(t)

	cases := map[string]string{
		"en-US,en;q=0.9": "Email and password required",
		"fr-FR,fr;q=0.9": "Email et mot de passe requis",
	}
	for accept, want := range cases {
		t.Run(accept, func(t *testing.T) {
			req := post("/login", url.Values{"email": {""}, "password": {""}})
			req.Header.Set("Accept-Language", accept)
			rr := httptest.NewRecorder()
			HandleLogin(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", rr.Code)
			}
			if got := rr.Header().Get("X-Error-Code"); got != ErrValidation {
				t.Errorf("X-Error-Code: got %q, want %q", got, ErrValidation)
			}
			if !strings.Contains(rr.Body.String(), want) {
				t.Errorf("message: got %q, want contains %q", rr.Body.String(), want)
			}
		})
	}
}

// Chemin authentifié : le message suit la préférence enregistrée sur le compte.
func TestFIN14_AuthenticatedErrorFollowsAccountLanguage(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "fin14@example.com", "ValidP@ss1!", "USER")

	cases := map[string]string{"fr": "Nom requis", "en": "Name required"}
	for lang, want := range cases {
		t.Run(lang, func(t *testing.T) {
			u := mu(uid, "USER")
			u.Language = lang
			// Accept-Language contradictoire : la préférence du compte prime.
			req := injectUser(post("/accounts", url.Values{"name": {""}}), u)
			req.Header.Set("Accept-Language", "de-DE")
			rr := httptest.NewRecorder()
			CreateAccount(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d", rr.Code)
			}
			if got := rr.Header().Get("X-Error-Code"); got != ErrValidation {
				t.Errorf("X-Error-Code: got %q, want %q", got, ErrValidation)
			}
			if !strings.Contains(rr.Body.String(), want) {
				t.Errorf("message: got %q, want contains %q", rr.Body.String(), want)
			}
		})
	}
}

// Les messages paramétrés conservent leur valeur : « {n} » est substitué dans
// les deux langues (clientErrorTn).
func TestFIN14_RateLimitMessageKeepsMinutes(t *testing.T) {
	setupHandlerTest(t)
	ratelimit.StopAll()

	orig := hookRateLimitCheck
	hookRateLimitCheck = func(identifier, action string) ratelimit.Result {
		return ratelimit.Result{Allowed: false, RetryAfterMs: 900000, Remaining: 0}
	}
	t.Cleanup(func() { hookRateLimitCheck = orig })

	cases := map[string]string{
		"fr-FR": "Trop de tentatives. Réessayez dans 16 min.",
		"en-US": "Too many attempts. Try again in 16 min.",
	}
	for accept, want := range cases {
		t.Run(accept, func(t *testing.T) {
			req := post("/login", url.Values{"email": {"x@example.com"}, "password": {"y"}})
			req.Header.Set("Accept-Language", accept)
			rr := httptest.NewRecorder()
			HandleLogin(rr, req)

			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("want 429, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), want) {
				t.Errorf("message: got %q, want contains %q", rr.Body.String(), want)
			}
		})
	}
}

// jsonErrorT : le corps JSON porte le code structuré ET le message traduit.
func TestFIN14_JSONErrorIsTranslated(t *testing.T) {
	setupHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/mfa/setup", nil)
	req.Header.Set("Accept-Language", "en-US")
	rr := httptest.NewRecorder()
	MFASetup(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("corps JSON illisible: %v (%q)", err, rr.Body.String())
	}
	if body["code"] != ErrAuthRequired {
		t.Errorf("code: got %q, want %q", body["code"], ErrAuthRequired)
	}
	if body["error"] != "Not authenticated" {
		t.Errorf("error: got %q, want %q", body["error"], "Not authenticated")
	}
}

// Toute clé passée aux helpers traduits doit exister dans les DEUX locales :
// une clé absente ferait retomber i18n.T sur la clé brute (« error.invalid_id »
// affiché tel quel à l'utilisateur).
func TestFIN14_ErrorKeysExistInBothLocales(t *testing.T) {
	setupHandlerTest(t)

	for _, key := range collectErrorKeys(t) {
		for _, lang := range []string{"fr", "en"} {
			if got := i18n.T(lang, key); got == key {
				t.Errorf("clé %q absente de locales/%s.json", key, lang)
			}
		}
	}
}

// collectErrorKeys extrait les clés i18n passées à clientErrorT / clientErrorTn
// / jsonErrorT par les sources de production du paquet handlers.
func collectErrorKeys(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	re := regexp.MustCompile(`(?:clientErrorTn?|jsonErrorT)\(w, r, Err[A-Za-z]+, "([a-z0-9_.]+)"`)
	seen := map[string]bool{}
	var keys []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				keys = append(keys, m[1])
			}
		}
	}
	if len(keys) < 50 {
		t.Fatalf("extraction cassée : %d clés trouvées, ~70 attendues", len(keys))
	}
	return keys
}
