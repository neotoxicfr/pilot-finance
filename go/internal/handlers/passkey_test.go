package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"pilot-finance/internal/auth"
	"pilot-finance/internal/db"
)

// --- PasskeyRegistrationStart ---

func TestPasskeyRegistrationStart_NilUser(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()

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
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()

	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_NoCookie(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "pkregnoc@example.com", "ValidP@ss1!", "USER")

	req := injectUser(httptest.NewRequest(http.MethodPost, "/api/passkey/register/finish", nil), mu(uid, "USER"))
	rr := httptest.NewRecorder()
	PasskeyRegistrationFinish(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 (no cookie), got %d", rr.Code)
	}
}

func TestPasskeyRegistrationFinish_InvalidJSON(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()

	req := withParam(httptest.NewRequest(http.MethodDelete, "/api/passkey/1", nil), "id", "1")
	rr := httptest.NewRecorder()
	DeletePasskey(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestDeletePasskey_InvalidID(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()

	req := withParam(httptest.NewRequest(http.MethodPatch, "/api/passkey/1/rename", nil), "id", "1")
	rr := httptest.NewRecorder()
	RenamePasskey(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestRenamePasskey_InvalidID(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
	cleanup := setupHandlerTest(t)
	defer cleanup()
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
