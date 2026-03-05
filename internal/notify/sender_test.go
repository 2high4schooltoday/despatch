package notify

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"despatch/internal/config"
)

type smtpTestCapture struct {
	from         string
	rcpt         string
	data         string
	startTLSSeen bool
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

func TestSMTPSenderHonorsStartTLSConfiguration(t *testing.T) {
	capture, host, port, stop := startSMTPTestServer(t, true)
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
	if !strings.Contains(capture.data, "TOKEN-123") {
		t.Fatalf("expected token in message body; got=%q", capture.data)
	}
}

func startSMTPTestServer(t *testing.T, requireStartTLS bool) (*smtpTestCapture, string, int, func()) {
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
					_ = tp.PrintfLine("250-STARTTLS")
					_ = tp.PrintfLine("250 OK")
				} else {
					_ = tp.PrintfLine("250-localhost")
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
			case strings.HasPrefix(upper, "MAIL FROM:"):
				if requireStartTLS && !tlsActive {
					_ = tp.PrintfLine("530 Must issue a STARTTLS command first")
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
