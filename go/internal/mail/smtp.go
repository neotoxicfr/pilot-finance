package mail

import (
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"

	cfgpkg "pilot-finance/internal/config"
	"pilot-finance/internal/i18n"
)

// dialTLS et smtpDataWrite sont des variables de fonction pour faciliter les tests.
var dialTLS = func(network, addr string, cfg *tls.Config) (net.Conn, error) {
	return tls.Dial(network, addr, cfg)
}

var smtpDataWrite = func(w io.Writer, msg []byte) (int, error) {
	return w.Write(msg)
}

// smtpDial est une variable de fonction pour faciliter les tests STARTTLS.
var smtpDial = func(addr string) (*smtp.Client, error) {
	return smtp.Dial(addr)
}

// clientStartTLS est une variable de fonction pour faciliter les tests STARTTLS.
var clientStartTLS = func(c *smtp.Client, cfg *tls.Config) error {
	return c.StartTLS(cfg)
}

// Config contient la configuration SMTP
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	Secure   bool
}

var config *Config

// Init initialise la configuration email
func Init() error {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		config = nil // Mail desactive
		return nil
	}

	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil || parsed <= 0 || parsed > 65535 {
			config = nil // Mail désactivé : port invalide
			return fmt.Errorf("mail: SMTP_PORT invalide %q (attendu un entier entre 1 et 65535)", p)
		}
		port = parsed
	}

	config = &Config{
		Host:     host,
		Port:     port,
		Username: os.Getenv("SMTP_USER"),
		Password: cfgpkg.ResolveEnv("SMTP_PASS"),
		From:     os.Getenv("SMTP_FROM"),
		Secure:   os.Getenv("SMTP_SECURE") == "true",
	}

	if config.From == "" {
		config.From = config.Username
	}

	return nil
}

// IsEnabled retourne true si le mail est configure
func IsEnabled() bool {
	return config != nil && config.Host != ""
}

// Send envoie un email
func Send(to, subject, body string) error {
	if !IsEnabled() {
		return fmt.Errorf("mail non configure")
	}

	msg := buildMessage(to, subject, body)

	var auth smtp.Auth
	if config.Username != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, config.Host)
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	if config.Secure || config.Port == 465 {
		return sendTLS(addr, auth, config.From, to, msg)
	}

	// Port 587 : STARTTLS obligatoire (pas de fallback en clair)
	return sendSTARTTLS(addr, auth, config.From, to, msg)
}

func sendTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName: strings.Split(addr, ":")[0],
	}

	conn, err := dialTLS("tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, strings.Split(addr, ":")[0])
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}

	if err := client.Rcpt(to); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}

	_, err = smtpDataWrite(w, msg)
	if err != nil {
		return err
	}

	return w.Close()
}

// sendSTARTTLS envoie un email via STARTTLS (port 587).
// Échoue si le serveur ne supporte pas STARTTLS — pas de fallback en clair.
func sendSTARTTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	client, err := smtpDial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	hostname := strings.Split(addr, ":")[0]
	tlsConfig := &tls.Config{ServerName: hostname}
	if err := clientStartTLS(client, tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS requis mais non supporté: %w", err)
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := smtpDataWrite(w, msg); err != nil {
		return err
	}
	return w.Close()
}

// headerReplacer supprime CR, LF et null pour prévenir l'injection d'en-têtes SMTP.
var headerReplacer = strings.NewReplacer("\r", "", "\n", "", "\x00", "")

func buildMessage(to, subject, body string) []byte {
	fromAddr := mail.Address{Address: config.From}
	toAddr := mail.Address{Address: headerReplacer.Replace(to)}

	var msg strings.Builder
	msg.WriteString("From: " + fromAddr.String() + "\r\n")
	msg.WriteString("To: " + toAddr.String() + "\r\n")
	msg.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", headerReplacer.Replace(subject)) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("\r\n" + body)

	return []byte(msg.String())
}

// SendPasswordReset envoie un email de reinitialisation de mot de passe.
// lang contrôle la langue du template ("fr" ou "en").
func SendPasswordReset(to, token, host, lang string) error {
	resetURL := fmt.Sprintf("https://%s/reset-password?token=%s", host, token)

	t := resetEmailTexts(lang)
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f1f5f9; padding: 40px; }
        .container { max-width: 500px; margin: 0 auto; background: white; border-radius: 16px; padding: 40px; }
        h1 { color: #0f172a; font-size: 24px; margin-bottom: 20px; }
        p { color: #475569; line-height: 1.6; }
        .btn { display: inline-block; background: #3b82f6; color: white; text-decoration: none; padding: 12px 24px; border-radius: 8px; font-weight: 600; margin: 20px 0; }
        .footer { margin-top: 30px; padding-top: 20px; border-top: 1px solid #e2e8f0; font-size: 12px; color: #94a3b8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>%s</h1>
        <p>%s</p>
        <p>%s</p>
        <a href="%s" class="btn">%s</a>
        <p>%s</p>
        <p>%s</p>
        <div class="footer">
            <p>%s</p>
        </div>
    </div>
</body>
</html>
`, t.title, t.intro, t.action, resetURL, t.btn, t.expiry, t.ignore, t.footer)

	return Send(to, t.subject, body)
}

type resetEmailContent struct {
	subject string
	title   string
	intro   string
	action  string
	btn     string
	expiry  string
	ignore  string
	footer  string
}

func resetEmailTexts(lang string) resetEmailContent {
	return resetEmailContent{
		subject: i18n.T(lang, "email.reset_subject"),
		title:   i18n.T(lang, "email.reset_title"),
		intro:   i18n.T(lang, "email.reset_intro"),
		action:  i18n.T(lang, "email.reset_action"),
		btn:     i18n.T(lang, "email.reset_btn"),
		expiry:  i18n.T(lang, "email.reset_expiry"),
		ignore:  i18n.T(lang, "email.reset_ignore"),
		footer:  i18n.T(lang, "email.reset_footer"),
	}
}

// SendVerificationEmail envoie un email de vérification d'adresse.
// lang contrôle la langue du template ("fr" ou "en").
func SendVerificationEmail(to, token, host, lang string) error {
	verifyURL := fmt.Sprintf("https://%s/verify-email?token=%s", host, token)

	t := verifyEmailTexts(lang)
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f1f5f9; padding: 40px; }
        .container { max-width: 500px; margin: 0 auto; background: white; border-radius: 16px; padding: 40px; }
        h1 { color: #0f172a; font-size: 24px; margin-bottom: 20px; }
        p { color: #475569; line-height: 1.6; }
        .btn { display: inline-block; background: #f97316; color: white; text-decoration: none; padding: 12px 24px; border-radius: 8px; font-weight: 600; margin: 20px 0; }
        .footer { margin-top: 30px; padding-top: 20px; border-top: 1px solid #e2e8f0; font-size: 12px; color: #94a3b8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>%s</h1>
        <p>%s</p>
        <p>%s</p>
        <a href="%s" class="btn">%s</a>
        <p>%s</p>
        <div class="footer">
            <p>%s</p>
        </div>
    </div>
</body>
</html>
`, t.title, t.intro, t.action, verifyURL, t.btn, t.ignore, t.footer)

	return Send(to, t.subject, body)
}

type verifyEmailContent struct {
	subject string
	title   string
	intro   string
	action  string
	btn     string
	ignore  string
	footer  string
}

func verifyEmailTexts(lang string) verifyEmailContent {
	return verifyEmailContent{
		subject: i18n.T(lang, "email.verify_subject"),
		title:   i18n.T(lang, "email.verify_title"),
		intro:   i18n.T(lang, "email.verify_intro"),
		action:  i18n.T(lang, "email.verify_action"),
		btn:     i18n.T(lang, "email.verify_btn"),
		ignore:  i18n.T(lang, "email.verify_ignore"),
		footer:  i18n.T(lang, "email.verify_footer"),
	}
}
