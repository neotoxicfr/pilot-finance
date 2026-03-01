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
)

// dialTLS et smtpDataWrite sont des variables de fonction pour faciliter les tests.
var dialTLS = func(network, addr string, cfg *tls.Config) (net.Conn, error) {
	return tls.Dial(network, addr, cfg)
}

var smtpDataWrite = func(w io.Writer, msg []byte) (int, error) {
	return w.Write(msg)
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
		port, _ = strconv.Atoi(p)
	}

	config = &Config{
		Host:     host,
		Port:     port,
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASS"),
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

	return smtp.SendMail(addr, auth, config.From, []string{to}, msg)
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
	if lang == "en" {
		return resetEmailContent{
			subject: "Password Reset - Pilot Finance",
			title:   "Password Reset",
			intro:   "You have requested to reset your Pilot Finance password.",
			action:  "Click the button below to choose a new password:",
			btn:     "Reset my password",
			expiry:  "This link expires in 1 hour.",
			ignore:  "If you did not request this reset, please ignore this email.",
			footer:  "Pilot Finance - Your personal financial cockpit",
		}
	}
	return resetEmailContent{
		subject: "Reinitialisation de votre mot de passe - Pilot Finance",
		title:   "Reinitialisation du mot de passe",
		intro:   "Vous avez demande a reinitialiser votre mot de passe Pilot Finance.",
		action:  "Cliquez sur le bouton ci-dessous pour choisir un nouveau mot de passe :",
		btn:     "Reinitialiser mon mot de passe",
		expiry:  "Ce lien expire dans 1 heure.",
		ignore:  "Si vous n'avez pas demande cette reinitialisation, ignorez cet email.",
		footer:  "Pilot Finance - Votre cockpit financier personnel",
	}
}
