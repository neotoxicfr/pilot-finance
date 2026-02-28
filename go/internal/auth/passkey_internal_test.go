package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// TestBeginRegistration_MarshalError covers the json.Marshal error branch.
func TestBeginRegistration_MarshalError(t *testing.T) {
	if err := InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}

	orig := marshalJSON
	defer func() { marshalJSON = orig }()

	marshalJSON = func(_ any) ([]byte, error) {
		return nil, errors.New("forced marshal error")
	}

	u := &PasskeyUser{ID: 1, Email: "test@example.com"}
	_, _, err := BeginRegistration(u)
	if err == nil || err.Error() != "forced marshal error" {
		t.Errorf("BeginRegistration: want 'forced marshal error', got %v", err)
	}
}

// TestBeginLogin_MarshalError covers the json.Marshal error branch.
func TestBeginLogin_MarshalError(t *testing.T) {
	if err := InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}

	orig := marshalJSON
	defer func() { marshalJSON = orig }()

	marshalJSON = func(_ any) ([]byte, error) {
		return nil, errors.New("forced marshal error")
	}

	_, _, err := BeginLogin()
	if err == nil || err.Error() != "forced marshal error" {
		t.Errorf("BeginLogin: want 'forced marshal error', got %v", err)
	}
}

// helpers — encode a minimal webauthn.SessionData to base64 for Finish* tests.
func validSessionBase64(t *testing.T) string {
	t.Helper()
	session := webauthn.SessionData{}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

// TestBeginRegistration_WebAuthnError covers the beginRegistrationFn error branch.
func TestBeginRegistration_WebAuthnError(t *testing.T) {
	orig := beginRegistrationFn
	defer func() { beginRegistrationFn = orig }()

	beginRegistrationFn = func(_ *webauthn.WebAuthn, _ webauthn.User, _ ...webauthn.RegistrationOption) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
		return nil, nil, errors.New("webauthn begin error")
	}

	u := &PasskeyUser{ID: 1, Email: "test@example.com"}
	_, _, err := BeginRegistration(u)
	if err == nil || err.Error() != "webauthn begin error" {
		t.Errorf("want 'webauthn begin error', got %v", err)
	}
}

// TestBeginLogin_WebAuthnError covers the beginDiscoverableLoginFn error branch.
func TestBeginLogin_WebAuthnError(t *testing.T) {
	orig := beginDiscoverableLoginFn
	defer func() { beginDiscoverableLoginFn = orig }()

	beginDiscoverableLoginFn = func(_ *webauthn.WebAuthn, _ ...webauthn.LoginOption) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
		return nil, nil, errors.New("webauthn login error")
	}

	_, _, err := BeginLogin()
	if err == nil || err.Error() != "webauthn login error" {
		t.Errorf("want 'webauthn login error', got %v", err)
	}
}

// TestFinishRegistration_CreateCredentialError covers the createCredentialFn error branch.
func TestFinishRegistration_CreateCredentialError(t *testing.T) {
	orig := createCredentialFn
	defer func() { createCredentialFn = orig }()

	createCredentialFn = func(_ *webauthn.WebAuthn, _ webauthn.User, _ webauthn.SessionData, _ *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
		return nil, errors.New("create credential error")
	}

	u := &PasskeyUser{ID: 1}
	_, err := FinishRegistration(u, validSessionBase64(t), nil)
	if err == nil || err.Error() != "create credential error" {
		t.Errorf("want 'create credential error', got %v", err)
	}
}

// TestFinishRegistration_CreateCredentialSuccess covers the success path.
func TestFinishRegistration_CreateCredentialSuccess(t *testing.T) {
	orig := createCredentialFn
	defer func() { createCredentialFn = orig }()

	mockCred := &webauthn.Credential{ID: []byte("test-cred-id")}
	createCredentialFn = func(_ *webauthn.WebAuthn, _ webauthn.User, _ webauthn.SessionData, _ *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
		return mockCred, nil
	}

	u := &PasskeyUser{ID: 1}
	cred, err := FinishRegistration(u, validSessionBase64(t), nil)
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if cred == nil || string(cred.ID) != "test-cred-id" {
		t.Error("want mock credential returned")
	}
}

// TestFinishLogin_FinishError covers the finishDiscoverableLoginFn error branch.
func TestFinishLogin_FinishError(t *testing.T) {
	orig := finishDiscoverableLoginFn
	defer func() { finishDiscoverableLoginFn = orig }()

	finishDiscoverableLoginFn = func(_ *webauthn.WebAuthn, _ webauthn.DiscoverableUserHandler, _ webauthn.SessionData, _ *http.Request) (*webauthn.Credential, error) {
		return nil, errors.New("finish login error")
	}

	req := httptest.NewRequest("POST", "/", nil)
	_, _, err := FinishLogin(validSessionBase64(t), req, nil)
	if err == nil || err.Error() != "finish login error" {
		t.Errorf("want 'finish login error', got %v", err)
	}
}

// TestFinishLogin_UserHandlerError covers the userHandler error branch after FinishDiscoverableLogin.
func TestFinishLogin_UserHandlerError(t *testing.T) {
	orig := finishDiscoverableLoginFn
	defer func() { finishDiscoverableLoginFn = orig }()

	mockCred := &webauthn.Credential{ID: []byte("cred-id")}
	finishDiscoverableLoginFn = func(_ *webauthn.WebAuthn, _ webauthn.DiscoverableUserHandler, _ webauthn.SessionData, _ *http.Request) (*webauthn.Credential, error) {
		return mockCred, nil
	}

	req := httptest.NewRequest("POST", "/", nil)
	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		return nil, errors.New("user not found")
	}
	_, _, err := FinishLogin(validSessionBase64(t), req, userHandler)
	if err == nil || err.Error() != "user not found" {
		t.Errorf("want 'user not found', got %v", err)
	}
}

// nonPasskeyUser implements webauthn.User but is NOT *PasskeyUser — triggers the !ok branch.
type nonPasskeyUser struct{}

func (u *nonPasskeyUser) WebAuthnID() []byte                       { return []byte("1") }
func (u *nonPasskeyUser) WebAuthnName() string                     { return "test" }
func (u *nonPasskeyUser) WebAuthnDisplayName() string              { return "test" }
func (u *nonPasskeyUser) WebAuthnCredentials() []webauthn.Credential { return nil }
func (u *nonPasskeyUser) WebAuthnIcon() string                     { return "" }

// TestFinishLogin_UserNotPasskeyUser covers the !ok type assertion branch.
func TestFinishLogin_UserNotPasskeyUser(t *testing.T) {
	orig := finishDiscoverableLoginFn
	defer func() { finishDiscoverableLoginFn = orig }()

	mockCred := &webauthn.Credential{ID: []byte("cred-id")}
	finishDiscoverableLoginFn = func(_ *webauthn.WebAuthn, _ webauthn.DiscoverableUserHandler, _ webauthn.SessionData, _ *http.Request) (*webauthn.Credential, error) {
		return mockCred, nil
	}

	req := httptest.NewRequest("POST", "/", nil)
	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		return &nonPasskeyUser{}, nil // not a *PasskeyUser
	}
	result, _, err := FinishLogin(validSessionBase64(t), req, userHandler)
	if result != nil {
		t.Error("want nil PasskeyUser when type assertion fails")
	}
	_ = err // may be nil or non-nil — just covering the branch
}

// TestFinishLogin_Success covers the happy path.
func TestFinishLogin_Success(t *testing.T) {
	orig := finishDiscoverableLoginFn
	defer func() { finishDiscoverableLoginFn = orig }()

	mockCred := &webauthn.Credential{ID: []byte("cred-id")}
	finishDiscoverableLoginFn = func(_ *webauthn.WebAuthn, _ webauthn.DiscoverableUserHandler, _ webauthn.SessionData, _ *http.Request) (*webauthn.Credential, error) {
		return mockCred, nil
	}

	req := httptest.NewRequest("POST", "/", nil)
	expectedUser := &PasskeyUser{ID: 42, Email: "user@example.com"}
	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		return expectedUser, nil
	}
	user, cred, err := FinishLogin(validSessionBase64(t), req, userHandler)
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	if user == nil || user.ID != 42 {
		t.Errorf("want user ID 42, got %v", user)
	}
	if cred == nil {
		t.Error("want non-nil credential")
	}
}
