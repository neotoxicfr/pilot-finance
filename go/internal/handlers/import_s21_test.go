package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pilot-finance/internal/db"
)

// ----- Simulation et modèle CSV (audit S-21) -----
//
// L'import écrasait les soldes dès le choix du fichier : aucune confirmation,
// aucun aperçu, aucun retour arrière. Ces tests figent les deux garanties qui
// rendent l'opération réversible dans la tête de l'utilisateur — une simulation
// ne laisse rien derrière elle, et la confirmation applique EXACTEMENT ce que
// l'aperçu annonçait.

// csvDryRunRequest : même multipart que csvImportRequest, avec le drapeau de
// simulation. C'est un CHAMP du formulaire et non un paramètre d'URL, pour que
// la confirmation reposte le même corps sans lui.
func csvDryRunRequest(t *testing.T, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("dry_run", "1"); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
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

func decodeImport(t *testing.T, rr *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var d map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&d); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, rr.Body.String())
	}
	return d
}

func TestImportBalances_DryRunWritesNothing(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "dryrun@example.com", "ValidP@ss1!", "USER")
	accID := namedAcc(t, uid, "Livret A", 100000)

	var audited bool
	origAudit := hookLogAudit
	hookLogAudit = func(int64, string, string, string) { audited = true }
	t.Cleanup(func() { hookLogAudit = origAudit })

	rr := httptest.NewRecorder()
	ImportBalances(rr, injectUser(csvDryRunRequest(t, "Livret A,2500.50\n"), mu(uid, "USER")))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	d := decodeImport(t, rr)
	if d["dry_run"] != true {
		t.Errorf("dry_run: want true, got %v", d["dry_run"])
	}
	if d["updated"] != float64(1) {
		t.Errorf("updated: want 1, got %v", d["updated"])
	}

	accounts, err := db.GetAccountsByUserID(uid)
	if err != nil {
		t.Fatalf("GetAccountsByUserID: %v", err)
	}
	for _, a := range accounts {
		if a.ID == accID && a.Balance != 100000 {
			t.Errorf("la simulation a MODIFIE le solde : want 100000, got %d", a.Balance)
		}
	}
	if audited {
		t.Error("la simulation ne doit pas produire d'entree d'audit")
	}
}

// L'apercu doit montrer « de X vers Y » : sans le solde courant, l'utilisateur
// ne peut pas juger de ce que la confirmation va ecraser.
func TestImportBalances_PreviewShowsBeforeAndAfter(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "preview@example.com", "ValidP@ss1!", "USER")
	namedAcc(t, uid, "Livret A", 100000)

	rr := httptest.NewRecorder()
	ImportBalances(rr, injectUser(csvDryRunRequest(t, "Livret A,2500.50\nInconnu,10\n"), mu(uid, "USER")))

	d := decodeImport(t, rr)
	changes, ok := d["changes"].([]interface{})
	if !ok || len(changes) != 1 {
		t.Fatalf("changes: want 1 entree, got %v", d["changes"])
	}
	c := changes[0].(map[string]interface{})
	if c["name"] != "Livret A" {
		t.Errorf("name: want Livret A, got %v", c["name"])
	}
	if c["from"] != float64(100000) {
		t.Errorf("from: want 100000 centimes, got %v", c["from"])
	}
	if c["to"] != float64(250050) {
		t.Errorf("to: want 250050 centimes, got %v", c["to"])
	}
	// Un compte inconnu n'a pas de changement a previsualiser, mais reste
	// signale : l'apercu doit dire ce qui NE sera PAS importe.
	if u, _ := d["unknown"].([]interface{}); len(u) != 1 {
		t.Errorf("unknown: want 1, got %v", d["unknown"])
	}
}

// La confirmation applique exactement ce que l'apercu annoncait.
func TestImportBalances_ConfirmAppliesPreview(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "confirm@example.com", "ValidP@ss1!", "USER")
	accID := namedAcc(t, uid, "Livret A", 100000)

	const content = "Livret A,2500.50\n"
	rrDry := httptest.NewRecorder()
	ImportBalances(rrDry, injectUser(csvDryRunRequest(t, content), mu(uid, "USER")))
	previewTo := decodeImport(t, rrDry)["changes"].([]interface{})[0].(map[string]interface{})["to"]

	rr := httptest.NewRecorder()
	ImportBalances(rr, injectUser(csvImportRequest(t, content), mu(uid, "USER")))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if d := decodeImport(t, rr); d["dry_run"] != false {
		t.Errorf("dry_run: want false sur l'application reelle, got %v", d["dry_run"])
	}

	accounts, _ := db.GetAccountsByUserID(uid)
	for _, a := range accounts {
		if a.ID == accID && float64(a.Balance) != previewTo {
			t.Errorf("solde applique %d != apercu %v", a.Balance, previewTo)
		}
	}
}

// Le modele doit etre directement REIMPORTABLE : l'import fait correspondre les
// noms a l'identique, donc un modele vide n'aiderait pas.
func TestImportTemplate_RoundTrips(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "template@example.com", "ValidP@ss1!", "USER")
	namedAcc(t, uid, "Livret A", 1234567)
	namedAcc(t, uid, "Decouvert", -50000)

	rr := httptest.NewRecorder()
	ImportTemplate(rr, injectUser(httptest.NewRequest(http.MethodGet, "/settings/import/template", nil), mu(uid, "USER")))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type: want text/csv, got %q", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition: want attachment, got %q", cd)
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "\xEF\xBB\xBF") {
		t.Error("BOM UTF-8 attendu (Excel lit mal les accents sans lui)")
	}
	for _, want := range []string{"Livret A", "12345.67", "Decouvert", "-500.00"} {
		if !strings.Contains(body, want) {
			t.Errorf("modele : %q absent de %q", want, body)
		}
	}

	// Le point qui compte : relire le modele produit exactement les memes
	// soldes, sans aucun changement.
	updates, invalid := parseBalancesCSV(strings.NewReader(body))
	if len(invalid) != 0 {
		t.Errorf("le modele contient des lignes invalides : %v", invalid)
	}
	got := map[string]int64{}
	for _, u := range updates {
		got[u.name] = u.cents
	}
	if got["Livret A"] != 1234567 || got["Decouvert"] != -50000 {
		t.Errorf("aller-retour incorrect : %v", got)
	}
}

func TestImportTemplate_Errors(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "tmplerr@example.com", "ValidP@ss1!", "USER")

	t.Run("non authentifie", func(t *testing.T) {
		rr := httptest.NewRecorder()
		ImportTemplate(rr, httptest.NewRequest(http.MethodGet, "/settings/import/template", nil))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", rr.Code)
		}
	})
	t.Run("echec de lecture des comptes", func(t *testing.T) {
		orig := hookGetAccountsByUserID
		hookGetAccountsByUserID = func(int64) ([]db.Account, error) { return nil, errTest }
		t.Cleanup(func() { hookGetAccountsByUserID = orig })
		rr := httptest.NewRecorder()
		ImportTemplate(rr, injectUser(httptest.NewRequest(http.MethodGet, "/x", nil), mu(uid, "USER")))
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("want 500, got %d", rr.Code)
		}
	})
	t.Run("nom indechiffrable ignore", func(t *testing.T) {
		namedAcc(t, uid, "Lisible", 100)
		orig := hookDecryptStr
		hookDecryptStr = func(string) (string, error) { return "", errTest }
		t.Cleanup(func() { hookDecryptStr = orig })
		rr := httptest.NewRecorder()
		ImportTemplate(rr, injectUser(httptest.NewRequest(http.MethodGet, "/x", nil), mu(uid, "USER")))
		if rr.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rr.Code)
		}
		if strings.Contains(rr.Body.String(), "Lisible") {
			t.Error("un nom indechiffrable ne doit pas apparaitre")
		}
	})
}

func TestFormatCentsPlain(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{100, "1.00"},
		{1234567, "12345.67"},
		{-50000, "-500.00"},
		{-5, "-0.05"},
	}
	for _, tc := range cases {
		if got := formatCentsPlain(tc.cents); got != tc.want {
			t.Errorf("formatCentsPlain(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}
