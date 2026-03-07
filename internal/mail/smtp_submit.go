package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

var ErrSMTPAuthFailed = errors.New("smtp auth failed")

func WrapSMTPAuthFailed(err error) error {
	if err == nil {
		return ErrSMTPAuthFailed
	}
	return fmt.Errorf("%w: %v", ErrSMTPAuthFailed, err)
}

type SMTPSubmissionConfig struct {
	Host               string
	Port               int
	TLS                bool
	StartTLS           bool
	InsecureSkipVerify bool
	Username           string
	Password           string
	Timeout            time.Duration
}

func SubmitSMTP(ctx context.Context, cfg SMTPSubmissionConfig, from string, rcpt []string, raw []byte) error {
	client, err := connectSMTP(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := smtpMail(client, from); err != nil {
		return err
	}
	for _, r := range rcpt {
		if err := client.Rcpt(strings.TrimSpace(r)); err != nil {
			if IsSMTPSenderPolicyError(err) {
				return WrapSMTPSenderRejected(err)
			}
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

func ProbeSMTPSubmission(ctx context.Context, cfg SMTPSubmissionConfig, from string) error {
	client, err := connectSMTP(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	if strings.TrimSpace(from) == "" {
		return client.Quit()
	}
	if err := smtpMail(client, from); err != nil {
		return err
	}
	_ = client.Reset()
	return client.Quit()
}

func connectSMTP(ctx context.Context, cfg SMTPSubmissionConfig) (*smtp.Client, error) {
	host := strings.TrimSpace(cfg.Host)
	addr := net.JoinHostPort(host, strconv.Itoa(cfg.Port))
	tlsConfig := &tls.Config{ServerName: host, InsecureSkipVerify: cfg.InsecureSkipVerify}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultDialTimeout
	}
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	if cfg.TLS {
		conn = tls.Client(conn, tlsConfig)
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if cfg.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				_ = client.Close()
				return nil, err
			}
		} else {
			_ = client.Close()
			return nil, fmt.Errorf("SMTP STARTTLS extension not available")
		}
	}

	username := strings.TrimSpace(cfg.Username)
	password := cfg.Password
	if username != "" || password != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			_ = client.Close()
			return nil, WrapSMTPAuthFailed(fmt.Errorf("SMTP AUTH extension not available"))
		}
		auth := smtp.PlainAuth("", username, password, host)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			return nil, WrapSMTPAuthFailed(err)
		}
	}
	return client, nil
}

func smtpMail(client *smtp.Client, from string) error {
	if err := client.Mail(strings.TrimSpace(from)); err != nil {
		if IsSMTPSenderPolicyError(err) {
			return WrapSMTPSenderRejected(err)
		}
		return err
	}
	return nil
}
