package service

import (
	"strings"

	"despatch/internal/models"
)

const (
	MailProviderTypeGeneric = "generic"
	MailProviderTypeGmail   = "gmail"
	MailProviderTypeLibero  = "libero"

	MailAccountAuthKindPassword    = "password"
	MailAccountAuthKindAppPassword = "app_password"

	MailConnectionModeIMAPSMTP = "imap_smtp"
)

func NormalizeMailProviderType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case MailProviderTypeGmail:
		return MailProviderTypeGmail
	case MailProviderTypeLibero:
		return MailProviderTypeLibero
	default:
		return MailProviderTypeGeneric
	}
}

func NormalizeMailAccountAuthKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case MailAccountAuthKindAppPassword:
		return MailAccountAuthKindAppPassword
	default:
		return MailAccountAuthKindPassword
	}
}

func NormalizeMailConnectionMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case MailConnectionModeIMAPSMTP:
		return MailConnectionModeIMAPSMTP
	default:
		return MailConnectionModeIMAPSMTP
	}
}

func MailProviderCatalog() []models.MailProvider {
	return []models.MailProvider{
		{
			ID:             MailProviderTypeGmail,
			Label:          "Gmail / Google Workspace",
			SourceKind:     "external",
			AuthKinds:      []string{MailAccountAuthKindAppPassword},
			ConnectionMode: MailConnectionModeIMAPSMTP,
			Defaults: models.MailProviderDefaults{
				IMAPHost:     "imap.gmail.com",
				IMAPPort:     993,
				IMAPSecurity: "ssl",
				SMTPHost:     "smtp.gmail.com",
				SMTPPort:     587,
				SMTPSecurity: "starttls",
			},
			Capabilities: models.MailProviderCapabilities{
				ReceiveMail:         true,
				SendMail:            true,
				RemoteFolders:       true,
				ServerRulesAPI:      false,
				ServerRulesAssisted: true,
				ForwardingAPI:       false,
				ForwardingAssisted:  true,
				BulkAccountImport:   true,
				ReplyFunnels:        true,
				OAuth:               false,
				AppPassword:         true,
				MultiaccountRead:    true,
				MultiaccountWrite:   true,
				ProviderStatusProbe: "partial",
			},
			HelperLinks: map[string]string{
				"imap_smtp":            "https://support.google.com/mail/answer/7126229?hl=en",
				"imap_settings":        "https://support.google.com/mail/answer/78892?hl=en",
				"app_password":         "https://support.google.com/mail/answer/185833?hl=en",
				"forwarding":           "https://support.google.com/mail/answer/10957?hl=en",
				"filters":              "https://support.google.com/mail/answer/6579?hl=en",
				"workspace_forwarding": "https://support.google.com/a/answer/14724207?hl=en",
				"workspace_oauth":      "https://support.google.com/a/answer/14114704?hl=en",
				"client_guidance":      "https://support.google.com/mail/answer/7126229?hl=en",
			},
		},
		{
			ID:             MailProviderTypeLibero,
			Label:          "Libero Mail",
			SourceKind:     "external",
			AuthKinds:      []string{MailAccountAuthKindPassword, MailAccountAuthKindAppPassword},
			ConnectionMode: MailConnectionModeIMAPSMTP,
			Defaults: models.MailProviderDefaults{
				IMAPHost:     "imapmail.libero.it",
				IMAPPort:     993,
				IMAPSecurity: "ssl",
				SMTPHost:     "smtp.libero.it",
				SMTPPort:     465,
				SMTPSecurity: "ssl",
			},
			Capabilities: models.MailProviderCapabilities{
				ReceiveMail:         true,
				SendMail:            true,
				RemoteFolders:       true,
				ServerRulesAPI:      false,
				ServerRulesAssisted: true,
				ForwardingAPI:       false,
				ForwardingAssisted:  true,
				BulkAccountImport:   true,
				ReplyFunnels:        true,
				OAuth:               false,
				AppPassword:         true,
				MultiaccountRead:    true,
				MultiaccountWrite:   true,
				ProviderStatusProbe: "partial",
			},
			HelperLinks: map[string]string{
				"imap_smtp":       "https://aiuto.libero.it/articolo/mail/configurare-libero-mail-con-client-di-posta-imap-e-smtp/",
				"app_password":    "https://aiuto.libero.it/articolo/tutorial/password-specifica-per-app/",
				"filters":         "https://aiuto.libero.it/articolo/mail/gestione-dei-filtri/",
				"forwarding":      "https://aiuto.libero.it/articolo/mail/le-notifiche-di-avviso-in-libero-mail/",
				"forwarding_plus": "https://aiuto.libero.it/articolo/mail-plus/come-impostare-inoltro-automatico-in-mail-plus/",
				"multiaccount":    "https://liberomail.libero.it/funzionalita/multiaccount.php",
				"client_guidance": "https://info.libero.it/contratti/libero-mail-app/",
			},
		},
		{
			ID:             MailProviderTypeGeneric,
			Label:          "Generic IMAP/SMTP",
			SourceKind:     "external",
			AuthKinds:      []string{MailAccountAuthKindPassword},
			ConnectionMode: MailConnectionModeIMAPSMTP,
			Capabilities: models.MailProviderCapabilities{
				ReceiveMail:         true,
				SendMail:            true,
				RemoteFolders:       true,
				ServerRulesAPI:      false,
				ServerRulesAssisted: false,
				ForwardingAPI:       false,
				ForwardingAssisted:  false,
				BulkAccountImport:   false,
				ReplyFunnels:        true,
				OAuth:               false,
				AppPassword:         false,
				MultiaccountRead:    true,
				MultiaccountWrite:   true,
				ProviderStatusProbe: "partial",
			},
		},
	}
}

func MailProviderByID(raw string) models.MailProvider {
	id := NormalizeMailProviderType(raw)
	for _, item := range MailProviderCatalog() {
		if item.ID == id {
			return item
		}
	}
	return MailProviderCatalog()[len(MailProviderCatalog())-1]
}

func ApplyMailProviderDefaults(account *models.MailAccount) {
	if account == nil {
		return
	}
	provider := MailProviderByID(account.ProviderType)
	account.ProviderType = provider.ID
	account.ProviderLabel = provider.Label
	account.AuthKind = NormalizeMailAccountAuthKind(account.AuthKind)
	if len(provider.AuthKinds) > 0 && !MailProviderSupportsAuthKind(provider.ID, account.AuthKind) {
		account.AuthKind = NormalizeMailAccountAuthKind(provider.AuthKinds[0])
	}
	account.ConnectionMode = NormalizeMailConnectionMode(account.ConnectionMode)
	account.Capabilities = provider.Capabilities
	account.HelperLinks = provider.HelperLinks
	if account.ConnectionMode == "" {
		account.ConnectionMode = provider.ConnectionMode
	}
	if strings.TrimSpace(account.IMAPHost) == "" {
		account.IMAPHost = strings.TrimSpace(provider.Defaults.IMAPHost)
	}
	if account.IMAPPort <= 0 {
		account.IMAPPort = provider.Defaults.IMAPPort
	}
	if strings.TrimSpace(account.SMTPHost) == "" {
		account.SMTPHost = strings.TrimSpace(provider.Defaults.SMTPHost)
	}
	if account.SMTPPort <= 0 {
		account.SMTPPort = provider.Defaults.SMTPPort
	}
	switch provider.Defaults.IMAPSecurity {
	case "ssl":
		account.IMAPTLS = true
		account.IMAPStartTLS = false
	case "starttls":
		account.IMAPTLS = false
		account.IMAPStartTLS = true
	}
	switch provider.Defaults.SMTPSecurity {
	case "ssl":
		account.SMTPTLS = true
		account.SMTPStartTLS = false
	case "starttls":
		account.SMTPTLS = false
		account.SMTPStartTLS = true
	}
}

func MailProviderSupportsAssistedForwarding(providerType string) bool {
	return MailProviderByID(providerType).Capabilities.ForwardingAssisted
}

func MailProviderSupportsAuthKind(providerType, authKind string) bool {
	provider := MailProviderByID(providerType)
	normalized := NormalizeMailAccountAuthKind(authKind)
	if len(provider.AuthKinds) == 0 {
		return normalized == MailAccountAuthKindPassword
	}
	for _, candidate := range provider.AuthKinds {
		if NormalizeMailAccountAuthKind(candidate) == normalized {
			return true
		}
	}
	return false
}

func GmailMailboxKind(login string) string {
	login = strings.TrimSpace(login)
	at := strings.LastIndex(login, "@")
	if at < 0 || at >= len(login)-1 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(login[at+1:])) {
	case "gmail.com", "googlemail.com":
		return "personal"
	default:
		return "workspace"
	}
}
