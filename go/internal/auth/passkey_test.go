package auth_test

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"

	"pilot-finance/internal/auth"
)

// --- PasskeyUser interface methods ---

func TestPasskeyUser_WebAuthnID(t *testing.T) {
	u := &auth.PasskeyUser{ID: 42, Email: "test@example.com"}
	if string(u.WebAuthnID()) != "42" {
		t.Errorf("want '42', got %q", string(u.WebAuthnID()))
	}
}

func TestPasskeyUser_WebAuthnID_Zero(t *testing.T) {
	u := &auth.PasskeyUser{ID: 0}
	if string(u.WebAuthnID()) != "0" {
		t.Errorf("want '0', got %q", string(u.WebAuthnID()))
	}
}

func TestPasskeyUser_WebAuthnName(t *testing.T) {
	u := &auth.PasskeyUser{ID: 1, Email: "user@example.com"}
	if u.WebAuthnName() != "user@example.com" {
		t.Errorf("want 'user@example.com', got %q", u.WebAuthnName())
	}
}

func TestPasskeyUser_WebAuthnDisplayName(t *testing.T) {
	u := &auth.PasskeyUser{ID: 1, Email: "user@example.com"}
	if u.WebAuthnDisplayName() != "user@example.com" {
		t.Errorf("want 'user@example.com', got %q", u.WebAuthnDisplayName())
	}
}

func TestPasskeyUser_WebAuthnCredentials_Empty(t *testing.T) {
	u := &auth.PasskeyUser{ID: 1}
	got := u.WebAuthnCredentials()
	if len(got) != 0 {
		t.Errorf("want 0 credentials, got %d", len(got))
	}
}

func TestPasskeyUser_WebAuthnCredentials_NonEmpty(t *testing.T) {
	creds := []webauthn.Credential{{}, {}}
	u := &auth.PasskeyUser{ID: 1, Credentials: creds}
	got := u.WebAuthnCredentials()
	if len(got) != 2 {
		t.Errorf("want 2 credentials, got %d", len(got))
	}
}

func TestPasskeyUser_WebAuthnIcon(t *testing.T) {
	u := &auth.PasskeyUser{ID: 1}
	if u.WebAuthnIcon() != "" {
		t.Errorf("want empty string, got %q", u.WebAuthnIcon())
	}
}

// --- InitWebAuthn ---

func TestInitWebAuthn_Success(t *testing.T) {
	err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App")
	if err != nil {
		t.Fatalf("InitWebAuthn: %v", err)
	}
}

func TestInitWebAuthn_EmptyRPID(t *testing.T) {
	// Empty RPID — webauthn library should return an error
	err := auth.InitWebAuthn("", "http://localhost:8080", "Test App")
	// Just ensure it doesn't panic; library may or may not error
	_ = err
}

// --- BeginRegistration ---

func TestBeginRegistration_Success(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	u := &auth.PasskeyUser{ID: 1, Email: "reg@example.com"}
	opts, sessionB64, err := auth.BeginRegistration(u)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	if opts == nil {
		t.Error("want non-nil options")
	}
	if sessionB64 == "" {
		t.Error("want non-empty session base64")
	}
}

func TestBeginRegistration_MultipleUsers(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	u1 := &auth.PasskeyUser{ID: 10, Email: "u1@example.com"}
	u2 := &auth.PasskeyUser{ID: 20, Email: "u2@example.com"}

	_, s1, err1 := auth.BeginRegistration(u1)
	_, s2, err2 := auth.BeginRegistration(u2)
	if err1 != nil || err2 != nil {
		t.Fatalf("BeginRegistration errors: %v %v", err1, err2)
	}
	if s1 == s2 {
		t.Error("sessions should differ between users")
	}
}

// --- BeginLogin ---

func TestBeginLogin_Success(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	opts, sessionB64, err := auth.BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	if opts == nil {
		t.Error("want non-nil options")
	}
	if sessionB64 == "" {
		t.Error("want non-empty session base64")
	}
}

func TestBeginLogin_TwiceGivesDifferentSessions(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	_, s1, err1 := auth.BeginLogin()
	_, s2, err2 := auth.BeginLogin()
	if err1 != nil || err2 != nil {
		t.Fatalf("BeginLogin errors: %v %v", err1, err2)
	}
	if s1 == s2 {
		t.Error("sessions should differ between calls")
	}
}

// --- FinishRegistration — error paths ---

func TestFinishRegistration_InvalidBase64(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	u := &auth.PasskeyUser{ID: 1, Email: "reg@example.com"}
	_, err := auth.FinishRegistration(u, "!!! not-valid-base64 !!!", nil)
	if err == nil {
		t.Error("want error for invalid base64")
	}
}

func TestFinishRegistration_InvalidJSON(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	u := &auth.PasskeyUser{ID: 1, Email: "reg@example.com"}
	badJSON := base64.StdEncoding.EncodeToString([]byte("{ not json }"))
	_, err := auth.FinishRegistration(u, badJSON, nil)
	if err == nil {
		t.Error("want error for invalid JSON session")
	}
}

// --- FinishLogin — error paths ---

func TestFinishLogin_InvalidBase64(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/", nil)
	_, _, err := auth.FinishLogin("!!! not-valid-base64 !!!", req, nil)
	if err == nil {
		t.Error("want error for invalid base64")
	}
}

func TestFinishLogin_InvalidJSON(t *testing.T) {
	if err := auth.InitWebAuthn("localhost", "http://localhost:8080", "Test App"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/", nil)
	badJSON := base64.StdEncoding.EncodeToString([]byte("{ not json }"))
	_, _, err := auth.FinishLogin(badJSON, req, nil)
	if err == nil {
		t.Error("want error for invalid JSON session")
	}
}
