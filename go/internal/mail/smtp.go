package mail

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

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
		return nil // Mail desactive
	}

	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		port, _ = strconv.Atoi(p)
	}

	config = &Config{
		Host:     host,
		Port:     port,
		Username: os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
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

	conn, err := tls.Dial("tcp", addr, tlsConfig)
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

	_, err = w.Write(msg)
	if err != nil {
		return err
	}

	return w.Close()
}

// sanitizeHeader supprime les caractères de contrôle pour prévenir l'injection d'en-têtes
func sanitizeHeader(s string) string {
	// Supprimer CR, LF et autres caractères de contrôle
	result := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\x00' {
			return -1
		}
		return r
	}, s)
	return result
}

func buildMessage(to, subject, body string) []byte {
	// Sanitize headers pour prévenir l'injection
	safeTo := sanitizeHeader(to)
	safeSubject := sanitizeHeader(subject)

	headers := make(map[string]string)
	headers["From"] = config.From
	headers["To"] = safeTo
	headers["Subject"] = safeSubject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(k + ": " + v + "\r\n")
	}
	msg.WriteString("\r\n" + body)

	return []byte(msg.String())
}

// SendPasswordReset envoie un email de reinitialisation de mot de passe
func SendPasswordReset(to, token, host string) error {
	resetURL := fmt.Sprintf("https://%s/reset-password?token=%s", host, token)

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
        <h1>Reinitialisation du mot de passe</h1>
        <p>Vous avez demande a reinitialiser votre mot de passe Pilot Finance.</p>
        <p>Cliquez sur le bouton ci-dessous pour choisir un nouveau mot de passe :</p>
        <a href="%s" class="btn">Reinitialiser mon mot de passe</a>
        <p>Ce lien expire dans 1 heure.</p>
        <p>Si vous n'avez pas demande cette reinitialisation, ignorez cet email.</p>
        <div class="footer">
            <p>Pilot Finance - Votre cockpit financier personnel</p>
        </div>
    </div>
</body>
</html>
`, resetURL)

	return Send(to, "Reinitialisation de votre mot de passe - Pilot Finance", body)
}
