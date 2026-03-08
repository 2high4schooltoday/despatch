package notify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"mime"
	"net/mail"
	"strings"
	"time"

	"despatch/internal/config"
	internalmail "despatch/internal/mail"
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
	user               string
	pass               string
}

func PasswordResetFromAddress(cfg config.Config) string {
	candidate := strings.ToLower(strings.TrimSpace(cfg.PasswordResetFrom))
	if candidate == "" || strings.HasSuffix(candidate, "@example.com") {
		domain := strings.ToLower(strings.TrimSpace(cfg.BaseDomain))
		if domain == "" {
			domain = "example.com"
		}
		if strings.HasPrefix(domain, "mail.") && len(domain) > len("mail.") {
			domain = domain[len("mail."):]
		}
		candidate = "no-reply@" + domain
	}
	return candidate
}

func NewSender(cfg config.Config) Sender {
	switch cfg.PasswordResetSender {
	case "log":
		return LogSender{baseURL: cfg.PasswordResetBaseURL}
	case "smtp":
		fallthrough
	case "":
		return SMTPSender{
			host:               cfg.SMTPHost,
			port:               cfg.SMTPPort,
			from:               PasswordResetFromAddress(cfg),
			baseURL:            cfg.PasswordResetBaseURL,
			tls:                cfg.SMTPTLS,
			startTLS:           cfg.SMTPStartTLS,
			insecureSkipVerify: cfg.SMTPInsecureSkipVerify,
			user:               strings.TrimSpace(cfg.PasswordResetSMTPUser),
			pass:               cfg.PasswordResetSMTPPass,
		}
	default:
		return SMTPSender{
			host:               cfg.SMTPHost,
			port:               cfg.SMTPPort,
			from:               PasswordResetFromAddress(cfg),
			baseURL:            cfg.PasswordResetBaseURL,
			tls:                cfg.SMTPTLS,
			startTLS:           cfg.SMTPStartTLS,
			insecureSkipVerify: cfg.SMTPInsecureSkipVerify,
			user:               strings.TrimSpace(cfg.PasswordResetSMTPUser),
			pass:               cfg.PasswordResetSMTPPass,
		}
	}
}

func (s SMTPSender) SendPasswordReset(ctx context.Context, toEmail, token string) error {
	link := strings.TrimRight(s.baseURL, "/")
	if link != "" {
		link = fmt.Sprintf("%s/#/reset?token=%s", link, token)
	}
	raw, err := s.buildRFC822(toEmail, token, link)
	if err != nil {
		return err
	}
	return internalmail.SubmitSMTP(ctx, s.submitConfig(), s.from, []string{toEmail}, raw)
}

func (s SMTPSender) ProbePasswordReset(ctx context.Context) error {
	cfg := s.submitConfig()
	cfg.Timeout = 5 * time.Second
	return internalmail.ProbeSMTPSubmission(ctx, cfg, s.from)
}

func ProbePasswordResetSMTP(ctx context.Context, cfg config.Config) error {
	smtpCfg := passwordResetSMTPConfig(cfg)
	smtpCfg.Timeout = 5 * time.Second
	return internalmail.ProbeSMTPSubmission(ctx, smtpCfg, PasswordResetFromAddress(cfg))
}

func passwordResetSMTPConfig(cfg config.Config) internalmail.SMTPSubmissionConfig {
	return internalmail.SMTPSubmissionConfig{
		Host:               cfg.SMTPHost,
		Port:               cfg.SMTPPort,
		TLS:                cfg.SMTPTLS,
		StartTLS:           cfg.SMTPStartTLS,
		InsecureSkipVerify: cfg.SMTPInsecureSkipVerify,
		Username:           strings.TrimSpace(cfg.PasswordResetSMTPUser),
		Password:           cfg.PasswordResetSMTPPass,
	}
}

func (s SMTPSender) submitConfig() internalmail.SMTPSubmissionConfig {
	return internalmail.SMTPSubmissionConfig{
		Host:               s.host,
		Port:               s.port,
		TLS:                s.tls,
		StartTLS:           s.startTLS,
		InsecureSkipVerify: s.insecureSkipVerify,
		Username:           strings.TrimSpace(s.user),
		Password:           s.pass,
	}
}

func (s SMTPSender) buildRFC822(toEmail, token, link string) ([]byte, error) {
	fromAddr, err := mail.ParseAddress(strings.TrimSpace(s.from))
	if err != nil {
		return nil, err
	}
	toAddr, err := mail.ParseAddress(strings.TrimSpace(toEmail))
	if err != nil {
		return nil, err
	}
	body := "Use this token to reset your Despatch password:\r\n" + token + "\r\n"
	if link != "" {
		body += "\r\nOr open this link:\r\n" + link + "\r\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", encodeAddress(fromAddr))
	fmt.Fprintf(&b, "To: %s\r\n", encodeAddress(toAddr))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: %s\r\n", generateMessageID(fromAddr.Address))
	fmt.Fprintf(&b, "Subject: Password Reset Token\r\n")
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=UTF-8\r\n")
	fmt.Fprintf(&b, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprintf(&b, "\r\n%s", body)
	return []byte(b.String()), nil
}

func encodeAddress(addr *mail.Address) string {
	if addr == nil {
		return ""
	}
	name := strings.TrimSpace(addr.Name)
	if name != "" {
		return mime.QEncoding.Encode("utf-8", name) + " <" + addr.Address + ">"
	}
	return addr.Address
}

func generateMessageID(from string) string {
	domain := "localhost"
	if at := strings.LastIndex(strings.TrimSpace(from), "@"); at >= 0 && at+1 < len(strings.TrimSpace(from)) {
		domain = strings.TrimSpace(from)[at+1:]
	}
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("<%d@%s>", time.Now().UTC().UnixNano(), domain)
	}
	return fmt.Sprintf("<%s@%s>", hex.EncodeToString(buf), domain)
}
