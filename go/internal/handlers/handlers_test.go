package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"errors"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/i18n"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/ratelimit"
	"pilot-finance/internal/templates"
)

// errReader simule un io.Reader qui échoue
type errReader struct{}

func (e *errReader) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}

const (
	hTestEncKey  = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hTestBlindKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	hTestJWTKey  = "test-jwt-secret-32-chars-minimum!!"
)

// goRoot renvoie le chemin absolu du dossier go/ (go/internal/handlers → ../.. → go/)
func goRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../..")
}

// setupHandlerTest initialise tous les sous-systèmes requis par les handlers.
func setupHandlerTest(t *testing.T) {
	t.Helper()
	root := goRoot()

	ratelimit.StopAll() // reset global in-memory state between tests
	crypto.ResetForTest()
	db.ResetForTest()

	if err := crypto.Init(hTestEncKey, hTestBlindKey); err != nil {
		t.Fatalf("crypto.Init: %v", err)
	}
	auth.InitJWT(hTestJWTKey)

	// Réduire le coût bcrypt au minimum pour accélérer les tests (surtout avec -race)
	origHash := hookHashPassword
	hookHashPassword = func(pwd string) (string, error) {
		b, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.MinCost)
		return string(b), err
	}
	t.Cleanup(func() { hookHashPassword = origHash })

	// Remplacer le dummy hash bcrypt cost 12 par un hash MinCost pour ne pas
	// ralentir les tests qui exercent la branche "user nil" (timing oracle fix).
	origDummy := dummyPasswordHash
	if dummy, err := bcrypt.GenerateFromPassword([]byte("dummy"), bcrypt.MinCost); err == nil {
		dummyPasswordHash = string(dummy)
	}
	t.Cleanup(func() { dummyPasswordHash = origDummy })

	dir := t.TempDir()
	if err := db.Init(db.Config{Path: dir + "/test.db"}); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	if err := i18n.Load(filepath.Join(root, "locales")); err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	if err := templates.Init(filepath.Join(root, "templates")); err != nil {
		t.Fatalf("templates.Init: %v", err)
	}
	t.Cleanup(func() { db.Close() })
}

// newUser crée un utilisateur de test et retourne son ID.
func newUser(t *testing.T, email, password, role string) int64 {
	t.Helper()
	enc, err := crypto.Encrypt(email)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	bi := crypto.ComputeBlindIndex(email)
	hash, err := hookHashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	id, err := db.CreateUser(enc, bi, hash, role)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

// injectUser injecte un middleware.User dans le contexte de la requête.
func injectUser(r *http.Request, u *middleware.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserContextKey, u)
	return r.WithContext(ctx)
}

// post construit une requête POST avec form-encoded body.
func post(path string, form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// ----- HealthCheck -----

func TestHealthCheck(t *testing.T) {
	setupHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	HealthCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status: got %v, want ok", resp["status"])
	}
	if resp["database"] != "ok" {
		t.Errorf("database: got %v, want ok", resp["database"])
	}
}

// ----- CSPReport -----

func TestCSPReport_ValidReport(t *testing.T) {
	body := strings.NewReader(`{"csp-report":{"document-uri":"https://example.com","violated-directive":"script-src"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", body)
	rr := httptest.NewRecorder()
	CSPReport(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d, want 204", rr.Code)
	}
}

func TestCSPReport_ReadError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", &errReader{})
	rr := httptest.NewRecorder()
	CSPReport(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestCSPReport_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/csp-report", strings.NewReader(""))
	rr := httptest.NewRecorder()
	CSPReport(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("got %d, want 204", rr.Code)
	}
}

// ----- HandleLogin -----

func TestHandleLogin_MissingFields(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{"email": {""}, "password": {""}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestHandleLogin_UnknownUser(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"nobody@example.com"},
		"password": {"SomeP@ss1!"},
	}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "login@example.com", "CorrectP@ss1!", "USER")

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"login@example.com"},
		"password": {"WrongP@ss1!"},
	}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestHandleLogin_Success(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "success@example.com", "ValidP@ss1!", "USER")

	rr := httptest.NewRecorder()
	HandleLogin(rr, post("/login", url.Values{
		"email":    {"success@example.com"},
		"password": {"ValidP@ss1!"},
	}))
	// htmxRedirect sans HX-Request → 303 SeeOther
	if rr.Code != http.StatusSeeOther {
		t.Errorf("got %d, want 303", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("Location: got %q, want /", loc)
	}
}

// ----- HandleRegister -----

func TestHandleRegister_WeakPassword(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"new@example.com"},
		"password":        {"weak"},
		"confirmPassword": {"weak"},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestHandleRegister_InvalidEmail(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"notanemail"},
		"password":        {"ValidP@ss1!"},
		"confirmPassword": {"ValidP@ss1!"},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestHandleRegister_PasswordMismatch(t *testing.T) {
	setupHandlerTest(t)
	t.Setenv("ALLOW_REGISTER", "true")

	rr := httptest.NewRecorder()
	HandleRegister(rr, post("/register", url.Values{
		"email":           {"mismatch@example.com"},
		"password":        {"ValidP@ss1!"},
		"confirmPassword": {"OtherP@ss1!"},
	}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

// ----- CreateAccount -----

func TestCreateAccount_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	CreateAccount(rr, post("/accounts", url.Values{"name": {"Test"}}))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestCreateAccount_NoName(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "noname@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{"name": {""}}),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestCreateAccount_InvalidBalance(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "invbal@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{"name": {"Savings"}, "balance": {"not-a-number"}}),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestCreateAccount_InvalidColor(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "invcolor@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":    {"Savings"},
		"balance": {"1000"},
		"color":   {"notacolor"},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestCreateAccount_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "create@example.com", "ValidP@ss1!", "USER")

	req := injectUser(post("/accounts", url.Values{
		"name":    {"Livret A"},
		"balance": {"5000"},
		"color":   {"#3b82f6"},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	CreateAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}
}

// ----- ChangePassword -----

func TestChangePassword_MissingFields(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pwd@example.com", "OldP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"current_password": {""},
		"newPassword":     {""},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pwdwrong@example.com", "OldP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"current_password": {"WrongP@ss1!"},
		"newPassword":     {"NewValidP@ss1!!"},
		"confirmPassword": {"NewValidP@ss1!!"},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestChangePassword_PasswordMismatch(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pwdmatch@example.com", "OldP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"current_password": {"OldP@ss1!"},
		"newPassword":     {"NewP@ss1!"},
		"confirmPassword": {"DifferentP@ss1!"},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestChangePassword_WeakNewPassword(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pwdweak@example.com", "OldP@ss1!", "USER")

	req := injectUser(post("/settings/password", url.Values{
		"current_password": {"OldP@ss1!"},
		"newPassword":     {"weak"},
		"confirmPassword": {"weak"},
	}), &middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ChangePassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

// ----- ExportData -----

func TestExportData_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	ExportData(rr, httptest.NewRequest(http.MethodGet, "/settings/export", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestExportData_ContainsAllFields(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "export@example.com", "ExportP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/settings/export", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	ExportData(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition: got %q, want attachment", cd)
	}

	var export map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&export); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"exported_at", "user", "accounts", "recurrings", "audit_log", "passkeys"} {
		if _, ok := export[field]; !ok {
			t.Errorf("export missing field %q", field)
		}
	}
	userInfo, _ := export["user"].(map[string]interface{})
	if userInfo["email"] != "export@example.com" {
		t.Errorf("user.email: got %v, want export@example.com", userInfo["email"])
	}
}

// ----- DeleteSelfAccount -----

func TestDeleteSelfAccount_Unauthorized(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, httptest.NewRequest(http.MethodDelete, "/settings/account", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestDeleteSelfAccount_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "del@example.com", "DelP@ss1!", "USER")

	body := strings.NewReader("current_password=DelP%40ss1!")
	req := injectUser(httptest.NewRequest(http.MethodDelete, "/settings/account", body),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	DeleteSelfAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
	if rr.Header().Get("HX-Redirect") != "/login" {
		t.Errorf("HX-Redirect: got %q, want /login", rr.Header().Get("HX-Redirect"))
	}
}

// ----- AuditPage -----

func TestAuditPage_NonAdmin(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "user@example.com", "UserP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil),
		&middleware.User{ID: uid, Role: "USER", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestAuditPage_Admin(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "admin@example.com", "AdminP@ss1!", "ADMIN")

	req := injectUser(httptest.NewRequest(http.MethodGet, "/admin/audit", nil),
		&middleware.User{ID: uid, Role: "ADMIN", Language: "fr", Currency: "EUR", SessionVersion: 1})
	rr := httptest.NewRecorder()
	AuditPage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}
