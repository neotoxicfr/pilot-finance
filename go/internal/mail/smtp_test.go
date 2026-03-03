package mail

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"strings"
	"testing"

	"pilot-finance/internal/i18n"
)

func TestMain(m *testing.M) {
	_ = i18n.Load("../../locales")
	os.Exit(m.Run())
}

// --- helpers ---

// smtpLines wraps a net.Conn with line-level helpers.
type smtpLines struct {
	r *bufio.Reader
	w *bufio.Writer
}

func newSMTPLines(conn net.Conn) *smtpLines {
	return &smtpLines{
		r: bufio.NewReader(conn),
		w: bufio.NewWriter(conn),
	}
}

func (s *smtpLines) send(line string) {
	s.w.WriteString(line + "\r\n")
	s.w.Flush()
}

func (s *smtpLines) recv() {
	s.r.ReadString('\n') //nolint:errcheck
}

// minimalSMTPFlow performs a happy-path SMTP exchange (no auth).
// Works for both sendTLS (client closes after data) and smtp.SendMail (sends QUIT).
func minimalSMTPFlow(srv net.Conn) {
	s := newSMTPLines(srv)
	s.send("220 localhost ESMTP")
	s.recv() // EHLO
	s.send("250 localhost")
	s.recv() // MAIL FROM
	s.send("250 OK")
	s.recv() // RCPT TO
	s.send("250 OK")
	s.recv() // DATA
	s.send("354 Start input")
	// Read until "." terminator using s.r (same buffered reader — avoid dual-reader bug)
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		if strings.TrimRight(line, "\r\n") == "." {
			break
		}
	}
	s.send("250 OK")
	// Handle QUIT (smtp.SendMail) or clean close (sendTLS); errors are ignored.
	s.recv()
	s.send("221 Bye")
}

// --- Init ---

func TestInit_NoSMTPHost(t *testing.T) {
	os.Unsetenv("SMTP_HOST")
	if err := Init(); err != nil {
		t.Fatalf("Init without host: %v", err)
	}
	if config != nil {
		t.Error("config should be nil when SMTP_HOST is not set")
	}
}

func TestInit_WithDefaultPort(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	os.Unsetenv("SMTP_PORT")
	t.Setenv("SMTP_USER", "user@example.com")
	t.Setenv("SMTP_PASS", "secret")
	t.Setenv("SMTP_FROM", "from@example.com")
	t.Setenv("SMTP_SECURE", "false")
	defer func() { config = nil }()

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.Port != 587 {
		t.Errorf("default port: want 587, got %d", config.Port)
	}
}

func TestInit_WithCustomPort(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USER", "user@example.com")
	t.Setenv("SMTP_FROM", "from@example.com")
	defer func() { config = nil }()

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if config.Port != 465 {
		t.Errorf("custom port: want 465, got %d", config.Port)
	}
}

func TestInit_FromFallsBackToUsername(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "auto@example.com")
	os.Unsetenv("SMTP_FROM")
	defer func() { config = nil }()

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if config.From != "auto@example.com" {
		t.Errorf("From fallback: want 'auto@example.com', got %q", config.From)
	}
}

func TestInit_FromExplicitlySet(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_USER", "user@example.com")
	t.Setenv("SMTP_FROM", "explicit@example.com")
	defer func() { config = nil }()

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if config.From != "explicit@example.com" {
		t.Errorf("From explicit: want 'explicit@example.com', got %q", config.From)
	}
}

// --- IsEnabled ---

func TestIsEnabled_NilConfig(t *testing.T) {
	config = nil
	if IsEnabled() {
		t.Error("IsEnabled: want false when config is nil")
	}
}

func TestIsEnabled_EmptyHost(t *testing.T) {
	config = &Config{Host: ""}
	defer func() { config = nil }()
	if IsEnabled() {
		t.Error("IsEnabled: want false when Host is empty")
	}
}

func TestIsEnabled_True(t *testing.T) {
	config = &Config{Host: "smtp.example.com"}
	defer func() { config = nil }()
	if !IsEnabled() {
		t.Error("IsEnabled: want true when Host is set")
	}
}

// --- headerReplacer ---

func TestHeaderReplacer_CRLF(t *testing.T) {
	got := headerReplacer.Replace("Subject\r\nX-Injected: evil")
	if strings.Contains(got, "\r") || strings.Contains(got, "\n") {
		t.Errorf("headerReplacer did not remove CRLF: %q", got)
	}
}

func TestHeaderReplacer_NullByte(t *testing.T) {
	got := headerReplacer.Replace("hello\x00world")
	if strings.Contains(got, "\x00") {
		t.Errorf("headerReplacer did not remove null byte: %q", got)
	}
}

func TestHeaderReplacer_Clean(t *testing.T) {
	input := "normal subject"
	if got := headerReplacer.Replace(input); got != input {
		t.Errorf("headerReplacer modified clean string: got %q", got)
	}
}

// --- buildMessage ---

func TestBuildMessage_ContainsHeaders(t *testing.T) {
	config = &Config{From: "from@example.com"}
	defer func() { config = nil }()

	msg := buildMessage("to@example.com", "Test Subject", "<b>body</b>")
	s := string(msg)

	if !strings.Contains(s, "From: <from@example.com>") {
		t.Errorf("message missing From header, got: %s", s)
	}
	if !strings.Contains(s, "To: <to@example.com>") {
		t.Errorf("message missing To header, got: %s", s)
	}
	if !strings.Contains(s, "Subject:") || !strings.Contains(s, "Test Subject") {
		t.Errorf("message missing Subject header, got: %s", s)
	}
	if !strings.Contains(s, "Content-Type: text/html") {
		t.Error("message missing Content-Type header")
	}
	if !strings.Contains(s, "<b>body</b>") {
		t.Error("message missing body")
	}
}

func TestBuildMessage_CRLFInjection(t *testing.T) {
	config = &Config{From: "from@example.com"}
	defer func() { config = nil }()

	msg := buildMessage("to@example.com\r\nBcc: evil@example.com", "Sub\r\nject", "body")
	s := string(msg)
	// CRLF stripped → no new header line can be injected
	if strings.Contains(s, "\r\nBcc:") || strings.Contains(s, "\nBcc:") {
		t.Error("buildMessage should prevent CRLF header injection in To")
	}
	// Subject should not contain raw CRLF either
	for _, line := range strings.Split(s, "\r\n") {
		if strings.HasPrefix(line, "Subject:") {
			if strings.Contains(line, "\n") {
				t.Error("Subject header contains injected newline")
			}
		}
	}
}

// --- Send ---

func TestSend_NotEnabled(t *testing.T) {
	config = nil
	err := Send("to@example.com", "Subject", "Body")
	if err == nil {
		t.Error("Send: want error when mail not configured")
	}
}

func TestSend_PlainPath(t *testing.T) {
	// Plain path now uses sendSTARTTLS; mock smtpDial + clientStartTLS.
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		c, err := smtp.NewClient(cli, "localhost")
		return c, err
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error {
		return nil // no-op: TLS already "upgraded"
	}
	go func() {
		defer srv.Close()
		minimalSMTPFlow(srv)
	}()

	config = &Config{
		Host: "localhost",
		Port: 587,
		From: "from@example.com",
	}
	defer func() { config = nil }()

	if err := Send("to@example.com", "Subject", "Body"); err != nil {
		t.Errorf("Send (plain): %v", err)
	}
}

func TestSend_SecurePath_NoAuth(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		minimalSMTPFlow(srv)
	}()

	config = &Config{
		Host:   "smtp.example.com",
		Port:   587,
		From:   "from@example.com",
		Secure: true,
	}
	defer func() { config = nil }()

	if err := Send("to@example.com", "Subject", "Body"); err != nil {
		t.Errorf("Send (secure, no auth): %v", err)
	}
}

func TestSend_WithUsername_CreatesAuth(t *testing.T) {
	// Tests that config.Username != "" creates an smtp.Auth object.
	// We use sendTLS via Secure=true with a fake server that accepts PLAIN auth.
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250-localhost")
		s.send("250 AUTH PLAIN")
		s.recv() // AUTH PLAIN ...
		s.send("235 Authentication successful")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("250 OK")
		s.recv() // DATA
		s.send("354 Start input")
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimRight(line, "\r\n") == "." {
				break
			}
		}
		s.send("250 OK")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	config = &Config{
		Host:     "localhost",
		Port:     465,
		From:     "from@example.com",
		Username: "user",
		Password: "pass",
		Secure:   true,
	}
	defer func() { config = nil }()

	if err := Send("to@example.com", "Subject", "Body"); err != nil {
		t.Errorf("Send (with auth): %v", err)
	}
}

// --- sendTLS ---

func TestSendTLS_DialError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return nil, errors.New("dial failed")
	}

	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil || err.Error() != "dial failed" {
		t.Errorf("want 'dial failed', got %v", err)
	}
}

func TestSendTLS_NewClientError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		// Send a non-220 greeting → smtp.NewClient fails
		srv.Write([]byte("500 Not ready\r\n")) //nolint:errcheck
		io.Copy(io.Discard, srv)              //nolint:errcheck
	}()

	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want error from smtp.NewClient, got nil")
	}
}

func TestSendTLS_AuthError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250-localhost")
		s.send("250 AUTH PLAIN")
		s.recv() // AUTH PLAIN ...
		s.send("535 Authentication failed")
		// After 535, smtp.Client sends "*\r\n" abort and expects 501.
		// Respond to unblock the client, then drain.
		s.recv()                 // "*" abort
		s.send("501 Aborted")   // unblock c.cmd(501, "*")
		s.recv()                 // QUIT — Go 1.26: smtp.Client.Auth calls c.Quit() after abort
		s.send("221 Bye")       // unblock c.Quit()
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	auth := smtp.PlainAuth("", "user", "pass", "localhost")
	err := sendTLS("localhost:465", auth, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want auth error, got nil")
	}
}

func TestSendTLS_MailError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("550 Rejected")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want MAIL FROM error, got nil")
	}
}

func TestSendTLS_RcptError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("550 No such user")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want RCPT TO error, got nil")
	}
}

func TestSendTLS_DataCommandError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("250 OK")
		s.recv() // DATA
		s.send("550 DATA rejected")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want DATA command error, got nil")
	}
}

func TestSendTLS_WriteError(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()
	origW := smtpDataWrite
	defer func() { smtpDataWrite = origW }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("250 OK")
		s.recv() // DATA
		s.send("354 Start input")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	smtpDataWrite = func(_ io.Writer, _ []byte) (int, error) {
		return 0, fmt.Errorf("write failed")
	}

	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil || err.Error() != "write failed" {
		t.Errorf("want 'write failed', got %v", err)
	}
}

func TestSendTLS_Success(t *testing.T) {
	orig := dialTLS
	defer func() { dialTLS = orig }()

	srv, cli := net.Pipe()
	dialTLS = func(_, _ string, _ *tls.Config) (net.Conn, error) {
		return cli, nil
	}
	go func() {
		defer srv.Close()
		minimalSMTPFlow(srv)
	}()

	msg := []byte("Subject: test\r\n\r\nHello!")
	err := sendTLS("localhost:465", nil, "from@example.com", "to@example.com", msg)
	if err != nil {
		t.Errorf("sendTLS success: %v", err)
	}
}

// TestSendTLS_DefaultDialTLS couvre smtp.go:15-17 — corps de dialTLS par défaut.
// Sans injection, tls.Dial est appelé sur un port inexistant → connexion refusée, return exécuté.
func TestSendTLS_DefaultDialTLS(t *testing.T) {
	err := sendTLS("127.0.0.1:19999", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want connection error from real tls.Dial on non-existent port")
	}
}

// --- sendSTARTTLS ---

func TestSendSTARTTLS_DialError(t *testing.T) {
	origDial := smtpDial
	defer func() { smtpDial = origDial }()

	smtpDial = func(_ string) (*smtp.Client, error) {
		return nil, errors.New("dial failed")
	}

	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil || err.Error() != "dial failed" {
		t.Errorf("want 'dial failed', got %v", err)
	}
}

func TestSendSTARTTLS_StartTLSError(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error {
		return errors.New("TLS not supported")
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS requis") {
		t.Errorf("want STARTTLS error, got %v", err)
	}
}

func TestSendSTARTTLS_AuthError(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250-localhost")
		s.send("250 AUTH PLAIN")
		s.recv() // AUTH PLAIN
		s.send("535 Authentication failed")
		s.recv()                 // "*" abort
		s.send("501 Aborted")   // unblock abort
		s.recv()                 // QUIT
		s.send("221 Bye")       // unblock Quit
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	auth := smtp.PlainAuth("", "user", "pass", "localhost")
	err := sendSTARTTLS("localhost:587", auth, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want auth error, got nil")
	}
}

func TestSendSTARTTLS_MailError(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("550 Rejected")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want MAIL FROM error, got nil")
	}
}

func TestSendSTARTTLS_RcptError(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("550 No such user")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want RCPT TO error, got nil")
	}
}

func TestSendSTARTTLS_DataCommandError(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("250 OK")
		s.recv() // DATA
		s.send("550 DATA rejected")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want DATA command error, got nil")
	}
}

func TestSendSTARTTLS_WriteError(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	origW := smtpDataWrite
	defer func() { smtpDial = origDial; clientStartTLS = origTLS; smtpDataWrite = origW }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	smtpDataWrite = func(_ io.Writer, _ []byte) (int, error) {
		return 0, fmt.Errorf("write failed")
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250 localhost")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("250 OK")
		s.recv() // DATA
		s.send("354 Start input")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil || err.Error() != "write failed" {
		t.Errorf("want 'write failed', got %v", err)
	}
}

func TestSendSTARTTLS_Success(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	go func() {
		defer srv.Close()
		minimalSMTPFlow(srv)
	}()

	msg := []byte("Subject: test\r\n\r\nHello!")
	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", msg)
	if err != nil {
		t.Errorf("sendSTARTTLS success: %v", err)
	}
}

func TestSendSTARTTLS_WithAuth_Success(t *testing.T) {
	origDial := smtpDial
	origTLS := clientStartTLS
	defer func() { smtpDial = origDial; clientStartTLS = origTLS }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250-localhost")
		s.send("250 AUTH PLAIN")
		s.recv() // AUTH PLAIN ...
		s.send("235 Authentication successful")
		s.recv() // MAIL FROM
		s.send("250 OK")
		s.recv() // RCPT TO
		s.send("250 OK")
		s.recv() // DATA
		s.send("354 Start input")
		for {
			line, err := s.r.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimRight(line, "\r\n") == "." {
				break
			}
		}
		s.send("250 OK")
		io.Copy(io.Discard, srv) //nolint:errcheck
	}()

	auth := smtp.PlainAuth("", "user", "pass", "localhost")
	err := sendSTARTTLS("localhost:587", auth, "from@example.com", "to@example.com", []byte("msg"))
	if err != nil {
		t.Errorf("sendSTARTTLS with auth: %v", err)
	}
}

// TestSendSTARTTLS_DefaultSmtpDial couvre smtp.go — corps de smtpDial par défaut.
func TestSendSTARTTLS_DefaultSmtpDial(t *testing.T) {
	origTLS := clientStartTLS
	defer func() { clientStartTLS = origTLS }()
	clientStartTLS = func(_ *smtp.Client, _ *tls.Config) error { return nil }

	err := sendSTARTTLS("127.0.0.1:19998", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want connection error from real smtp.Dial on non-existent port")
	}
}

// TestSendSTARTTLS_DefaultClientStartTLS couvre smtp.go — corps de clientStartTLS par défaut.
func TestSendSTARTTLS_DefaultClientStartTLS(t *testing.T) {
	origDial := smtpDial
	defer func() { smtpDial = origDial }()

	srv, cli := net.Pipe()
	smtpDial = func(_ string) (*smtp.Client, error) {
		return smtp.NewClient(cli, "localhost")
	}
	go func() {
		defer srv.Close()
		s := newSMTPLines(srv)
		s.send("220 localhost ESMTP")
		s.recv() // EHLO
		s.send("250-localhost")
		s.send("250 STARTTLS")
		s.recv() // STARTTLS command
		s.send("220 Ready to start TLS")
		// Close immediately — TLS handshake will fail with EOF
	}()

	// Uses the default clientStartTLS → client.StartTLS which fails because
	// server closes after faking 220 (no real TLS handshake)
	err := sendSTARTTLS("localhost:587", nil, "from@example.com", "to@example.com", []byte("msg"))
	if err == nil {
		t.Error("want STARTTLS error from real client.StartTLS on plain connection")
	}
}

// --- SendPasswordReset ---

func TestSendPasswordReset_MailDisabled(t *testing.T) {
	config = nil
	err := SendPasswordReset("to@example.com", "mytoken", "example.com", "fr")
	if err == nil {
		t.Error("SendPasswordReset: want error when mail disabled")
	}
}

func TestSendPasswordReset_EnglishTemplate(t *testing.T) {
	config = nil
	err := SendPasswordReset("to@example.com", "mytoken", "example.com", "en")
	if err == nil {
		t.Error("SendPasswordReset: want error when mail disabled")
	}
}

// --- resetEmailTexts ---

func TestResetEmailTexts_French(t *testing.T) {
	txt := resetEmailTexts("fr")
	if txt.subject == "" || txt.title == "" {
		t.Error("French email texts should not be empty")
	}
}

func TestResetEmailTexts_English(t *testing.T) {
	txt := resetEmailTexts("en")
	if txt.subject == "" || txt.title == "" {
		t.Error("English email texts should not be empty")
	}
	if txt.subject != "Password Reset - Pilot Finance" {
		t.Errorf("English subject: want 'Password Reset - Pilot Finance', got %q", txt.subject)
	}
}
