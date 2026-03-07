package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const (
	FeatureFlagPasskeySignIn               = "passkey_sign_in"
	FeatureFlagAdminMFARequired            = "admin_mfa_required"
	FeatureFlagPublicPasswordReset         = "public_password_reset"
	FeatureFlagRegistrationCaptchaRequired = "registration_captcha_required"
	FeatureFlagMailSecServiceEnabled       = "mailsec_service_enabled"
	FeatureFlagSoftwareUpdateEnabled       = "software_update_enabled"
	FeatureFlagSignedUpdatesRequired       = "signed_updates_required"
	FeatureFlagPAMResetHelperEnabled       = "pam_reset_helper_enabled"
	FeatureFlagMappedLoginRequiredForReset = "mapped_login_required_for_password_reset"
	featureFlagSettingPasskeySignIn        = "feature_flag.passkey_sign_in"
	featureFlagSettingPublicPasswordReset  = "feature_flag.public_password_reset"
	featureFlagSettingAdminMFARequired     = "enforce_admin_mfa"
	featureFlagAuditActionUpdate           = "feature_flag.update"
	featureFlagAuditActionReset            = "feature_flag.reset"
)

var (
	ErrFeatureFlagNotFound = errors.New("feature_flag_not_found")
	ErrFeatureFlagReadOnly = errors.New("feature_flag_read_only")
)

type FeatureFlagState struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Enabled         bool   `json:"enabled"`
	DefaultEnabled  bool   `json:"default_enabled"`
	Source          string `json:"source"`
	Editable        bool   `json:"editable"`
	RequiresRestart bool   `json:"requires_restart"`
	Note            string `json:"note"`
}

type featureFlagDef struct {
	ID              string
	Name            string
	Description     string
	Category        string
	Editable        bool
	RequiresRestart bool
	Note            string
	SettingKey      string
	DefaultValue    func(context.Context, *Service) (bool, error)
}

func (s *Service) featureFlagCatalog() []featureFlagDef {
	return []featureFlagDef{
		{
			ID:          FeatureFlagPasskeySignIn,
			Name:        "Passkey Sign-In",
			Description: "Allow passkeys as a primary sign-in method.",
			Category:    "Authentication",
			Editable:    true,
			Note:        "When disabled, users can still enroll passkeys for MFA.",
			SettingKey:  featureFlagSettingPasskeySignIn,
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.PasskeyPasswordlessEnabled, nil
			},
		},
		{
			ID:          FeatureFlagAdminMFARequired,
			Name:        "Require MFA For Admins",
			Description: "Require administrators to complete MFA setup and verification.",
			Category:    "Security",
			Editable:    true,
			Note:        "Disabling this is not recommended outside isolated development environments.",
			SettingKey:  featureFlagSettingAdminMFARequired,
			DefaultValue: func(ctx context.Context, svc *Service) (bool, error) {
				raw, ok, err := svc.st.GetSetting(ctx, "setup_completed_at")
				if err != nil {
					return false, err
				}
				return ok && strings.TrimSpace(raw) != "", nil
			},
		},
		{
			ID:          FeatureFlagPublicPasswordReset,
			Name:        "Public Password Reset",
			Description: "Allow unauthenticated users to request password reset tokens.",
			Category:    "Authentication",
			Editable:    true,
			Note:        "Availability still depends on sender health and PAM helper requirements.",
			SettingKey:  featureFlagSettingPublicPasswordReset,
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.PasswordResetPublicEnabled, nil
			},
		},
		{
			ID:              FeatureFlagRegistrationCaptchaRequired,
			Name:            "Registration CAPTCHA Required",
			Description:     "Require CAPTCHA verification for new account registration.",
			Category:        "Security",
			Editable:        false,
			RequiresRestart: true,
			Note:            "Managed by server configuration.",
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.CaptchaEnabled, nil
			},
		},
		{
			ID:              FeatureFlagMailSecServiceEnabled,
			Name:            "Mail Security Service Enabled",
			Description:     "Enable mail security runtime integrations used by passkeys and cryptographic flows.",
			Category:        "System",
			Editable:        false,
			RequiresRestart: true,
			Note:            "Managed by server configuration.",
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.MailSecEnabled, nil
			},
		},
		{
			ID:              FeatureFlagSoftwareUpdateEnabled,
			Name:            "Software Update Panel Enabled",
			Description:     "Enable one-click update checks and apply actions in Admin.",
			Category:        "System",
			Editable:        false,
			RequiresRestart: true,
			Note:            "Managed by server configuration.",
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.UpdateEnabled, nil
			},
		},
		{
			ID:              FeatureFlagSignedUpdatesRequired,
			Name:            "Require Signed Updates",
			Description:     "Require signature verification for update artifacts.",
			Category:        "System",
			Editable:        false,
			RequiresRestart: true,
			Note:            "Managed by server configuration.",
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.UpdateRequireSignature, nil
			},
		},
		{
			ID:              FeatureFlagPAMResetHelperEnabled,
			Name:            "PAM Reset Helper Enabled",
			Description:     "Allow PAM-mode password reset through privileged helper service.",
			Category:        "System",
			Editable:        false,
			RequiresRestart: true,
			Note:            "Managed by server configuration.",
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.PAMResetHelperEnabled, nil
			},
		},
		{
			ID:              FeatureFlagMappedLoginRequiredForReset,
			Name:            "Mapped Login Required For Password Reset",
			Description:     "Require mapped mailbox login identity for password reset in PAM mode.",
			Category:        "Security",
			Editable:        false,
			RequiresRestart: true,
			Note:            "Managed by server configuration.",
			DefaultValue: func(_ context.Context, svc *Service) (bool, error) {
				return svc.cfg.PasswordResetRequireMappedLogin, nil
			},
		},
	}
}

func (s *Service) findFeatureFlagDef(id string) (featureFlagDef, bool) {
	needle := strings.ToLower(strings.TrimSpace(id))
	for _, def := range s.featureFlagCatalog() {
		if strings.EqualFold(def.ID, needle) {
			return def, true
		}
	}
	return featureFlagDef{}, false
}

func parseFeatureFlagBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enabled":
		return true, true
	case "0", "false", "no", "off", "disabled":
		return false, true
	default:
		return false, false
	}
}

func boolToFeatureFlagSetting(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func (s *Service) resolveFeatureFlagState(ctx context.Context, def featureFlagDef) (FeatureFlagState, error) {
	defaultEnabled, err := def.DefaultValue(ctx, s)
	if err != nil {
		return FeatureFlagState{}, err
	}
	resolvedEnabled := defaultEnabled
	source := "default"
	if def.Editable && strings.TrimSpace(def.SettingKey) != "" {
		raw, ok, getErr := s.st.GetSetting(ctx, def.SettingKey)
		if getErr != nil {
			return FeatureFlagState{}, getErr
		}
		if ok {
			if parsed, parseOK := parseFeatureFlagBool(raw); parseOK {
				resolvedEnabled = parsed
				source = "override"
			}
		}
	}
	return FeatureFlagState{
		ID:              def.ID,
		Name:            def.Name,
		Description:     def.Description,
		Category:        def.Category,
		Enabled:         resolvedEnabled,
		DefaultEnabled:  defaultEnabled,
		Source:          source,
		Editable:        def.Editable,
		RequiresRestart: def.RequiresRestart,
		Note:            def.Note,
	}, nil
}

func (s *Service) ListFeatureFlags(ctx context.Context) ([]FeatureFlagState, error) {
	defs := s.featureFlagCatalog()
	out := make([]FeatureFlagState, 0, len(defs))
	for _, def := range defs {
		item, err := s.resolveFeatureFlagState(ctx, def)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) FeatureFlagEnabled(ctx context.Context, id string) (bool, error) {
	def, ok := s.findFeatureFlagDef(id)
	if !ok {
		return false, ErrFeatureFlagNotFound
	}
	item, err := s.resolveFeatureFlagState(ctx, def)
	if err != nil {
		return false, err
	}
	return item.Enabled, nil
}

func (s *Service) SetFeatureFlag(ctx context.Context, actorUserID, id string, enabled bool) (FeatureFlagState, error) {
	def, ok := s.findFeatureFlagDef(id)
	if !ok {
		return FeatureFlagState{}, ErrFeatureFlagNotFound
	}
	if !def.Editable || strings.TrimSpace(def.SettingKey) == "" {
		return FeatureFlagState{}, ErrFeatureFlagReadOnly
	}
	if err := s.st.UpsertSetting(ctx, def.SettingKey, boolToFeatureFlagSetting(enabled)); err != nil {
		return FeatureFlagState{}, err
	}
	meta, _ := json.Marshal(map[string]any{
		"flag_id": def.ID,
		"enabled": enabled,
		"source":  "override",
	})
	_ = s.st.InsertAudit(ctx, strings.TrimSpace(actorUserID), featureFlagAuditActionUpdate, def.ID, string(meta))
	return s.resolveFeatureFlagState(ctx, def)
}

func (s *Service) ResetFeatureFlag(ctx context.Context, actorUserID, id string) (FeatureFlagState, error) {
	def, ok := s.findFeatureFlagDef(id)
	if !ok {
		return FeatureFlagState{}, ErrFeatureFlagNotFound
	}
	if !def.Editable || strings.TrimSpace(def.SettingKey) == "" {
		return FeatureFlagState{}, ErrFeatureFlagReadOnly
	}
	if err := s.st.DeleteSetting(ctx, def.SettingKey); err != nil {
		return FeatureFlagState{}, err
	}
	meta, _ := json.Marshal(map[string]any{
		"flag_id": def.ID,
		"source":  "default",
	})
	_ = s.st.InsertAudit(ctx, strings.TrimSpace(actorUserID), featureFlagAuditActionReset, def.ID, string(meta))
	return s.resolveFeatureFlagState(ctx, def)
}

func (s *Service) PasskeySignInEnabled(ctx context.Context) (bool, error) {
	return s.FeatureFlagEnabled(ctx, FeatureFlagPasskeySignIn)
}

func (s *Service) AdminMFARequired(ctx context.Context) (bool, error) {
	return s.FeatureFlagEnabled(ctx, FeatureFlagAdminMFARequired)
}

func (s *Service) PublicPasswordResetEnabled(ctx context.Context) (bool, error) {
	return s.FeatureFlagEnabled(ctx, FeatureFlagPublicPasswordReset)
}
