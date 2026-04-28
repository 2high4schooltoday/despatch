package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"

	"despatch/internal/captcha"
	"despatch/internal/config"
	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/rate"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/update"
	"despatch/internal/util"
	"despatch/internal/version"
	"despatch/internal/webi18n"
)

type Handlers struct {
	cfg             config.Config
	svc             *service.Service
	limiter         *rate.Limiter
	captchaVerifier captcha.Verifier
	updateMgr       *update.Manager
	resetKey        []byte
	mailboxCache    *mailboxListCache
	threadCache     *threadResponseCache
	readiness       *readinessProbeCache
	webI18n         *webi18n.Runtime
}

var mailClientFactory = func(cfg config.Config) mail.Client {
	return mail.NewIMAPSMTPClient(cfg)
}

const (
	maxUploadAttachmentBytes    = 25 << 20
	maxUploadTotalBytes         = 35 << 20
	trustedDeviceTTL            = 30 * 24 * time.Hour
	threadMessagesMaxScanPages  = 20
	threadMessagesMaxPagesPerMB = 10
	threadMessagesScanPageSize  = 50
	mailRemoteImageFetchTimeout = 8 * time.Second
	mailRemoteImageMaxBytes     = 10 << 20
	mailRemoteImageMaxRedirects = 3
)

func NewRouter(cfg config.Config, svc *service.Service) http.Handler {
	var runtimeI18n *webi18n.Runtime
	if loaded, err := webi18n.Load(filepath.Join("web", "locales"), "despatch"); err != nil {
		log.Printf("web_i18n_init_failed err=%v", err)
	} else {
		runtimeI18n = loaded
	}
	h := &Handlers{
		cfg:             cfg,
		svc:             svc,
		limiter:         rate.NewLimiter(),
		captchaVerifier: captcha.NewVerifier(cfg),
		updateMgr:       update.NewManager(cfg),
		resetKey:        util.Derive32ByteKey(cfg.SessionEncryptKey + "|reset-limiter"),
		mailboxCache:    newMailboxListCache(),
		threadCache:     newThreadResponseCache(),
		readiness:       newReadinessProbeCache(cfg, svc),
		webI18n:         runtimeI18n,
	}
	h.readiness.prime()
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.RequestIDMiddleware)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.SecurityHeaders)
	if len(cfg.CORSAllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   cfg.CORSAllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "X-CSRF-Token"},
			AllowCredentials: true,
		}))
	}

	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) {
		util.WriteJSON(w, 200, map[string]string{"status": "ok"})
	})
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		probe, _ := h.readiness.snapshot(r.Context())
		ready := map[string]any{
			"checked_at": probe.CheckedAt.Format(time.RFC3339),
			"components": map[string]any{},
		}
		comps := ready["components"].(map[string]any)

		readyOK := true
		if probe.SQLiteErr != nil {
			readyOK = false
			comps["sqlite"] = map[string]any{"ok": false, "error": probe.SQLiteErr.Error()}
		} else {
			comps["sqlite"] = map[string]any{"ok": true}
		}

		if probe.IMAPErr != nil {
			comps["imap"] = map[string]any{"ok": false, "error": probe.IMAPErr.Error()}
			readyOK = false
		} else {
			comps["imap"] = map[string]any{"ok": true}
		}

		if probe.SMTPError != nil {
			comps["smtp"] = map[string]any{"ok": false, "error": probe.SMTPError.Error()}
			readyOK = false
		} else {
			comps["smtp"] = map[string]any{"ok": true}
		}

		authCaps := h.authCapabilities(r)
		comps["passkey"] = map[string]any{
			"passkey_mfa_available":          authCaps["passkey_mfa_available"],
			"passkey_passwordless_available": authCaps["passkey_passwordless_available"],
			"reason":                         authCaps["reason"],
		}
		resetCaps := h.svc.PasswordResetCapabilities(r.Context())
		resetSenderOK := resetCaps.SenderStatus == "ready" || resetCaps.SenderStatus == "external"
		publicResetEnabled := false
		if enabled, err := h.svc.PublicPasswordResetEnabled(r.Context()); err == nil {
			publicResetEnabled = enabled
		} else {
			publicResetEnabled = h.cfg.PasswordResetPublicEnabled
		}
		comps["password_reset_sender"] = map[string]any{
			"ok":      resetSenderOK,
			"status":  resetCaps.SenderStatus,
			"reason":  resetCaps.SenderReason,
			"address": resetCaps.SenderAddress,
		}
		if publicResetEnabled && !resetSenderOK {
			readyOK = false
		}

		if readyOK {
			ready["status"] = "ready"
			util.WriteJSON(w, 200, ready)
			return
		}
		ready["status"] = "degraded"
		util.WriteJSON(w, 503, ready)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/public/captcha/config", h.PublicCaptchaConfig)
		r.Get("/public/password-reset/capabilities", h.PublicPasswordResetCapabilities)
		r.Get("/public/auth/capabilities", h.PublicAuthCapabilities)
		r.Get("/public/i18n/{locale}", h.PublicI18nBundle)
		r.Get("/setup/status", h.SetupStatus)
		r.With(middleware.RateLimit(h.limiter, "setup_complete", 20, time.Minute, h.cfg.TrustProxy)).Post("/setup/complete", h.SetupComplete)
		r.With(middleware.RateLimit(h.limiter, "register", 10, time.Minute, h.cfg.TrustProxy)).Post("/register", h.Register)
		r.With(middleware.RateLimit(h.limiter, "login", 20, time.Minute, h.cfg.TrustProxy)).Post("/login", h.Login)
		r.With(middleware.RateLimit(h.limiter, "login_passkey_begin_v1", 30, time.Minute, h.cfg.TrustProxy)).Post("/login/passkey/begin", h.V1PasskeyLoginBegin)
		r.With(middleware.RateLimit(h.limiter, "login_passkey_finish_v1", 30, time.Minute, h.cfg.TrustProxy)).Post("/login/passkey/finish", h.V1PasskeyLoginFinish)
		r.Post("/logout", h.Logout)
		r.With(middleware.RateLimit(h.limiter, "reset_request", 10, time.Minute, h.cfg.TrustProxy)).Post("/password/reset/request", h.PasswordResetRequest)
		r.With(middleware.RateLimit(h.limiter, "reset_confirm_ip", 20, time.Minute, h.cfg.TrustProxy)).Post("/password/reset/confirm", h.PasswordResetConfirm)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Authn(h.svc, h.cfg.SessionCookieName, h.cfg.TrustProxy))
			r.Get("/me", h.Me)
			r.Group(func(r chi.Router) {
				r.Use(middleware.CSRFFromCookie(h.cfg.CSRFCookieName))
				r.Post("/session/mail-secret/unlock", h.V1SessionMailSecretUnlock)
			})
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireMFAStageAuthenticated(h.svc))
				r.Get("/mailboxes", h.ListMailboxes)
				r.Get("/mailboxes/special", h.ListSpecialMailboxes)
				r.Get("/messages", h.ListMessages)
				r.Get("/messages/{id}", h.GetMessage)
				r.Get("/messages/{id}/raw", h.GetMessageRaw)
				r.Get("/messages/{id}/remote-image", h.GetMessageRemoteImage)
				r.Get("/threads/{id}/messages", h.ListThreadMessages)
				r.Get("/compose/identities", h.ComposeIdentities)
				r.Get("/search", h.Search)
				r.Get("/attachments/{id}", h.GetAttachment)

				r.Group(func(r chi.Router) {
					r.Use(middleware.CSRFFromCookie(h.cfg.CSRFCookieName))
					r.Post("/me/recovery-email", h.MeUpdateRecoveryEmail)
					r.With(middleware.RateLimit(h.limiter, "send", 30, time.Minute, h.cfg.TrustProxy)).Post("/messages/send", h.SendMessage)
					r.With(middleware.RateLimit(h.limiter, "send", 30, time.Minute, h.cfg.TrustProxy)).Post("/messages/{id}/reply", h.ReplyMessage)
					r.With(middleware.RateLimit(h.limiter, "send", 30, time.Minute, h.cfg.TrustProxy)).Post("/messages/{id}/forward", h.ForwardMessage)
					r.Post("/messages/{id}/unsubscribe", h.V1UnsubscribeMessage)
					r.Post("/messages/{id}/flags", h.SetMessageFlags)
					r.Post("/messages/{id}/move", h.MoveMessage)
					r.Post("/mailboxes", h.CreateMailbox)
					r.Patch("/mailboxes", h.RenameMailbox)
					r.Delete("/mailboxes", h.DeleteMailbox)
					r.Post("/mailboxes/special/{role}", h.UpsertSpecialMailbox)
				})

				r.Route("/admin", func(r chi.Router) {
					r.Use(middleware.AdminOnly)
					r.Get("/registrations", h.AdminListRegistrations)
					r.Get("/users", h.AdminListUsers)
					r.Get("/users/{id}/mailboxes", h.AdminListUserMailboxes)
					r.Get("/audit-log", h.AdminAuditLog)
					r.Get("/system/mail-health", h.AdminMailHealth)
					r.Get("/system/version", h.AdminVersion)
					r.Get("/system/update/status", h.AdminUpdateStatus)
					r.Get("/system/feature-flags", h.AdminListFeatureFlags)
					r.Group(func(r chi.Router) {
						r.Use(middleware.CSRFFromCookie(h.cfg.CSRFCookieName))
						r.Post("/registrations/{id}/approve", h.AdminApproveRegistration)
						r.Post("/registrations/{id}/reject", h.AdminRejectRegistration)
						r.Post("/registrations/bulk/decision", h.AdminBulkRegistrationDecision)
						r.Post("/users", h.AdminCreateUser)
						r.Post("/users/{id}/mailboxes", h.AdminCreateUserMailbox)
						r.Post("/users/{id}/suspend", h.AdminSuspendUser)
						r.Post("/users/{id}/unsuspend", h.AdminUnsuspendUser)
						r.Post("/users/bulk/action", h.AdminBulkUserAction)
						r.Post("/users/{id}/reset-password", h.AdminResetPassword)
						r.Post("/users/{id}/retry-provision", h.AdminRetryProvisionUser)
						r.Delete("/users/{id}/mailboxes", h.AdminDeleteUserMailbox)
						r.With(middleware.RateLimit(h.limiter, "update_check", 20, time.Minute, h.cfg.TrustProxy)).Post("/system/update/check", h.AdminUpdateCheck)
						r.With(middleware.RateLimit(h.limiter, "update_apply", 10, time.Minute, h.cfg.TrustProxy)).Post("/system/update/apply", h.AdminUpdateApply)
						r.With(middleware.RateLimit(h.limiter, "update_auto", 20, time.Minute, h.cfg.TrustProxy)).Post("/system/update/automatic", h.AdminUpdateAutomatic)
						r.With(middleware.RateLimit(h.limiter, "update_cancel_scheduled", 20, time.Minute, h.cfg.TrustProxy)).Post("/system/update/cancel-scheduled", h.AdminUpdateCancelScheduled)
						r.Post("/system/feature-flags/{id}", h.AdminSetFeatureFlag)
						r.Post("/system/feature-flags/{id}/reset", h.AdminResetFeatureFlag)
					})
				})
			})
		})
	})
	r.Route("/api/v2", func(r chi.Router) {
		r.With(middleware.RateLimit(h.limiter, "login_v2", 20, time.Minute, h.cfg.TrustProxy)).Post("/login", h.V2Login)
		r.With(middleware.RateLimit(h.limiter, "login_passkey_begin_v2", 30, time.Minute, h.cfg.TrustProxy)).Post("/login/passkey/begin", h.V2PasskeyLoginBegin)
		r.With(middleware.RateLimit(h.limiter, "login_passkey_finish_v2", 30, time.Minute, h.cfg.TrustProxy)).Post("/login/passkey/finish", h.V2PasskeyLoginFinish)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Authn(h.svc, h.cfg.SessionCookieName, h.cfg.TrustProxy))

			r.Group(func(r chi.Router) {
				r.Use(middleware.CSRFFromCookie(h.cfg.CSRFCookieName))
				r.Post("/session/mail-secret/unlock", h.V2SessionMailSecretUnlock)
				r.Post("/mfa/totp/verify", h.V2MFATOTPVerify)
				r.Post("/mfa/webauthn/begin", h.V2MFAWebAuthnBegin)
				r.Post("/mfa/webauthn/finish", h.V2MFAWebAuthnFinish)
				r.Post("/mfa/recovery-code/verify", h.V2MFARecoveryCodeVerify)
				r.With(middleware.RateLimit(h.limiter, "mfa_recovery_email_send_v2", 5, 10*time.Minute, h.cfg.TrustProxy)).Post("/mfa/recovery-email/send", h.V2MFARecoveryEmailSend)
				r.With(middleware.RateLimit(h.limiter, "mfa_recovery_email_verify_v2", 20, 10*time.Minute, h.cfg.TrustProxy)).Post("/mfa/recovery-email/verify", h.V2MFARecoveryEmailVerify)
				r.With(middleware.RateLimit(h.limiter, "mfa_device_approval_request_v2", 10, 10*time.Minute, h.cfg.TrustProxy)).Post("/mfa/device-approval/request", h.V2MFADeviceApprovalRequest)
				r.Post("/mfa/device-approval/cancel", h.V2MFADeviceApprovalCancel)
				r.Post("/security/mfa/totp/enroll", h.V2MFAEnrollTOTP)
				r.Post("/security/mfa/totp/confirm", h.V2MFAConfirmTOTP)
				r.Post("/security/mfa/webauthn/register/begin", h.V2MFAWebAuthnRegisterBegin)
				r.Post("/security/mfa/webauthn/register/finish", h.V2MFAWebAuthnRegisterFinish)
				r.Post("/security/mfa/recovery-codes/ack", h.V2MFARecoveryCodesAck)
				r.Post("/security/mfa/preference", h.V2MFAUpdatePreference)
				r.Post("/security/mfa/legacy-dismiss", h.V2MFALegacyDismiss)
			})

			r.Get("/mfa/device-approval/status", h.V2MFADeviceApprovalStatus)
			r.Get("/security/mfa/status", h.V2GetMFAStatus)
			r.Get("/security/mfa/webauthn", h.V2MFAWebAuthnList)

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireMFAStageAuthenticated(h.svc))
				r.Get("/accounts", h.V2ListAccounts)
				r.Get("/mail/providers", h.V2ListMailProviders)
				r.Get("/accounts/health", h.V2ListAccountHealth)
				r.Get("/accounts/{id}/mailboxes", h.V2ListAccountMailboxes)
				r.Get("/accounts/{id}/identities", h.V2ListIdentities)
				r.Get("/accounts/{id}/rules", h.V2ListAccountRules)
				r.Get("/mail/senders", h.V2ListSenders)
				r.Get("/mail/session-profile", h.V2GetSessionMailProfile)
				r.Get("/contacts", h.V2ListContacts)
				r.Get("/contacts/{id}", h.V2GetContact)
				r.Get("/contacts/export", h.V2ExportContacts)
				r.Get("/contact-groups", h.V2ListContactGroups)
				r.Get("/contact-groups/{id}", h.V2GetContactGroup)
				r.Get("/mailboxes", h.V2ListMailboxMappings)
				r.Get("/mailboxes/aggregate", h.V2ListAggregateMailboxes)
				r.Get("/threads", h.V2ListThreads)
				r.Get("/threads/{id}", h.V2GetThread)
				r.Get("/messages", h.V2ListMessages)
				r.Get("/messages/{id}", h.V2GetIndexedMessage)
				r.Get("/messages/{id}/attachments/{attachment_id}", h.V2GetIndexedMessageAttachment)
				r.Get("/messages/{id}/raw", h.V2GetIndexedMessageRaw)
				r.Get("/mail-triage/catalog", h.V2ListMailTriageCatalog)
				r.Get("/mail-triage/reminders/due", h.V2PollDueMailTriageReminders)
				r.Get("/recipients/suggest", h.V2SuggestRecipients)
				r.Get("/search", h.V2Search)
				r.Get("/saved-searches", h.V2ListSavedSearches)
				r.Get("/funnels", h.V2ListReplyFunnels)
				r.Get("/funnels/{id}", h.V2GetReplyFunnel)
				r.Get("/outbound/campaigns", h.V2ListOutboundCampaigns)
				r.Get("/outbound/playbooks", h.V2ListOutboundPlaybooks)
				r.Get("/outbound/campaigns/{id}", h.V2GetOutboundCampaign)
				r.Get("/outbound/campaigns/{id}/steps", h.V2ListOutboundCampaignSteps)
				r.Get("/outbound/campaigns/{id}/events", h.V2ListOutboundCampaignEvents)
				r.Get("/outbound/campaigns/{id}/enrollments", h.V2ListOutboundEnrollments)
				r.Get("/outbound/enrollments/{id}", h.V2GetOutboundEnrollment)
				r.Get("/reply-ops/queue", h.V2ListReplyOpsQueue)
				r.Get("/reply-ops/queue/{bucket}", h.V2ListReplyOpsBucket)
				r.Get("/reply-ops/items/{id}", h.V2GetReplyOpsItem)
				r.Get("/outbound/recipients", h.V2ListOutboundRecipients)
				r.Get("/outbound/recipients/{email}", h.V2GetOutboundRecipient)
				r.Get("/outbound/suppressions", h.V2ListOutboundSuppressions)
				r.Get("/outbound/diagnostics/senders", h.V2OutboundSenderDiagnostics)
				r.Get("/outbound/diagnostics/domains", h.V2OutboundDomainDiagnostics)
				r.Get("/drafts", h.V2ListDrafts)
				r.Get("/drafts/{id}", h.V2GetDraft)
				r.Get("/drafts/{id}/attachments/{attachment_id}", h.V2GetDraftAttachment)
				r.Get("/drafts/{id}/versions", h.V2ListDraftVersions)
				r.Get("/rules/scripts", h.V2ListRuleScripts)
				r.Get("/rules/scripts/{name}", h.V2GetRuleScript)
				r.Get("/preferences", h.V2GetPreferences)
				r.Get("/security/sessions", h.V2ListSessions)
				r.Get("/security/mfa/trusted-devices", h.V2ListTrustedDevices)
				r.Get("/security/mfa/device-approvals/pending", h.V2ListPendingMFADeviceApprovals)
				r.Get("/quota", h.V2GetQuota)
				r.Get("/security/crypto/keyrings", h.V2ListCryptoKeyrings)
				r.Get("/security/crypto/trust-policies", h.V2ListCryptoTrustPolicies)

				r.Group(func(r chi.Router) {
					r.Use(middleware.CSRFFromCookie(h.cfg.CSRFCookieName))
					r.Post("/mail/providers/validate", h.V2ValidateMailProvider)
					r.Post("/accounts", h.V2CreateAccount)
					r.Patch("/accounts/{id}", h.V2UpdateAccount)
					r.Delete("/accounts/{id}", h.V2DeleteAccount)
					r.Post("/accounts/{id}/activate", h.V2ActivateAccount)
					r.Post("/accounts/{id}/health/sync", h.V2QueueAccountHealthSync)
					r.Post("/accounts/{id}/health/quota-refresh", h.V2QueueAccountQuotaRefresh)
					r.Post("/accounts/{id}/health/reindex", h.V2QueueAccountReindex)
					r.Post("/accounts/{id}/mailboxes", h.V2CreateAccountMailbox)
					r.Patch("/accounts/{id}/mailboxes", h.V2RenameAccountMailbox)
					r.Delete("/accounts/{id}/mailboxes", h.V2DeleteAccountMailbox)
					r.Post("/accounts/{id}/mailboxes/special/{role}", h.V2UpsertAccountSpecialMailbox)
					r.Post("/accounts/{id}/identities", h.V2CreateIdentity)
					r.Post("/accounts/{id}/rules", h.V2CreateAccountRule)
					r.Patch("/accounts/{id}/rules/{rule_id}", h.V2UpdateAccountRule)
					r.Delete("/accounts/{id}/rules/{rule_id}", h.V2DeleteAccountRule)
					r.Post("/accounts/{id}/rules/reorder", h.V2ReorderAccountRules)
					r.Post("/accounts/{id}/rules/activate-managed", h.V2ActivateManagedRuleScript)
					r.Post("/accounts/{id}/senders", h.V2CreateSender)
					r.Patch("/identities/{id}", h.V2UpdateIdentity)
					r.Patch("/mail/senders/{sender_id}", h.V2UpdateSender)
					r.Delete("/identities/{id}", h.V2DeleteIdentity)
					r.Delete("/mail/senders/{sender_id}", h.V2DeleteSender)
					r.Post("/contacts", h.V2CreateContact)
					r.Post("/contacts/import", h.V2ImportContacts)
					r.Patch("/contacts/{id}", h.V2UpdateContact)
					r.Delete("/contacts/{id}", h.V2DeleteContact)
					r.Post("/contact-groups", h.V2CreateContactGroup)
					r.Patch("/contact-groups/{id}", h.V2UpdateContactGroup)
					r.Delete("/contact-groups/{id}", h.V2DeleteContactGroup)
					r.Patch("/mail/session-profile", h.V2UpdateSessionMailProfile)

					r.Post("/mailboxes", h.V2UpsertMailboxMapping)
					r.Patch("/mailboxes/{id}", h.V2UpsertMailboxMapping)
					r.Delete("/mailboxes/{id}", h.V2DeleteMailboxMapping)

					r.Post("/messages/bulk", h.V2BulkMessages)
					r.Get("/mail/snippets", h.V2ListMailSnippets)
					r.Post("/mail/snippets", h.V2CreateMailSnippet)
					r.Patch("/mail/snippets/{id}", h.V2UpdateMailSnippet)
					r.Delete("/mail/snippets/{id}", h.V2DeleteMailSnippet)
					r.Get("/mail/favorites", h.V2ListMailFavorites)
					r.Post("/mail/favorites", h.V2CreateMailFavorite)
					r.Delete("/mail/favorites/{id}", h.V2DeleteMailFavorite)
					r.Post("/mail-triage/actions", h.V2ApplyMailTriage)
					r.Post("/mail-triage/categories", h.V2CreateMailTriageCategory)
					r.Patch("/mail-triage/categories/{id}", h.V2UpdateMailTriageCategory)
					r.Delete("/mail-triage/categories/{id}", h.V2DeleteMailTriageCategory)
					r.Post("/mail-triage/tags", h.V2CreateMailTriageTag)
					r.Patch("/mail-triage/tags/{id}", h.V2UpdateMailTriageTag)
					r.Delete("/mail-triage/tags/{id}", h.V2DeleteMailTriageTag)
					r.Post("/messages/{id}/remote-images/allow", h.V2AllowRemoteImages)
					r.Post("/messages/{id}/unsubscribe", h.V2UnsubscribeIndexedMessage)
					r.Post("/messages/{id}/sweep", h.V2SweepIndexedMessage)
					r.Post("/messages/{id}/crypto/decrypt", h.V2DecryptIndexedMessage)
					r.Post("/messages/{id}/crypto/verify", h.V2VerifyIndexedMessage)

					r.Post("/saved-searches", h.V2CreateSavedSearch)
					r.Patch("/saved-searches/{id}", h.V2UpdateSavedSearch)
					r.Delete("/saved-searches/{id}", h.V2DeleteSavedSearch)
					r.Post("/funnels", h.V2CreateReplyFunnel)
					r.Patch("/funnels/{id}", h.V2UpdateReplyFunnel)
					r.Patch("/funnels/{id}/assisted-forwarding/{account_id}", h.V2UpdateReplyFunnelAssistedForwarding)
					r.Delete("/funnels/{id}", h.V2DeleteReplyFunnel)
					r.Post("/outbound/campaigns", h.V2CreateOutboundCampaign)
					r.Patch("/outbound/campaigns/{id}", h.V2UpdateOutboundCampaign)
					r.Post("/outbound/campaigns/{id}/apply-playbook", h.V2ApplyOutboundPlaybook)
					r.Post("/outbound/campaigns/{id}/launch", h.V2LaunchOutboundCampaign)
					r.Post("/outbound/campaigns/{id}/pause", h.V2PauseOutboundCampaign)
					r.Post("/outbound/campaigns/{id}/resume", h.V2ResumeOutboundCampaign)
					r.Post("/outbound/campaigns/{id}/archive", h.V2ArchiveOutboundCampaign)
					r.Post("/outbound/campaigns/{id}/steps", h.V2CreateOutboundCampaignStep)
					r.Patch("/outbound/steps/{step_id}", h.V2UpdateOutboundCampaignStep)
					r.Delete("/outbound/steps/{step_id}", h.V2DeleteOutboundCampaignStep)
					r.Post("/outbound/campaigns/{id}/steps/reorder", h.V2ReorderOutboundCampaignSteps)
					r.Post("/outbound/campaigns/{id}/audience/preview", h.V2PreviewOutboundAudience)
					r.Post("/outbound/campaigns/{id}/enrollments/import", h.V2ImportOutboundEnrollments)
					r.Patch("/outbound/enrollments/{id}", h.V2UpdateOutboundEnrollment)
					r.Post("/outbound/enrollments/{id}/pause", h.V2PauseOutboundEnrollment)
					r.Post("/outbound/enrollments/{id}/resume", h.V2ResumeOutboundEnrollment)
					r.Post("/outbound/enrollments/{id}/stop", h.V2StopOutboundEnrollment)
					r.Post("/outbound/enrollments/{id}/assign", h.V2AssignOutboundEnrollment)
					r.Post("/outbound/campaigns/{id}/preflight", h.V2PreflightOutboundCampaign)
					r.Post("/reply-ops/items/{id}/classify", h.V2ClassifyReplyOpsItem)
					r.Post("/reply-ops/items/{id}/takeover", h.V2TakeoverReplyOpsItem)
					r.Post("/reply-ops/items/{id}/apply-action", h.V2ApplyReplyOpsAction)
					r.Patch("/outbound/recipients/{email}", h.V2UpdateOutboundRecipient)
					r.Post("/outbound/suppressions", h.V2CreateOutboundSuppression)
					r.Delete("/outbound/suppressions/{id}", h.V2DeleteOutboundSuppression)

					r.Post("/drafts", h.V2CreateDraft)
					r.Patch("/drafts/{id}", h.V2UpdateDraft)
					r.Delete("/drafts/{id}", h.V2DeleteDraft)
					r.Post("/drafts/{id}/attachments", h.V2UploadDraftAttachments)
					r.Delete("/drafts/{id}/attachments/{attachment_id}", h.V2DeleteDraftAttachment)
					r.Post("/drafts/{id}/send", h.V2SendDraft)
					r.Post("/drafts/{id}/undo-send", h.V2UndoDraftSend)
					r.With(middleware.RateLimit(h.limiter, "send_v2", 30, time.Minute, h.cfg.TrustProxy)).Post("/messages/send", h.V2SendMessage)

					r.Put("/rules/scripts/{name}", h.V2PutRuleScript)
					r.Post("/rules/scripts/{name}/activate", h.V2ActivateRuleScript)
					r.Delete("/rules/scripts/{name}", h.V2DeleteRuleScript)
					r.Post("/rules/validate", h.V2ValidateRuleScript)

					r.Put("/preferences", h.V2UpdatePreferences)
					r.Patch("/preferences", h.V2UpdatePreferences)
					r.Patch("/security/mfa/webauthn/{id}", h.V2MFAWebAuthnRename)
					r.Delete("/security/mfa/webauthn/{id}", h.V2MFAWebAuthnDelete)
					r.Post("/security/mfa/device-approvals/{id}/decision", h.V2DecidePendingMFADeviceApproval)
					r.Post("/security/mfa/trusted-devices/revoke-all", h.V2RevokeAllTrustedDevices)
					r.Post("/security/mfa/trusted-devices/{id}/revoke", h.V2RevokeTrustedDevice)
					r.Post("/security/sessions/{id}/revoke", h.V2RevokeSession)
					r.Post("/security/crypto/keyrings", h.V2CreateCryptoKeyring)
					r.Patch("/security/crypto/keyrings/{id}", h.V2UpdateCryptoKeyring)
					r.Delete("/security/crypto/keyrings/{id}", h.V2DeleteCryptoKeyring)
					r.Post("/security/crypto/trust-policies", h.V2CreateCryptoTrustPolicy)
					r.Patch("/security/crypto/trust-policies/{id}", h.V2UpdateCryptoTrustPolicy)
					r.Delete("/security/crypto/trust-policies/{id}", h.V2DeleteCryptoTrustPolicy)
				})
			})
		})
	})

	fs := http.FileServer(http.Dir("web"))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/health/") {
			http.NotFound(w, r)
			return
		}
		if p == "/" {
			http.ServeFile(w, r, filepath.Join("web", "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})

	return r
}

type registerRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	RecoveryEmail string `json:"recovery_email"`
	CaptchaToken  string `json:"captcha_token"`
	MFAPreference string `json:"mfa_preference"`
}

func (h *Handlers) PublicCaptchaConfig(w http.ResponseWriter, r *http.Request) {
	mode := "disabled"
	provider := ""
	siteKey := ""
	widgetAPIURL := ""
	if h.cfg.CaptchaEnabled {
		mode = "required"
		provider = strings.ToLower(strings.TrimSpace(h.cfg.CaptchaProvider))
		if provider == "cap" {
			siteKey = strings.TrimSpace(h.cfg.CaptchaSiteKey)
			widgetAPIURL = strings.TrimSpace(h.cfg.CaptchaWidgetURL)
		}
	}
	util.WriteJSON(w, 200, map[string]any{
		"enabled":        h.cfg.CaptchaEnabled,
		"provider":       provider,
		"site_key":       siteKey,
		"widget_api_url": widgetAPIURL,
		"mode":           mode,
	})
}

func (h *Handlers) PublicPasswordResetCapabilities(w http.ResponseWriter, r *http.Request) {
	caps := h.svc.PasswordResetCapabilities(r.Context())
	util.WriteJSON(w, 200, caps)
}

func (h *Handlers) PublicAuthCapabilities(w http.ResponseWriter, r *http.Request) {
	util.WriteJSON(w, 200, h.authCapabilities(r))
}

func (h *Handlers) PublicI18nBundle(w http.ResponseWriter, r *http.Request) {
	if h.webI18n == nil {
		http.NotFound(w, r)
		return
	}
	bundle, ok := h.webI18n.BundleFor(chi.URLParam(r, "locale"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	util.WriteJSON(w, http.StatusOK, bundle)
}

func (h *Handlers) SetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.SetupStatus(r.Context())
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if enabled, autoErr := h.updateMgr.AutomaticEnabled(r.Context(), h.svc.Store()); autoErr == nil {
		status.AutomaticUpdatesEnabled = enabled
	} else {
		status.AutomaticUpdatesEnabled = true
	}
	util.WriteJSON(w, 200, status)
}

func (h *Handlers) SetupComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseDomain                  string `json:"base_domain"`
		AdminEmail                  string `json:"admin_email"`
		AdminIdentifier             string `json:"admin_identifier"`
		AdminRecoveryEmail          string `json:"admin_recovery_email"`
		AdminMailboxLogin           string `json:"admin_mailbox_login"`
		AdminPassword               string `json:"admin_password"`
		DefaultFormatLocale         string `json:"default_format_locale"`
		DefaultTimezone             string `json:"default_timezone"`
		InstanceMode                string `json:"instance_mode"`
		PasskeyPrimarySignInEnabled *bool  `json:"passkey_primary_sign_in_enabled"`
		AutomaticUpdatesEnabled     *bool  `json:"automatic_updates_enabled"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	token, user, err := h.svc.CompleteSetup(r.Context(), service.SetupCompleteRequest{
		BaseDomain:                  req.BaseDomain,
		AdminEmail:                  firstNonEmptyString(req.AdminIdentifier, req.AdminEmail),
		AdminRecoveryEmail:          req.AdminRecoveryEmail,
		AdminMailboxLogin:           req.AdminMailboxLogin,
		AdminPassword:               req.AdminPassword,
		DefaultFormatLocale:         req.DefaultFormatLocale,
		DefaultTimezone:             req.DefaultTimezone,
		InstanceMode:                req.InstanceMode,
		PasskeyPrimarySignInEnabled: req.PasskeyPrimarySignInEnabled,
	}, r.RemoteAddr, r.UserAgent())
	if err != nil {
		msg := err.Error()
		status := http.StatusBadRequest
		code := "setup_failed"
		failureClass := "setup_failed"
		pamAttempts := ""
		pamAttemptCount := 0
		var pamCredErr *service.PAMCredentialsInvalidError
		switch {
		case strings.EqualFold(strings.TrimSpace(msg), "setup already completed"):
			status = http.StatusConflict
			code = "setup_already_complete"
			failureClass = "setup_already_complete"
		case errors.Is(err, service.ErrPAMVerifierDown):
			status = http.StatusBadGateway
			code = "pam_verifier_unavailable"
			failureClass = "verifier_unavailable"
			msg = "cannot validate PAM credentials because IMAP connectivity failed; check IMAP_HOST/IMAP_PORT/IMAP_TLS/IMAP_STARTTLS"
			lowerErr := strings.ToLower(err.Error())
			if strings.Contains(lowerErr, "x509") || strings.Contains(lowerErr, "certificate") || strings.Contains(lowerErr, "tls") {
				msg = "IMAP TLS verification failed while validating PAM credentials. If using IMAP_HOST=127.0.0.1, set IMAP_INSECURE_SKIP_VERIFY=true or set IMAP_HOST to your mail FQDN."
			}
		case strings.Contains(strings.ToLower(msg), "invalid domain"):
			code = "invalid_domain"
		case strings.Contains(strings.ToLower(msg), "invalid admin email"):
			code = "invalid_admin_email"
		case strings.Contains(strings.ToLower(msg), "admin username"):
			code = "invalid_admin_identifier"
		case strings.Contains(strings.ToLower(msg), "must use @"):
			code = "admin_email_domain_mismatch"
		case errors.Is(err, service.ErrRecoveryEmailRequired):
			code = "recovery_email_required"
			msg = "recovery email is required"
		case errors.Is(err, service.ErrRecoveryEmailMatchesLogin):
			code = "recovery_email_matches_login"
			msg = "recovery email must differ from admin email"
		case errors.Is(err, service.ErrInvalidRecoveryEmail):
			code = "invalid_recovery_email"
			msg = "enter a valid recovery email"
		case strings.Contains(strings.ToLower(msg), "format locale"):
			code = "invalid_format_locale"
			msg = "enter a valid regional format locale"
		case strings.Contains(strings.ToLower(msg), "invalid timezone"):
			code = "invalid_timezone"
			msg = "enter a valid IANA time zone"
		case strings.Contains(strings.ToLower(msg), "password"):
			code = "invalid_password"
		case errors.As(err, &pamCredErr):
			code = "pam_credentials_invalid"
			failureClass = "invalid_identity_or_password"
			msg = "PAM auth mode is enabled. The password or mailbox login identity is invalid."
			if pamCredErr != nil && len(pamCredErr.Attempts) > 0 {
				pamAttemptCount = len(pamCredErr.Attempts)
				pamAttempts = strings.Join(pamCredErr.Attempts, ",")
				msg = fmt.Sprintf("%s Attempted logins: %s.", msg, strings.Join(pamCredErr.Attempts, ", "))
			}
		case strings.Contains(strings.ToLower(msg), "dovecot/pam"):
			code = "pam_credentials_invalid"
			failureClass = "invalid_identity_or_password"
			msg = "PAM auth mode is enabled. The password or mailbox login identity is invalid."
		}
		log.Printf("setup_complete_failed code=%s class=%s status=%d admin_identifier=%s base_domain=%s instance_mode=%s request_id=%s pam_attempt_count=%d pam_attempts=%q err=%q",
			code,
			failureClass,
			status,
			strings.ToLower(strings.TrimSpace(firstNonEmptyString(req.AdminIdentifier, req.AdminEmail))),
			strings.ToLower(strings.TrimSpace(req.BaseDomain)),
			service.NormalizeInstanceMode(req.InstanceMode),
			middleware.RequestID(r.Context()),
			pamAttemptCount,
			pamAttempts,
			err.Error(),
		)
		util.WriteError(w, status, code, msg, middleware.RequestID(r.Context()))
		return
	}
	autoEnabled := true
	if req.AutomaticUpdatesEnabled != nil {
		autoEnabled = *req.AutomaticUpdatesEnabled
	}
	if err := h.updateMgr.PersistAutomaticPreference(r.Context(), h.svc.Store(), user.Email, autoEnabled); err != nil {
		util.WriteError(w, 500, "internal_error", "cannot persist automatic update preference", middleware.RequestID(r.Context()))
		return
	}
	if autoEnabled {
		_ = h.updateMgr.AutomaticTick(r.Context(), h.svc.Store())
	}
	csrfToken, err := randomToken()
	if err != nil {
		util.WriteError(w, 500, "internal_error", "failed to generate token", middleware.RequestID(r.Context()))
		return
	}
	sess, stage, err := h.resolveLoginStage(r.Context(), w, r, token, user)
	if err != nil {
		util.WriteError(w, 500, "internal_error", "cannot finalize setup session", middleware.RequestID(r.Context()))
		return
	}
	h.setAuthCookies(w, r, token, csrfToken)
	out := map[string]any{
		"status":     "ok",
		"user_id":    user.ID,
		"email":      user.Email,
		"identifier": user.Email,
		"role":       user.Role,
		"csrf_token": csrfToken,
	}
	applyAuthStageFields(out, stage)
	out["mail_secret_required"] = strings.TrimSpace(sess.MailSecret) == ""
	h.applyUserInterfacePreferenceFields(r.Context(), out, user.ID)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	if mode, err := h.svc.InstanceMode(r.Context()); err != nil {
		util.WriteError(w, 500, "internal_error", "cannot resolve instance mode", middleware.RequestID(r.Context()))
		return
	} else if service.NormalizeInstanceMode(mode) == service.InstanceModeExternalAccounts {
		util.WriteError(w, 403, "registration_disabled", "account registration is unavailable in external accounts mode", middleware.RequestID(r.Context()))
		return
	}
	var req registerRequest
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	captchaOK := !h.cfg.CaptchaEnabled
	if h.cfg.CaptchaEnabled {
		ip := middleware.ClientIP(r, h.cfg.TrustProxy)
		if err := h.captchaVerifier.Verify(r.Context(), req.CaptchaToken, ip); err != nil {
			if errors.Is(err, captcha.ErrCaptchaUnavailable) {
				log.Printf("captcha_verify_failed code=captcha_unavailable class=verifier_unavailable status=503 request_id=%s err=%q",
					middleware.RequestID(r.Context()),
					err.Error(),
				)
				util.WriteError(w, 503, "captcha_unavailable", "captcha verification service unavailable", middleware.RequestID(r.Context()))
				return
			}
			log.Printf("captcha_verify_failed code=captcha_required class=invalid_or_missing_token status=400 request_id=%s err=%q",
				middleware.RequestID(r.Context()),
				err.Error(),
			)
			util.WriteError(w, 400, "captcha_required", "captcha validation failed", middleware.RequestID(r.Context()))
			return
		}
		captchaOK = true
	}
	if err := h.svc.Register(r.Context(), req.Email, req.Password, req.RecoveryEmail, r.RemoteAddr, r.UserAgent(), captchaOK, req.MFAPreference); err != nil {
		switch {
		case errors.Is(err, service.ErrRecoveryEmailRequired):
			util.WriteError(w, 400, "recovery_email_required", "recovery_email is required", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrRecoveryEmailMatchesLogin):
			util.WriteError(w, 400, "recovery_email_matches_login", "recovery_email must differ from email", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrInvalidRecoveryEmail):
			util.WriteError(w, 400, "invalid_recovery_email", "valid recovery_email is required", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 400, "register_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 201, map[string]string{"status": "pending_approval"})
}

type loginRequest struct {
	Email      string `json:"email"`
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeLocaleTag(value string) string {
	raw := strings.TrimSpace(strings.ReplaceAll(value, "_", "-"))
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "-")
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if index == 0 {
			parts[index] = strings.ToLower(part)
			continue
		}
		if len(part) == 2 {
			parts[index] = strings.ToUpper(part)
			continue
		}
		parts[index] = strings.ToLower(part)
	}
	return strings.Join(parts, "-")
}

func normalizeTimeZoneName(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	loc, err := time.LoadLocation(raw)
	if err != nil {
		return ""
	}
	return loc.String()
}

func (h *Handlers) applyUserInterfacePreferenceFields(ctx context.Context, out map[string]any, userID string) {
	if out == nil {
		return
	}
	if h == nil || h.svc == nil {
		return
	}
	prefs, err := h.svc.Store().GetUserPreferences(ctx, userID)
	if err != nil {
		return
	}
	out["locale"] = normalizeLocaleTag(prefs.Locale)
	out["format_locale"] = normalizeLocaleTag(prefs.FormatLocale)
	out["timezone"] = normalizeTimeZoneName(prefs.Timezone)
}

func loginRequestIdentifier(req loginRequest) string {
	return firstNonEmptyString(req.Identifier, req.Email)
}

func applyAuthStageFields(out map[string]any, stage service.MFAStage) {
	out["auth_stage"] = stage.AuthStage
	out["mfa_required"] = stage.MFARequired
	out["mfa_setup_required"] = stage.MFASetupRequired
	out["mfa_setup_method"] = stage.MFASetupMethod
	out["mfa_setup_step"] = stage.MFASetupStep
	out["mfa_enrolled"] = stage.MFAEnrolled
	out["legacy_mfa_prompt"] = stage.LegacyMFAPrompt
	out["mfa_preference"] = stage.MFAPreference
	out["mfa_trusted_supported"] = true
}

func (h *Handlers) resolveLoginStage(ctx context.Context, w http.ResponseWriter, r *http.Request, token string, user models.User) (models.Session, service.MFAStage, error) {
	sum := sha256.Sum256([]byte(token))
	sess, err := h.svc.Store().GetSessionByTokenHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return models.Session{}, service.MFAStage{}, err
	}
	stage, err := h.svc.ResolveMFAStage(ctx, user, &sess)
	if err != nil {
		return models.Session{}, service.MFAStage{}, err
	}
	if stage.AuthStage == service.AuthStageMFARequired {
		trustedOK, err := h.tryAuthenticateTrustedDevice(ctx, w, r, user, sess)
		if err != nil {
			return models.Session{}, service.MFAStage{}, err
		}
		if trustedOK {
			sess, err = h.svc.Store().GetSessionByTokenHash(ctx, hex.EncodeToString(sum[:]))
			if err != nil {
				return models.Session{}, service.MFAStage{}, err
			}
			stage, err = h.svc.ResolveMFAStage(ctx, user, &sess)
			if err != nil {
				return models.Session{}, service.MFAStage{}, err
			}
		}
	}
	if stage.AuthStage != service.AuthStageAuthenticated {
		if err := h.svc.Store().ClearSessionMFAVerified(ctx, sess.ID); err != nil {
			return models.Session{}, service.MFAStage{}, err
		}
		sess.MFAVerifiedAt = nil
		return sess, stage, nil
	}
	if sess.MFAVerifiedAt == nil {
		if err := h.svc.Store().SetSessionMFAVerified(ctx, sess.ID, "password"); err != nil {
			return models.Session{}, service.MFAStage{}, err
		}
		now := time.Now().UTC()
		sess.MFAVerifiedAt = &now
	}
	return sess, stage, nil
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	var req loginRequest
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	identifier := loginRequestIdentifier(req)
	token, user, err := h.svc.Login(r.Context(), identifier, req.Password, r.RemoteAddr, r.UserAgent())
	if err != nil {
		if errors.Is(err, service.ErrPAMVerifierDown) {
			msg := "cannot validate PAM credentials because IMAP connectivity failed; check IMAP_HOST/IMAP_PORT/IMAP_TLS/IMAP_STARTTLS"
			lowerErr := strings.ToLower(err.Error())
			if strings.Contains(lowerErr, "x509") || strings.Contains(lowerErr, "certificate") || strings.Contains(lowerErr, "tls") {
				msg = "IMAP TLS verification failed while validating PAM credentials. If using IMAP_HOST=127.0.0.1, set IMAP_INSECURE_SKIP_VERIFY=true or set IMAP_HOST to your mail FQDN."
			}
			log.Printf("login_failed code=pam_verifier_unavailable class=verifier_unavailable status=502 email=%s request_id=%s err=%q",
				strings.ToLower(strings.TrimSpace(identifier)),
				middleware.RequestID(r.Context()),
				err.Error(),
			)
			util.WriteError(w, http.StatusBadGateway, "pam_verifier_unavailable", msg, middleware.RequestID(r.Context()))
			return
		}

		normalizedEmail := strings.ToLower(strings.TrimSpace(identifier))
		ip := middleware.ClientIP(r, h.cfg.TrustProxy)
		key := ip + "|" + normalizedEmail
		windowStart := time.Now().UTC().Truncate(15 * time.Minute)
		failCount, _ := h.svc.Store().IncrementRateEvent(r.Context(), key, "login_failed", windowStart)
		_ = h.svc.Store().CleanupRateEventsBefore(r.Context(), time.Now().UTC().Add(-24*time.Hour))
		if failCount > 3 {
			backoff := time.Duration(1<<(minInt(failCount-3, 5))) * time.Second
			select {
			case <-time.After(backoff):
			case <-r.Context().Done():
			}
		}

		status := 401
		code := "invalid_credentials"
		if failCount > 6 {
			status, code = 429, "rate_limited"
		}
		if err == service.ErrPendingApproval {
			status, code = 403, "pending_approval"
		}
		if err == service.ErrSuspended {
			status, code = 403, "suspended"
		}
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	normalizedEmail := strings.ToLower(strings.TrimSpace(identifier))
	ip := middleware.ClientIP(r, h.cfg.TrustProxy)
	_ = h.svc.Store().DeleteRateEvents(r.Context(), ip+"|"+normalizedEmail, "login_failed")

	if sess, stage, err := h.resolveLoginStage(r.Context(), w, r, token, user); err != nil {
		util.WriteError(w, 500, "internal_error", "cannot finalize login session", middleware.RequestID(r.Context()))
		return
	} else {
		csrfToken, tokenErr := randomToken()
		if tokenErr != nil {
			util.WriteError(w, 500, "internal_error", "failed to generate token", middleware.RequestID(r.Context()))
			return
		}
		h.setAuthCookies(w, r, token, csrfToken)
		payload := map[string]any{"user_id": user.ID, "email": user.Email, "identifier": user.Email, "role": user.Role, "csrf_token": csrfToken}
		applyAuthStageFields(payload, stage)
		payload["mail_secret_required"] = strings.TrimSpace(sess.MailSecret) == ""
		h.applyUserInterfacePreferenceFields(r.Context(), payload, user.ID)
		util.WriteJSON(w, 200, payload)
		return
	}
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(h.cfg.SessionCookieName)
	if c != nil && c.Value != "" {
		_ = h.svc.Logout(r.Context(), c.Value)
	}
	h.clearAuthCookies(w, r)
	util.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handlers) PasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	norm := strings.ToLower(strings.TrimSpace(req.Email))
	if !h.allowResetRateKey(r.Context(), "reset_request_ident", h.resetIdentifierRateKey(norm), 6, 15*time.Minute) {
		util.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}
	if err := h.svc.RequestPasswordReset(r.Context(), req.Email); err != nil {
		if errors.Is(err, service.ErrPasswordResetUnavailable) {
			util.WriteError(w, http.StatusServiceUnavailable, "password_reset_unavailable", "password reset is currently unavailable", middleware.RequestID(r.Context()))
			return
		}
		// Keep request behavior generic for public callers.
		log.Printf("password_reset_request_soft_error request_id=%s email_hash=%s err=%q", middleware.RequestID(r.Context()), h.resetIdentifierRateKey(norm), err.Error())
		util.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}
	util.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handlers) PasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	if !h.ensureSetupComplete(w, r) {
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	tokenKey := h.resetTokenPrefixRateKey(req.Token)
	if tokenKey != "" && !h.allowResetRateKey(r.Context(), "reset_confirm_token", tokenKey, 12, 15*time.Minute) {
		util.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.ConfirmPasswordReset(r.Context(), req.Token, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrPasswordResetUnavailable) {
			util.WriteError(w, http.StatusServiceUnavailable, "password_reset_unavailable", "password reset is currently unavailable", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrPasswordResetHelperDown) {
			util.WriteError(w, http.StatusServiceUnavailable, "password_reset_helper_unavailable", "password reset helper is unavailable", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrPasswordResetHelperFailed) {
			util.WriteError(w, http.StatusBadGateway, "password_reset_helper_failed", "password reset helper failed", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "reset_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "updated"})
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	sess, _ := middleware.Session(r.Context())
	recoveryEmail := ""
	if u.RecoveryEmail != nil {
		recoveryEmail = strings.TrimSpace(*u.RecoveryEmail)
	}
	stage, err := h.svc.ResolveMFAStage(r.Context(), u, &sess)
	if err != nil {
		util.WriteError(w, 500, "internal_error", "cannot resolve mfa stage", middleware.RequestID(r.Context()))
		return
	}
	out := map[string]any{
		"id":                   u.ID,
		"session_id":           sess.ID,
		"email":                u.Email,
		"role":                 u.Role,
		"status":               u.Status,
		"recovery_email":       recoveryEmail,
		"needs_recovery_email": h.svc.RecoveryEmailNeedsSetup(u),
		"mail_secret_required": strings.TrimSpace(sess.MailSecret) == "",
	}
	applyAuthStageFields(out, stage)
	h.applyUserInterfacePreferenceFields(r.Context(), out, u.ID)
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) MeUpdateRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		RecoveryEmail string `json:"recovery_email"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	normalized, err := h.svc.UpdateRecoveryEmail(r.Context(), u.ID, req.RecoveryEmail)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRecoveryEmail) || errors.Is(err, service.ErrRecoveryEmailRequired) {
			util.WriteError(w, 400, "invalid_recovery_email", "valid recovery_email is required", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrRecoveryEmailMatchesLogin) {
			util.WriteError(w, 400, "recovery_email_matches_login", "recovery_email must differ from email", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"status":               "ok",
		"recovery_email":       normalized,
		"needs_recovery_email": false,
	})
}

func (h *Handlers) ListMailboxes(w http.ResponseWriter, r *http.Request) {
	items, _, _, _, err := h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, 500, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, items)
}

func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	mbox := r.URL.Query().Get("mailbox")
	if mbox == "" {
		mbox = "INBOX"
	}
	page, pageSize := parsePagination(r)
	mailLogin := service.MailAuthLogin(u)
	filter := parseMailTriageOnlyFilter(r)
	items, err := h.listLiveMessagesWithTriage(r.Context(), u.ID, mailLogin, pass, mbox, "", page, pageSize, filter)
	if err != nil {
		util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"page": page, "page_size": pageSize, "items": items})
}

func (h *Handlers) GetMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	mailLogin := service.MailAuthLogin(u)
	msg, err := h.svc.Mail().GetMessage(r.Context(), mailLogin, pass, id)
	if err != nil {
		util.WriteError(w, 404, "not_found", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	msg = presentLiveMessage(msg)
	msg, err = decorateLiveMessageWithTriageStore(r.Context(), h, u.ID, msg)
	if err != nil {
		util.WriteError(w, 500, "mail_triage_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if strings.TrimSpace(msg.BodyHTML) != "" {
		msg.BodyHTML = rewriteMessageHTML(id, msg.BodyHTML, msg.Attachments)
	}
	util.WriteJSON(w, 200, msg)
}

func (h *Handlers) GetMessageRaw(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	mailLogin := service.MailAuthLogin(u)
	raw, err := h.svc.Mail().GetRawMessage(r.Context(), mailLogin, pass, id)
	if err != nil {
		util.WriteError(w, 404, "not_found", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	writeRawMessageResponse(w, r, raw, id)
}

func (h *Handlers) GetMessageRemoteImage(w http.ResponseWriter, r *http.Request) {
	if _, err := h.sessionMailPassword(r); err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}

	messageID := strings.TrimSpace(chi.URLParam(r, "id"))
	if messageID == "" {
		util.WriteError(w, 400, "bad_request", "message id is required", middleware.RequestID(r.Context()))
		return
	}
	if _, _, err := mail.DecodeMessageID(messageID); err != nil {
		util.WriteError(w, 400, "bad_request", "invalid message id", middleware.RequestID(r.Context()))
		return
	}

	remoteURL, err := validateRemoteImageTarget(r.Context(), r.URL.Query().Get("url"))
	if err != nil {
		util.WriteError(w, 400, "bad_request", err.Error(), middleware.RequestID(r.Context()))
		return
	}

	client := remoteImageHTTPClientFactory()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		util.WriteError(w, 400, "bad_request", "invalid remote image request", middleware.RequestID(r.Context()))
		return
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", "despatch-mail-image-proxy/1.0")

	resp, err := client.Do(req)
	if err != nil {
		util.WriteError(w, 502, "remote_image_fetch_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		util.WriteError(w, 502, "remote_image_fetch_failed", fmt.Sprintf("remote server returned HTTP %d", resp.StatusCode), middleware.RequestID(r.Context()))
		return
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if contentType == "" || !strings.HasPrefix(contentType, "image/") {
		util.WriteError(w, 415, "unsupported_media_type", "remote resource is not an image", middleware.RequestID(r.Context()))
		return
	}

	if rawLen := strings.TrimSpace(resp.Header.Get("Content-Length")); rawLen != "" {
		if n, parseErr := strconv.ParseInt(rawLen, 10, 64); parseErr == nil && n > mailRemoteImageMaxBytes {
			util.WriteError(w, 413, "remote_image_too_large", fmt.Sprintf("remote image exceeds %d bytes", mailRemoteImageMaxBytes), middleware.RequestID(r.Context()))
			return
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	copyWriter := &maxBytesCopyWriter{w: w, maxBytes: mailRemoteImageMaxBytes}
	if _, err := io.Copy(copyWriter, resp.Body); err != nil && !errors.Is(err, errRemoteImageMaxBytesExceeded) {
		return
	}
}

func (h *Handlers) ListThreadMessages(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}

	threadID := strings.TrimSpace(chi.URLParam(r, "id"))
	if threadID == "" {
		util.WriteError(w, 400, "bad_request", "thread id is required", middleware.RequestID(r.Context()))
		return
	}
	mailbox := strings.TrimSpace(r.URL.Query().Get("mailbox"))
	if mailbox == "" {
		mailbox = "INBOX"
	}
	scope := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "mailbox"
	}
	if scope != "mailbox" && scope != "conversation" {
		util.WriteError(w, 400, "bad_request", "scope must be mailbox or conversation", middleware.RequestID(r.Context()))
		return
	}
	page, pageSize := parsePagination(r)
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	mailLogin := service.MailAuthLogin(u)
	scanMailboxes := []string{mailbox}
	if scope == "conversation" {
		available, _, _, _, listErr := h.listMailboxesWithSpecialRoles(r)
		if listErr != nil {
			if isSessionMailAuthError(listErr) {
				h.writeMailAuthError(w, r, listErr)
				return
			}
			util.WriteError(w, 500, "special_mailboxes_failed", listErr.Error(), middleware.RequestID(r.Context()))
			return
		}
		scanMailboxes = threadConversationScanMailboxes(mailbox, available)
	}

	payload, err := h.threadCache.get(r.Context(), mailLogin, scope, mailbox, threadID, page, pageSize, func(ctx context.Context) (threadResponse, error) {
		out := make([]mail.MessageSummary, 0, pageSize)
		matched := 0
		done := false
		truncated := false
		pagesScanned := 0
		for _, scanMailbox := range scanMailboxes {
			mailboxExhausted := false
			for scanPage := 1; scanPage <= threadMessagesMaxPagesPerMB; scanPage++ {
				if pagesScanned >= threadMessagesMaxScanPages {
					truncated = true
					done = false
					goto finish
				}
				pagesScanned++

				items, listErr := h.svc.Mail().ListMessages(ctx, mailLogin, pass, scanMailbox, scanPage, threadMessagesScanPageSize)
				if listErr != nil {
					return threadResponse{}, listErr
				}
				if len(items) == 0 {
					mailboxExhausted = true
					break
				}
				for _, item := range items {
					candidate := presentLiveMessageSummary(item, scanMailbox)
					if candidate.ThreadID != threadID {
						continue
					}
					if matched < offset {
						matched++
						continue
					}
					if len(out) < pageSize {
						out = append(out, candidate)
						matched++
						continue
					}
					done = true
					break
				}
				if done {
					break
				}
			}
			if done {
				break
			}
			if !mailboxExhausted {
				truncated = true
			}
		}
	finish:
		return threadResponse{
			ThreadID:         threadID,
			Mailbox:          mailbox,
			Scope:            scope,
			MailboxesScanned: append([]string(nil), scanMailboxes...),
			Truncated:        truncated,
			Items:            out,
		}, nil
	})
	if err != nil {
		util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	payload.Items, err = decorateLiveMessageSummariesWithTriage(r.Context(), h, u.ID, payload.Items)
	if err != nil {
		util.WriteError(w, 500, "mail_triage_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"thread_id":         payload.ThreadID,
		"mailbox":           payload.Mailbox,
		"scope":             payload.Scope,
		"mailboxes_scanned": payload.MailboxesScanned,
		"page":              page,
		"page_size":         pageSize,
		"truncated":         payload.Truncated,
		"items":             payload.Items,
	})
}

func threadConversationScanMailboxes(current string, available []mail.Mailbox) []string {
	out := make([]string, 0, len(available)+1)
	seen := map[string]struct{}{}
	add := func(name string) {
		clean := strings.TrimSpace(name)
		if clean == "" {
			return
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	roleOf := func(mb mail.Mailbox) string {
		role := strings.ToLower(strings.TrimSpace(mb.Role))
		if role != "" {
			return role
		}
		return mail.MailboxRole(mb.Name, nil)
	}

	currentRole := ""
	for _, mb := range available {
		if strings.EqualFold(strings.TrimSpace(mb.Name), current) {
			currentRole = roleOf(mb)
			break
		}
	}
	if currentRole != "drafts" {
		add(current)
	}
	for _, role := range []string{"inbox", "sent", "archive"} {
		if resolved := mail.ResolveMailboxByRole(available, role); resolved != "" {
			add(resolved)
		}
	}
	for _, mb := range available {
		role := roleOf(mb)
		if role == "drafts" || role == "trash" || role == "junk" {
			continue
		}
		add(mb.Name)
	}
	for _, mb := range available {
		role := roleOf(mb)
		if role == "trash" || role == "junk" {
			add(mb.Name)
		}
	}
	if len(out) == 0 {
		add("INBOX")
	}
	return out
}

func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	h.handleSend(w, r, "", false)
}

func (h *Handlers) ReplyMessage(w http.ResponseWriter, r *http.Request) {
	h.handleSend(w, r, chi.URLParam(r, "id"), true)
}

func (h *Handlers) ForwardMessage(w http.ResponseWriter, r *http.Request) {
	h.handleSend(w, r, chi.URLParam(r, "id"), false)
}

func (h *Handlers) ComposeIdentities(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	senders, err := h.svc.ListSenderProfiles(r.Context(), u)
	if err != nil {
		util.WriteError(w, 500, "compose_identities_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	accounts, err := h.svc.Store().ListMailAccounts(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "compose_identities_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}

	type composeIdentityItem struct {
		AccountID         string `json:"account_id"`
		AccountDisplay    string `json:"account_display_name"`
		AccountLogin      string `json:"account_login"`
		AccountIsDefault  bool   `json:"account_is_default"`
		IdentityID        string `json:"identity_id"`
		IdentityDisplay   string `json:"identity_display_name"`
		IdentityFromEmail string `json:"from_email"`
		ReplyTo           string `json:"reply_to"`
		SignatureText     string `json:"signature_text"`
		SignatureHTML     string `json:"signature_html"`
		IdentityIsDefault bool   `json:"identity_is_default"`
		IsDefault         bool   `json:"is_default"`
		IsSession         bool   `json:"is_session"`
	}
	accountByID := make(map[string]models.MailAccount, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	items := make([]composeIdentityItem, 0, len(senders))
	for _, sender := range senders {
		account := accountByID[sender.AccountID]
		accountDisplay := strings.TrimSpace(sender.AccountLabel)
		if accountDisplay == "" {
			accountDisplay = strings.TrimSpace(account.DisplayName)
		}
		accountLogin := strings.TrimSpace(account.Login)
		if accountLogin == "" {
			accountLogin = strings.TrimSpace(sender.FromEmail)
		}
		accountID := strings.TrimSpace(sender.AccountID)
		accountIsDefault := sender.AccountIsDefault
		identityIsDefault := sender.IsDefault || sender.IsAccountDefault
		isDefault := sender.IsDefault
		if sender.IsPrimary {
			accountID = ""
			accountDisplay = "Primary sender"
			accountLogin = strings.TrimSpace(sender.FromEmail)
			accountIsDefault = true
			identityIsDefault = true
			if !isDefault {
				isDefault = true
			}
		}
		items = append(items, composeIdentityItem{
			AccountID:         accountID,
			AccountDisplay:    accountDisplay,
			AccountLogin:      accountLogin,
			AccountIsDefault:  accountIsDefault,
			IdentityID:        sender.ID,
			IdentityDisplay:   strings.TrimSpace(sender.Name),
			IdentityFromEmail: strings.TrimSpace(sender.FromEmail),
			ReplyTo:           strings.TrimSpace(sender.ReplyTo),
			SignatureText:     strings.TrimSpace(sender.SignatureText),
			SignatureHTML:     strings.TrimSpace(sender.SignatureHTML),
			IdentityIsDefault: identityIsDefault,
			IsDefault:         isDefault,
			IsSession:         sender.IsPrimary,
		})
	}

	util.WriteJSON(w, 200, map[string]any{
		"auth_email":               strings.TrimSpace(service.MailIdentity(u)),
		"manual_fallback_required": len(items) == 0,
		"items":                    items,
	})
}

func (h *Handlers) handleSend(w http.ResponseWriter, r *http.Request, inReply string, markAnswered bool) {
	u, _ := middleware.User(r.Context())
	decoded, err := decodeSendRequest(w, r)
	if err != nil {
		if errors.Is(err, errJSONTooLarge) {
			writeJSONDecodeError(w, r, err)
			return
		}
		util.WriteError(w, 400, "bad_request", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	req := decoded.Request
	originalMessageID := ""
	mailLogin := service.MailAuthLogin(u)
	sessionPass := ""
	if strings.TrimSpace(inReply) != "" {
		sessionPass, err = h.sessionMailPassword(r)
		if err != nil {
			h.writeMailAuthError(w, r, err)
			return
		}
		original, getErr := h.svc.Mail().GetMessage(r.Context(), mailLogin, sessionPass, strings.TrimSpace(inReply))
		if getErr != nil {
			util.WriteError(w, 404, "not_found", getErr.Error(), middleware.RequestID(r.Context()))
			return
		}
		originalMessageID = strings.TrimSpace(original.MessageID)
		req.InReplyToID, req.References = mail.BuildReplyHeaders(
			original.MessageID,
			original.InReplyTo,
			original.References,
		)
	}

	sendAccountID := ""
	sender, err := h.svc.ResolveComposeSender(r.Context(), u, decoded.SenderProfileID, decoded.FromMode, decoded.IdentityID, decoded.FromManual)
	if err != nil {
		status := 400
		code := "send_failed"
		switch {
		case strings.Contains(err.Error(), "identity_id is required"):
			code = "sender_identity_required"
		case strings.Contains(err.Error(), "manual sender must match authenticated account email"):
			code = "invalid_sender_manual"
		case strings.Contains(err.Error(), "selected identity is missing from_email"), strings.Contains(err.Error(), "selected sender is missing from_email"), strings.Contains(err.Error(), "selected sender requires a sending account"), strings.Contains(err.Error(), "selected sender is not available for sending"):
			code = "sender_identity_invalid"
		case errors.Is(err, store.ErrNotFound):
			code = "sender_identity_not_found"
			status = 404
		default:
			code = "sender_identity_lookup_failed"
			status = 500
		}
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	req.HeaderFromName = sender.HeaderFromName
	req.HeaderFromEmail = sender.HeaderFromEmail
	req.EnvelopeFrom = sender.EnvelopeFrom
	req.ReplyTo = sender.ReplyTo
	req.From = sender.HeaderFromEmail
	sendAccountID = sender.AccountID

	if sendAccountID == "" && strings.TrimSpace(decoded.AccountID) != "" {
		accountID := strings.TrimSpace(decoded.AccountID)
		if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				util.WriteError(w, 403, "sender_account_forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
				return
			}
			util.WriteError(w, 500, "sender_account_lookup_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		sendAccountID = accountID
	}

	var sendResult mail.SendResult
	sendResult, err = h.v2SendWithAccount(r.Context(), u, sendAccountID, req)
	if err != nil {
		if isInvalidMessageHeaderError(err) {
			util.WriteError(w, 400, "invalid_sender_identity", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, mail.ErrSMTPSenderRejected) {
			util.WriteError(w, 422, "smtp_sender_rejected", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 502, "smtp_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if markAnswered && strings.TrimSpace(inReply) != "" && sessionPass != "" && originalMessageID != "" {
		_ = h.svc.Mail().UpdateFlags(r.Context(), mailLogin, sessionPass, strings.TrimSpace(inReply), mail.FlagPatch{
			Add: []string{imap.AnsweredFlag},
		})
	}
	h.invalidateMailCaches(mailLogin)
	util.WriteJSON(w, 200, map[string]any{
		"status":             "sent",
		"saved_copy":         sendResult.SavedCopy,
		"saved_copy_mailbox": sendResult.SavedCopyMailbox,
		"warning":            sendResult.Warning,
	})
}

func (h *Handlers) SetMessageFlags(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		Flags  []string `json:"flags"`
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	mailLogin := service.MailAuthLogin(u)
	if len(req.Add) > 0 || len(req.Remove) > 0 {
		if err := h.svc.Mail().UpdateFlags(r.Context(), mailLogin, pass, id, mail.FlagPatch{
			Add:    req.Add,
			Remove: req.Remove,
		}); err != nil {
			util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		h.invalidateMailCaches(mailLogin)
		util.WriteJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	if err := h.svc.Mail().SetFlags(r.Context(), mailLogin, pass, id, req.Flags); err != nil {
		util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.invalidateMailCaches(mailLogin)
	util.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handlers) MoveMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		Mailbox string `json:"mailbox"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	mailLogin := service.MailAuthLogin(u)
	if err := h.svc.Mail().Move(r.Context(), mailLogin, pass, id, req.Mailbox); err != nil {
		util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.invalidateMailCaches(mailLogin)
	util.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	q := r.URL.Query().Get("q")
	mailbox := strings.TrimSpace(r.URL.Query().Get("mailbox"))
	if mailbox == "" {
		mailbox = "INBOX"
	}
	page, pageSize := parsePagination(r)
	mailLogin := service.MailAuthLogin(u)
	filter := parseMailTriageOnlyFilter(r)
	items, err := h.listLiveMessagesWithTriage(r.Context(), u.ID, mailLogin, pass, mailbox, q, page, pageSize, filter)
	if err != nil {
		util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"page": page, "page_size": pageSize, "items": items})
}

func presentLiveMessageSummary(item mail.MessageSummary, mailbox string) mail.MessageSummary {
	if strings.TrimSpace(item.Mailbox) == "" {
		item.Mailbox = strings.TrimSpace(mailbox)
	}
	if strings.TrimSpace(item.ThreadID) == "" {
		item.ThreadID = mail.DeriveLiveThreadID(item.Mailbox, "", "", nil, item.Subject, item.From)
	}
	item.Source = "live"
	if item.Triage.Tags == nil {
		item.Triage = mail.DefaultTriageState()
	}
	return item
}

func presentLiveMessageSummaries(items []mail.MessageSummary, mailbox string) []mail.MessageSummary {
	seen := make(map[string]struct{}, len(items))
	out := make([]mail.MessageSummary, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id != "" {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
		}
		out = append(out, presentLiveMessageSummary(item, mailbox))
	}
	return out
}

func presentLiveMessage(item mail.Message) mail.Message {
	if strings.TrimSpace(item.ThreadID) == "" {
		mail.PopulateLiveMessageThreadID(&item)
	}
	item.Source = "live"
	if item.Triage.Tags == nil {
		item.Triage = mail.DefaultTriageState()
	}
	return item
}

func (h *Handlers) GetAttachment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	mailLogin := service.MailAuthLogin(u)
	meta, stream, err := h.svc.Mail().GetAttachmentStream(r.Context(), mailLogin, pass, id)
	if err != nil {
		util.WriteError(w, 404, "not_found", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", meta.ContentType)
	disposition := "attachment"
	if meta.Inline {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+meta.Filename+`"`)
	if meta.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, stream)
}

func (h *Handlers) AdminListRegistrations(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	page, pageSize := parsePagination(r)
	items, total, err := h.svc.ListRegistrations(r.Context(), models.RegistrationQuery{
		Status: status,
		Q:      strings.TrimSpace(r.URL.Query().Get("q")),
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		Order:  strings.TrimSpace(r.URL.Query().Get("order")),
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	type dto struct {
		ID        string     `json:"id"`
		Email     string     `json:"email"`
		Status    string     `json:"status"`
		CreatedAt time.Time  `json:"created_at"`
		DecidedAt *time.Time `json:"decided_at,omitempty"`
		DecidedBy *string    `json:"decided_by,omitempty"`
		Reason    *string    `json:"reason,omitempty"`
	}
	out := make([]dto, 0, len(items))
	for _, it := range items {
		out = append(out, dto{
			ID:        it.ID,
			Email:     it.Email,
			Status:    it.Status,
			CreatedAt: it.CreatedAt,
			DecidedAt: it.DecidedAt,
			DecidedBy: it.DecidedBy,
			Reason:    it.Reason,
		})
	}
	util.WriteJSON(w, 200, map[string]any{"items": out, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handlers) AdminApproveRegistration(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.ApproveRegistration(r.Context(), admin.ID, id); err != nil {
		if errors.Is(err, store.ErrConflict) {
			util.WriteError(w, 409, "already_decided", "registration has already been decided", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "approve_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "approved"})
}

func (h *Handlers) AdminRejectRegistration(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := h.svc.RejectRegistration(r.Context(), admin.ID, id, req.Reason); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "registration_not_found", "registration not found", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, store.ErrConflict) {
			util.WriteError(w, 409, "already_decided", "registration has already been decided", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, store.ErrUserStateConflict) {
			util.WriteError(w, 409, "registration_user_state_conflict", "registration linked user is not pending", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "reject_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "rejected"})
}

func (h *Handlers) AdminBulkRegistrationDecision(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	var req struct {
		IDs      []string `json:"ids"`
		Decision string   `json:"decision"`
		Reason   string   `json:"reason"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	decision := strings.TrimSpace(strings.ToLower(req.Decision))
	if decision != "approve" && decision != "reject" {
		util.WriteError(w, 400, "bad_request", "decision must be approve or reject", middleware.RequestID(r.Context()))
		return
	}
	applied := make([]string, 0, len(req.IDs))
	failed := make([]map[string]string, 0, len(req.IDs))
	for _, rawID := range req.IDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		var err error
		switch decision {
		case "approve":
			err = h.svc.ApproveRegistration(r.Context(), admin.ID, id)
		default:
			err = h.svc.RejectRegistration(r.Context(), admin.ID, id, req.Reason)
		}
		if err == nil {
			applied = append(applied, id)
			continue
		}
		code := "action_failed"
		if errors.Is(err, store.ErrNotFound) {
			code = "registration_not_found"
		} else if errors.Is(err, store.ErrConflict) {
			code = "already_decided"
		} else if errors.Is(err, store.ErrUserStateConflict) {
			code = "registration_user_state_conflict"
		}
		failed = append(failed, map[string]string{"id": id, "code": code, "message": err.Error()})
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "applied": applied, "failed": failed})
}

func (h *Handlers) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	users, total, err := h.svc.ListUsers(r.Context(), models.UserQuery{
		Q:              strings.TrimSpace(r.URL.Query().Get("q")),
		Status:         strings.TrimSpace(r.URL.Query().Get("status")),
		Role:           strings.TrimSpace(r.URL.Query().Get("role")),
		ProvisionState: strings.TrimSpace(r.URL.Query().Get("provision_state")),
		Sort:           strings.TrimSpace(r.URL.Query().Get("sort")),
		Order:          strings.TrimSpace(r.URL.Query().Get("order")),
		Limit:          pageSize,
		Offset:         (page - 1) * pageSize,
	})
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	type dto struct {
		ID             string            `json:"id"`
		Email          string            `json:"email"`
		Role           string            `json:"role"`
		Status         models.UserStatus `json:"status"`
		ProvisionState string            `json:"provision_state"`
		ProvisionError *string           `json:"provision_error,omitempty"`
	}
	out := make([]dto, 0, len(users))
	for _, u := range users {
		out = append(out, dto{
			ID:             u.ID,
			Email:          u.Email,
			Role:           u.Role,
			Status:         u.Status,
			ProvisionState: u.ProvisionState,
			ProvisionError: u.ProvisionError,
		})
	}
	util.WriteJSON(w, 200, map[string]any{"items": out, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handlers) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	var req struct {
		Email         string `json:"email"`
		RecoveryEmail string `json:"recovery_email"`
		MailboxLogin  string `json:"mailbox_login"`
		Password      string `json:"password"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	user, err := h.svc.AdminCreateUser(r.Context(), admin.ID, service.AdminCreateUserRequest{
		Email:         req.Email,
		RecoveryEmail: req.RecoveryEmail,
		MailboxLogin:  req.MailboxLogin,
		Password:      req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, http.StatusConflict, "user_exists", "user already exists", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrRecoveryEmailMatchesLogin):
			util.WriteError(w, http.StatusBadRequest, "recovery_email_matches_login", "recovery_email must differ from email", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrInvalidRecoveryEmail):
			util.WriteError(w, http.StatusBadRequest, "invalid_recovery_email", "valid recovery_email is required", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrPAMVerifierDown):
			util.WriteError(w, http.StatusBadGateway, "pam_verifier_unavailable", "cannot validate PAM credentials", middleware.RequestID(r.Context()))
		default:
			var pamCredErr *service.PAMCredentialsInvalidError
			if errors.As(err, &pamCredErr) {
				util.WriteError(w, http.StatusBadRequest, "pam_credentials_invalid", err.Error(), middleware.RequestID(r.Context()))
				return
			}
			util.WriteError(w, http.StatusBadRequest, "create_user_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, http.StatusCreated, map[string]any{
		"status": "created",
		"user": map[string]any{
			"id":              user.ID,
			"email":           user.Email,
			"role":            user.Role,
			"status":          user.Status,
			"provision_state": user.ProvisionState,
		},
	})
}

func (h *Handlers) AdminSuspendUser(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.SuspendUser(r.Context(), admin.ID, id); err != nil {
		util.WriteError(w, 400, "suspend_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "suspended"})
}

func (h *Handlers) AdminUnsuspendUser(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.UnsuspendUser(r.Context(), admin.ID, id); err != nil {
		util.WriteError(w, 400, "unsuspend_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "active"})
}

func (h *Handlers) AdminBulkUserAction(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if action != "suspend" && action != "unsuspend" {
		util.WriteError(w, 400, "bad_request", "action must be suspend or unsuspend", middleware.RequestID(r.Context()))
		return
	}
	applied := make([]string, 0, len(req.IDs))
	failed := make([]map[string]string, 0, len(req.IDs))
	for _, rawID := range req.IDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		var err error
		switch action {
		case "suspend":
			err = h.svc.SuspendUser(r.Context(), admin.ID, id)
		default:
			err = h.svc.UnsuspendUser(r.Context(), admin.ID, id)
		}
		if err == nil {
			applied = append(applied, id)
			continue
		}
		failed = append(failed, map[string]string{"id": id, "code": "action_failed", "message": err.Error()})
	}
	util.WriteJSON(w, 200, map[string]any{"status": "ok", "applied": applied, "failed": failed})
}

func (h *Handlers) AdminResetPassword(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if req.NewPassword == "" {
		util.WriteError(w, 400, "bad_request", "new_password is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.AdminResetPassword(r.Context(), admin.ID, id, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrPasswordResetLoginUnmapped) {
			util.WriteError(w, 400, "password_reset_login_unmapped", "mapped mailbox login is required for PAM password reset", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrPasswordResetHelperDown) {
			util.WriteError(w, 503, "password_reset_helper_unavailable", "password reset helper is unavailable", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrPasswordResetHelperFailed) {
			util.WriteError(w, 502, "password_reset_helper_failed", "password reset helper failed", middleware.RequestID(r.Context()))
			return
		}
		if errors.Is(err, service.ErrPasswordResetUnavailable) {
			util.WriteError(w, 503, "password_reset_unavailable", "password reset is currently unavailable", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "reset_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "updated"})
}

func (h *Handlers) AdminRetryProvisionUser(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.svc.RetryProvisionUser(r.Context(), admin.ID, id); err != nil {
		util.WriteError(w, 400, "retry_provision_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "provisioned"})
}

func (h *Handlers) AdminAuditLog(w http.ResponseWriter, r *http.Request) {
	page, pageSize := parsePagination(r)
	from, err := parseDateParam(r.URL.Query().Get("from"), false)
	if err != nil {
		util.WriteError(w, 400, "bad_request", "invalid from date", middleware.RequestID(r.Context()))
		return
	}
	to, err := parseDateParam(r.URL.Query().Get("to"), true)
	if err != nil {
		util.WriteError(w, 400, "bad_request", "invalid to date", middleware.RequestID(r.Context()))
		return
	}
	items, total, err := h.svc.ListAudit(r.Context(), models.AuditQuery{
		Q:      strings.TrimSpace(r.URL.Query().Get("q")),
		Action: strings.TrimSpace(r.URL.Query().Get("action")),
		Actor:  strings.TrimSpace(r.URL.Query().Get("actor")),
		Target: strings.TrimSpace(r.URL.Query().Get("target")),
		From:   from,
		To:     to,
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		Order:  strings.TrimSpace(r.URL.Query().Get("order")),
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handlers) AdminMailHealth(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"checked_at":            time.Now().UTC().Format(time.RFC3339),
		"imap":                  map[string]any{"ok": true},
		"smtp":                  map[string]any{"ok": true},
		"password_reset_sender": map[string]any{"ok": true},
	}
	if err := mail.ProbeIMAP(r.Context(), h.cfg); err != nil {
		out["imap"] = map[string]any{"ok": false, "error": err.Error()}
	}
	if err := mail.ProbeSMTP(r.Context(), h.cfg); err != nil {
		out["smtp"] = map[string]any{"ok": false, "error": err.Error()}
	}
	resetCaps := h.svc.PasswordResetCapabilities(r.Context())
	resetSenderOK := resetCaps.SenderStatus == "ready" || resetCaps.SenderStatus == "external"
	out["password_reset_sender"] = map[string]any{
		"ok":      resetSenderOK,
		"status":  resetCaps.SenderStatus,
		"reason":  resetCaps.SenderReason,
		"address": resetCaps.SenderAddress,
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) AdminVersion(w http.ResponseWriter, r *http.Request) {
	util.WriteJSON(w, 200, version.Current())
}

func (h *Handlers) AdminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.updateMgr.Status(r.Context(), h.svc.Store(), false)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, status)
}

func (h *Handlers) AdminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	status, err := h.updateMgr.Status(r.Context(), h.svc.Store(), true)
	if err != nil {
		util.WriteError(w, 502, "update_check_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	_ = h.updateMgr.AutomaticTick(r.Context(), h.svc.Store())
	if refreshed, err := h.updateMgr.Status(r.Context(), h.svc.Store(), false); err == nil {
		status = refreshed
	}
	util.WriteJSON(w, 200, status)
}

func (h *Handlers) AdminUpdateApply(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	var req struct {
		TargetVersion string `json:"target_version"`
	}
	if r.Body != nil {
		if err := decodeJSON(w, r, &req, jsonLimitAuthControl, true); err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
	}
	applyReq, err := h.updateMgr.QueueApply(
		r.Context(),
		h.svc.Store(),
		admin.Email,
		req.TargetVersion,
		middleware.RequestID(r.Context()),
	)
	if err != nil {
		code := update.ApplyErrorCode(err)
		status := 500
		switch code {
		case "updater_not_configured":
			status = 503
		case "update_in_progress":
			status = 409
		case "invalid_target_version":
			status = 400
		}
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 202, map[string]any{
		"status":         "queued",
		"request_id":     applyReq.RequestID,
		"requested_at":   applyReq.RequestedAt,
		"target_version": applyReq.TargetVersion,
	})
}

func (h *Handlers) AdminUpdateAutomatic(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	auto, err := h.updateMgr.SetAutomaticEnabled(r.Context(), h.svc.Store(), admin.Email, req.Enabled)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if req.Enabled {
		_ = h.updateMgr.AutomaticTick(r.Context(), h.svc.Store())
	}
	util.WriteJSON(w, 200, map[string]any{"auto_update": auto})
}

func (h *Handlers) AdminUpdateCancelScheduled(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	auto, err := h.updateMgr.CancelScheduledUpdate(r.Context(), h.svc.Store(), admin.Email)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"auto_update": auto})
}

func (h *Handlers) AdminListFeatureFlags(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListFeatureFlags(r.Context())
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) AdminSetFeatureFlag(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "feature flag id is required", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitAuthControl, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if req.Enabled == nil {
		util.WriteError(w, 400, "bad_request", "enabled is required", middleware.RequestID(r.Context()))
		return
	}
	item, err := h.svc.SetFeatureFlag(r.Context(), admin.ID, id, *req.Enabled)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFeatureFlagNotFound):
			util.WriteError(w, 404, "feature_flag_not_found", "feature flag not found", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrFeatureFlagReadOnly):
			util.WriteError(w, 400, "feature_flag_read_only", "feature flag is read-only", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "updated", "item": item})
}

func (h *Handlers) AdminResetFeatureFlag(w http.ResponseWriter, r *http.Request) {
	admin, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		util.WriteError(w, 400, "bad_request", "feature flag id is required", middleware.RequestID(r.Context()))
		return
	}
	item, err := h.svc.ResetFeatureFlag(r.Context(), admin.ID, id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrFeatureFlagNotFound):
			util.WriteError(w, 404, "feature_flag_not_found", "feature flag not found", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrFeatureFlagReadOnly):
			util.WriteError(w, 400, "feature_flag_read_only", "feature flag is read-only", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "reset", "item": item})
}

func parsePagination(r *http.Request) (int, int) {
	page := 1
	pageSize := 25
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil {
			if ps < 1 {
				ps = 1
			}
			if ps > 100 {
				ps = 100
			}
			pageSize = ps
		}
	}
	return page, pageSize
}

func parseDateParam(raw string, endOfDay bool) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t.UTC(), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Printf("auth_rng_failure error=%v", err)
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func trustedDeviceTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func trustedDeviceUserAgentHash(userAgent string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(userAgent)))
	return hex.EncodeToString(sum[:])
}

func trustedDeviceLabel(userAgent string) string {
	label := strings.TrimSpace(userAgent)
	if label == "" {
		return "Trusted device"
	}
	if len(label) > 120 {
		return label[:120]
	}
	return label
}

func (h *Handlers) trustedDeviceCookieName() string {
	name := strings.TrimSpace(h.cfg.MFATrustedCookieName)
	if name == "" {
		return "despatch_mfa_trusted"
	}
	return name
}

func (h *Handlers) setTrustedDeviceCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	secure := h.cfg.ResolveCookieSecure(r)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     h.trustedDeviceCookieName(),
		Value:    strings.TrimSpace(token),
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  expiresAt,
	})
}

func (h *Handlers) clearTrustedDeviceCookie(w http.ResponseWriter, r *http.Request) {
	secure := h.cfg.ResolveCookieSecure(r)
	expiredAt := time.Unix(1, 0).UTC()
	http.SetCookie(w, &http.Cookie{
		Name:     h.trustedDeviceCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  expiredAt,
	})
}

func (h *Handlers) issueTrustedDevice(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string) error {
	now := time.Now().UTC()
	token, err := randomToken()
	if err != nil {
		return err
	}
	expiresAt := now.Add(trustedDeviceTTL)
	_, err = h.svc.Store().CreateMFATrustedDevice(ctx, models.MFATrustedDevice{
		ID:          uuid.NewString(),
		UserID:      userID,
		TokenHash:   trustedDeviceTokenHash(token),
		UAHash:      trustedDeviceUserAgentHash(r.UserAgent()),
		IPHint:      middleware.ClientIP(r, h.cfg.TrustProxy),
		DeviceLabel: trustedDeviceLabel(r.UserAgent()),
		CreatedAt:   now,
		LastUsedAt:  now,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return err
	}
	h.setTrustedDeviceCookie(w, r, token, expiresAt)
	return nil
}

func (h *Handlers) rotateTrustedDevice(ctx context.Context, w http.ResponseWriter, r *http.Request, userID, trustedDeviceID string) error {
	now := time.Now().UTC()
	token, err := randomToken()
	if err != nil {
		return err
	}
	expiresAt := now.Add(trustedDeviceTTL)
	_, err = h.svc.Store().RotateMFATrustedDeviceToken(
		ctx,
		userID,
		trustedDeviceID,
		trustedDeviceTokenHash(token),
		trustedDeviceUserAgentHash(r.UserAgent()),
		middleware.ClientIP(r, h.cfg.TrustProxy),
		expiresAt,
	)
	if err != nil {
		return err
	}
	h.setTrustedDeviceCookie(w, r, token, expiresAt)
	return nil
}

func (h *Handlers) tryAuthenticateTrustedDevice(ctx context.Context, w http.ResponseWriter, r *http.Request, user models.User, sess models.Session) (bool, error) {
	cookie, err := r.Cookie(h.trustedDeviceCookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false, nil
	}
	trusted, err := h.svc.Store().GetActiveMFATrustedDeviceByTokenHash(
		ctx,
		user.ID,
		trustedDeviceTokenHash(cookie.Value),
		time.Now().UTC(),
	)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.clearTrustedDeviceCookie(w, r)
			return false, nil
		}
		return false, err
	}
	expectedUAHash := trustedDeviceUserAgentHash(r.UserAgent())
	if strings.TrimSpace(trusted.UAHash) != "" && subtle.ConstantTimeCompare([]byte(trusted.UAHash), []byte(expectedUAHash)) != 1 {
		h.clearTrustedDeviceCookie(w, r)
		return false, nil
	}
	if err := h.svc.Store().SetSessionMFAVerified(ctx, sess.ID, "trusted_device"); err != nil {
		return false, err
	}
	if err := h.rotateTrustedDevice(ctx, w, r, user.ID, trusted.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (h *Handlers) setAuthCookies(w http.ResponseWriter, r *http.Request, sessionToken, csrfToken string) {
	secure := h.cfg.ResolveCookieSecure(r)
	maxAge := int(h.cfg.SessionAbsoluteDuration().Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.SessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
	// Frontend currently reads a fixed cookie name for CSRF. Mirror the configured
	// CSRF token into the legacy default name for compatibility with custom configs.
	if strings.TrimSpace(h.cfg.CSRFCookieName) != "despatch_csrf" {
		http.SetCookie(w, &http.Cookie{
			Name:     "despatch_csrf",
			Value:    csrfToken,
			Path:     "/",
			HttpOnly: false,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   maxAge,
		})
	}
}

func (h *Handlers) clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := h.cfg.ResolveCookieSecure(r)
	expiredAt := time.Unix(1, 0).UTC()
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  expiredAt,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     h.cfg.CSRFCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  expiredAt,
	})
	// Clear the legacy mirror cookie too so browsers don't keep stale CSRF tokens.
	if strings.TrimSpace(h.cfg.CSRFCookieName) != "despatch_csrf" {
		http.SetCookie(w, &http.Cookie{
			Name:     "despatch_csrf",
			Value:    "",
			Path:     "/",
			HttpOnly: false,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Expires:  expiredAt,
		})
	}
}

type decodedSendRequest struct {
	Request         mail.SendRequest
	SenderProfileID string
	FromMode        string
	IdentityID      string
	AccountID       string
	FromManual      string
}

func decodeSendRequest(w http.ResponseWriter, r *http.Request) (decodedSendRequest, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxUploadAttachmentBytes); err != nil {
			return decodedSendRequest{}, err
		}
		req := decodedSendRequest{
			Request: mail.SendRequest{
				To:       splitCSV(r.FormValue("to")),
				CC:       splitCSV(r.FormValue("cc")),
				BCC:      splitCSV(r.FormValue("bcc")),
				Subject:  r.FormValue("subject"),
				Body:     r.FormValue("body"),
				BodyHTML: r.FormValue("body_html"),
			},
			SenderProfileID: strings.TrimSpace(r.FormValue("sender_profile_id")),
			FromMode:        strings.ToLower(strings.TrimSpace(r.FormValue("from_mode"))),
			IdentityID:      strings.TrimSpace(r.FormValue("identity_id")),
			AccountID:       strings.TrimSpace(r.FormValue("account_id")),
			FromManual:      strings.TrimSpace(r.FormValue("from_manual")),
		}
		files := r.MultipartForm.File["attachments"]
		var totalBytes int64
		for _, fh := range files {
			if fh.Size > maxUploadAttachmentBytes {
				return decodedSendRequest{}, errors.New("attachment exceeds per-file size limit")
			}
			f, err := fh.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(f, maxUploadAttachmentBytes))
			_ = f.Close()
			if err != nil {
				continue
			}
			totalBytes += int64(len(data))
			if totalBytes > maxUploadTotalBytes {
				return decodedSendRequest{}, errors.New("attachments exceed total size limit")
			}
			req.Request.Attachments = append(req.Request.Attachments, mail.SendAttachment{
				Filename:    fh.Filename,
				ContentType: fh.Header.Get("Content-Type"),
				Data:        data,
			})
		}
		inlineFiles := r.MultipartForm.File["inline_images"]
		inlineCIDs := r.MultipartForm.Value["inline_image_cids"]
		for i, fh := range inlineFiles {
			if fh.Size > maxUploadAttachmentBytes {
				return decodedSendRequest{}, errors.New("attachment exceeds per-file size limit")
			}
			f, err := fh.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(io.LimitReader(f, maxUploadAttachmentBytes))
			_ = f.Close()
			if err != nil {
				continue
			}
			totalBytes += int64(len(data))
			if totalBytes > maxUploadTotalBytes {
				return decodedSendRequest{}, errors.New("attachments exceed total size limit")
			}
			contentID := ""
			if i < len(inlineCIDs) {
				contentID = strings.TrimSpace(inlineCIDs[i])
			}
			if contentID == "" {
				contentID = fmt.Sprintf("inline-image-%d", i+1)
			}
			req.Request.Attachments = append(req.Request.Attachments, mail.SendAttachment{
				Filename:    fh.Filename,
				ContentType: fh.Header.Get("Content-Type"),
				Data:        data,
				Inline:      true,
				ContentID:   contentID,
			})
		}
		return req, nil
	}
	var req struct {
		To              []string `json:"to"`
		CC              []string `json:"cc"`
		BCC             []string `json:"bcc"`
		Subject         string   `json:"subject"`
		Body            string   `json:"body"`
		BodyHTML        string   `json:"body_html"`
		SenderProfileID string   `json:"sender_profile_id"`
		FromMode        string   `json:"from_mode"`
		IdentityID      string   `json:"identity_id"`
		AccountID       string   `json:"account_id"`
		FromManual      string   `json:"from_manual"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		return decodedSendRequest{}, err
	}
	return decodedSendRequest{
		Request: mail.SendRequest{
			To:       normalizeAddressList(req.To),
			CC:       normalizeAddressList(req.CC),
			BCC:      normalizeAddressList(req.BCC),
			Subject:  req.Subject,
			Body:     req.Body,
			BodyHTML: req.BodyHTML,
		},
		SenderProfileID: strings.TrimSpace(req.SenderProfileID),
		FromMode:        strings.ToLower(strings.TrimSpace(req.FromMode)),
		IdentityID:      strings.TrimSpace(req.IdentityID),
		AccountID:       strings.TrimSpace(req.AccountID),
		FromManual:      strings.TrimSpace(req.FromManual),
	}, nil
}

func splitCSV(v string) []string {
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeAddressList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		out = append(out, splitCSV(item)...)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (h *Handlers) resetIdentifierRateKey(normalizedEmail string) string {
	value := strings.TrimSpace(strings.ToLower(normalizedEmail))
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, h.resetKey)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))[:20]
}

func (h *Handlers) resetTokenPrefixRateKey(rawToken string) string {
	value := strings.TrimSpace(rawToken)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:20]
}

func (h *Handlers) allowResetRateKey(ctx context.Context, route, key string, limit int, window time.Duration) bool {
	key = strings.TrimSpace(key)
	if key == "" || limit <= 0 {
		return true
	}
	windowStart := time.Now().UTC().Truncate(window)
	count, err := h.svc.Store().IncrementRateEvent(ctx, key, route, windowStart)
	if err != nil {
		return true
	}
	_ = h.svc.Store().CleanupRateEventsBefore(ctx, time.Now().UTC().Add(-24*time.Hour))
	return count <= limit
}

func (h *Handlers) sessionMailPassword(r *http.Request) (string, error) {
	sess, ok := middleware.Session(r.Context())
	if !ok {
		return "", service.ErrInvalidCredentials
	}
	return h.svc.SessionMailPassword(sess)
}

func (h *Handlers) writeMailAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrInvalidCredentials) {
		h.clearAuthCookies(w, r)
		util.WriteError(w, 401, "session_invalid", "session is invalid or expired; sign in again", middleware.RequestID(r.Context()))
		return
	}
	util.WriteError(w, 401, "mail_auth_missing", "mail authentication unavailable; sign in again", middleware.RequestID(r.Context()))
}

func (h *Handlers) ensureSetupComplete(w http.ResponseWriter, r *http.Request) bool {
	status, err := h.svc.SetupStatus(r.Context())
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return false
	}
	if status.Required {
		util.WriteError(w, 423, "setup_required", "first-run setup is required", middleware.RequestID(r.Context()))
		return false
	}
	return true
}
