package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
)

// ----- parseCentsFlexible -----

func TestParseCentsFlexible(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"1234.56", 123456, false},
		{"1234,56", 123456, false},
		{"1.234,56", 123456, false},
		{"1,234.56", 123456, false},
		{"1 234,56 €", 123456, false},
		{"-42", -4200, false},
		{"$99", 9900, false},
		{"€ ", 0, true},
		{"abc", 0, true},
		{"1,2,3", 0, true},
		// audit FIN-7 : point + exactement 3 décimales sans virgule = ambigu → rejet.
		{"1.234", 0, true},
		{"12.345", 0, true},
		{"-1.234", 0, true},
		// mais 2 décimales (non ambigu) reste une décimale.
		{"1.23", 123, false},
		// 4 décimales : pas un groupe de milliers, décimale acceptée.
		{"1.2345", 123, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseCentsFlexible(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseCentsFlexible(%q): err=%v, wantErr=%t", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseCentsFlexible(%q): want %d, got %d", tc.in, tc.want, got)
			}
		})
	}
}

// ----- parseBalancesCSV -----

type importErrReader struct{}

func (importErrReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseBalancesCSV_ReadErrorOrEmpty(t *testing.T) {
	if upd, inv := parseBalancesCSV(importErrReader{}); upd != nil || inv != nil {
		t.Errorf("read error: want (nil, nil), got (%v, %v)", upd, inv)
	}
	if upd, inv := parseBalancesCSV(strings.NewReader("")); upd != nil || inv != nil {
		t.Errorf("empty: want (nil, nil), got (%v, %v)", upd, inv)
	}
}

func TestParseBalancesCSV(t *testing.T) {
	cases := []struct {
		name        string
		csv         string
		wantUpdates []balanceUpdate
		wantInvalid []int
	}{
		{
			name:        "virgule + en-tête ignoré",
			csv:         "nom,solde\nLivret A,100.50\nPEL,2000",
			wantUpdates: []balanceUpdate{{"Livret A", 10050}, {"PEL", 200000}},
		},
		{
			name:        "point-virgule + décimale à virgule",
			csv:         "Livret A;1 234,56\nPEL;42",
			wantUpdates: []balanceUpdate{{"Livret A", 123456}, {"PEL", 4200}},
		},
		{
			name:        "pas d'en-tête, première ligne valide",
			csv:         "Livret A,7",
			wantUpdates: []balanceUpdate{{"Livret A", 700}},
		},
		{
			// audit FIN-8 : BOM UTF-8, fichier sans en-tête → 1re ligne conservée.
			name:        "BOM UTF-8 sans en-tête",
			csv:         "\xef\xbb\xbfLivret A,7",
			wantUpdates: []balanceUpdate{{"Livret A", 700}},
		},
		{
			// audit FIN-7 : "1.234" ambigu → ligne rejetée, pas devinée.
			name:        "point milliers ambigu → invalide",
			csv:         "nom,solde\nLivret A,1.234",
			wantInvalid: []int{2},
		},
		{
			name:        "une seule colonne → invalide",
			csv:         "nom,solde\nSansSolde",
			wantInvalid: []int{2},
		},
		{
			name:        "nom vide → invalide",
			csv:         "nom,solde\n,100",
			wantInvalid: []int{2},
		},
		{
			name:        "montant illisible → invalide",
			csv:         "nom,solde\nLivret A,abc",
			wantInvalid: []int{2},
		},
		{
			name:        "champ excédentaire non vide → invalide",
			csv:         "nom,solde\nLivret A,100,50",
			wantInvalid: []int{2},
		},
		{
			name:        "champ excédentaire vide toléré",
			csv:         "nom,solde\nLivret A,100,",
			wantUpdates: []balanceUpdate{{"Livret A", 10000}},
		},
		{
			name:        "erreur de quoting CSV → invalide",
			csv:         "nom,solde\n\"bad\"quote,1\nPEL,42",
			wantUpdates: []balanceUpdate{{"PEL", 4200}},
			wantInvalid: []int{2},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upd, inv := parseBalancesCSV(strings.NewReader(tc.csv))
			if len(upd) != len(tc.wantUpdates) {
				t.Fatalf("updates: want %v, got %v", tc.wantUpdates, upd)
			}
			for i := range upd {
				if upd[i] != tc.wantUpdates[i] {
					t.Errorf("update %d: want %v, got %v", i, tc.wantUpdates[i], upd[i])
				}
			}
			if len(inv) != len(tc.wantInvalid) {
				t.Fatalf("invalid: want %v, got %v", tc.wantInvalid, inv)
			}
			for i := range inv {
				if inv[i] != tc.wantInvalid[i] {
					t.Errorf("invalid %d: want %d, got %d", i, tc.wantInvalid[i], inv[i])
				}
			}
		})
	}
}

// ----- ImportBalances -----

// csvImportRequest construit une requête multipart avec un fichier CSV.
func csvImportRequest(t *testing.T, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/settings/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// namedAcc crée un compte avec un nom donné et retourne son ID.
func namedAcc(t *testing.T, userID int64, name string, balance int64) int64 {
	t.Helper()
	enc, err := crypto.Encrypt(name)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := db.CreateAccountWithYield(userID, enc, balance, "#3b82f6", 0, false, "FIXED", 0, 0, 100, nil, "MONTHLY"); err != nil {
		t.Fatalf("CreateAccountWithYield: %v", err)
	}
	accs, _ := db.GetAccountsByUserID(userID)
	return accs[len(accs)-1].ID
}

type importResponse struct {
	Updated      int      `json:"updated"`
	Unknown      []string `json:"unknown"`
	Ambiguous    []string `json:"ambiguous"`
	InvalidLines []int    `json:"invalid_lines"`
}

func doImport(t *testing.T, uid int64, csv string) (*httptest.ResponseRecorder, importResponse) {
	t.Helper()
	req := injectUser(csvImportRequest(t, csv), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	ImportBalances(rr, req)
	var resp importResponse
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rr, resp
}

func TestImportBalances_Unauthorized(t *testing.T) {
	setupHandlerTest(t)
	rr := httptest.NewRecorder()
	ImportBalances(rr, csvImportRequest(t, "A,1"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestImportBalances_MissingFile(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-nofile@example.com", "ValidP@ss1!", "USER")
	req := injectUser(httptest.NewRequest(http.MethodPost, "/settings/import", strings.NewReader("x=1")), mu(uid, "USER"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	ImportBalances(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestImportBalances_EmptyCSV(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-empty@example.com", "ValidP@ss1!", "USER")
	rr, _ := doImport(t, uid, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestImportBalances_HappyPath(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-ok@example.com", "ValidP@ss1!", "USER")
	idA := namedAcc(t, uid, "Livret A", 100)
	idB := namedAcc(t, uid, "PEL", 200)

	auditCalled := false
	origAudit := hookLogAudit
	hookLogAudit = func(userID int64, action, ip, ua string) {
		auditCalled = true
		if action != db.AuditImportBalances {
			t.Errorf("audit action: want %s, got %s", db.AuditImportBalances, action)
		}
	}
	defer func() { hookLogAudit = origAudit }()

	// Matching insensible à la casse ; séparateur ; + décimale à virgule.
	rr, resp := doImport(t, uid, "nom;solde\nlivret a;1 234,56\nPEL;42")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if resp.Updated != 2 || len(resp.Unknown) != 0 || len(resp.Ambiguous) != 0 || len(resp.InvalidLines) != 0 {
		t.Errorf("resp: %+v", resp)
	}
	if !auditCalled {
		t.Error("audit log not called")
	}

	accs, _ := db.GetAccountsByUserID(uid)
	balances := map[int64]int64{}
	for _, a := range accs {
		balances[a.ID] = a.Balance
	}
	if balances[idA] != 123456 {
		t.Errorf("Livret A: want 123456, got %d", balances[idA])
	}
	if balances[idB] != 4200 {
		t.Errorf("PEL: want 4200, got %d", balances[idB])
	}
}

func TestImportBalances_UnknownAmbiguousInvalid(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-mixed@example.com", "ValidP@ss1!", "USER")
	namedAcc(t, uid, "Dup", 0)
	namedAcc(t, uid, "Dup", 0)
	idOK := namedAcc(t, uid, "Solo", 0)

	rr, resp := doImport(t, uid, "Inconnu,10\nDup,20\nSolo,30\n,40")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if resp.Updated != 1 {
		t.Errorf("updated: want 1, got %d", resp.Updated)
	}
	if len(resp.Unknown) != 1 || resp.Unknown[0] != "Inconnu" {
		t.Errorf("unknown: %v", resp.Unknown)
	}
	if len(resp.Ambiguous) != 1 || resp.Ambiguous[0] != "Dup" {
		t.Errorf("ambiguous: %v", resp.Ambiguous)
	}
	if len(resp.InvalidLines) != 1 || resp.InvalidLines[0] != 4 {
		t.Errorf("invalid_lines: %v", resp.InvalidLines)
	}

	accs, _ := db.GetAccountsByUserID(uid)
	for _, a := range accs {
		if a.ID == idOK && a.Balance != 3000 {
			t.Errorf("Solo: want 3000, got %d", a.Balance)
		}
	}
}

func TestImportBalances_OnlyInvalidLines_NoAudit(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-inv@example.com", "ValidP@ss1!", "USER")

	auditCalled := false
	origAudit := hookLogAudit
	hookLogAudit = func(int64, string, string, string) { auditCalled = true }
	defer func() { hookLogAudit = origAudit }()

	// Ligne 1 en-tête, ligne 2 invalide → aucune mise à jour, pas d'audit.
	rr, resp := doImport(t, uid, "nom,solde\nX,abc")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if resp.Updated != 0 || len(resp.InvalidLines) != 1 {
		t.Errorf("resp: %+v", resp)
	}
	if auditCalled {
		t.Error("audit should not be logged when nothing was updated")
	}
}

func TestImportBalances_GetAccountsError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-geterr@example.com", "ValidP@ss1!", "USER")

	orig := hookGetAccountsByUserID
	hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errors.New("boom") }
	defer func() { hookGetAccountsByUserID = orig }()

	rr, _ := doImport(t, uid, "A,1")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

func TestImportBalances_UpdateError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-uperr@example.com", "ValidP@ss1!", "USER")
	namedAcc(t, uid, "Livret A", 100)

	orig := hookUpdateAccountBalancesTx
	hookUpdateAccountBalancesTx = func(int64, []db.AccountBalanceUpdate) error { return errors.New("boom") }
	defer func() { hookUpdateAccountBalancesTx = orig }()

	rr, _ := doImport(t, uid, "Livret A,1")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rr.Code)
	}
}

// audit FIN-9 : une erreur en cours d'import ne laisse aucune mise à jour partielle.
func TestImportBalances_Atomic(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-atomic@example.com", "ValidP@ss1!", "USER")
	namedAcc(t, uid, "A", 111)
	namedAcc(t, uid, "B", 222)

	orig := hookUpdateAccountBalancesTx
	hookUpdateAccountBalancesTx = func(int64, []db.AccountBalanceUpdate) error { return errors.New("boom") }
	defer func() { hookUpdateAccountBalancesTx = orig }()

	rr, _ := doImport(t, uid, "A,1000\nB,2000")
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rr.Code)
	}
	// Aucun solde ne doit avoir changé (le hook a tout rejeté).
	accs, _ := db.GetAccountsByUserID(uid)
	for _, a := range accs {
		if a.Balance != 111 && a.Balance != 222 {
			t.Errorf("solde modifié malgré l'échec: %d", a.Balance)
		}
	}
}

func TestImportBalances_UndecryptableNameSkipped(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "import-decerr@example.com", "ValidP@ss1!", "USER")
	namedAcc(t, uid, "Livret A", 100)

	orig := hookDecryptStr
	hookDecryptStr = func(string) (string, error) { return "", errors.New("boom") }
	defer func() { hookDecryptStr = orig }()

	rr, resp := doImport(t, uid, "Livret A,1")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if resp.Updated != 0 || len(resp.Unknown) != 1 {
		t.Errorf("resp: %+v", resp)
	}
}
