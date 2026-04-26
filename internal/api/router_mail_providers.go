package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/util"
)

var smtpSubmissionProbe = mail.ProbeSMTPSubmission

type mailAccountValidationResult struct {
	IMAPReady           bool      `json:"imap_ready"`
	SMTPReady           bool      `json:"smtp_ready"`
	AuthReady           bool      `json:"auth_ready"`
	AppPasswordRequired bool      `json:"app_password_required"`
	AccountKind         string    `json:"account_kind,omitempty"`
	Message             string    `json:"message,omitempty"`
	Code                string    `json:"code,omitempty"`
	Mailboxes           []string  `json:"mailboxes,omitempty"`
	CheckedAt           time.Time `json:"checked_at"`
}

type mailProviderValidateRequest struct {
	ProviderType   string `json:"provider_type"`
	AuthKind       string `json:"auth_kind"`
	ConnectionMode string `json:"connection_mode"`
	Login          string `json:"login"`
	Password       string `json:"password"`
	IMAPHost       string `json:"imap_host"`
	IMAPPort       int    `json:"imap_port"`
	IMAPTLS        *bool  `json:"imap_tls"`
	IMAPStartTLS   *bool  `json:"imap_starttls"`
	SMTPHost       string `json:"smtp_host"`
	SMTPPort       int    `json:"smtp_port"`
	SMTPTLS        *bool  `json:"smtp_tls"`
	SMTPStartTLS   *bool  `json:"smtp_starttls"`
	DisplayName    string `json:"display_name"`
}

func shouldValidateMailAccount(providerType string, explicit *bool) bool {
	if explicit != nil {
		return *explicit
	}
	switch service.NormalizeMailProviderType(providerType) {
	case service.MailProviderTypeLibero, service.MailProviderTypeGmail:
		return true
	default:
		return false
	}
}

func copyMailAccountTransport(base models.MailAccount, req mailProviderValidateRequest) models.MailAccount {
	account := base
	account.ProviderType = service.NormalizeMailProviderType(firstNonEmptyString(req.ProviderType, account.ProviderType))
	account.AuthKind = service.NormalizeMailAccountAuthKind(firstNonEmptyString(req.AuthKind, account.AuthKind))
	account.ConnectionMode = service.NormalizeMailConnectionMode(firstNonEmptyString(req.ConnectionMode, account.ConnectionMode))
	account.DisplayName = strings.TrimSpace(firstNonEmptyString(req.DisplayName, account.DisplayName))
	account.Login = strings.TrimSpace(firstNonEmptyString(req.Login, account.Login))
	account.IMAPHost = strings.TrimSpace(firstNonEmptyString(req.IMAPHost, account.IMAPHost))
	account.IMAPPort = firstPositive(req.IMAPPort, account.IMAPPort)
	if req.IMAPTLS != nil {
		account.IMAPTLS = *req.IMAPTLS
	}
	if req.IMAPStartTLS != nil {
		account.IMAPStartTLS = *req.IMAPStartTLS
	}
	account.SMTPHost = strings.TrimSpace(firstNonEmptyString(req.SMTPHost, account.SMTPHost))
	account.SMTPPort = firstPositive(req.SMTPPort, account.SMTPPort)
	if req.SMTPTLS != nil {
		account.SMTPTLS = *req.SMTPTLS
	}
	if req.SMTPStartTLS != nil {
		account.SMTPStartTLS = *req.SMTPStartTLS
	}
	service.ApplyMailProviderDefaults(&account)
	return account
}

func looksLikeMailAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, mail.ErrSMTPAuthFailed) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	hints := []string{
		"auth",
		"login failed",
		"invalid credentials",
		"authentication",
		"535",
		"username and password not accepted",
	}
	for _, hint := range hints {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

func pickMailboxNames(items []mail.Mailbox) []string {
	out := make([]string, 0, min(len(items), 8))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func validationFailureForProvider(account models.MailAccount, checkedAt time.Time, err error, code string) (mailAccountValidationResult, error) {
	result := mailAccountValidationResult{
		Code:      code,
		CheckedAt: checkedAt,
		Message:   strings.TrimSpace(err.Error()),
	}
	switch service.NormalizeMailProviderType(account.ProviderType) {
	case service.MailProviderTypeLibero:
		if looksLikeMailAuthError(err) {
			result.AppPasswordRequired = true
			result.Code = "app_password_required"
			result.Message = "Libero may need an app password for this sign-in."
		}
	case service.MailProviderTypeGmail:
		if looksLikeMailAuthError(err) {
			result.AccountKind = service.GmailMailboxKind(account.Login)
			switch result.AccountKind {
			case "workspace":
				result.Code = "workspace_oauth_or_app_password"
				result.Message = "This Google Workspace mailbox may require OAuth or an admin-approved App Password. Google ended basic username/password IMAP and SMTP access for Workspace accounts in 2025."
			default:
				result.AppPasswordRequired = true
				result.Code = "app_password_required"
				result.Message = "Gmail in Despatch uses a Google App Password. Turn on 2-Step Verification, then create a 16-digit App Password for this mailbox."
			}
		}
	}
	return result, err
}

func (h *Handlers) validateMailAccountConnection(ctx context.Context, account models.MailAccount, password string) (mailAccountValidationResult, error) {
	checkedAt := time.Now().UTC()
	if strings.TrimSpace(account.Login) == "" {
		return mailAccountValidationResult{Code: "login_required", Message: "login is required", CheckedAt: checkedAt}, errors.New("login is required")
	}
	if strings.TrimSpace(password) == "" {
		return mailAccountValidationResult{Code: "password_required", Message: "password is required", CheckedAt: checkedAt}, errors.New("password is required")
	}
	cfg := h.cfg
	cfg.IMAPHost = account.IMAPHost
	cfg.IMAPPort = account.IMAPPort
	cfg.IMAPTLS = account.IMAPTLS
	cfg.IMAPStartTLS = account.IMAPStartTLS
	cfg.SMTPHost = account.SMTPHost
	cfg.SMTPPort = account.SMTPPort
	cfg.SMTPTLS = account.SMTPTLS
	cfg.SMTPStartTLS = account.SMTPStartTLS

	client := mailClientFactory(cfg)
	mailboxes, err := client.ListMailboxes(ctx, account.Login, password)
	if err != nil {
		return validationFailureForProvider(account, checkedAt, err, "imap_login_failed")
	}
	result := mailAccountValidationResult{
		IMAPReady: true,
		Mailboxes: pickMailboxNames(mailboxes),
		CheckedAt: checkedAt,
		AccountKind: func() string {
			if service.NormalizeMailProviderType(account.ProviderType) == service.MailProviderTypeGmail {
				return service.GmailMailboxKind(account.Login)
			}
			return ""
		}(),
	}
	smtpErr := smtpSubmissionProbe(ctx, mail.SMTPSubmissionConfig{
		Host:               account.SMTPHost,
		Port:               account.SMTPPort,
		TLS:                account.SMTPTLS,
		StartTLS:           account.SMTPStartTLS,
		InsecureSkipVerify: h.cfg.SMTPInsecureSkipVerify,
		Username:           account.Login,
		Password:           password,
	}, account.Login)
	if smtpErr != nil {
		result.Message = strings.TrimSpace(smtpErr.Error())
		switch service.NormalizeMailProviderType(account.ProviderType) {
		case service.MailProviderTypeLibero:
			if looksLikeMailAuthError(smtpErr) {
				result.AppPasswordRequired = true
				result.Code = "app_password_required"
				result.Message = "Libero may need an app password for SMTP access."
				return result, smtpErr
			}
		case service.MailProviderTypeGmail:
			if looksLikeMailAuthError(smtpErr) {
				result.AccountKind = service.GmailMailboxKind(account.Login)
				switch result.AccountKind {
				case "workspace":
					result.Code = "workspace_oauth_or_app_password"
					result.Message = "This Google Workspace mailbox may require OAuth or an admin-approved App Password. Google ended basic username/password IMAP and SMTP access for Workspace accounts in 2025."
				default:
					result.AppPasswordRequired = true
					result.Code = "app_password_required"
					result.Message = "Gmail in Despatch uses a Google App Password. If this mailbox recently changed its Google password, create a new 16-digit App Password and try again."
				}
				return result, smtpErr
			}
		}
		if result.Code == "" {
			result.Code = "smtp_login_failed"
		}
		return result, smtpErr
	}
	result.SMTPReady = true
	result.AuthReady = true
	result.Code = "connected"
	result.Message = "Connected"
	return result, nil
}

func applyMailAccountValidation(account *models.MailAccount, result mailAccountValidationResult, err error) {
	if account == nil {
		return
	}
	account.ValidationIMAPReady = result.IMAPReady
	account.ValidationSMTPReady = result.SMTPReady
	account.ValidationAppPasswordRequired = result.AppPasswordRequired
	account.LastValidatedAt = result.CheckedAt
	if err != nil {
		account.ValidationError = strings.TrimSpace(result.Message)
		return
	}
	account.ValidationError = ""
}

func (h *Handlers) decorateMailAccount(account models.MailAccount) models.MailAccount {
	service.ApplyMailProviderDefaults(&account)
	if strings.TrimSpace(account.ProviderLabel) == "" {
		account.ProviderLabel = service.MailProviderByID(account.ProviderType).Label
	}
	return account
}

func (h *Handlers) V2ListMailProviders(w http.ResponseWriter, r *http.Request) {
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"items": service.MailProviderCatalog(),
	})
}

func (h *Handlers) V2ValidateMailProvider(w http.ResponseWriter, r *http.Request) {
	var req mailProviderValidateRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	account := copyMailAccountTransport(models.MailAccount{}, req)
	result, err := h.validateMailAccountConnection(r.Context(), account, req.Password)
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":             err == nil,
		"provider_type":  account.ProviderType,
		"provider_label": account.ProviderLabel,
		"capabilities":   account.Capabilities,
		"helper_links":   account.HelperLinks,
		"validation":     result,
	})
}

func updateAccountListForResponse(items []models.MailAccount, decorate func(models.MailAccount) models.MailAccount) []models.MailAccount {
	for i := range items {
		items[i].SecretEnc = ""
		items[i] = decorate(items[i])
	}
	return items
}

func providerValidationErrorCode(account models.MailAccount, result mailAccountValidationResult) string {
	if result.Code != "" {
		return result.Code
	}
	if (service.NormalizeMailProviderType(account.ProviderType) == service.MailProviderTypeLibero ||
		service.NormalizeMailProviderType(account.ProviderType) == service.MailProviderTypeGmail) && result.AppPasswordRequired {
		return "app_password_required"
	}
	return "account_validation_failed"
}
