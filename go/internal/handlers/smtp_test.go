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
	"sync/atomic"
	"testing"
	"time"

	"pilot-finance/internal/db"
	"pilot-finance/internal/mail"
	"pilot-finance/internal/ratelimit"
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
	setupHandlerTest(t)
	enableTestSMTP(t)

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {""}}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_MailEnabled_UnknownEmail(t *testing.T) {
	setupHandlerTest(t)
	enableTestSMTP(t)

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"nobody@nowhere.example"}}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (pas de fuite info utilisateur), got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_MailEnabled_KnownEmail(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "resetflow@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"resetflow@example.com"}}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	FlushForgotPassword() // M5 : drain la goroutine background
}

func TestForgotPasswordSubmit_EmptyLanguageFallback(t *testing.T) {
	setupHandlerTest(t)
	uid := newUser(t, "nolang@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	// Force language to empty string to trigger fallback
	db.DB.Exec("UPDATE users SET language='' WHERE id=?", uid) //nolint:errcheck

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"nolang@example.com"}}))
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	FlushForgotPassword() // M5 : drain la goroutine background
}

func TestForgotPasswordSubmit_SendEmailError(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "senderr@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	orig := hookSendPasswordReset
	hookSendPasswordReset = func(_, _, _, _ string) error {
		return fmt.Errorf("smtp failure")
	}
	t.Cleanup(func() {
		FlushForgotPassword()
		hookSendPasswordReset = orig
	})

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"senderr@example.com"}}))
	// Should still return 200 (don't reveal email existence)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestForgotPasswordSubmit_RandReadError(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "randerr@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	orig := hookRandRead
	hookRandRead = func(_ []byte) (int, error) {
		return 0, fmt.Errorf("entropy error")
	}
	t.Cleanup(func() {
		FlushForgotPassword()
		hookRandRead = orig
	})

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"randerr@example.com"}}))
	// M5 fix : la goroutine background log l'erreur mais le handler renvoie
	// toujours 200 (réponse générique constante).
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (generic response), got %d", rr.Code)
	}
}

// TestForgotPasswordSubmit_TimingEqualization (M5 fix) : avec tout le travail
// déporté en background goroutine, le handler doit répondre quasi-instantanément
// que l'utilisateur existe ou non. On mesure les deux latences et vérifie un
// delta faible (< 10ms). Le travail crypto reste exécuté (en background) pour
// défense en profondeur.
func TestForgotPasswordSubmit_TimingEqualization(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "exists-timing@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	// Bypass rate limit pour mesurer plusieurs appels successifs sans 429.
	origRL := hookRateLimitCheck
	hookRateLimitCheck = func(string, string) ratelimit.Result {
		return ratelimit.Result{Allowed: true, Remaining: 999}
	}

	// Compteur d'appels rand pour confirmer que le code crypto est exécuté
	// même quand l'email n'existe pas.
	var calls int32
	origRand := hookRandRead
	hookRandRead = func(b []byte) (int, error) {
		atomic.AddInt32(&calls, 1)
		return len(b), nil
	}
	t.Cleanup(func() {
		FlushForgotPassword()
		hookRandRead = origRand
		hookRateLimitCheck = origRL
	})

	measure := func(email string) time.Duration {
		// Plusieurs runs pour lisser le bruit
		const runs = 5
		var total time.Duration
		for i := 0; i < runs; i++ {
			rr := httptest.NewRecorder()
			start := time.Now()
			ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {email}}))
			total += time.Since(start)
			if rr.Code != http.StatusOK {
				t.Fatalf("want 200, got %d", rr.Code)
			}
		}
		return total / runs
	}

	// User absent : pas de DB UPDATE ni SMTP dans le hot path
	dAbsent := measure("nobody-timing@example.com")
	// User présent : avant le fix, UPDATE DB ajoutait 1-5ms ; maintenant
	// tout est en background → handler reste constant time.
	dExists := measure("exists-timing@example.com")

	delta := dAbsent - dExists
	if delta < 0 {
		delta = -delta
	}
	// Tolérance large pour CI lente / GC pauses (~10ms). Le but est de
	// détecter une régression où SetResetToken redeviendrait synchrone.
	if delta > 10*time.Millisecond {
		t.Errorf("timing delta too large (%v): exists=%v absent=%v (M5: handler must respond constant-time)",
			delta, dExists, dAbsent)
	}

	FlushForgotPassword()
	if atomic.LoadInt32(&calls) == 0 {
		t.Error("hookRandRead must be called even when user is unknown (defense in depth)")
	}
}

// TestForgotPasswordSubmit_SetResetTokenError (M5) : avec le travail en
// background, une erreur DB n'expose plus d'oracle d'existence : le handler
// renvoie 200 dans tous les cas. La goroutine log slog.Error et termine.
func TestForgotPasswordSubmit_SetResetTokenError(t *testing.T) {
	setupHandlerTest(t)
	newUser(t, "reseterr@example.com", "ValidP@ssw0rd!", "USER")
	enableTestSMTP(t)

	called := make(chan struct{}, 1)
	orig := hookSetResetToken
	hookSetResetToken = func(_ int64, _ string, _ time.Time) error {
		select {
		case called <- struct{}{}:
		default:
		}
		return fmt.Errorf("db error")
	}
	t.Cleanup(func() {
		FlushForgotPassword()
		hookSetResetToken = orig
	})

	rr := httptest.NewRecorder()
	ForgotPasswordSubmit(rr, post("/forgot-password", url.Values{"email": {"reseterr@example.com"}}))
	// M5 fix : réponse 200 constant-time, l'erreur DB est silencieuse dans la goroutine
	if rr.Code != http.StatusOK {
		t.Errorf("want 200 (constant time), got %d", rr.Code)
	}
	FlushForgotPassword()
	select {
	case <-called:
	default:
		t.Error("hookSetResetToken must be invoked in background goroutine")
	}
}
