package service

import (
	"context"
	"fmt"
	netmail "net/mail"
	"strings"

	"github.com/google/uuid"

	"despatch/internal/models"
	"despatch/internal/store"
)

type ResolvedComposeSender struct {
	IdentityID      string
	SenderProfileID string
	AccountID       string
	HeaderFromName  string
	HeaderFromEmail string
	EnvelopeFrom    string
	ReplyTo         string
}

func sessionMailIdentityForUser(u models.User) string {
	candidate := strings.TrimSpace(MailIdentity(u))
	if candidate == "" {
		return ""
	}
	parsed, err := netmail.ParseAddress(candidate)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Address)
}

func (s *Service) EnsureSessionMailProfile(ctx context.Context, u models.User) (models.SessionMailProfile, error) {
	fromEmail := sessionMailIdentityForUser(u)
	profile, err := s.st.GetSessionMailProfile(ctx, u.ID, fromEmail)
	if err == nil {
		return profile, nil
	}
	if err != nil && err != store.ErrNotFound {
		return models.SessionMailProfile{}, err
	}
	return s.st.UpsertSessionMailProfile(ctx, models.SessionMailProfile{
		ID:        uuid.NewString(),
		UserID:    u.ID,
		FromEmail: fromEmail,
	})
}

func senderAccountLabel(account models.MailAccount) string {
	if label := strings.TrimSpace(account.DisplayName); label != "" {
		return label
	}
	if login := strings.TrimSpace(account.Login); login != "" {
		return login
	}
	return strings.TrimSpace(account.ID)
}

func senderStatusForAccount(account *models.MailAccount) (string, bool) {
	if account == nil || strings.TrimSpace(account.ID) == "" {
		return "needs_account", false
	}
	if strings.EqualFold(strings.TrimSpace(account.Status), "active") || strings.TrimSpace(account.Status) == "" {
		return "ok", true
	}
	return "unavailable", false
}

func chooseDefaultMailAccount(accounts []models.MailAccount) *models.MailAccount {
	for i := range accounts {
		if accounts[i].IsDefault {
			return &accounts[i]
		}
	}
	if len(accounts) == 0 {
		return nil
	}
	return &accounts[0]
}

func (s *Service) SetDefaultSenderProfile(ctx context.Context, u models.User, senderProfileID string) error {
	senderProfileID = strings.TrimSpace(senderProfileID)
	if senderProfileID != "" {
		if _, err := s.GetSenderProfileByID(ctx, u, senderProfileID); err != nil {
			return err
		}
	}
	prefs, err := s.st.GetUserPreferences(ctx, u.ID)
	if err != nil {
		return err
	}
	prefs.UserID = u.ID
	prefs.DefaultSenderID = senderProfileID
	_, err = s.st.UpsertUserPreferences(ctx, prefs)
	return err
}

func (s *Service) ListSenderProfiles(ctx context.Context, u models.User) ([]models.SenderProfile, error) {
	sessionProfile, err := s.EnsureSessionMailProfile(ctx, u)
	if err != nil {
		return nil, err
	}
	accounts, err := s.st.ListMailAccounts(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	prefs, err := s.st.GetUserPreferences(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	defaultAccount := chooseDefaultMailAccount(accounts)

	items := make([]models.SenderProfile, 0, len(accounts)+1)
	primaryFromEmail := strings.TrimSpace(sessionProfile.FromEmail)
	if primaryFromEmail == "" && defaultAccount != nil {
		primaryFromEmail = strings.TrimSpace(defaultAccount.Login)
	}
	if primaryFromEmail != "" {
		primary := models.SenderProfile{
			ID:            sessionProfile.ID,
			Kind:          "primary",
			Name:          strings.TrimSpace(sessionProfile.DisplayName),
			FromEmail:     primaryFromEmail,
			ReplyTo:       strings.TrimSpace(sessionProfile.ReplyTo),
			SignatureText: strings.TrimSpace(sessionProfile.SignatureText),
			SignatureHTML: strings.TrimSpace(sessionProfile.SignatureHTML),
			IsPrimary:     true,
			CanDelete:     false,
		}
		if defaultAccount != nil {
			primary.AccountID = defaultAccount.ID
			primary.AccountLabel = senderAccountLabel(*defaultAccount)
			primary.AccountIsDefault = defaultAccount.IsDefault
			primary.IsAccountDefault = defaultAccount.IsDefault
		}
		primary.Status, primary.CanSchedule = senderStatusForAccount(defaultAccount)
		items = append(items, primary)
	}

	defaultProfileID := strings.TrimSpace(prefs.DefaultSenderID)
	defaultAccountID := ""
	if defaultAccount != nil {
		defaultAccountID = defaultAccount.ID
	}
	defaultAccountAliasID := ""

	for _, account := range accounts {
		identities, err := s.st.ListMailIdentities(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		for _, identity := range identities {
			fromEmail := strings.TrimSpace(identity.FromEmail)
			if fromEmail == "" {
				continue
			}
			status, canSchedule := senderStatusForAccount(&account)
			if account.ID == defaultAccountID && identity.IsDefault && defaultAccountAliasID == "" {
				defaultAccountAliasID = identity.ID
			}
			items = append(items, models.SenderProfile{
				ID:               identity.ID,
				Kind:             "alias",
				Name:             strings.TrimSpace(identity.DisplayName),
				FromEmail:        fromEmail,
				ReplyTo:          strings.TrimSpace(identity.ReplyTo),
				SignatureText:    strings.TrimSpace(identity.SignatureText),
				SignatureHTML:    strings.TrimSpace(identity.SignatureHTML),
				AccountID:        account.ID,
				AccountLabel:     senderAccountLabel(account),
				CanDelete:        true,
				CanSchedule:      canSchedule,
				Status:           status,
				AccountIsDefault: account.IsDefault,
				IsAccountDefault: identity.IsDefault,
			})
		}
	}

	if defaultProfileID == "" {
		switch {
		case defaultAccountAliasID != "":
			defaultProfileID = defaultAccountAliasID
		default:
			defaultProfileID = sessionProfile.ID
		}
	}
	seenDefault := false
	for i := range items {
		if items[i].ID == defaultProfileID {
			items[i].IsDefault = true
			seenDefault = true
			break
		}
	}
	if !seenDefault && len(items) > 0 {
		items[0].IsDefault = true
	}
	return items, nil
}

func (s *Service) GetSenderProfileByID(ctx context.Context, u models.User, senderProfileID string) (models.SenderProfile, error) {
	senderProfileID = strings.TrimSpace(senderProfileID)
	if senderProfileID == "" {
		return models.SenderProfile{}, store.ErrNotFound
	}
	items, err := s.ListSenderProfiles(ctx, u)
	if err != nil {
		return models.SenderProfile{}, err
	}
	for _, item := range items {
		if item.ID == senderProfileID {
			return item, nil
		}
	}
	return models.SenderProfile{}, store.ErrNotFound
}

func (s *Service) DefaultSenderProfileID(ctx context.Context, u models.User) (string, error) {
	items, err := s.ListSenderProfiles(ctx, u)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.IsDefault {
			return item.ID, nil
		}
	}
	return "", nil
}

func (s *Service) DeriveDraftSenderProfileID(ctx context.Context, u models.User, draft models.Draft) (string, error) {
	if senderProfileID := strings.TrimSpace(draft.SenderProfileID); senderProfileID != "" {
		if _, err := s.GetSenderProfileByID(ctx, u, senderProfileID); err == nil {
			return senderProfileID, nil
		} else if err != store.ErrNotFound {
			return "", err
		}
	}
	identityID := strings.TrimSpace(draft.IdentityID)
	if identityID != "" {
		if _, err := s.GetSenderProfileByID(ctx, u, identityID); err == nil {
			return identityID, nil
		} else if err != store.ErrNotFound {
			return "", err
		}
	}
	switch strings.ToLower(strings.TrimSpace(draft.FromMode)) {
	case "", "default", "manual":
		sessionProfile, err := s.EnsureSessionMailProfile(ctx, u)
		if err != nil {
			return "", err
		}
		return sessionProfile.ID, nil
	case "identity":
		if identityID != "" {
			return identityID, nil
		}
	}
	return s.DefaultSenderProfileID(ctx, u)
}

func (s *Service) ResolveComposeSender(ctx context.Context, u models.User, senderProfileID, fromMode, identityID, fromManual string) (ResolvedComposeSender, error) {
	authEmail := strings.TrimSpace(MailIdentity(u))
	if authEmail == "" {
		return ResolvedComposeSender{}, fmt.Errorf("authenticated session mail identity is required")
	}
	senderProfileID = strings.TrimSpace(senderProfileID)
	if senderProfileID == "" {
		var err error
		senderProfileID, err = s.resolveLegacySenderProfileID(ctx, u, fromMode, identityID, fromManual)
		if err != nil {
			return ResolvedComposeSender{}, err
		}
	}
	profile, err := s.GetSenderProfileByID(ctx, u, senderProfileID)
	if err != nil {
		return ResolvedComposeSender{}, err
	}
	if strings.TrimSpace(profile.FromEmail) == "" {
		return ResolvedComposeSender{}, fmt.Errorf("selected sender is missing from_email")
	}
	if strings.TrimSpace(profile.AccountID) == "" {
		if profile.IsPrimary {
			return ResolvedComposeSender{
				IdentityID:      profile.ID,
				SenderProfileID: profile.ID,
				AccountID:       "",
				HeaderFromName:  strings.TrimSpace(profile.Name),
				HeaderFromEmail: strings.TrimSpace(profile.FromEmail),
				EnvelopeFrom:    strings.TrimSpace(profile.FromEmail),
				ReplyTo:         strings.TrimSpace(profile.ReplyTo),
			}, nil
		}
		return ResolvedComposeSender{}, fmt.Errorf("selected sender requires a sending account")
	}
	account, err := s.st.GetMailAccountByID(ctx, u.ID, profile.AccountID)
	if err != nil {
		return ResolvedComposeSender{}, err
	}
	if status, _ := senderStatusForAccount(&account); status != "ok" {
		if profile.IsPrimary {
			return ResolvedComposeSender{
				IdentityID:      profile.ID,
				SenderProfileID: profile.ID,
				AccountID:       "",
				HeaderFromName:  strings.TrimSpace(profile.Name),
				HeaderFromEmail: strings.TrimSpace(profile.FromEmail),
				EnvelopeFrom:    strings.TrimSpace(profile.FromEmail),
				ReplyTo:         strings.TrimSpace(profile.ReplyTo),
			}, nil
		}
		return ResolvedComposeSender{}, fmt.Errorf("selected sender is not available for sending")
	}
	return ResolvedComposeSender{
		IdentityID:      profile.ID,
		SenderProfileID: profile.ID,
		AccountID:       account.ID,
		HeaderFromName:  strings.TrimSpace(profile.Name),
		HeaderFromEmail: strings.TrimSpace(profile.FromEmail),
		EnvelopeFrom:    strings.TrimSpace(profile.FromEmail),
		ReplyTo:         strings.TrimSpace(profile.ReplyTo),
	}, nil
}

func (s *Service) resolveLegacySenderProfileID(ctx context.Context, u models.User, fromMode, identityID, fromManual string) (string, error) {
	authEmail := strings.TrimSpace(MailIdentity(u))
	sessionProfile, err := s.EnsureSessionMailProfile(ctx, u)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(fromMode)) {
	case "", "default":
		return sessionProfile.ID, nil
	case "manual":
		manualSender := strings.TrimSpace(fromManual)
		if manualSender == "" {
			manualSender = authEmail
		}
		if !strings.EqualFold(manualSender, authEmail) {
			return "", fmt.Errorf("manual sender must match authenticated account email")
		}
		return sessionProfile.ID, nil
	case "identity":
		identityID = strings.TrimSpace(identityID)
		if identityID == "" {
			return "", fmt.Errorf("identity_id is required when from_mode=identity")
		}
		if _, err := s.GetSenderProfileByID(ctx, u, identityID); err == nil {
			return identityID, nil
		} else if err == store.ErrNotFound {
			return "", err
		} else {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported from_mode")
	}
}
