package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"despatch/internal/config"
	"despatch/internal/mail"
	"despatch/internal/service"
)

type providerValidateTestClient struct {
	mail.NoopClient
	mailboxes []mail.Mailbox
	listErr   error
}

func (c providerValidateTestClient) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return c.mailboxes, nil
}

func TestV2ListMailProvidersIncludesLiberoCapabilities(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail/providers", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			ID           string `json:"id"`
			Label        string `json:"label"`
			Capabilities struct {
				ForwardingAssisted bool `json:"forwarding_assisted"`
				AppPassword        bool `json:"app_password"`
			} `json:"capabilities"`
		} `json:"items"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode providers payload: %v body=%s", err, rec.Body.String())
	}
	found := false
	for _, item := range payload.Items {
		if item.ID != service.MailProviderTypeLibero {
			continue
		}
		found = true
		if !item.Capabilities.ForwardingAssisted || !item.Capabilities.AppPassword {
			t.Fatalf("unexpected Libero capabilities: %+v", item.Capabilities)
		}
	}
	if !found {
		t.Fatalf("expected Libero provider in payload: %+v", payload.Items)
	}
}

func TestV2ListMailProvidersIncludesGmailCapabilities(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail/providers", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			ID           string `json:"id"`
			Label        string `json:"label"`
			Capabilities struct {
				ForwardingAssisted bool `json:"forwarding_assisted"`
				AppPassword        bool `json:"app_password"`
				OAuth              bool `json:"oauth"`
			} `json:"capabilities"`
			AuthKinds []string `json:"auth_kinds"`
		} `json:"items"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode providers payload: %v body=%s", err, rec.Body.String())
	}
	found := false
	for _, item := range payload.Items {
		if item.ID != service.MailProviderTypeGmail {
			continue
		}
		found = true
		if item.Label != "Gmail / Google Workspace" {
			t.Fatalf("unexpected Gmail label %q", item.Label)
		}
		if !item.Capabilities.ForwardingAssisted || !item.Capabilities.AppPassword || item.Capabilities.OAuth {
			t.Fatalf("unexpected Gmail capabilities: %+v", item.Capabilities)
		}
		if len(item.AuthKinds) != 1 || item.AuthKinds[0] != service.MailAccountAuthKindAppPassword {
			t.Fatalf("unexpected Gmail auth kinds: %+v", item.AuthKinds)
		}
	}
	if !found {
		t.Fatalf("expected Gmail provider in payload: %+v", payload.Items)
	}
}

func TestV2ValidateLiberoProviderReturnsAppPasswordHintOnAuthFailure(t *testing.T) {
	oldFactory := mailClientFactory
	oldProbe := smtpSubmissionProbe
	mailClientFactory = func(cfg config.Config) mail.Client {
		return providerValidateTestClient{listErr: errors.New("authentication failed")}
	}
	smtpSubmissionProbe = func(ctx context.Context, cfg mail.SMTPSubmissionConfig, from string) error {
		return nil
	}
	t.Cleanup(func() {
		mailClientFactory = oldFactory
		smtpSubmissionProbe = oldProbe
	})

	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mail/providers/validate", map[string]any{
		"provider_type": "libero",
		"auth_kind":     "password",
		"login":         "john@libero.it",
		"password":      "secret",
	}, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK         bool `json:"ok"`
		Validation struct {
			AppPasswordRequired bool   `json:"app_password_required"`
			Code                string `json:"code"`
		} `json:"validation"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode validation payload: %v body=%s", err, rec.Body.String())
	}
	if payload.OK {
		t.Fatalf("expected validation to fail, payload=%+v", payload)
	}
	if !payload.Validation.AppPasswordRequired || payload.Validation.Code != "app_password_required" {
		t.Fatalf("expected app password hint, payload=%+v", payload)
	}
}

func TestV2ValidateGmailProviderReturnsAppPasswordHintOnAuthFailure(t *testing.T) {
	oldFactory := mailClientFactory
	oldProbe := smtpSubmissionProbe
	mailClientFactory = func(cfg config.Config) mail.Client {
		return providerValidateTestClient{listErr: errors.New("authentication failed")}
	}
	smtpSubmissionProbe = func(ctx context.Context, cfg mail.SMTPSubmissionConfig, from string) error {
		return nil
	}
	t.Cleanup(func() {
		mailClientFactory = oldFactory
		smtpSubmissionProbe = oldProbe
	})

	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mail/providers/validate", map[string]any{
		"provider_type": "gmail",
		"auth_kind":     "app_password",
		"login":         "john@gmail.com",
		"password":      "secret",
	}, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK         bool `json:"ok"`
		Validation struct {
			AppPasswordRequired bool   `json:"app_password_required"`
			AccountKind         string `json:"account_kind"`
			Code                string `json:"code"`
		} `json:"validation"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode validation payload: %v body=%s", err, rec.Body.String())
	}
	if payload.OK {
		t.Fatalf("expected validation to fail, payload=%+v", payload)
	}
	if !payload.Validation.AppPasswordRequired || payload.Validation.Code != "app_password_required" || payload.Validation.AccountKind != "personal" {
		t.Fatalf("expected Gmail app password hint, payload=%+v", payload)
	}
}

func TestV2ValidateGmailWorkspaceProviderReturnsWorkspaceGuidanceOnAuthFailure(t *testing.T) {
	oldFactory := mailClientFactory
	oldProbe := smtpSubmissionProbe
	mailClientFactory = func(cfg config.Config) mail.Client {
		return providerValidateTestClient{listErr: errors.New("authentication failed")}
	}
	smtpSubmissionProbe = func(ctx context.Context, cfg mail.SMTPSubmissionConfig, from string) error {
		return nil
	}
	t.Cleanup(func() {
		mailClientFactory = oldFactory
		smtpSubmissionProbe = oldProbe
	})

	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mail/providers/validate", map[string]any{
		"provider_type": "gmail",
		"auth_kind":     "app_password",
		"login":         "john@example.com",
		"password":      "secret",
	}, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		OK         bool `json:"ok"`
		Validation struct {
			AppPasswordRequired bool   `json:"app_password_required"`
			AccountKind         string `json:"account_kind"`
			Code                string `json:"code"`
			Message             string `json:"message"`
		} `json:"validation"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode validation payload: %v body=%s", err, rec.Body.String())
	}
	if payload.OK {
		t.Fatalf("expected validation to fail, payload=%+v", payload)
	}
	if payload.Validation.AppPasswordRequired || payload.Validation.Code != "workspace_oauth_or_app_password" || payload.Validation.AccountKind != "workspace" {
		t.Fatalf("expected Workspace guidance, payload=%+v", payload)
	}
	if payload.Validation.Message == "" {
		t.Fatalf("expected Workspace guidance message, payload=%+v", payload)
	}
}

func TestV2CreateLiberoAccountAppliesProviderDefaults(t *testing.T) {
	oldFactory := mailClientFactory
	oldProbe := smtpSubmissionProbe
	mailClientFactory = func(cfg config.Config) mail.Client {
		return providerValidateTestClient{mailboxes: []mail.Mailbox{{Name: "INBOX"}}}
	}
	smtpSubmissionProbe = func(ctx context.Context, cfg mail.SMTPSubmissionConfig, from string) error {
		return nil
	}
	t.Cleanup(func() {
		mailClientFactory = oldFactory
		smtpSubmissionProbe = oldProbe
	})

	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name":  "Libero",
		"provider_type": "libero",
		"auth_kind":     "app_password",
		"login":         "john@libero.it",
		"password":      "secret",
	}, sess, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ProviderType string `json:"provider_type"`
		IMAPHost     string `json:"imap_host"`
		SMTPHost     string `json:"smtp_host"`
		IMAPReady    bool   `json:"imap_ready"`
		SMTPReady    bool   `json:"smtp_ready"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create payload: %v body=%s", err, rec.Body.String())
	}
	if payload.ProviderType != service.MailProviderTypeLibero {
		t.Fatalf("expected provider_type=libero, payload=%+v", payload)
	}
	if payload.IMAPHost != "imapmail.libero.it" || payload.SMTPHost != "smtp.libero.it" {
		t.Fatalf("expected Libero transport defaults, payload=%+v", payload)
	}
	if !payload.IMAPReady || !payload.SMTPReady {
		t.Fatalf("expected validated readiness flags, payload=%+v", payload)
	}
}

func TestV2CreateGmailAccountAppliesProviderDefaults(t *testing.T) {
	oldFactory := mailClientFactory
	oldProbe := smtpSubmissionProbe
	mailClientFactory = func(cfg config.Config) mail.Client {
		return providerValidateTestClient{mailboxes: []mail.Mailbox{{Name: "INBOX"}}}
	}
	smtpSubmissionProbe = func(ctx context.Context, cfg mail.SMTPSubmissionConfig, from string) error {
		return nil
	}
	t.Cleanup(func() {
		mailClientFactory = oldFactory
		smtpSubmissionProbe = oldProbe
	})

	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name":  "Gmail",
		"provider_type": "gmail",
		"auth_kind":     "app_password",
		"login":         "john@gmail.com",
		"password":      "secret",
	}, sess, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ProviderType string `json:"provider_type"`
		AuthKind     string `json:"auth_kind"`
		IMAPHost     string `json:"imap_host"`
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     int    `json:"smtp_port"`
		SMTPTLS      bool   `json:"smtp_tls"`
		SMTPStartTLS bool   `json:"smtp_starttls"`
		IMAPReady    bool   `json:"imap_ready"`
		SMTPReady    bool   `json:"smtp_ready"`
	}
	if err := decodeJSONResponse(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode create payload: %v body=%s", err, rec.Body.String())
	}
	if payload.ProviderType != service.MailProviderTypeGmail || payload.AuthKind != service.MailAccountAuthKindAppPassword {
		t.Fatalf("expected Gmail app-password account, payload=%+v", payload)
	}
	if payload.IMAPHost != "imap.gmail.com" || payload.SMTPHost != "smtp.gmail.com" {
		t.Fatalf("expected Gmail transport defaults, payload=%+v", payload)
	}
	if payload.SMTPPort != 587 || payload.SMTPTLS || !payload.SMTPStartTLS {
		t.Fatalf("expected Gmail SMTP STARTTLS defaults, payload=%+v", payload)
	}
	if !payload.IMAPReady || !payload.SMTPReady {
		t.Fatalf("expected validated readiness flags, payload=%+v", payload)
	}
}

func decodeJSONResponse(body []byte, out any) error {
	return json.Unmarshal(body, out)
}
