package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"despatch/internal/models"
	"despatch/internal/store"
)

func (s *Service) EnsureAuthenticatedMailAccount(ctx context.Context, u models.User) error {
	mode, err := s.InstanceMode(ctx)
	if err != nil {
		return err
	}
	if !InstanceModeUsesLocalMail(mode) {
		return nil
	}
	login := strings.TrimSpace(MailAuthLogin(u))
	if login == "" {
		return nil
	}
	secretEnc, ok, err := s.st.GetUserMailSecret(ctx, u.ID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil
		}
		return err
	}
	if !ok || strings.TrimSpace(secretEnc) == "" {
		return nil
	}

	candidates := []string{login}
	if s.usesPAMAuth() {
		candidates = pamLoginCandidates("", login, u.Email)
	}
	s.mailAccountMu.Lock()
	defer s.mailAccountMu.Unlock()

	accounts, err := s.st.ListMailAccounts(ctx, u.ID)
	if err != nil {
		return err
	}
	hasDefault := false
	match := -1
	for i := range accounts {
		if accounts[i].IsDefault {
			hasDefault = true
		}
		if match >= 0 {
			continue
		}
		if mailLoginMatchesAny(accounts[i].Login, candidates) {
			match = i
		}
	}
	if match >= 0 {
		account := accounts[match]
		changed := false
		if strings.TrimSpace(account.SecretEnc) != strings.TrimSpace(secretEnc) {
			account.SecretEnc = secretEnc
			account.Status = "active"
			account.LastError = ""
			changed = true
		}
		if !account.IsDefault && !hasDefault {
			account.IsDefault = true
			changed = true
		}
		if changed {
			_, err = s.st.UpdateMailAccount(ctx, account)
			return err
		}
		return nil
	}

	_, err = s.st.CreateMailAccount(ctx, models.MailAccount{
		ID:             uuid.NewString(),
		UserID:         u.ID,
		Login:          login,
		SecretEnc:      secretEnc,
		IMAPHost:       s.cfg.IMAPHost,
		IMAPPort:       s.cfg.IMAPPort,
		IMAPTLS:        s.cfg.IMAPTLS,
		IMAPStartTLS:   s.cfg.IMAPStartTLS,
		SMTPHost:       s.cfg.SMTPHost,
		SMTPPort:       s.cfg.SMTPPort,
		SMTPTLS:        s.cfg.SMTPTLS,
		SMTPStartTLS:   s.cfg.SMTPStartTLS,
		ProviderType:   MailProviderTypeGeneric,
		ProviderLabel:  MailProviderByID(MailProviderTypeGeneric).Label,
		AuthKind:       MailAccountAuthKindPassword,
		ConnectionMode: MailConnectionModeIMAPSMTP,
		IsDefault:      !hasDefault,
		Status:         "active",
	})
	return err
}

func mailLoginMatchesAny(login string, candidates []string) bool {
	login = strings.TrimSpace(login)
	if login == "" {
		return false
	}
	for _, candidate := range candidates {
		if strings.EqualFold(login, strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}
