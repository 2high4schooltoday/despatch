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

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/notify"
	"despatch/internal/pamreset"
	"despatch/internal/store"
	"despatch/internal/util"
)

var (
	ErrInvalidCredentials         = errors.New("invalid credentials")
	ErrPendingApproval            = errors.New("pending approval")
	ErrSuspended                  = errors.New("account suspended")
	ErrForbidden                  = errors.New("forbidden")
	ErrPAMVerifierDown            = errors.New("cannot reach Dovecot IMAP for PAM verification")
	ErrPasswordResetUnavailable   = errors.New("password reset is currently unavailable")
	ErrPasswordResetDelivery      = errors.New("password_reset_delivery_failed")
	ErrPasswordResetLoginUnmapped = errors.New("password_reset_login_unmapped")
	ErrPasswordResetHelperDown    = errors.New("password_reset_helper_unavailable")
	ErrPasswordResetHelperFailed  = errors.New("password_reset_helper_failed")
	ErrInvalidRecoveryEmail       = errors.New("invalid recovery email")
	ErrRecoveryEmailRequired      = errors.New("recovery email is required")
	ErrRecoveryEmailMatchesLogin  = errors.New("recovery email must differ from login email")
)

var domainRx = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

const (
	passwordResetSenderSettingAddress = "password_reset_sender.address"
	passwordResetSenderSettingStatus  = "password_reset_sender.status"
	passwordResetSenderSettingReason  = "password_reset_sender.reason"
	passwordResetSenderSettingHash    = "password_reset_sender.password_hash"
	passwordResetSenderSettingManaged = "password_reset_sender.managed_by"

	passwordResetSenderStatusReady    = "ready"
	passwordResetSenderStatusDegraded = "degraded"
	passwordResetSenderStatusExternal = "external"

	passwordResetSenderReasonLogOnly             = "log_delivery_disabled"
	passwordResetSenderReasonExternalUnconfirmed = "external_sender_unconfirmed"
	passwordResetSenderReasonSMTPUnreachable     = "smtp_unreachable"
	passwordResetSenderReasonSMTPAuthFailed      = "smtp_auth_failed"
	passwordResetSenderReasonSMTPSenderRejected  = "smtp_sender_rejected"
	passwordResetSenderReasonSMTPProbeFailed     = "smtp_probe_failed"
	passwordResetReservationLease                = 30 * time.Second
)

type SetupStatus struct {
	Required                    bool   `json:"required"`
	BaseDomain                  string `json:"base_domain"`
	DefaultAdminEmail           string `json:"default_admin_email"`
	AuthMode                    string `json:"auth_mode"`
	PasswordMinLength           int    `json:"password_min_length"`
	PasswordMaxLength           int    `json:"password_max_length"`
	PasswordClassMin            int    `json:"password_class_min"`
	PasskeyPrimarySignInEnabled bool   `json:"passkey_primary_sign_in_enabled"`
}

type SetupCompleteRequest struct {
	BaseDomain                  string
	AdminEmail                  string
	AdminMailboxLogin           string
	AdminPassword               string
	Region                      string
	PasskeyPrimarySignInEnabled *bool
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
	cfg         config.Config
	st          *store.Store
	mail        mail.Client
	provision   mail.AuthProvisioner
	sender      notify.Sender
	pamResetter pamPasswordResetter
	encryptKey  []byte
}

type pamPasswordResetter interface {
	ResetPassword(ctx context.Context, username, newPassword string) error
}

type passwordResetSenderTransportProber interface {
	ProbePasswordReset(ctx context.Context) error
}

type PasswordResetCapabilities struct {
	AuthMode            string `json:"auth_mode"`
	SelfServiceEnabled  bool   `json:"self_service_enabled"`
	AdminResetEnabled   bool   `json:"admin_reset_enabled"`
	Delivery            string `json:"delivery"`
	TokenTTLMinutes     int    `json:"token_ttl_minutes"`
	RequiresMappedLogin bool   `json:"requires_mapped_login"`
	SenderAddress       string `json:"sender_address"`
	SenderStatus        string `json:"sender_status"`
	SenderReason        string `json:"sender_reason,omitempty"`
}

type passwordResetSenderState struct {
	Address string
	Status  string
	Reason  string
	Ready   bool
}

func New(cfg config.Config, st *store.Store, m mail.Client, p mail.AuthProvisioner, sender notify.Sender) *Service {
	if sender == nil {
		sender = notify.LogSender{}
	}
	return &Service{
		cfg:        cfg,
		st:         st,
		mail:       m,
		provision:  p,
		sender:     sender,
		encryptKey: util.Derive32ByteKey(cfg.SessionEncryptKey),
	}
}

func hashUA(ua string) string {
	s := sha256.Sum256([]byte(ua))
	return hex.EncodeToString(s[:])
}

func (s *Service) Register(ctx context.Context, email, password, recoveryEmail, ip, userAgent string, captchaOK bool, mfaPreference string) error {
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
	normalizedRecovery, err := normalizeDistinctRecoveryEmail(email, recoveryEmail)
	if err != nil {
		return err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	preference := NormalizeMFAPreference(mfaPreference)
	if _, err := s.st.CreateUserWithMFARecovery(ctx, email, hash, "user", models.UserPending, preference, normalizedRecovery); err != nil {
		return err
	}
	_, err = s.st.CreateRegistrationWithMFA(ctx, email, ip, hashUA(userAgent), captchaOK, preference)
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
	if err := s.st.UpsertUserMailSecret(ctx, u.ID, mailSecret); err != nil {
		return "", models.User{}, err
	}

	now := time.Now().UTC()
	stage, err := s.ResolveMFAStage(ctx, u, nil)
	if err != nil {
		return "", models.User{}, err
	}
	var mfaVerifiedAt *time.Time
	if stage.AuthStage == AuthStageAuthenticated {
		mfaVerifiedAt = &now
	}
	sess := models.Session{
		ID:            uuid.NewString(),
		UserID:        u.ID,
		TokenHash:     tokenHash,
		MailSecret:    mailSecret,
		IPHint:        ip,
		UserAgentHash: hashUA(userAgent),
		AuthMethod:    "password",
		MFAVerifiedAt: mfaVerifiedAt,
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

func (s *Service) CreatePasskeySession(ctx context.Context, user models.User, ip, userAgent, mailSecret string) (string, models.Session, error) {
	if user.Status != models.UserActive && user.Role != "admin" {
		return "", models.Session{}, ErrForbidden
	}
	raw, tokenHash, err := auth.NewOpaqueToken()
	if err != nil {
		return "", models.Session{}, err
	}
	now := time.Now().UTC()
	sess := models.Session{
		ID:            uuid.NewString(),
		UserID:        user.ID,
		TokenHash:     tokenHash,
		MailSecret:    strings.TrimSpace(mailSecret),
		IPHint:        ip,
		UserAgentHash: hashUA(userAgent),
		AuthMethod:    "passkey",
		MFAVerifiedAt: &now,
		ExpiresAt:     now.Add(s.cfg.SessionAbsoluteDuration()),
		IdleExpiresAt: now.Add(s.cfg.SessionIdleDuration()),
		CreatedAt:     now,
		LastSeenAt:    now,
	}
	if err := s.st.CreateSession(ctx, sess); err != nil {
		return "", models.Session{}, err
	}
	_ = s.st.TouchUserLastLogin(ctx, user.ID, now)
	return raw, sess, nil
}

func (s *Service) UnlockSessionMailSecret(ctx context.Context, userID, sessionID, password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return ErrInvalidCredentials
	}
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return ErrInvalidCredentials
	}
	if s.usesPAMAuth() {
		stored := ""
		if u.MailLogin != nil {
			stored = *u.MailLogin
		}
		acceptedLogin, err := s.verifyMailCredentials(ctx, u.Email, password, "", stored)
		if err != nil {
			if isMailConnectivityError(err) {
				return fmt.Errorf("%w: %v", ErrPAMVerifierDown, err)
			}
			return ErrInvalidCredentials
		}
		currentLogin := strings.TrimSpace(stored)
		if strings.TrimSpace(acceptedLogin) != currentLogin {
			if err := s.st.UpdateUserMailLogin(ctx, u.ID, acceptedLogin); err != nil {
				return err
			}
		}
	} else {
		if !auth.VerifyPassword(u.PasswordHash, password) {
			return ErrInvalidCredentials
		}
	}
	mailSecret, err := util.EncryptString(s.encryptKey, password)
	if err != nil {
		return err
	}
	if err := s.st.UpsertUserMailSecret(ctx, u.ID, mailSecret); err != nil {
		return err
	}
	if err := s.st.UpdateSessionMailSecret(ctx, sessionID, mailSecret); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]string{
		"session_id": sessionID,
		"mode":       strings.ToLower(strings.TrimSpace(s.cfg.DovecotAuthMode)),
	})
	s.insertAuditBestEffort(ctx, u.ID, "auth.mail_secret.unlock", u.ID, string(meta))
	return nil
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

func (s *Service) PasswordResetCapabilities(ctx context.Context) PasswordResetCapabilities {
	publicResetEnabled := s.cfg.PasswordResetPublicEnabled
	if enabled, err := s.PublicPasswordResetEnabled(ctx); err == nil {
		publicResetEnabled = enabled
	}

	delivery := normalizePasswordResetSender(strings.TrimSpace(strings.ToLower(s.cfg.PasswordResetSender)))
	if !publicResetEnabled {
		delivery = "disabled"
	}

	senderState, err := s.passwordResetSenderState(ctx)
	if err != nil {
		senderState = passwordResetSenderState{
			Address: notify.PasswordResetFromAddress(s.cfg),
			Status:  passwordResetSenderStatusDegraded,
			Reason:  "sender_state_read_failed",
			Ready:   false,
		}
	}
	selfServiceEnabled := publicResetEnabled
	adminResetEnabled := true
	if s.usesPAMAuth() {
		selfServiceEnabled = selfServiceEnabled && s.cfg.PAMResetHelperEnabled
		adminResetEnabled = s.cfg.PAMResetHelperEnabled
	}
	selfServiceEnabled = selfServiceEnabled && senderState.Ready
	return PasswordResetCapabilities{
		AuthMode:            strings.ToLower(strings.TrimSpace(s.cfg.DovecotAuthMode)),
		SelfServiceEnabled:  selfServiceEnabled,
		AdminResetEnabled:   adminResetEnabled,
		Delivery:            delivery,
		TokenTTLMinutes:     s.cfg.PasswordResetTokenTTLMinutes,
		RequiresMappedLogin: false,
		SenderAddress:       senderState.Address,
		SenderStatus:        senderState.Status,
		SenderReason:        senderState.Reason,
	}
}

func (s *Service) EnsurePasswordResetSenderIdentity(ctx context.Context) (passwordResetSenderState, error) {
	state := passwordResetSenderState{
		Address: notify.PasswordResetFromAddress(s.cfg),
		Status:  passwordResetSenderStatusReady,
		Reason:  "",
		Ready:   true,
	}
	delivery := normalizePasswordResetSender(strings.TrimSpace(strings.ToLower(s.cfg.PasswordResetSender)))
	if delivery == "log" {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = passwordResetSenderReasonLogOnly
		state.Ready = false
		return state, s.persistPasswordResetSenderState(ctx, state, "app")
	}
	if delivery != "smtp" {
		return state, s.persistPasswordResetSenderState(ctx, state, "app")
	}
	if s.usesPAMAuth() {
		state = s.externalPasswordResetSenderState()
		if state.Ready {
			state = s.probePasswordResetSenderTransport(ctx, state)
		}
		return state, s.persistPasswordResetSenderState(ctx, state, "external")
	}
	if !s.hasProvisioner() {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = "sql_provisioner_unconfigured"
		state.Ready = false
		return state, s.persistPasswordResetSenderState(ctx, state, "app")
	}

	hash, ok, err := s.st.GetSetting(ctx, passwordResetSenderSettingHash)
	if err != nil {
		return state, err
	}
	hash = strings.TrimSpace(hash)
	if !ok || hash == "" {
		raw, _, tokenErr := auth.NewOpaqueToken()
		if tokenErr != nil {
			return state, tokenErr
		}
		hash, err = auth.HashPassword(raw + "|password-reset-sender")
		if err != nil {
			return state, err
		}
		if err := s.st.UpsertSetting(ctx, passwordResetSenderSettingHash, hash); err != nil {
			return state, err
		}
	}

	if err := s.provision.UpsertActiveUser(ctx, state.Address, hash); err != nil {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = "sender_provision_failed"
		state.Ready = false
		if persistErr := s.persistPasswordResetSenderState(ctx, state, "app"); persistErr != nil {
			return state, persistErr
		}
		return state, nil
	}
	state.Status = passwordResetSenderStatusReady
	state.Reason = ""
	state.Ready = true
	state = s.probePasswordResetSenderTransport(ctx, state)
	return state, s.persistPasswordResetSenderState(ctx, state, "app")
}

func (s *Service) hasProvisioner() bool {
	switch s.provision.(type) {
	case mail.NoopProvisioner, *mail.NoopProvisioner:
		return false
	default:
		return true
	}
}

func (s *Service) persistPasswordResetSenderState(ctx context.Context, state passwordResetSenderState, managedBy string) error {
	if err := s.st.UpsertSetting(ctx, passwordResetSenderSettingAddress, strings.TrimSpace(state.Address)); err != nil {
		return err
	}
	if err := s.st.UpsertSetting(ctx, passwordResetSenderSettingStatus, strings.TrimSpace(state.Status)); err != nil {
		return err
	}
	if err := s.st.UpsertSetting(ctx, passwordResetSenderSettingReason, strings.TrimSpace(state.Reason)); err != nil {
		return err
	}
	if err := s.st.UpsertSetting(ctx, passwordResetSenderSettingManaged, strings.TrimSpace(managedBy)); err != nil {
		return err
	}
	return nil
}

func (s *Service) passwordResetSenderState(ctx context.Context) (passwordResetSenderState, error) {
	state := passwordResetSenderState{
		Address: notify.PasswordResetFromAddress(s.cfg),
		Status:  passwordResetSenderStatusReady,
		Reason:  "",
		Ready:   true,
	}
	status, ok, err := s.st.GetSetting(ctx, passwordResetSenderSettingStatus)
	if err != nil {
		return state, err
	}
	if ok {
		state.Status = strings.TrimSpace(status)
		if reason, reasonOK, reasonErr := s.st.GetSetting(ctx, passwordResetSenderSettingReason); reasonErr == nil && reasonOK {
			state.Reason = strings.TrimSpace(reason)
		}
		if state.Status == passwordResetSenderStatusExternal {
			state = s.externalPasswordResetSenderState()
		} else {
			state.Ready = state.Status == passwordResetSenderStatusReady
		}
		return state, nil
	}

	delivery := normalizePasswordResetSender(strings.TrimSpace(strings.ToLower(s.cfg.PasswordResetSender)))
	if delivery == "log" {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = passwordResetSenderReasonLogOnly
		state.Ready = false
		return state, nil
	}
	if delivery != "smtp" {
		state.Status = passwordResetSenderStatusReady
		state.Ready = true
		return state, nil
	}
	if s.usesPAMAuth() {
		state = s.externalPasswordResetSenderState()
		if !state.Ready {
			return state, nil
		}
		return s.probePasswordResetSenderTransport(ctx, state), nil
	}
	if !s.hasProvisioner() {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = "sql_provisioner_unconfigured"
		state.Ready = false
		return state, nil
	}
	hash, hashOK, hashErr := s.st.GetSetting(ctx, passwordResetSenderSettingHash)
	if hashErr != nil {
		return state, hashErr
	}
	if !hashOK || strings.TrimSpace(hash) == "" {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = "sender_not_initialized"
		state.Ready = false
		return state, nil
	}
	return s.probePasswordResetSenderTransport(ctx, state), nil
}

func (s *Service) externalPasswordResetSenderState() passwordResetSenderState {
	state := passwordResetSenderState{
		Address: notify.PasswordResetFromAddress(s.cfg),
		Status:  passwordResetSenderStatusExternal,
		Reason:  "",
		Ready:   true,
	}
	if !s.cfg.PasswordResetExternalSenderReady {
		state.Reason = passwordResetSenderReasonExternalUnconfirmed
		state.Ready = false
	}
	return state
}

func (s *Service) probePasswordResetSenderTransport(ctx context.Context, state passwordResetSenderState) passwordResetSenderState {
	prober, ok := s.sender.(passwordResetSenderTransportProber)
	if !ok {
		return state
	}
	if err := prober.ProbePasswordReset(ctx); err != nil {
		state.Status = passwordResetSenderStatusDegraded
		state.Reason = passwordResetSMTPReason(err)
		state.Ready = false
		return state
	}
	return state
}

func passwordResetSMTPReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, mail.ErrSMTPSenderRejected) {
		return passwordResetSenderReasonSMTPSenderRejected
	}
	if errors.Is(err, mail.ErrSMTPAuthFailed) {
		return passwordResetSenderReasonSMTPAuthFailed
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "network is unreachable") {
		return passwordResetSenderReasonSMTPUnreachable
	}
	return passwordResetSenderReasonSMTPProbeFailed
}

func passwordResetLookupReason(err error) string {
	switch {
	case errors.Is(err, store.ErrConflict):
		return "ambiguous_recovery_email"
	case errors.Is(err, store.ErrNotFound):
		return "account_not_found"
	default:
		return "lookup_failed"
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
	publicResetEnabled, err := s.PublicPasswordResetEnabled(ctx)
	if err != nil {
		return ErrPasswordResetUnavailable
	}
	if !publicResetEnabled {
		return ErrPasswordResetUnavailable
	}

	if state, err := s.EnsurePasswordResetSenderIdentity(ctx); err != nil || !state.Ready {
		return ErrPasswordResetUnavailable
	}

	caps := s.PasswordResetCapabilities(ctx)
	if !caps.SelfServiceEnabled {
		return ErrPasswordResetUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	u, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			u, err = s.st.GetUserByRecoveryEmail(ctx, email)
			if err != nil {
				// Don't leak presence or ambiguity.
				if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) {
					log.Printf("password_reset_request_soft_skip reason=%s identifier=%q", passwordResetLookupReason(err), email)
					return nil
				}
				return fmt.Errorf("%w: lookup_failed", ErrPasswordResetDelivery)
			}
		} else {
			return fmt.Errorf("%w: lookup_failed", ErrPasswordResetDelivery)
		}
	}
	if RecoveryEmailNeedsSetup(u.Email, u.RecoveryEmail) {
		log.Printf("password_reset_request_soft_skip reason=no_recovery_email user_id=%s email=%q", u.ID, u.Email)
		return nil
	}
	if err := s.issuePasswordResetToken(ctx, u, strings.ToLower(strings.TrimSpace(*u.RecoveryEmail))); err != nil {
		return fmt.Errorf("%w: %v", ErrPasswordResetDelivery, err)
	}
	return nil
}

func (s *Service) ConfirmPasswordReset(ctx context.Context, rawToken, newPassword string) error {
	publicResetEnabled, err := s.PublicPasswordResetEnabled(ctx)
	if err != nil || !publicResetEnabled {
		return ErrPasswordResetUnavailable
	}
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(sum[:])
	reservationID := uuid.NewString()
	t, err := s.st.ReservePasswordResetToken(ctx, tokenHash, reservationID, passwordResetReservationLease)
	if err != nil {
		return ErrInvalidCredentials
	}
	releaseReservation := true
	defer func() {
		if releaseReservation {
			_ = s.st.ReleasePasswordResetTokenReservation(context.Background(), tokenHash, reservationID)
		}
	}()
	u, err := s.st.GetUserByID(ctx, t.UserID)
	if err != nil {
		return ErrInvalidCredentials
	}
	if s.usesPAMAuth() {
		if err := s.resetPasswordViaPAM(ctx, u, newPassword); err != nil {
			return err
		}
		if err := s.consumeReservedPasswordResetToken(ctx, tokenHash, reservationID); err != nil {
			return err
		}
		releaseReservation = false
	}
	h, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.st.UpdateUserPasswordHash(ctx, t.UserID, h); err != nil {
		return err
	}
	if !s.usesPAMAuth() {
		if err := s.consumeReservedPasswordResetToken(ctx, tokenHash, reservationID); err != nil {
			return err
		}
		releaseReservation = false
	}
	if mailSecret, encErr := util.EncryptString(s.encryptKey, newPassword); encErr == nil {
		_ = s.st.UpsertUserMailSecret(ctx, t.UserID, mailSecret)
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
	_ = s.st.RevokeAllMFATrustedDevices(ctx, t.UserID)
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
	if mailSecret, encErr := util.EncryptString(s.encryptKey, newPassword); encErr == nil {
		_ = s.st.UpsertUserMailSecret(ctx, userID, mailSecret)
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
	_ = s.st.RevokeAllMFATrustedDevices(ctx, userID)
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
	if err := s.sender.SendPasswordReset(ctx, deliveryEmail, raw); err != nil {
		_ = s.st.InvalidatePasswordResetToken(ctx, hash)
		return err
	}
	return nil
}

func (s *Service) UpdateRecoveryEmail(ctx context.Context, userID, recoveryEmail string) (string, error) {
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	normalized, err := normalizeDistinctRecoveryEmail(u.Email, recoveryEmail)
	if err != nil {
		return "", err
	}
	if err := s.st.UpdateUserRecoveryEmail(ctx, userID, normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func (s *Service) resetPasswordViaPAM(ctx context.Context, u models.User, newPassword string) error {
	login := ""
	if u.MailLogin != nil {
		login = strings.TrimSpace(*u.MailLogin)
	}
	if login == "" {
		login = strings.TrimSpace(u.Email)
	}
	if s.pamResetter != nil {
		return s.pamResetter.ResetPassword(ctx, login, newPassword)
	}
	if !s.cfg.PAMResetHelperEnabled {
		return ErrPasswordResetHelperDown
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

func (s *Service) consumeReservedPasswordResetToken(ctx context.Context, tokenHash, reservationID string) error {
	if _, err := s.st.ConsumeReservedPasswordResetToken(ctx, tokenHash, reservationID); err != nil {
		_ = s.st.InvalidatePasswordResetToken(ctx, tokenHash)
		return err
	}
	return nil
}

func normalizePasswordResetSender(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "smtp":
		return "smtp"
	case "log":
		return "log"
	default:
		return "smtp"
	}
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
	passkeyPrimaryEnabled := s.cfg.PasskeyPasswordlessEnabled
	if enabled, flagErr := s.PasskeySignInEnabled(ctx); flagErr == nil {
		passkeyPrimaryEnabled = enabled
	}
	return SetupStatus{
		Required:                    adminCount == 0,
		BaseDomain:                  baseDomain,
		DefaultAdminEmail:           defaultAdminEmail(baseDomain),
		AuthMode:                    s.cfg.DovecotAuthMode,
		PasswordMinLength:           s.cfg.PasswordMinLength,
		PasswordMaxLength:           s.cfg.PasswordMaxLength,
		PasswordClassMin:            3,
		PasskeyPrimarySignInEnabled: passkeyPrimaryEnabled,
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
	passkeyPrimaryEnabled := s.cfg.PasskeyPasswordlessEnabled
	if enabled, flagErr := s.PasskeySignInEnabled(ctx); flagErr == nil {
		passkeyPrimaryEnabled = enabled
	}
	if req.PasskeyPrimarySignInEnabled != nil {
		passkeyPrimaryEnabled = *req.PasskeyPrimarySignInEnabled
	}
	if err := s.st.UpsertSetting(ctx, featureFlagSettingPasskeySignIn, boolToFeatureFlagSetting(passkeyPrimaryEnabled)); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.UpsertSetting(ctx, "enforce_admin_mfa", "1"); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.UpdateUserMFAPreference(ctx, adminUser.ID, MFAPreferenceTOTP); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.SetMFASetupSwitchUsed(ctx, adminUser.ID, false); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.SetUserMFABackupCompleted(ctx, adminUser.ID, false); err != nil {
		return "", models.User{}, err
	}
	if err := s.st.SetLegacyMFAPromptPending(ctx, adminUser.ID, false); err != nil {
		return "", models.User{}, err
	}
	adminUser.MFAPreference = MFAPreferenceTOTP
	adminUser.MFASetupSwitchUsed = false
	adminUser.MFABackupCompleted = false
	adminUser.LegacyMFAPromptPending = false
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

func normalizeDistinctRecoveryEmail(loginEmail, recoveryEmail string) (string, error) {
	if strings.TrimSpace(recoveryEmail) == "" {
		return "", ErrRecoveryEmailRequired
	}
	addr, err := normalizeRecoveryEmail(recoveryEmail)
	if err != nil {
		return "", err
	}
	login := strings.ToLower(strings.TrimSpace(loginEmail))
	if login != "" && strings.EqualFold(addr, login) {
		return "", ErrRecoveryEmailMatchesLogin
	}
	return addr, nil
}

func RecoveryEmailNeedsSetup(loginEmail string, recoveryEmail *string) bool {
	if recoveryEmail == nil || strings.TrimSpace(*recoveryEmail) == "" {
		return true
	}
	addr, err := normalizeRecoveryEmail(*recoveryEmail)
	if err != nil {
		return true
	}
	login := strings.ToLower(strings.TrimSpace(loginEmail))
	return login == "" || strings.EqualFold(addr, login)
}

func (s *Service) RecoveryEmailNeedsSetup(user models.User) bool {
	return RecoveryEmailNeedsSetup(user.Email, user.RecoveryEmail)
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
