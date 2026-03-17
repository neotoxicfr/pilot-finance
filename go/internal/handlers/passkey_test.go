package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
)

// --- PasskeyRegistrationStart ---

func TestPasskeyRegistrationStart_NilUser(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestPasskeyRegistrationStart_Success(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test"); err != nil {
		t.Fatalf("InitWebAuthn: %v", err)
	}
	setupHandlerTest(t)
	uid := newUser(t, "pkregstart@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- PasskeyRegistrationFinish ---

func TestPasskeyRegistrationFinish_NilUser(t *testing.T) {
	setupHandlerTest(t)

	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_NoCookie(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pkregnoc@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (no cookie), got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_InvalidJSON(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pkregjson@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", bytes.NewBufferString(`not-json`)),
		mu(uid, "USER"),
	)
	req.AddCookie(&http.Cookie{Name: "passkey_challenge", Value: "dummyvalue"})
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (invalid JSON), got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_ParseError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pkregparse@example.com", "ValidP@ss1!", "USER")

	// JSON structurellement valide mais données invalides → response.Parse() échoue
	body := bytes.NewBufferString(`{"id":"","rawId":"","type":"public-key","response":{"clientDataJSON":"","attestationObject":""}}`)
	req := injectUser(
		httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", body),
		mu(uid, "USER"),
	)
	req.AddCookie(&http.Cookie{Name: "passkey_challenge", Value: "dummyvalue"})
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (parse error), got %d", rr.Code)
	}
}

// --- PasskeyLoginStart ---

func TestPasskeyLoginStart_Success(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test"); err != nil {
		t.Fatalf("InitWebAuthn: %v", err)
	}

	rr := httptest.NewRecorder()
	PasskeyLoginStart(rr, httptest.NewRequest(http.MethodGet, "/api/passkey/auth/start", nil))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- PasskeyLoginFinish ---

func TestPasskeyLoginFinish_NoCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (no cookie), got %d", rr.Code)
	}
}

// --- DeletePasskey ---

func TestDeletePasskey_NilUser(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(httptest.NewRequest(http.MethodDelete, "/api/passkey/1", nil), "id", "1")
	rr := httptest.NewRecorder()
	DeletePasskey(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestDeletePasskey_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delpkid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/api/passkey/abc", nil), "id", "abc"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeletePasskey(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestDeletePasskey_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "delpksucc@example.com", "ValidP@ss1!", "USER")

	if err := db.CreateAuthenticator("cred-del-test", "pubkey-test", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}
	auths, _ := db.GetAuthenticatorsByUserID(uid)
	if len(auths) == 0 {
		t.Fatal("no authenticators after create")
	}
	authID := auths[0].ID

	idStr := strconv.FormatInt(authID, 10)
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodDelete, "/api/passkey/"+idStr, nil), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	DeletePasskey(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("want 204, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- RenamePasskey ---

func TestRenamePasskey_NilUser(t *testing.T) {
	setupHandlerTest(t)

	req := withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/1/rename", nil), "id", "1")
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestRenamePasskey_InvalidID(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rnpkid@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/abc/rename", bytes.NewBufferString(`{"name":"test"}`)), "id", "abc"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestRenamePasskey_InvalidJSON(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rnpkjson@example.com", "ValidP@ss1!", "USER")

	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/1/rename", bytes.NewBufferString(`not-json`)), "id", "1"),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestRenamePasskey_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "rnpksucc@example.com", "ValidP@ss1!", "USER")

	if err := db.CreateAuthenticator("cred-rename-test", "pubkey-test", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}
	auths, _ := db.GetAuthenticatorsByUserID(uid)
	if len(auths) == 0 {
		t.Fatal("no authenticators after create")
	}
	authID := auths[0].ID

	idStr := strconv.FormatInt(authID, 10)
	body, _ := json.Marshal(map[string]string{"name": "iPhone de Neo"})
	req := injectUser(
		withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/"+idStr+"/rename", bytes.NewReader(body)), "id", idStr),
		mu(uid, "USER"),
	)
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- PasskeyRegistrationStart — error paths ---

func TestPasskeyRegistrationStart_GetAuthsError(t *testing.T) {
	orig := hookGetAuthenticatorsByUserID
	defer func() { hookGetAuthenticatorsByUserID = orig }()
	hookGetAuthenticatorsByUserID = func(id int64) ([]db.Authenticator, error) {
		return nil, errors.New("db error")
	}
	setupHandlerTest(t)
	uid := newUser(t, "pkregauths@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestPasskeyRegistrationStart_BeginRegError(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test"); err != nil {
		t.Fatalf("InitWebAuthn: %v", err)
	}
	orig := hookBeginRegistration
	defer func() { hookBeginRegistration = orig }()
	hookBeginRegistration = func(u *auth.PasskeyUser) (*protocol.CredentialCreation, string, error) {
		return nil, "", errors.New("webauthn error")
	}
	setupHandlerTest(t)
	uid := newUser(t, "pkregbeg@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/start", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationStart(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- PasskeyRegistrationFinish — post-Parse error paths ---

func TestPasskeyRegistrationFinish_FinishError(t *testing.T) {
	origParse := hookParseCCR
	defer func() { hookParseCCR = origParse }()
	hookParseCCR = func(r *protocol.CredentialCreationResponse) (*protocol.ParsedCredentialCreationData, error) {
		return nil, nil // Parse "succeeds" avec nil response
	}
	origFinish := hookFinishRegistration
	defer func() { hookFinishRegistration = origFinish }()
	hookFinishRegistration = func(u *auth.PasskeyUser, s string, p *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
		return nil, errors.New("finish error")
	}
	setupHandlerTest(t)
	uid := newUser(t, "pkregfin@example.com", "ValidP@ss1!", "USER")

	// "AA" = base64url valide (1 octet 0x00) — json.Decode réussit, hookParseCCR est appelée
	body := bytes.NewBufferString(`{"id":"x","rawId":"AA","type":"public-key","response":{"clientDataJSON":"AA","attestationObject":"AA"}}`)
	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", body), mu(uid, "USER"))
	req.AddCookie(&http.Cookie{Name: "passkey_challenge", Value: "dummyvalue"})
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_CreateAuthError(t *testing.T) {
	origParse := hookParseCCR
	defer func() { hookParseCCR = origParse }()
	hookParseCCR = func(r *protocol.CredentialCreationResponse) (*protocol.ParsedCredentialCreationData, error) {
		return nil, nil
	}
	origFinish := hookFinishRegistration
	defer func() { hookFinishRegistration = origFinish }()
	hookFinishRegistration = func(u *auth.PasskeyUser, s string, p *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
		return &webauthn.Credential{ID: []byte("cred"), PublicKey: []byte("pk")}, nil
	}
	origCreate := hookCreateAuthenticator
	defer func() { hookCreateAuthenticator = origCreate }()
	hookCreateAuthenticator = func(credentialID, publicKey string, counter int, deviceType string, backedUp, backupEligible bool, transports string, userID int64) error {
		return errors.New("db create error")
	}
	setupHandlerTest(t)
	uid := newUser(t, "pkregcreate@example.com", "ValidP@ss1!", "USER")

	body := bytes.NewBufferString(`{"id":"x","rawId":"AA","type":"public-key","response":{"clientDataJSON":"AA","attestationObject":"AA"}}`)
	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", body), mu(uid, "USER"))
	req.AddCookie(&http.Cookie{Name: "passkey_challenge", Value: "dummyvalue"})
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_Success(t *testing.T) {
	origParse := hookParseCCR
	defer func() { hookParseCCR = origParse }()
	hookParseCCR = func(r *protocol.CredentialCreationResponse) (*protocol.ParsedCredentialCreationData, error) {
		return nil, nil
	}
	origFinish := hookFinishRegistration
	defer func() { hookFinishRegistration = origFinish }()
	hookFinishRegistration = func(u *auth.PasskeyUser, s string, p *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
		return &webauthn.Credential{ID: []byte("cred-ok"), PublicKey: []byte("pk-ok")}, nil
	}
	setupHandlerTest(t)
	uid := newUser(t, "pkregok@example.com", "ValidP@ss1!", "USER")

	body := bytes.NewBufferString(`{"id":"x","rawId":"AA","type":"public-key","response":{"clientDataJSON":"AA","attestationObject":"AA"}}`)
	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", body), mu(uid, "USER"))
	req.AddCookie(&http.Cookie{Name: "passkey_challenge", Value: "dummyvalue"})
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (success), got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

// --- PasskeyLoginStart — error path ---

func TestPasskeyLoginStart_BeginLoginError(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test"); err != nil {
		t.Fatalf("InitWebAuthn: %v", err)
	}
	orig := hookBeginLogin
	defer func() { hookBeginLogin = orig }()
	hookBeginLogin = func() (*protocol.CredentialAssertion, string, error) {
		return nil, "", errors.New("webauthn error")
	}

	rr := httptest.NewRecorder()
	PasskeyLoginStart(rr, httptest.NewRequest(http.MethodGet, "/api/passkey/auth/start", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

// --- PasskeyLoginFinish — error paths + success ---

func TestPasskeyLoginFinish_FinishError(t *testing.T) {
	orig := hookFinishLogin
	defer func() { hookFinishLogin = orig }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		return nil, nil, errors.New("auth failed")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestPasskeyLoginFinish_AuthNil(t *testing.T) {
	setupHandlerTest(t)

	orig := hookFinishLogin
	defer func() { hookFinishLogin = orig }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		// appelle h() avec credID inexistant → couvre authenticator==nil dans la closure
		h([]byte("nonexistent-cred-id"), nil) //nolint:errcheck
		return nil, nil, errors.New("not found")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestPasskeyLoginFinish_GetUserError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pklguser@example.com", "ValidP@ss1!", "USER")

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		return &auth.PasskeyUser{ID: uid}, &webauthn.Credential{}, nil
	}
	origGetUser := hookGetUserByID
	defer func() { hookGetUserByID = origGetUser }()
	hookGetUserByID = func(id int64) (*db.User, error) {
		return nil, errors.New("db error")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestPasskeyLoginFinish_GetUserNil(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pklgnil@example.com", "ValidP@ss1!", "USER")

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		return &auth.PasskeyUser{ID: uid}, &webauthn.Credential{}, nil
	}
	origGetUser := hookGetUserByID
	defer func() { hookGetUserByID = origGetUser }()
	hookGetUserByID = func(id int64) (*db.User, error) {
		return nil, nil // user not found but no error
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (user nil), got %d", rr.Code)
	}
}

func TestPasskeyLoginFinish_GenerateTokenError(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pklgtoken@example.com", "ValidP@ss1!", "USER")

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		return &auth.PasskeyUser{ID: uid}, &webauthn.Credential{}, nil
	}
	origToken := hookGenerateToken
	defer func() { hookGenerateToken = origToken }()
	hookGenerateToken = func(userID int64, role, language, currency string, sessionVersion int) (string, error) {
		return "", errors.New("token error")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestPasskeyLoginFinish_Success(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "pklgsucc@example.com", "ValidP@ss1!", "USER")

	// Créer un authenticator dans le test DB
	rawCredID := []byte("testcred-success-12345")
	credIDBase64 := base64.StdEncoding.EncodeToString(rawCredID)
	if err := db.CreateAuthenticator(credIDBase64, "pubkey-test", 0, "multiDevice", false, false, "[]", uid); err != nil {
		t.Fatalf("CreateAuthenticator: %v", err)
	}

	origFinish := hookFinishLogin
	defer func() { hookFinishLogin = origFinish }()
	hookFinishLogin = func(s string, r *http.Request, h func([]byte, []byte) (webauthn.User, error)) (*auth.PasskeyUser, *webauthn.Credential, error) {
		// Appelle la closure userHandler avec un credID valide → couvre le chemin complet de la closure
		user, err := h(rawCredID, nil)
		if err != nil || user == nil {
			return nil, nil, errors.New("userHandler failed")
		}
		return user.(*auth.PasskeyUser), &webauthn.Credential{ID: rawCredID}, nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/passkey/auth/finish", nil)
	req.AddCookie(&http.Cookie{Name: "passkey_auth_challenge", Value: "dummysession"})
	rr := httptest.NewRecorder()
	PasskeyLoginFinish(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}
