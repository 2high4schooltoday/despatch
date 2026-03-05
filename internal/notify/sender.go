package notify

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"despatch/internal/config"
)

type Sender interface {
	SendPasswordReset(ctx context.Context, toEmail, token string) error
}

type LogSender struct {
	baseURL string
}

func (s LogSender) SendPasswordReset(ctx context.Context, toEmail, token string) error {
	_ = ctx
	link := strings.TrimRight(s.baseURL, "/")
	if link != "" {
		link = fmt.Sprintf("%s/#/reset?token=%s", link, token)
	}
	sum := sha256.Sum256([]byte(token))
	prefix := hex.EncodeToString(sum[:])[:16]
	if link != "" {
		log.Printf("password reset generated for %s token_hash_prefix=%s", toEmail, prefix)
		return nil
	}
	log.Printf("password reset generated for %s token_hash_prefix=%s", toEmail, prefix)
	return nil
}

type SMTPSender struct {
	host               string
	port               int
	from               string
	baseURL            string
	tls                bool
	startTLS           bool
	insecureSkipVerify bool
}

func PasswordResetFromAddress(cfg config.Config) string {
	candidate := strings.ToLower(strings.TrimSpace(cfg.PasswordResetFrom))
	if candidate == "" || strings.HasSuffix(candidate, "@example.com") {
		domain := strings.ToLower(strings.TrimSpace(cfg.BaseDomain))
		if domain == "" {
			domain = "example.com"
		}
		candidate = "no-reply@" + domain
	}
	return candidate
}

func NewSender(cfg config.Config) Sender {
	switch cfg.PasswordResetSender {
	case "smtp":
		return SMTPSender{
			host:               cfg.SMTPHost,
			port:               cfg.SMTPPort,
			from:               PasswordResetFromAddress(cfg),
			baseURL:            cfg.PasswordResetBaseURL,
			tls:                cfg.SMTPTLS,
			startTLS:           cfg.SMTPStartTLS,
			insecureSkipVerify: cfg.SMTPInsecureSkipVerify,
		}
	default:
		return LogSender{baseURL: cfg.PasswordResetBaseURL}
	}
}

func (s SMTPSender) SendPasswordReset(ctx context.Context, toEmail, token string) error {
	link := strings.TrimRight(s.baseURL, "/")
	if link != "" {
		link = fmt.Sprintf("%s/#/reset?token=%s", link, token)
	}
	body := "Subject: Password Reset Token\r\n\r\nUse this token to reset your password:\r\n" + token + "\r\n"
	if link != "" {
		body += "\r\nOr open this link:\r\n" + link + "\r\n"
	}
	return s.sendSMTP(ctx, []string{toEmail}, []byte(body))
}

func (s SMTPSender) sendSMTP(ctx context.Context, rcpt []string, raw []byte) error {
	addr := net.JoinHostPort(strings.TrimSpace(s.host), strconv.Itoa(s.port))
	tlsConfig := &tls.Config{
		ServerName:         strings.TrimSpace(s.host),
		InsecureSkipVerify: s.insecureSkipVerify,
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	if s.tls {
		conn = tls.Client(conn, tlsConfig)
	}

	client, err := smtp.NewClient(conn, strings.TrimSpace(s.host))
	if err != nil {
		return err
	}
	defer client.Close()

	if s.startTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("SMTP STARTTLS extension not available")
		}
	}

	if err := client.Mail(s.from); err != nil {
		return err
	}
	for _, r := range rcpt {
		if err := client.Rcpt(strings.TrimSpace(r)); err != nil {
			return err
		}
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(raw); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return client.Quit()
}
