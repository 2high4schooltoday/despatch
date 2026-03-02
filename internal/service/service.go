package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	netmail "net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"mailclient/internal/auth"
	"mailclient/internal/config"
	"mailclient/internal/mail"
	"mailclient/internal/models"
	"mailclient/internal/notify"
	"mailclient/internal/pamreset"
	"mailclient/internal/store"
	"mailclient/internal/util"
)

var (
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrPendingApproval            = errors.New("pending approval")
	ErrSuspended                  = errors.New("account suspended")
	ErrForbidden                  = errors.New("forbidden")
	ErrPAMVerifierDown            = errors.New("cannot reach Dovecot IMAP for PAM verification")
	ErrPasswordResetUnavailable   = errors.New("password reset is currently unavailable")
	ErrPasswordResetLoginUnmapped = errors.New("password_reset_login_unmapped")
	ErrPasswordResetHelperDown    = errors.New("password_reset_helper_unavailable")
	ErrPasswordResetHelperFailed  = errors.New("password_reset_helper_failed")
	ErrInvalidRecoveryEmail       = errors.New("invalid recovery email")
)

var domainRx = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

type SetupStatus struct {
	Required          bool   `json:"required"`
	BaseDomain        string `json:"base_domain"`
	DefaultAdminEmail string `json:"default_admin_email"`
	AuthMode          string `json:"auth_mode"`
	PasswordMinLength int    `json:"password_min_length"`
	PasswordMaxLength int    `json:"password_max_length"`
	PasswordClassMin  int    `json:"password_class_min"`
}

type SetupCompleteRequest struct {
	BaseDomain        string
	AdminEmail        string
	AdminMailboxLogin string
	AdminPassword     string
	Region            string
}

type PAMCredentialsInvalidError struct {
	Attempts []string
}

func (e *PAMCredentialsInvalidError) Error() string {
	if e == nil || len(e.Attempts) == 0 {
		return "admin credentials are not valid for Dovecot/PAM"
	}
	return fmt.Sprintf("admin credentials are not valid for Dovecot/PAM (attempted logins: %s)", strings.Join(e.Attempts, ", "))
}

type Service struct {
	cfg        config.Config
	st         *store.Store
	mail       mail.Client
	provision  mail.AuthProvisioner
	sender     notify.Sender
	encryptKey []byte
}

type PasswordResetCapabilities struct {
	AuthMode            string `json:"auth_mode"`
	SelfServiceEnabled  bool   `json:"self_service_enabled"`
	AdminResetEnabled   bool   `json:"admin_reset_enabled"`
	Delivery            string `json:"delivery"`
	TokenTTLMinutes     int    `json:"token_ttl_minutes"`
	RequiresMappedLogin bool   `json:"requires_mapped_login"`
}

func New(cfg config.Config, st *store.Store, m mail.Client, p mail.AuthProvisioner, sender notify.Sender) *Service {
	if sender == nil {
		sender = notify.LogSender{}
	}
	return &Service{cfg: cfg, st: st, mail: m, provision: p, sender: sender, encryptKey: util.Derive32ByteKey(cfg.SessionEncryptKey)}
}

func hashUA(ua string) string {
	s := sha256.Sum256([]byte(ua))
	return hex.EncodeToString(s[:])
}

func (s *Service) Register(ctx context.Context, email, password, ip, userAgent string, captchaOK bool) error {
	status, err := s.SetupStatus(ctx)
	if err != nil {
		return err
	}
	if status.Required {
		return errors.New("setup is not complete")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return errors.New("email and password are required")
	}
	if err := s.ValidatePassword(password); err != nil {
		return err
	}
	if !strings.HasSuffix(email, "@"+status.BaseDomain) {
		return fmt.Errorf("email must use @%s", status.BaseDomain)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := s.st.CreateUser(ctx, email, hash, "user", models.UserPending); err != nil {
		return err
	}
	_, err = s.st.CreateRegistration(ctx, email, ip, hashUA(userAgent), captchaOK)
	return err
}

func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (rawToken string, user models.User, err error) {
	u, err := s.st.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return "", models.User{}, ErrInvalidCredentials
	}
	if s.usesPAMAuth() {
		stored := ""
		if u.MailLogin != nil {
			stored = *u.MailLogin
		}
		acceptedLogin, err := s.verifyMailCredentials(ctx, u.Email, password, "", stored)
		if err != nil {
			if isMailConnectivityError(err) {
				return "", models.User{}, fmt.Errorf("%w: %v", ErrPAMVerifierDown, err)
			}
			return "", models.User{}, ErrInvalidCredentials
		}
		currentLogin := strings.TrimSpace(stored)
		if strings.TrimSpace(acceptedLogin) != currentLogin {
			if err := s.st.UpdateUserMailLogin(ctx, u.ID, acceptedLogin); err != nil {
				return "", models.User{}, err
			}
			acceptedCopy := strings.TrimSpace(acceptedLogin)
			if acceptedCopy != "" {
				u.MailLogin = &acceptedCopy
			}
		}
	} else {
		if !auth.VerifyPassword(u.PasswordHash, password) {
			return "", models.User{}, ErrInvalidCredentials
		}
	}
	switch u.Status {
	case models.UserPending:
		return "", models.User{}, ErrPendingApproval
	case models.UserSuspended:
		return "", models.User{}, ErrSuspended
	case models.UserRejected:
		return "", models.User{}, ErrForbidden
	}
	if u.Status != models.UserActive && u.Role != "admin" {
		return "", models.User{}, ErrForbidden
	}

	raw, tokenHash, err := auth.NewOpaqueToken()
	if err != nil {
		return "", models.User{}, err
	}
	mailSecret, err := util.EncryptString(s.encryptKey, password)
	if err != nil {
		return "", models.User{}, err
	}

	now := time.Now().UTC()
	sess := models.Session{
		ID:            uuid.NewString(),
		UserID:        u.ID,
		TokenHash:     tokenHash,
		MailSecret:    mailSecret,
		IPHint:        ip,
		UserAgentHash: hashUA(userAgent),
		ExpiresAt:     now.Add(s.cfg.SessionAbsoluteDuration()),
		IdleExpiresAt: now.Add(s.cfg.SessionIdleDuration()),
		CreatedAt:     now,
		LastSeenAt:    now,
	}
	if err := s.st.CreateSession(ctx, sess); err != nil {
		return "", models.User{}, err
	}
	_ = s.st.TouchUserLastLogin(ctx, u.ID, now)
	return raw, u, nil
}

func (s *Service) ValidateSession(ctx context.Context, rawToken string) (models.User, models.Session, error) {
	sum := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(sum[:])
	sess, err := s.st.GetSessionByTokenHash(ctx, hash)
	if err != nil {
		return models.User{}, models.Session{}, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	if sess.RevokedAt != nil || now.After(sess.ExpiresAt) || now.After(sess.IdleExpiresAt) {
		return models.User{}, models.Session{}, ErrInvalidCredentials
	}
	_ = s.st.TouchSession(ctx, sess.ID, now.Add(s.cfg.SessionIdleDuration()))

	u, err := s.st.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return models.User{}, models.Session{}, ErrInvalidCredentials
	}
	if u.Status != models.UserActive && u.Role != "admin" {
		return models.User{}, models.Session{}, ErrForbidden
	}
	return u, sess, nil
}

func (s *Service) SessionMailPassword(sess models.Session) (string, error) {
	if strings.TrimSpace(sess.MailSecret) == "" {
		return "", fmt.Errorf("missing mail credentials")
	}
	pass, err := util.DecryptString(s.encryptKey, sess.MailSecret)
	if err != nil {
		// Session exists but encrypted mail secret is no longer decryptable
		// (typically key rotation/mismatch). Treat as invalid session.
		return "", ErrInvalidCredentials
	}
	return pass, nil
}

func MailIdentity(u models.User) string {
	if u.MailLogin != nil {
		if login := strings.TrimSpace(*u.MailLogin); login != "" {
			return login
		}
	}
	return strings.TrimSpace(u.Email)
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	sum := sha256.Sum256([]byte(rawToken))
	hash := hex.EncodeToString(sum[:])
	sess, err := s.st.GetSessionByTokenHash(ctx, hash)
	if err != nil {
		return nil
	}
	return s.st.RevokeSession(ctx, sess.ID)
}

func (s *Service) ListRegistrations(ctx context.Context, query models.RegistrationQuery) ([]models.Registration, int, error) {
	return s.st.ListRegistrations(ctx, query)
}

func (s *Service) ApproveRegistration(ctx context.Context, adminID, regID string) error {
	r, err := s.st.GetRegistrationByID(ctx, regID)
	if err != nil {
		return err
	}
	if r.Status != "pending" {
		return store.ErrConflict
	}
	u, err := s.st.GetUserByEmail(ctx, r.Email)
	if err != nil {
		return err
	}
	if err := s.provision.UpsertActiveUser(ctx, u.Email, u.PasswordHash); err != nil {
		msg := err.Error()
		_ = s.st.UpdateProvisionState(ctx, u.ID, "error", &msg)
		return err
	}
	_ = s.st.UpdateProvisionState(ctx, u.ID, "ok", nil)
	if err := s.st.SetRegistrationDecision(ctx, regID, "approved", adminID, ""); err != nil {
		return err
	}
	if err := s.st.UpdateUserStatus(ctx, u.ID, models.UserActive, &adminID); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]string{"registration_id": regID, "user_id": u.ID})
	return s.st.InsertAudit(ctx, adminID, "registration.approve", u.ID, string(meta))
}

func (s *Service) RejectRegistration(ctx context.Context, adminID, regID, reason string) error {
	userID, err := s.st.RejectRegistrationAndDeletePendingUser(ctx, regID, adminID, reason)
	if err != nil {
		return err
	}
	target := userID
	if strings.TrimSpace(target) == "" {
		target = regID
	}
	meta, _ := json.Marshal(map[string]string{"registration_id": regID, "user_id": userID, "reason": reason})
	s.insertAuditBestEffort(ctx, adminID, "registration.reject", target, string(meta))
	return nil
}

func (s *Service) SuspendUser(ctx context.Context, adminID, userID string) error {
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.provision.DisableUser(ctx, u.Email); err != nil {
		return err
	}
	if err := s.st.UpdateUserStatus(ctx, userID, models.UserSuspended, nil); err != nil {
		return err
	}
	if err := s.st.RevokeUserSessions(ctx, userID); err != nil {
		return err
	}
	return s.st.InsertAudit(ctx, adminID, "user.suspend", userID, `{}`)
}

func (s *Service) UnsuspendUser(ctx context.Context, adminID, userID string) error {
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.provision.UpsertActiveUser(ctx, u.Email, u.PasswordHash); err != nil {
		msg := err.Error()
		_ = s.st.UpdateProvisionState(ctx, u.ID, "error", &msg)
		return err
	}
	_ = s.st.UpdateProvisionState(ctx, u.ID, "ok", nil)
	if err := s.st.UpdateUserStatus(ctx, userID, models.UserActive, &adminID); err != nil {
		return err
	}
	return s.st.InsertAudit(ctx, adminID, "user.unsuspend", userID, `{}`)
}

func (s *Service) ListUsers(ctx context.Context, query models.UserQuery) ([]models.User, int, error) {
	return s.st.ListUsers(ctx, query)
}

func (s *Service) ListAudit(ctx context.Context, query models.AuditQuery) ([]models.AuditEntry, int, error) {
	items, total, err := s.st.ListAudit(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		code, text, severity, targetLabel := buildAuditSummary(items[i])
		items[i].SummaryCode = code
		items[i].SummaryText = text
		items[i].SummaryVersion = 1
		items[i].Severity = severity
		items[i].TargetLabel = targetLabel
	}
	return items, total, nil
}

func (s *Service) PasswordResetCapabilities() PasswordResetCapabilities {
	delivery := strings.TrimSpace(strings.ToLower(s.cfg.PasswordResetSender))
	if delivery == "" {
		delivery = "log"
	}
	if !s.cfg.PasswordResetPublicEnabled {
		delivery = "disabled"
	}
	selfServiceEnabled := s.cfg.PasswordResetPublicEnabled
	adminResetEnabled := true
	if s.usesPAMAuth() {
		selfServiceEnabled = selfServiceEnabled && s.cfg.PAMResetHelperEnabled
		adminResetEnabled = s.cfg.PAMResetHelperEnabled
	}
	return PasswordResetCapabilities{
		AuthMode:            strings.ToLower(strings.TrimSpace(s.cfg.DovecotAuthMode)),
		SelfServiceEnabled:  selfServiceEnabled,
		AdminResetEnabled:   adminResetEnabled,
		Delivery:            delivery,
		TokenTTLMinutes:     s.cfg.PasswordResetTokenTTLMinutes,
		RequiresMappedLogin: s.usesPAMAuth() && s.cfg.PasswordResetRequireMappedLogin,
	}
}

func (s *Service) insertAuditBestEffort(ctx context.Context, actorID, action, target, metadata string) {
	if err := s.st.InsertAudit(ctx, actorID, action, target, metadata); err != nil {
		log.Printf("audit_insert_failed action=%s actor=%s target=%s err=%q", action, actorID, target, err.Error())
	}
}

func buildAuditSummary(entry models.AuditEntry) (string, string, string, string) {
	meta := parseAuditMetadata(entry.MetadataJSON)
	targetLabel := strings.TrimSpace(entry.Target)
	if targetLabel == "" {
		if v := strings.TrimSpace(meta["email"]); v != "" {
			targetLabel = v
		} else if v := strings.TrimSpace(meta["user_id"]); v != "" {
			targetLabel = v
		} else if v := strings.TrimSpace(meta["registration_id"]); v != "" {
			targetLabel = v
		} else {
			targetLabel = "(n/a)"
		}
	}

	switch strings.TrimSpace(entry.Action) {
	case "registration.approve":
		return "registration.approved", fmt.Sprintf("Registration approved for %s.", targetLabel), "ok", targetLabel
	case "registration.reject":
		reason := strings.TrimSpace(meta["reason"])
		if reason != "" {
			return "registration.rejected", fmt.Sprintf("Registration rejected for %s (%s).", targetLabel, reason), "warning", targetLabel
		}
		return "registration.rejected", fmt.Sprintf("Registration rejected for %s.", targetLabel), "warning", targetLabel
	case "user.suspend":
		return "user.suspended", fmt.Sprintf("User suspended: %s.", targetLabel), "warning", targetLabel
	case "user.unsuspend":
		return "user.unsuspended", fmt.Sprintf("User unsuspended: %s.", targetLabel), "ok", targetLabel
	case "user.reset_password":
		return "user.password_reset", fmt.Sprintf("Password reset by admin for %s.", targetLabel), "info", targetLabel
	case "user.retry_provision":
		return "user.provision_retried", fmt.Sprintf("Provision retry requested for %s.", targetLabel), "info", targetLabel
	default:
		if action := strings.TrimSpace(entry.Action); action != "" {
			return "audit.event", fmt.Sprintf("Audit event %s on %s.", action, targetLabel), "info", targetLabel
		}
		return "audit.event", fmt.Sprintf("Audit event on %s.", targetLabel), "info", targetLabel
	}
}

func parseAuditMetadata(raw string) map[string]string {
	out := map[string]string{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return out
	}
	for key, value := range decoded {
		switch typed := value.(type) {
		case string:
			out[key] = typed
		case float64:
			out[key] = fmt.Sprintf("%.0f", typed)
		case bool:
			if typed {
				out[key] = "true"
			} else {
				out[key] = "false"
			}
		default:
			out[key] = fmt.Sprintf("%v", typed)
		}
	}
	return out
}

func (s *Service) RetryProvisionUser(ctx context.Context, adminID, userID string) error {
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.provision.UpsertActiveUser(ctx, u.Email, u.PasswordHash); err != nil {
		msg := err.Error()
		_ = s.st.UpdateProvisionState(ctx, u.ID, "error", &msg)
		return err
	}
	_ = s.st.UpdateProvisionState(ctx, u.ID, "ok", nil)
	meta, _ := json.Marshal(map[string]string{"user_id": userID})
	return s.st.InsertAudit(ctx, adminID, "user.retry_provision", userID, string(meta))
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if !s.cfg.PasswordResetPublicEnabled {
		return ErrPasswordResetUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	u, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		// don't leak existence
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return nil
	}
	if s.usesPAMAuth() {
		if !s.cfg.PAMResetHelperEnabled {
			return nil
		}
		if s.cfg.PasswordResetRequireMappedLogin {
			login := ""
			if u.MailLogin != nil {
				login = strings.TrimSpace(*u.MailLogin)
			}
			if login == "" {
				return nil
			}
		}
	}
	recoveryEmail := ""
	if u.RecoveryEmail != nil {
		recoveryEmail = strings.ToLower(strings.TrimSpace(*u.RecoveryEmail))
	}
	if recoveryEmail == "" {
		// Legacy accounts without recovery email are handled by post-login prompt.
		return nil
	}
	if err := s.issuePasswordResetToken(ctx, u, recoveryEmail); err != nil {
		// Keep public API leak-resistant: treat delivery failures as accepted.
		return nil
	}
	return nil
}

func (s *Service) ConfirmPasswordReset(ctx context.Context, rawToken, newPassword string) error {
	if !s.cfg.PasswordResetPublicEnabled {
		return ErrPasswordResetUnavailable
	}
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(sum[:])
	t, err := s.st.ConsumePasswordResetToken(ctx, tokenHash)
	if err != nil {
		return ErrInvalidCredentials
	}
	u, err := s.st.GetUserByID(ctx, t.UserID)
	if err != nil {
		return ErrInvalidCredentials
	}
	if s.usesPAMAuth() {
		if err := s.resetPasswordViaPAM(ctx, u, newPassword); err != nil {
			return err
		}
	}
	h, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.st.UpdateUserPasswordHash(ctx, t.UserID, h); err != nil {
		return err
	}
	if !s.usesPAMAuth() {
		if err := s.provision.UpsertActiveUser(ctx, u.Email, h); err != nil {
			msg := err.Error()
			_ = s.st.UpdateProvisionState(ctx, u.ID, "error", &msg)
		} else {
			_ = s.st.UpdateProvisionState(ctx, u.ID, "ok", nil)
		}
	}
	_ = s.st.RevokeUserSessions(ctx, t.UserID)
	return nil
}

func (s *Service) AdminResetPassword(ctx context.Context, adminID, userID, newPassword string) error {
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if s.usesPAMAuth() {
		if err := s.resetPasswordViaPAM(ctx, u, newPassword); err != nil {
			return err
		}
	}
	h, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.st.UpdateUserPasswordHash(ctx, userID, h); err != nil {
		return err
	}
	if !s.usesPAMAuth() {
		if err := s.provision.UpsertActiveUser(ctx, u.Email, h); err != nil {
			msg := err.Error()
			_ = s.st.UpdateProvisionState(ctx, u.ID, "error", &msg)
		} else {
			_ = s.st.UpdateProvisionState(ctx, u.ID, "ok", nil)
		}
	}
	_ = s.st.RevokeUserSessions(ctx, userID)
	meta, _ := json.Marshal(map[string]string{
		"user_id": userID,
		"mode":    strings.ToLower(strings.TrimSpace(s.cfg.DovecotAuthMode)),
	})
	return s.st.InsertAudit(ctx, adminID, "user.reset_password", userID, string(meta))
}

func (s *Service) issuePasswordResetToken(ctx context.Context, u models.User, deliveryEmail string) error {
	deliveryEmail = strings.ToLower(strings.TrimSpace(deliveryEmail))
	if deliveryEmail == "" {
		return ErrInvalidRecoveryEmail
	}
	raw, hash, err := auth.NewOpaqueToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(time.Duration(s.cfg.PasswordResetTokenTTLMinutes) * time.Minute)
	if _, err := s.st.CreatePasswordResetToken(ctx, u.ID, hash, expiresAt); err != nil {
		return err
	}
	return s.sender.SendPasswordReset(ctx, deliveryEmail, raw)
}

func (s *Service) UpdateRecoveryEmail(ctx context.Context, userID, recoveryEmail string) (string, error) {
	normalized, err := normalizeRecoveryEmail(recoveryEmail)
	if err != nil {
		return "", err
	}
	if err := s.st.UpdateUserRecoveryEmail(ctx, userID, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func (s *Service) resetPasswordViaPAM(ctx context.Context, u models.User, newPassword string) error {
	if !s.cfg.PAMResetHelperEnabled {
		return ErrPasswordResetHelperDown
	}
	login := ""
	if u.MailLogin != nil {
		login = strings.TrimSpace(*u.MailLogin)
	}
	if s.cfg.PasswordResetRequireMappedLogin && login == "" {
		return ErrPasswordResetLoginUnmapped
	}
	if login == "" {
		login = strings.TrimSpace(u.Email)
	}
	client := pamreset.Client{
		SocketPath: s.cfg.PAMResetHelperSocket,
		Timeout:    time.Duration(s.cfg.PAMResetHelperTimeoutSec) * time.Second,
	}
	_, err := client.ResetPassword(ctx, pamreset.Request{
		RequestID:   uuid.NewString(),
		Username:    login,
		NewPassword: newPassword,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, pamreset.ErrHelperUnavailable) {
		return ErrPasswordResetHelperDown
	}
	if errors.Is(err, pamreset.ErrHelperRejected) {
		return ErrPasswordResetHelperFailed
	}
	return fmt.Errorf("%w", ErrPasswordResetHelperFailed)
}

func (s *Service) Mail() mail.Client   { return s.mail }
func (s *Service) Store() *store.Store { return s.st }

func (s *Service) SetupStatus(ctx context.Context) (SetupStatus, error) {
	baseDomain, err := s.baseDomain(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	adminCount, err := s.st.CountAdmins(ctx)
	if err != nil {
		return SetupStatus{}, err
	}
	return SetupStatus{
		Required:          adminCount == 0,
		BaseDomain:        baseDomain,
		DefaultAdminEmail: defaultAdminEmail(baseDomain),
		AuthMode:          s.cfg.DovecotAuthMode,
		PasswordMinLength: s.cfg.PasswordMinLength,
		PasswordMaxLength: s.cfg.PasswordMaxLength,
		PasswordClassMin:  3,
	}, nil
}

func (s *Service) CompleteSetup(ctx context.Context, req SetupCompleteRequest, ip, userAgent string) (string, models.User, error) {
	adminCount, err := s.st.CountAdmins(ctx)
	if err != nil {
		return "", models.User{}, err
	}
	if adminCount > 0 {
		return "", models.User{}, errors.New("setup already completed")
	}

	baseDomain := normalizeDomain(req.BaseDomain)
	if baseDomain == "" {
		baseDomain = normalizeDomain(s.cfg.BaseDomain)
	}
	if !domainRx.MatchString(baseDomain) {
		return "", models.User{}, errors.New("invalid domain name")
	}

	adminEmail := strings.ToLower(strings.TrimSpace(req.AdminEmail))
	if adminEmail == "" {
		adminEmail = defaultAdminEmail(baseDomain)
	}
	parsed, err := netmail.ParseAddress(adminEmail)
	if err != nil {
		return "", models.User{}, errors.New("invalid admin email")
	}
	adminEmail = strings.ToLower(strings.TrimSpace(parsed.Address))
	if !strings.HasSuffix(adminEmail, "@"+baseDomain) {
		return "", models.User{}, fmt.Errorf("admin email must use @%s", baseDomain)
	}

	pamAcceptedLogin := ""
	if s.usesPAMAuth() {
		if strings.TrimSpace(req.AdminPassword) == "" {
			return "", models.User{}, errors.New("admin password is required")
		}
		acceptedLogin, err := s.verifyMailCredentials(ctx, adminEmail, req.AdminPassword, req.AdminMailboxLogin, "")
		if err != nil {
			if isMailConnectivityError(err) {
				return "", models.User{}, fmt.Errorf("%w: %v", ErrPAMVerifierDown, err)
			}
			return "", models.User{}, err
		}
		pamAcceptedLogin = acceptedLogin
	} else {
		if err := s.ValidatePassword(req.AdminPassword); err != nil {
			return "", models.User{}, err
		}
	}

	passwordHash, err := auth.HashPassword(req.AdminPassword)
	if err != nil {
		return "", models.User{}, err
	}
	if err := s.st.EnsureAdmin(ctx, adminEmail, passwordHash); err != nil {
		return "", models.User{}, err
	}
	adminUser, err := s.st.GetUserByEmail(ctx, adminEmail)
	if err != nil {
		return "", models.User{}, err
	}
	if s.usesPAMAuth() && strings.TrimSpace(pamAcceptedLogin) != "" {
		if err := s.st.UpdateUserMailLogin(ctx, adminUser.ID, pamAcceptedLogin); err != nil {
			return "", models.User{}, err
		}
		acceptedCopy := strings.TrimSpace(pamAcceptedLogin)
		adminUser.MailLogin = &acceptedCopy
	}
	if err := s.st.UpsertSetting(ctx, "base_domain", baseDomain); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.UpsertSetting(ctx, "primary_admin_email", adminEmail); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.UpsertSetting(ctx, "setup_completed_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return "", models.User{}, err
	}
	region := strings.TrimSpace(req.Region)
	if region != "" {
		if err := s.st.UpsertSetting(ctx, "region", region); err != nil {
			return "", models.User{}, err
		}
	}

	if err := s.provision.UpsertActiveUser(ctx, adminEmail, passwordHash); err != nil {
		msg := err.Error()
		_ = s.st.UpdateProvisionState(ctx, adminUser.ID, "error", &msg)
		return "", models.User{}, err
	}
	_ = s.st.UpdateProvisionState(ctx, adminUser.ID, "ok", nil)

	return s.Login(ctx, adminEmail, req.AdminPassword, ip, userAgent)
}

func (s *Service) baseDomain(ctx context.Context) (string, error) {
	if v, ok, err := s.st.GetSetting(ctx, "base_domain"); err != nil {
		return "", err
	} else if ok {
		domain := normalizeDomain(v)
		if domain != "" {
			return domain, nil
		}
	}
	domain := normalizeDomain(s.cfg.BaseDomain)
	if domain == "" {
		domain = "example.com"
	}
	return domain, nil
}

func normalizeDomain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimSuffix(v, "/")
	v = strings.TrimSuffix(v, ".")
	return v
}

func defaultAdminEmail(domain string) string {
	if domain == "" {
		return "webmaster@example.com"
	}
	return "webmaster@" + domain
}

func normalizeRecoveryEmail(v string) (string, error) {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "", ErrInvalidRecoveryEmail
	}
	parsed, err := netmail.ParseAddress(trimmed)
	if err != nil {
		return "", ErrInvalidRecoveryEmail
	}
	addr := strings.ToLower(strings.TrimSpace(parsed.Address))
	if addr == "" || len(addr) > 254 {
		return "", ErrInvalidRecoveryEmail
	}
	return addr, nil
}

func (s *Service) ValidatePassword(pw string) error {
	pw = strings.TrimSpace(pw)
	if pw == "" {
		return errors.New("password is required")
	}
	if len(pw) < s.cfg.PasswordMinLength {
		return fmt.Errorf("password must be at least %d characters", s.cfg.PasswordMinLength)
	}
	if len(pw) > s.cfg.PasswordMaxLength {
		return fmt.Errorf("password must be at most %d characters", s.cfg.PasswordMaxLength)
	}
	classes := 0
	if strings.IndexFunc(pw, func(r rune) bool { return r >= 'a' && r <= 'z' }) >= 0 {
		classes++
	}
	if strings.IndexFunc(pw, func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0 {
		classes++
	}
	if strings.IndexFunc(pw, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
		classes++
	}
	if strings.IndexFunc(pw, func(r rune) bool {
		return (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126)
	}) >= 0 {
		classes++
	}
	if classes < 3 {
		return errors.New("password must include at least 3 character classes (lower/upper/number/symbol)")
	}
	return nil
}

func (s *Service) usesPAMAuth() bool {
	return strings.EqualFold(strings.TrimSpace(s.cfg.DovecotAuthMode), "pam")
}

func (s *Service) verifyMailCredentials(ctx context.Context, email, password, explicitLogin, storedLogin string) (string, error) {
	if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
		return "", ErrInvalidCredentials
	}
	candidates := pamLoginCandidates(explicitLogin, storedLogin, email)
	if len(candidates) == 0 {
		return "", ErrInvalidCredentials
	}
	for _, candidate := range candidates {
		if _, err := s.mail.ListMailboxes(ctx, candidate, password); err != nil {
			if isMailConnectivityError(err) {
				return "", err
			}
			continue
		}
		return candidate, nil
	}
	return "", &PAMCredentialsInvalidError{Attempts: candidates}
}

func pamLoginCandidates(explicitLogin, storedLogin, email string) []string {
	added := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(v string) {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return
		}
		key := strings.ToLower(trimmed)
		if _, ok := added[key]; ok {
			return
		}
		added[key] = struct{}{}
		out = append(out, trimmed)
	}
	add(explicitLogin)
	add(storedLogin)
	add(email)
	if at := strings.Index(email, "@"); at > 0 {
		add(email[:at])
	}
	return out
}

func isMailConnectivityError(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	connectivityHints := []string{
		"dial",
		"connect",
		"timeout",
		"i/o timeout",
		"no such host",
		"connection refused",
		"tls",
		"certificate",
		"eof",
		"network is unreachable",
	}
	for _, hint := range connectivityHints {
		if strings.Contains(m, hint) {
			return true
		}
	}
	return false
}
