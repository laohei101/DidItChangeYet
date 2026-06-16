package alert

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/laohei101/diditchangeyet/config"
)

// Email sends alerts via SMTP using the standard library.
type Email struct {
	cfg config.AlertConfig
}

func NewEmail(cfg config.AlertConfig) (*Email, error) {
	if cfg.SMTPHost == "" {
		return nil, fmt.Errorf("email: smtp_host is required")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("email: from address is required")
	}
	if cfg.To == "" {
		return nil, fmt.Errorf("email: to address is required")
	}
	if cfg.SMTPPort == 0 {
		if cfg.TLS {
			cfg.SMTPPort = 465
		} else {
			cfg.SMTPPort = 587
		}
	}
	return &Email{cfg: cfg}, nil
}

func (e *Email) Name() string { return "email" }

func (e *Email) Send(message, watchID, current, previous string) error {
	subject := fmt.Sprintf("[http-watcher] Alert: %s", watchID)
	body := buildMIME(e.cfg.From, e.cfg.To, subject, message)
	addr := fmt.Sprintf("%s:%d", e.cfg.SMTPHost, e.cfg.SMTPPort)

	var auth smtp.Auth
	if e.cfg.Username != "" {
		auth = smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.SMTPHost)
	}

	if e.cfg.TLS {
		return sendTLS(addr, e.cfg.SMTPHost, auth, e.cfg.From, e.cfg.To, body)
	}
	return smtp.SendMail(addr, auth, e.cfg.From, []string{e.cfg.To}, []byte(body))
}

func buildMIME(from, to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("Date: " + time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 +0000") + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}

// sendTLS dials with implicit TLS (port 465 style).
func sendTLS(addr, host string, auth smtp.Auth, from, to string, body string) error {
	tlsCfg := &tls.Config{ServerName: host}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("email: TLS dial: %w", err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("email: SMTP client: %w", err)
	}
	defer c.Quit() //nolint:errcheck

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("email: auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("email: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("email: writing body: %w", err)
	}
	return w.Close()
}
