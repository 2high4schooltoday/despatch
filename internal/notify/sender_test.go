package notify

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"despatch/internal/config"
	internalmail "despatch/internal/mail"
)

type smtpTestCapture struct {
	from         string
	rcpt         string
	data         string
	startTLSSeen bool
	authUser     string
}

type smtpTestOptions struct {
	requireStartTLS bool
	requireAuth     bool
	rejectMailFrom  bool
}

func TestNewSenderDefaultsToSMTP(t *testing.T) {
	s := NewSender(config.Config{
		PasswordResetSender: "",
		SMTPHost:            "127.0.0.1",
		SMTPPort:            2525,
	})
	if _, ok := s.(SMTPSender); !ok {
		t.Fatalf("expected SMTPSender default, got %T", s)
	}
}

func TestNewSenderReturnsLogSenderWhenExplicitlyRequested(t *testing.T) {
	s := NewSender(config.Config{
		PasswordResetSender: "log",
	})
	if _, ok := s.(LogSender); !ok {
		t.Fatalf("expected LogSender for explicit log mode, got %T", s)
	}
}

func TestPasswordResetFromAddressStripsMailPrefixFromBaseDomain(t *testing.T) {
	got := PasswordResetFromAddress(config.Config{
		BaseDomain:        "mail.2h4s2d.ru",
		PasswordResetFrom: "",
	})
	if got != "no-reply@2h4s2d.ru" {
		t.Fatalf("expected derived sender from parent domain, got %q", got)
	}
}

func TestSMTPSenderHonorsStartTLSConfiguration(t *testing.T) {
	capture, host, port, stop := startSMTPTestServer(t, smtpTestOptions{requireStartTLS: true})
	defer stop()

	sender := SMTPSender{
		host:               host,
		port:               port,
		from:               "no-reply@example.com",
		startTLS:           true,
		insecureSkipVerify: true,
	}

	if err := sender.SendPasswordReset(context.Background(), "recovery@example.net", "TOKEN-123"); err != nil {
		t.Fatalf("send password reset via smtp: %v", err)
	}
	if !capture.startTLSSeen {
		t.Fatalf("expected STARTTLS handshake to be used")
	}
	if capture.from != "<no-reply@example.com>" {
		t.Fatalf("unexpected MAIL FROM: %q", capture.from)
	}
	if capture.rcpt != "<recovery@example.net>" {
		t.Fatalf("unexpected RCPT TO: %q", capture.rcpt)
	}
	if !strings.Contains(capture.data, "TOKEN-123") ||
		!strings.Contains(capture.data, "Subject: Password Reset Token") ||
		!strings.Contains(capture.data, "From: no-reply@example.com") ||
		!strings.Contains(capture.data, "To: recovery@example.net") ||
		!strings.Contains(capture.data, "Message-ID: <") ||
		!strings.Contains(capture.data, "Date: ") {
		t.Fatalf("expected token in message body; got=%q", capture.data)
	}
}

func TestSMTPSenderAuthenticatesWhenConfigured(t *testing.T) {
	capture, host, port, stop := startSMTPTestServer(t, smtpTestOptions{requireStartTLS: true, requireAuth: true})
	defer stop()

	sender := SMTPSender{
		host:               host,
		port:               port,
		from:               "no-reply@example.com",
		startTLS:           true,
		insecureSkipVerify: true,
		user:               "reset-user",
		pass:               "reset-pass",
	}

	if err := sender.SendPasswordReset(context.Background(), "recovery@example.net", "TOKEN-456"); err != nil {
		t.Fatalf("send password reset with smtp auth: %v", err)
	}
	if capture.authUser != "reset-user" {
		t.Fatalf("expected SMTP AUTH user reset-user, got %q", capture.authUser)
	}
}

func TestSMTPSenderClassifiesSenderPolicyRejection(t *testing.T) {
	_, host, port, stop := startSMTPTestServer(t, smtpTestOptions{rejectMailFrom: true})
	defer stop()

	sender := SMTPSender{
		host: host,
		port: port,
		from: "no-reply@example.com",
	}

	err := sender.SendPasswordReset(context.Background(), "recovery@example.net", "TOKEN-789")
	if !errors.Is(err, internalmail.ErrSMTPSenderRejected) {
		t.Fatalf("expected sender rejection classification, got %v", err)
	}
}

func startSMTPTestServer(t *testing.T, opts smtpTestOptions) (*smtpTestCapture, string, int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp test server: %v", err)
	}

	cert := selfSignedLocalCert(t)
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	capture := &smtpTestCapture{}
	done := make(chan struct{})

	go func() {
		defer close(done)
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

		tp := textproto.NewConn(conn)
		_ = tp.PrintfLine("220 localhost ESMTP test")

		tlsActive := false
		for {
			line, readErr := tp.ReadLine()
			if readErr != nil {
				return
			}
			upper := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				if !tlsActive {
					_ = tp.PrintfLine("250-localhost")
					if opts.requireStartTLS || opts.requireAuth {
						_ = tp.PrintfLine("250-STARTTLS")
					}
					_ = tp.PrintfLine("250 OK")
				} else {
					_ = tp.PrintfLine("250-localhost")
					if opts.requireAuth {
						_ = tp.PrintfLine("250-AUTH PLAIN")
					}
					_ = tp.PrintfLine("250 OK")
				}
			case strings.HasPrefix(upper, "STARTTLS"):
				_ = tp.PrintfLine("220 Ready to start TLS")
				tlsConn := tls.Server(conn, tlsCfg)
				if err := tlsConn.Handshake(); err != nil {
					return
				}
				conn = tlsConn
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				tp = textproto.NewConn(conn)
				tlsActive = true
				capture.startTLSSeen = true
			case strings.HasPrefix(upper, "AUTH PLAIN "):
				if opts.requireAuth && !tlsActive {
					_ = tp.PrintfLine("530 Must issue a STARTTLS command first")
					continue
				}
				encoded := strings.TrimSpace(line[len("AUTH PLAIN "):])
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					_ = tp.PrintfLine("535 5.7.8 Invalid authentication")
					continue
				}
				parts := strings.Split(string(decoded), "\x00")
				if len(parts) >= 3 {
					capture.authUser = parts[1]
				}
				_ = tp.PrintfLine("235 2.7.0 Authentication successful")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				if opts.requireStartTLS && !tlsActive {
					_ = tp.PrintfLine("530 Must issue a STARTTLS command first")
					continue
				}
				if opts.requireAuth && capture.authUser == "" {
					_ = tp.PrintfLine("530 5.7.0 Authentication required")
					continue
				}
				if opts.rejectMailFrom {
					_ = tp.PrintfLine("553 5.7.1 sender rejected")
					continue
				}
				capture.from = strings.TrimSpace(line[len("MAIL FROM:"):])
				_ = tp.PrintfLine("250 2.1.0 Ok")
			case strings.HasPrefix(upper, "RCPT TO:"):
				capture.rcpt = strings.TrimSpace(line[len("RCPT TO:"):])
				_ = tp.PrintfLine("250 2.1.5 Ok")
			case strings.HasPrefix(upper, "DATA"):
				_ = tp.PrintfLine("354 End data with <CR><LF>.<CR><LF>")
				var dataLines []string
				for {
					dataLine, dataErr := tp.ReadLine()
					if dataErr != nil {
						return
					}
					if dataLine == "." {
						break
					}
					dataLines = append(dataLines, dataLine)
				}
				capture.data = strings.Join(dataLines, "\n")
				_ = tp.PrintfLine("250 2.0.0 Ok")
			case strings.HasPrefix(upper, "QUIT"):
				_ = tp.PrintfLine("221 2.0.0 Bye")
				return
			default:
				_ = tp.PrintfLine("250 OK")
			}
		}
	}()

	host, rawPort, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr != nil {
		t.Fatalf("split host port: %v", splitErr)
	}
	port, convErr := strconv.Atoi(rawPort)
	if convErr != nil {
		t.Fatalf("parse smtp test port: %v", convErr)
	}

	stop := func() {
		_ = ln.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for smtp test server to stop")
		}
	}
	return capture, host, port, stop
}

func selfSignedLocalCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("generate certificate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("load x509 key pair: %v", err)
	}
	return cert
}
