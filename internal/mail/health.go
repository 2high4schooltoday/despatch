package mail

import (
	"context"
	"crypto/tls"
	"net"
	"strconv"
	"time"

	"despatch/internal/config"
)

func ProbeIMAP(ctx context.Context, cfg config.Config) error {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	addr := net.JoinHostPort(cfg.IMAPHost, strconv.Itoa(cfg.IMAPPort))
	tlsCfg := &tls.Config{ServerName: cfg.IMAPHost, InsecureSkipVerify: cfg.IMAPInsecureSkipVerify}

	if cfg.IMAPTLS {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		_ = conn.Close()
		return nil
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func ProbeSMTP(ctx context.Context, cfg config.Config) error {
	return ProbeSMTPSubmission(ctx, SMTPSubmissionConfig{
		Host:               cfg.SMTPHost,
		Port:               cfg.SMTPPort,
		TLS:                cfg.SMTPTLS,
		StartTLS:           cfg.SMTPStartTLS,
		InsecureSkipVerify: cfg.SMTPInsecureSkipVerify,
		Timeout:            5 * time.Second,
	}, "")
}
