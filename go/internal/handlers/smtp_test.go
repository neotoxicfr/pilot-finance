package handlers

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
)

// startFakeSMTP lance un serveur SMTP minimal sur un port aléatoire.
// Il gère les connexions jusqu'à la fermeture du listener (t.Cleanup).
func startFakeSMTP(t *testing.T) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startFakeSMTP: listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	addr := ln.Addr().(*net.TCPAddr)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener fermé
			}
			go handleFakeSMTP(conn)
		}
	}()
	return "127.0.0.1", addr.Port
}

// handleFakeSMTP traite une connexion SMTP en répondant au minimum requis
// pour que Go's net/smtp.SendMail réussisse sans TLS ni AUTH.
func handleFakeSMTP(conn net.Conn) {
	defer conn.Close()
	fmt.Fprintf(conn, "220 localhost ESMTP fake\r\n")
	sc := bufio.NewScanner(conn)
	inData := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if inData {
			if line == "." {
				fmt.Fprintf(conn, "250 OK\r\n")
				inData = false
			}
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upper, "MAIL FROM"):
			fmt.Fprintf(conn, "250 OK\r\n")
		case strings.HasPrefix(upper, "RCPT TO"):
			fmt.Fprintf(conn, "250 OK\r\n")
		case upper == "DATA":
			fmt.Fprintf(conn, "354 Start input\r\n")
			inData = true
		case strings.HasPrefix(upper, "QUIT"):
			fmt.Fprintf(conn, "221 Bye\r\n")
			return
		default:
			fmt.Fprintf(conn, "250 OK\r\n")
		}
	}
}

// enableTestSMTP configure mail.Init() vers le serveur fake et restaure l'état original
// à la fin du test. L'ordre LIFO des t.Cleanup est critique :
//  1. mail.Init() est enregistré EN PREMIER → s'exécute EN DERNIER (après restauration des env vars)
//  2. t.Setenv enregistre son propre cleanup EN SECOND → s'exécute EN PREMIER (restaure SMTP_HOST="")
//
// Résultat : quand le test termine, SMTP_HOST est restauré vide, puis mail.Init() voit
// SMTP_HOST="" et remet config=nil, désactivant mail pour les tests suivants.
func enableTestSMTP(t *testing.T) {
	t.Helper()
	smtpHost, smtpPort := startFakeSMTP(t)
	// Enregistrer EN PREMIER — s'exécutera EN DERNIER (après restauration env vars)
	t.Cleanup(func() { mail.Init() })
	// t.Setenv enregistre son cleanup EN SECOND — s'exécutera EN PREMIER
	t.Setenv("SMTP_HOST", smtpHost)
	t.Setenv("SMTP_PORT", strconv.Itoa(smtpPort))
	t.Setenv("SMTP_FROM", "test@pilot.local")
	mail.Init() // activer mail pour ce test
}

// --- ForgotPasswordSubmit avec mail activé ---

func TestForgotPasswordSubmit_MailEnabled_EmptyEmail(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	enableTestSMTP(t)

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {""}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_MailEnabled_UnknownEmail(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	enableTestSMTP(t)

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"nobody@nowhere.example"}}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (pas de fuite info utilisateur), got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_MailEnabled_KnownEmail(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	newUser(t, "resetflow@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"resetflow@example.com"}}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestForgotPasswordSubmit_EmptyLanguageFallback(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	uid := newUser(t, "nolang@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	// Force language to empty string to trigger fallback
	origPrefs := hookUpdateUserPrefs
	defer func() { hookUpdateUserPrefs = origPrefs }()
	db.DB.Exec("UPDATE users SET language='' WHERE id=?", uid) //nolint:errcheck

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"nolang@example.com"}}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_SendEmailError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	newUser(t, "senderr@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	orig := hookSendPasswordReset
	hookSendPasswordReset = func(_, _, _, _ string) error {
		return fmt.Errorf("smtp failure")
	}
	defer func() { hookSendPasswordReset = orig }()

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"senderr@example.com"}}))
	// Should still return 200 (don't reveal email existence)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_RandReadError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	newUser(t, "randerr@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	orig := hookRandRead
	hookRandRead = func(_ []byte) (int, error) {
		return 0, fmt.Errorf("entropy error")
	}
	defer func() { hookRandRead = orig }()

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"randerr@example.com"}}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_SetResetTokenError(t *testing.T) {
	cleanup := setupHandlerTest(t)
	defer cleanup()
	newUser(t, "reseterr@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	orig := hookSetResetToken
	hookSetResetToken = func(_ int64, _ string, _ time.Time) error {
		return fmt.Errorf("db error")
	}
	defer func() { hookSetResetToken = orig }()

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"reseterr@example.com"}}))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}
