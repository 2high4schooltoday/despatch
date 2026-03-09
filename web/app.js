function createThreadState(overrides = {}) {
  return {
    id: "",
    items: [],
    index: -1,
    truncated: false,
    mailbox: "",
    expanded: true,
    ...overrides,
  };
}

const state = {
  user: null,
  mailbox: "INBOX",
  messages: [],
  mail: {
    mailboxes: [],
    drafts: [],
    selectedDraftID: "",
    selectedMessageIDs: new Set(),
    pollTimer: 0,
    refreshInFlight: false,
  },
  selectedMessage: null,
  selectedMessageSummary: null,
  thread: createThreadState(),
  theme: "machine-dark",
  auth: {
    lastUnauthorizedAtMs: 0,
    lastUnauthorizedCode: "",
    resetCapabilities: null,
    capabilities: null,
    recoveryPromptShownForSession: false,
    legacyMFAOfferShownForSession: false,
    mfaFlowPromise: null,
  },
  setup: {
    required: false,
    step: 0,
    baseDomain: "",
    defaultAdminEmail: "",
    automaticUpdatesEnabled: true,
    passkeyPrimaryEnabled: true,
    authMode: "sql",
    passwordMinLength: 12,
    passwordMaxLength: 128,
    passwordClassMin: 3,
    submitting: false,
    adminEmailTouched: false,
    lastAutoAdminEmail: "",
    retryUntilMs: 0,
    retryTimer: 0,
    adminMailboxLogin: "",
  },
  update: {
    pollTimer: 0,
    checking: false,
    applying: false,
    autoSaving: false,
    cancelingScheduled: false,
    lastStatus: null,
    apiMissing: false,
  },
  captcha: {
    config: null,
    token: "",
    scriptLoaded: false,
    scriptPromise: null,
    widgetActive: false,
    widgetBlocked: false,
    widgetError: "",
  },
  compose: {
    authEmail: "",
    identities: [],
    fromMode: "default",
    typographyMode: "p",
    selectedIdentityID: "",
    selectedAccountID: "",
    manualFallbackRequired: false,
    identityLookupError: "",
    ccVisible: false,
    bccVisible: false,
    formatToolsVisible: false,
    recipients: {
      to: [],
      cc: [],
      bcc: [],
    },
    sendContext: {
      mode: "send",
      messageID: "",
    },
    assets: [],
    submitInFlight: false,
    draftID: "",
    draftLoaded: false,
    draftDirty: false,
    draftSaving: false,
    draftError: "",
    draftStatus: "draft",
    lastSendError: "",
    draftLastSavedAt: "",
    draftSaveTimer: 0,
    draftBaselineJSON: "",
  },
  ui: {
    activeAuthTask: "login",
    composeOpen: false,
    composeLastTrigger: null,
    modalOpen: false,
    modalLastTrigger: null,
    mfaModalOpen: false,
    mfaModalLastTrigger: null,
    activeMailPane: "mailboxes",
    activeKeyboardPane: "mailboxes",
    readerViewMode: "plain",
    activeSettingsSection: "signin",
    activeAdminSection: "system",
    settingsNav: {
      domain: "signin",
      page: "list",
      detailId: "",
    },
    adminNav: {
      domain: "system",
      page: "list",
      detailId: "",
    },
  },
  settings: {
    searchQuery: "",
    passkeys: {
      items: [],
      detailId: "",
    },
    devices: {
      items: [],
      detailId: "",
    },
    sessions: {
      items: [],
      detailId: "",
    },
  },
  admin: {
    registrations: {
      q: "",
      status: "pending",
      sort: "created_at",
      order: "desc",
      selected: new Set(),
      items: [],
      detailId: "",
    },
    users: {
      q: "",
      status: "all",
      role: "all",
      provision: "all",
      sort: "created_at",
      order: "desc",
      selected: new Set(),
      items: [],
      detailId: "",
    },
    audit: {
      q: "",
      action: "all",
      actor: "",
      target: "",
      from: "",
      to: "",
      sort: "created_at",
      order: "desc",
      items: [],
      detailId: "",
    },
    featureFlags: {
      items: [],
      detailId: "",
    },
  },
};

const el = {
  appShell: document.getElementById("app-shell"),
  status: document.getElementById("status-line"),
  btnTheme: document.getElementById("btn-theme"),
  tabSetup: document.getElementById("tab-setup"),
  tabAuth: document.getElementById("tab-auth"),
  tabMail: document.getElementById("tab-mail"),
  tabSettings: document.getElementById("tab-settings"),
  tabAdmin: document.getElementById("tab-admin"),
  btnLogout: document.getElementById("btn-logout"),
  viewSetup: document.getElementById("view-setup"),
  viewAuth: document.getElementById("view-auth"),
  viewSettings: document.getElementById("view-settings"),
  viewMail: document.getElementById("view-mail"),
  viewAdmin: document.getElementById("view-admin"),
  mailPaneMailboxes: document.getElementById("mail-pane-mailboxes"),
  mailPaneMessages: document.getElementById("mail-pane-messages"),
  mailPaneReader: document.getElementById("mail-pane-reader"),
  mailMobileBack: document.getElementById("mail-mobile-back"),
  mailBackToMailboxes: document.getElementById("mail-back-to-mailboxes"),
  mailBackToMessages: document.getElementById("mail-back-to-messages"),
  authModeLogin: document.getElementById("auth-mode-login"),
  authModeRegister: document.getElementById("auth-mode-register"),
  authModeReset: document.getElementById("auth-mode-reset"),
  authPaneLogin: document.getElementById("auth-pane-login"),
  authPaneRegister: document.getElementById("auth-pane-register"),
  authPaneReset: document.getElementById("auth-pane-reset"),
  resetRequestEmail: document.getElementById("reset-request-email"),
  resetTokenInput: document.getElementById("reset-token-input"),
  resetNewPasswordInput: document.getElementById("reset-new-password"),
  loginForm: document.getElementById("form-login"),
  passkeyLoginCard: document.getElementById("auth-passkey-card"),
  btnPasskeyLogin: document.getElementById("btn-passkey-login"),
  passkeyLoginHint: document.getElementById("passkey-login-hint"),
  resetCapabilityNote: document.getElementById("reset-capability-note"),
  mailboxes: document.getElementById("mailboxes"),
  messages: document.getElementById("messages"),
  meta: document.getElementById("message-meta"),
  messageSubjectAnchor: document.getElementById("message-subject-anchor"),
  threadStrip: document.getElementById("thread-strip"),
  threadPosition: document.getElementById("thread-position"),
  threadSelectionStatus: document.getElementById("thread-selection-status"),
  threadTruncated: document.getElementById("thread-truncated"),
  threadListWrap: document.getElementById("thread-list-wrap"),
  threadList: document.getElementById("thread-list"),
  btnThreadCollapse: document.getElementById("btn-thread-collapse"),
  btnThreadPrev: document.getElementById("btn-thread-prev"),
  btnThreadNext: document.getElementById("btn-thread-next"),
  btnReaderViewHTML: document.getElementById("btn-reader-view-html"),
  btnReaderViewPlain: document.getElementById("btn-reader-view-plain"),
  bodyHTMLWrap: document.getElementById("message-body-html-wrap"),
  bodyHTMLFrame: document.getElementById("message-body-html"),
  bodyPlain: document.getElementById("message-body-plain"),
  attachments: document.getElementById("attachment-list"),
  searchInput: document.getElementById("search-input"),
  btnSearch: document.getElementById("btn-search"),
  btnReply: document.getElementById("btn-reply"),
  btnForward: document.getElementById("btn-forward"),
  btnFlag: document.getElementById("btn-flag"),
  btnSeen: document.getElementById("btn-mark-seen"),
  btnArchive: document.getElementById("btn-archive"),
  btnMove: document.getElementById("btn-move"),
  btnTrash: document.getElementById("btn-trash"),
  mailMoveTarget: document.getElementById("mail-move-target"),
  mailBulkBar: document.getElementById("mail-bulk-bar"),
  mailCheckAll: document.getElementById("mail-check-all"),
  mailSelectionCount: document.getElementById("mail-selection-count"),
  btnMailSelectVisible: document.getElementById("btn-mail-select-visible"),
  btnMailClear: document.getElementById("btn-mail-clear"),
  btnComposeOpen: document.getElementById("btn-compose-open"),
  btnComposeClose: document.getElementById("btn-compose-close"),
  btnComposeDiscard: document.getElementById("btn-compose-discard"),
  composeOverlay: document.getElementById("compose-overlay"),
  composeDialog: document.getElementById("compose-dialog"),
  composeTitle: document.getElementById("compose-title"),
  composeForm: document.getElementById("form-compose"),
  btnComposeSend: document.getElementById("btn-compose-send"),
  composeToggleCc: document.getElementById("compose-toggle-cc"),
  composeToggleBcc: document.getElementById("compose-toggle-bcc"),
  composeCcRow: document.getElementById("compose-cc-row"),
  composeBccRow: document.getElementById("compose-bcc-row"),
  composeFromRow: document.getElementById("compose-from-row"),
  composeToField: document.getElementById("compose-to-field"),
  composeCcField: document.getElementById("compose-cc-field"),
  composeBccField: document.getElementById("compose-bcc-field"),
  composeToChips: document.getElementById("compose-to-chips"),
  composeCcChips: document.getElementById("compose-cc-chips"),
  composeBccChips: document.getElementById("compose-bcc-chips"),
  composeToInput: document.getElementById("compose-to-input"),
  composeCcInput: document.getElementById("compose-cc-input"),
  composeBccInput: document.getElementById("compose-bcc-input"),
  composeSubjectInput: document.getElementById("compose-subject-input"),
  composeFromSelect: document.getElementById("compose-from-select"),
  composeFromManualWrap: document.getElementById("compose-from-manual-wrap"),
  composeFromManualInput: document.getElementById("compose-from-manual"),
  composeFromNote: document.getElementById("compose-from-note"),
  composeFromModeInput: document.getElementById("compose-from-mode"),
  composeIdentityIDInput: document.getElementById("compose-identity-id"),
  composeAccountIDInput: document.getElementById("compose-account-id"),
  composeFromManualHiddenInput: document.getElementById("compose-from-manual-hidden"),
  composeBodyTextInput: document.getElementById("compose-body-text"),
  composeBodyHTMLInput: document.getElementById("compose-body-html"),
  composeDraftState: document.getElementById("compose-draft-state"),
  composeDraftNote: document.getElementById("compose-draft-note"),
  composeAssets: document.getElementById("compose-assets"),
  composeAssetsList: document.getElementById("compose-assets-list"),
  composeToggleFormatting: document.getElementById("compose-toggle-formatting"),
  composeEditorTools: document.getElementById("compose-editor-tools"),
  composeEditor: document.getElementById("compose-editor"),
  composeAttachmentsInput: document.getElementById("compose-attachments-input"),
  composeToolUndo: document.getElementById("compose-tool-undo"),
  composeToolTypography: document.getElementById("compose-tool-typography"),
  composeToolBold: document.getElementById("compose-tool-bold"),
  composeToolItalic: document.getElementById("compose-tool-italic"),
  composeToolUnderline: document.getElementById("compose-tool-underline"),
  composeToolList: document.getElementById("compose-tool-list"),
  composeToolLink: document.getElementById("compose-tool-link"),
  composeToolAttach: document.getElementById("compose-tool-attach"),
  composeToolClear: document.getElementById("compose-tool-clear"),
  uiModalOverlay: document.getElementById("ui-modal-overlay"),
  uiModalCard: document.getElementById("ui-modal-card"),
  uiModalTitle: document.getElementById("ui-modal-title"),
  uiModalBody: document.getElementById("ui-modal-body"),
  uiModalInputWrap: document.getElementById("ui-modal-input-wrap"),
  uiModalInputLabel: document.getElementById("ui-modal-input-label"),
  uiModalInput: document.getElementById("ui-modal-input"),
  uiModalCancel: document.getElementById("ui-modal-cancel"),
  uiModalConfirm: document.getElementById("ui-modal-confirm"),
  mfaModalOverlay: document.getElementById("mfa-modal-overlay"),
  mfaModalCard: document.getElementById("mfa-modal-card"),
  mfaModalTitle: document.getElementById("mfa-modal-title"),
  mfaModalBody: document.getElementById("mfa-modal-body"),
  mfaModalExtra: document.getElementById("mfa-modal-extra"),
  mfaModalError: document.getElementById("mfa-modal-error"),
  mfaModalInputWrap: document.getElementById("mfa-modal-input-wrap"),
  mfaModalInputLabel: document.getElementById("mfa-modal-input-label"),
  mfaModalInput: document.getElementById("mfa-modal-input"),
  mfaModalActions: document.getElementById("mfa-modal-actions"),
  registerForm: document.getElementById("form-register"),
  registerMFAPreference: document.getElementById("register-mfa-preference"),
  registerMFAHelp: document.getElementById("register-mfa-help"),
  registerSubmit: document.querySelector("#form-register button[type='submit']"),
  settingsSearchInput: document.getElementById("settings-search-input"),
  settingsSearchResults: document.getElementById("settings-search-results"),
  settingsNavSignIn: document.getElementById("settings-nav-signin"),
  settingsNavDevices: document.getElementById("settings-nav-devices"),
  settingsNavSessions: document.getElementById("settings-nav-sessions"),
  settingsSectionSignIn: document.getElementById("settings-section-signin"),
  settingsSectionDevices: document.getElementById("settings-section-devices"),
  settingsSectionSessions: document.getElementById("settings-section-sessions"),
  passkeysNote: document.getElementById("passkeys-note"),
  passkeysList: document.getElementById("passkeys-list"),
  settingsPasskeyDetail: document.getElementById("settings-passkey-detail"),
  btnPasskeysRefresh: document.getElementById("btn-passkeys-refresh"),
  btnPasskeysAdd: document.getElementById("btn-passkeys-add"),
  trustedDevicesList: document.getElementById("trusted-devices-list"),
  settingsDeviceDetail: document.getElementById("settings-device-detail"),
  btnTrustedDevicesRefresh: document.getElementById("btn-trusted-devices-refresh"),
  btnTrustedDevicesRevokeAll: document.getElementById("btn-trusted-devices-revoke-all"),
  sessionsList: document.getElementById("sessions-list"),
  settingsSessionDetail: document.getElementById("settings-session-detail"),
  btnSessionsRefresh: document.getElementById("btn-sessions-refresh"),
  captchaShell: document.getElementById("captcha-shell"),
  captchaNote: document.getElementById("captcha-note"),
  captchaError: document.getElementById("captcha-error"),
  captchaWidgetContainer: document.getElementById("captcha-widget-container"),
  captchaManualWrap: document.getElementById("captcha-manual-wrap"),
  captchaManualInput: document.getElementById("captcha-token-manual"),
  captchaTokenHidden: document.getElementById("captcha-token-hidden"),
  adminRegs: document.getElementById("admin-registrations"),
  adminRegsDetail: document.getElementById("admin-registrations-detail"),
  adminUsers: document.getElementById("admin-users"),
  adminUsersDetail: document.getElementById("admin-users-detail"),
  adminAudit: document.getElementById("admin-audit"),
  adminAuditDetail: document.getElementById("admin-audit-detail"),
  adminFeatureFlags: document.getElementById("admin-feature-flags"),
  adminFeatureFlagsDetail: document.getElementById("admin-feature-flags-detail"),
  adminSearchInput: document.getElementById("admin-search-input"),
  adminSearchResults: document.getElementById("admin-search-results"),
  adminNavSystem: document.getElementById("admin-nav-system"),
  adminNavRegistrations: document.getElementById("admin-nav-registrations"),
  adminNavUsers: document.getElementById("admin-nav-users"),
  adminNavAudit: document.getElementById("admin-nav-audit"),
  adminSectionSystem: document.getElementById("admin-section-system"),
  adminSectionRegistrations: document.getElementById("admin-section-registrations"),
  adminSectionUsers: document.getElementById("admin-section-users"),
  adminSectionAudit: document.getElementById("admin-section-audit"),
  adminRegQ: document.getElementById("admin-reg-q"),
  adminRegStatus: document.getElementById("admin-reg-status"),
  adminRegSort: document.getElementById("admin-reg-sort"),
  adminRegOrder: document.getElementById("admin-reg-order"),
  btnAdminRegApply: document.getElementById("btn-admin-reg-apply"),
  btnRegSelectAll: document.getElementById("btn-reg-select-all"),
  btnRegClear: document.getElementById("btn-reg-clear"),
  btnRegApprove: document.getElementById("btn-reg-approve"),
  btnRegReject: document.getElementById("btn-reg-reject"),
  adminRegCheckAll: document.getElementById("admin-reg-check-all"),
  adminUserQ: document.getElementById("admin-user-q"),
  adminUserStatus: document.getElementById("admin-user-status"),
  adminUserRole: document.getElementById("admin-user-role"),
  adminUserProvision: document.getElementById("admin-user-provision"),
  adminUserSort: document.getElementById("admin-user-sort"),
  adminUserOrder: document.getElementById("admin-user-order"),
  btnAdminUserApply: document.getElementById("btn-admin-user-apply"),
  btnUserSelectAll: document.getElementById("btn-user-select-all"),
  btnUserClear: document.getElementById("btn-user-clear"),
  btnUserSuspend: document.getElementById("btn-user-suspend"),
  btnUserUnsuspend: document.getElementById("btn-user-unsuspend"),
  adminUserCheckAll: document.getElementById("admin-user-check-all"),
  adminAuditQ: document.getElementById("admin-audit-q"),
  adminAuditAction: document.getElementById("admin-audit-action"),
  adminAuditActor: document.getElementById("admin-audit-actor"),
  adminAuditTarget: document.getElementById("admin-audit-target"),
  adminAuditFrom: document.getElementById("admin-audit-from"),
  adminAuditTo: document.getElementById("admin-audit-to"),
  adminAuditSort: document.getElementById("admin-audit-sort"),
  adminAuditOrder: document.getElementById("admin-audit-order"),
  btnAdminAuditApply: document.getElementById("btn-admin-audit-apply"),
  adminSystemBadge: document.getElementById("admin-system-badge"),
  updateHeroCard: document.getElementById("update-hero-card"),
  updateHeroIcon: document.getElementById("update-hero-icon"),
  updateHeroEyebrow: document.getElementById("update-hero-eyebrow"),
  updateHeroHeadline: document.getElementById("update-hero-headline"),
  updateHeroSubline: document.getElementById("update-hero-subline"),
  updateCurrentVersion: document.getElementById("update-current-version"),
  updateCurrentCommit: document.getElementById("update-current-commit"),
  updateLatestVersion: document.getElementById("update-latest-version"),
  updateLatestPublished: document.getElementById("update-latest-published"),
  updateAvailable: document.getElementById("update-available"),
  updateLastChecked: document.getElementById("update-last-checked"),
  updateApplyState: document.getElementById("update-apply-state"),
  updateScheduledFor: document.getElementById("update-scheduled-for"),
  updateAutoState: document.getElementById("update-auto-state"),
  updateSourceLink: document.getElementById("update-source-link"),
  updateSourceLinkWrap: document.getElementById("update-source-link-wrap"),
  updateNote: document.getElementById("update-note"),
  btnUpdateCheck: document.getElementById("btn-update-check"),
  btnUpdateApply: document.getElementById("btn-update-apply"),
  btnUpdateCancelScheduled: document.getElementById("btn-update-cancel-scheduled"),
  btnUpdateAuto: document.getElementById("btn-update-auto"),
  setupBackIcon: document.getElementById("setup-back-icon"),
  setupClose: document.getElementById("setup-close"),
  setupProgressLabel: document.getElementById("setup-progress-label"),
  setupProgressTitle: document.getElementById("setup-progress-title"),
  setupForm: document.getElementById("form-setup"),
  setupNext: document.getElementById("setup-next"),
  setupOpenMail: document.getElementById("setup-open-mail"),
  setupOpenAdmin: document.getElementById("setup-open-admin"),
  setupRegion: document.getElementById("setup-region"),
  setupThemeMachine: document.getElementById("setup-theme-machine"),
  setupThemePaper: document.getElementById("setup-theme-paper"),
  setupUpdatesAuto: document.getElementById("setup-updates-auto"),
  setupUpdatesManual: document.getElementById("setup-updates-manual"),
  setupDomain: document.getElementById("setup-domain"),
  setupAdminEmail: document.getElementById("setup-admin-email"),
  setupAdminRecoveryEmail: document.getElementById("setup-admin-recovery-email"),
  setupAdminMailboxLogin: document.getElementById("setup-admin-mailbox-login"),
  setupAdminMailboxLoginWrap: document.getElementById("setup-mailbox-login-wrap"),
  setupPassword: document.getElementById("setup-password"),
  setupPasswordConfirm: document.getElementById("setup-password-confirm"),
  setupPasskeyPrimaryEnabled: document.getElementById("setup-passkey-primary-enabled"),
  setupSummaryRegion: document.getElementById("setup-summary-region"),
  setupSummaryTheme: document.getElementById("setup-summary-theme"),
  setupSummaryUpdates: document.getElementById("setup-summary-updates"),
  setupSummaryDomain: document.getElementById("setup-summary-domain"),
  setupSummaryEmail: document.getElementById("setup-summary-email"),
  setupSummaryRecoveryEmail: document.getElementById("setup-summary-recovery-email"),
  setupSummaryPasskey: document.getElementById("setup-summary-passkey"),
  setupPasswordHint: document.getElementById("setup-password-hint"),
  setupInlineStatus: document.getElementById("setup-inline-status"),
  setupCompleteNote: document.getElementById("setup-complete-note"),
  setupModalOverlay: document.getElementById("setup-modal-overlay"),
  setupModalTitle: document.getElementById("setup-modal-title"),
  setupModalBody: document.getElementById("setup-modal-body"),
  setupModalCancel: document.getElementById("setup-modal-cancel"),
  setupModalConfirm: document.getElementById("setup-modal-confirm"),
};

const APP_DRAFTS_MAILBOX = "__despatch_app_drafts__";

const setupSteps = [
  document.getElementById("setup-step-0"),
  document.getElementById("setup-step-1"),
  document.getElementById("setup-step-2"),
  document.getElementById("setup-step-3"),
  document.getElementById("setup-step-4"),
  document.getElementById("setup-step-5"),
  document.getElementById("setup-step-6"),
  document.getElementById("setup-step-7"),
];

const setupStepTitles = [
  "Welcome",
  "Where Despatch will be set up",
  "Choose your look",
  "Software updates",
  "Admin account",
  "Sign-in and security",
  "Ready to initialize",
  "Finished",
];

const setupReviewStep = setupSteps.length - 2;
const setupCompleteStep = setupSteps.length - 1;

function setSetupChoicePressed(button, selected) {
  if (!button) return;
  button.classList.toggle("is-selected", !!selected);
  button.setAttribute("aria-pressed", selected ? "true" : "false");
}

function normalizeRecoveryEmailInput(value) {
  return String(value || "").trim().toLowerCase();
}

const ThemeController = {
  getTheme() {
    return state.theme;
  },
  setTheme(themeName) {
    const next = themeName === "machine-dark" ? "machine-dark" : "paper-light";
    state.theme = next;
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem("ui.theme", next);
    if (el.btnTheme) {
      el.btnTheme.textContent = next === "machine-dark" ? "Theme: Machine" : "Theme: Paper";
    }
  },
  initTheme() {
    const params = new URLSearchParams(window.location.search);
    const forced = params.get("theme");
    if (forced === "machine-dark" || forced === "paper-light") {
      this.setTheme(forced);
      return state.theme;
    }
    this.setTheme(localStorage.getItem("ui.theme") || "machine-dark");
    return state.theme;
  },
};

function setStatus(text, type = "info") {
  el.status.textContent = text;
  if (type === "error") el.status.style.color = "var(--sig-err)";
  else if (type === "ok") el.status.style.color = "var(--sig-ok)";
  else el.status.style.color = "var(--fg-0)";
}

function setSetupInlineStatus(text, type = "info") {
  if (!el.setupInlineStatus) return;
  el.setupInlineStatus.textContent = text || "";
  if (type === "error") el.setupInlineStatus.style.color = "var(--sig-err)";
  else if (type === "ok") el.setupInlineStatus.style.color = "var(--sig-ok)";
  else el.setupInlineStatus.style.color = "var(--fg-muted)";
}

function prefersReducedMotion() {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function apiRequestRef(err) {
  return err && err.requestID ? ` (request ${err.requestID})` : "";
}

function formatAPIError(err, fallbackMessage = "Request failed") {
  const fallback = String(fallbackMessage || "Request failed");
  if (!err || typeof err !== "object") {
    return fallback;
  }
  if (err.code === "csrf_failed") {
    return `Action blocked by CSRF protection. Refresh the page and retry.${apiRequestRef(err)}`;
  }
  if (err.status === 401 && isSessionErrorCode(String(err.code || ""))) {
    return `${reauthMessageForCode(String(err.code || ""))}${apiRequestRef(err)}`;
  }
  const base = String(err.message || fallback).trim() || fallback;
  return `${base}${apiRequestRef(err)}`;
}

function presentAPIError(err, fallbackMessage) {
  setStatus(formatAPIError(err, fallbackMessage), "error");
}

function setActiveAuthTask(task) {
  const next = ["login", "register", "reset"].includes(String(task || "")) ? String(task) : "login";
  state.ui.activeAuthTask = next;
  const modes = {
    login: el.authModeLogin,
    register: el.authModeRegister,
    reset: el.authModeReset,
  };
  const panes = {
    login: el.authPaneLogin,
    register: el.authPaneRegister,
    reset: el.authPaneReset,
  };
  Object.entries(modes).forEach(([key, button]) => {
    if (!button) return;
    const active = key === next;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-pressed", active ? "true" : "false");
  });
  Object.entries(panes).forEach(([key, panel]) => {
    if (!panel) return;
    const hidden = key !== next;
    panel.classList.toggle("hidden", hidden);
  });
  if (next === "reset" && !state.auth.resetCapabilities) {
    void loadResetCapabilities();
  }
}

function updateRegisterMFAHelp() {
  if (!el.registerMFAHelp || !el.registerMFAPreference) return;
  const value = String(el.registerMFAPreference.value || "none").toLowerCase();
  if (value === "totp") {
    el.registerMFAHelp.textContent = "Use an authenticator app (Google Authenticator, 1Password, Authy, etc.).";
    return;
  }
  if (value === "webauthn") {
    el.registerMFAHelp.textContent = "Use Face ID, Touch ID, Windows Hello, or a hardware security key.";
    return;
  }
  el.registerMFAHelp.textContent = "You can enable MFA later in Settings > Sign-In.";
}

function authCapabilityReasonMessage(reason, fallback = "Passkeys are currently unavailable.") {
  switch (String(reason || "").trim()) {
    case "mailsec_unavailable":
      return "Passkeys are unavailable because mailsec is not running.";
    case "insecure_origin":
      return "Passkeys require HTTPS, or localhost for local development.";
    case "rp_mismatch":
      return "Passkeys are blocked because RP ID does not match this host.";
    case "origin_mismatch":
      return "Passkeys are blocked because this origin is not allowed.";
    case "passwordless_disabled":
      return "Passkey sign-in is turned off in Admin > System > Feature Flags.";
    default:
      return fallback;
  }
}

function authCapabilities() {
  if (!state.auth.capabilities || typeof state.auth.capabilities !== "object") {
    return {};
  }
  return state.auth.capabilities;
}

function renderPasskeyLoginUI() {
  if (!el.btnPasskeyLogin || !el.passkeyLoginHint || !el.passkeyLoginCard) {
    return;
  }
  const caps = authCapabilities();
  const browserSupported = supportsWebAuthn();
  const available = browserSupported && !!caps.passkey_passwordless_available;
  el.passkeyLoginCard.classList.toggle("hidden", !available);
  el.passkeyLoginCard.setAttribute("aria-hidden", available ? "false" : "true");
  el.btnPasskeyLogin.disabled = !available;
  if (available) {
    el.passkeyLoginHint.textContent = "Use a discoverable passkey on this device. Account discovery is automatic.";
    return;
  }
  if (!browserSupported) {
    el.passkeyLoginHint.textContent = "Passkey login is not supported by this browser.";
    return;
  }
  el.passkeyLoginHint.textContent = authCapabilityReasonMessage(
    caps.reason,
    "Passkey login is not available on this server.",
  );
}

function renderPasskeySecurityNote() {
  if (!el.passkeysNote) {
    return;
  }
  const caps = authCapabilities();
  const lines = [];
  lines.push(caps.passkey_mfa_available
    ? "Passkeys are available as a second factor (MFA)."
    : authCapabilityReasonMessage(caps.reason, "Passkeys are currently unavailable for MFA."));
  lines.push(caps.passkey_passwordless_available
    ? "Passkeys are available for primary sign-in."
    : authCapabilityReasonMessage(caps.reason, "Passkey sign-in is currently unavailable."));
  lines.push("Passkey sign-in uses built-in account discovery and does not require an email prompt.");
  el.passkeysNote.textContent = lines.join(" ");
}

async function loadAuthCapabilities() {
  try {
    const payload = await api("/api/v1/public/auth/capabilities", { logErrors: false });
    state.auth.capabilities = payload && typeof payload === "object" ? payload : {};
  } catch {
    state.auth.capabilities = {};
  }
  renderPasskeyLoginUI();
  renderPasskeySecurityNote();
  if (el.btnPasskeysAdd) {
    const caps = authCapabilities();
    el.btnPasskeysAdd.disabled = !supportsWebAuthn() || !caps.passkey_mfa_available;
  }
  if (state.user) {
    void loadPasskeyCredentials();
  }
  return state.auth.capabilities;
}

function safeDomID(prefix, raw, fallback = "item") {
  const stem = String(raw || "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return `${prefix}${stem || fallback}`;
}

function syncMailboxActiveDescendant() {
  if (!el.mailboxes) return;
  const buttons = mailboxesButtons();
  const active = buttons.find((node) => String(node.dataset.mailboxName || "") === state.mailbox) || null;
  for (const button of buttons) {
    const isActive = button === active;
    button.classList.toggle("active", isActive);
    button.setAttribute("aria-selected", isActive ? "true" : "false");
    button.tabIndex = -1;
  }
  if (active) {
    if (!active.id) {
      active.id = safeDomID("mailbox-option-", active.dataset.mailboxName || "", `${buttons.indexOf(active)}`);
    }
    el.mailboxes.setAttribute("aria-activedescendant", active.id);
  } else {
    el.mailboxes.removeAttribute("aria-activedescendant");
  }
}

function syncMessageActiveDescendant() {
  if (!el.messages) return;
  const buttons = messageButtons();
  const selectedID = String(state.selectedMessage?.id || "");
  const active = buttons.find((node) => String(node.dataset.messageId || "") === selectedID) || null;
  for (const button of buttons) {
    const isActive = button === active;
    const row = button.closest(".message-row");
    if (row) row.classList.toggle("active", isActive);
    button.setAttribute("aria-selected", isActive ? "true" : "false");
    button.tabIndex = -1;
  }
  if (active) {
    if (!active.id) {
      active.id = safeDomID("message-option-", active.dataset.messageId || "", `${buttons.indexOf(active)}`);
    }
    el.messages.setAttribute("aria-activedescendant", active.id);
  } else {
    el.messages.removeAttribute("aria-activedescendant");
  }
}

function getCookie(name) {
  const m = document.cookie.match(new RegExp("(^| )" + name + "=([^;]+)"));
  return m ? decodeURIComponent(m[2]) : "";
}

function isProtectedAPIPath(path) {
  if (path.startsWith("/api/v1/public/")) return false;
  if (path.startsWith("/api/v1/setup/")) return false;
  if (path === "/api/v1/login" || path === "/api/v1/register" || path === "/api/v1/logout") return false;
  if (path === "/api/v1/login/passkey/begin" || path === "/api/v1/login/passkey/finish") return false;
  if (path.startsWith("/api/v1/password/reset/")) return false;
  if (path === "/api/v2/login") return false;
  if (path === "/api/v2/login/passkey/begin" || path === "/api/v2/login/passkey/finish") return false;
  return path.startsWith("/api/v1/") || path.startsWith("/api/v2/");
}

function isSessionErrorCode(code) {
  return code === "session_missing"
    || code === "session_invalid"
    || code === "unauthorized"
    || code === "mail_secret_required"
    || code === "mail_auth_missing";
}

function isMFAStageCode(code) {
  return code === "mfa_required" || code === "mfa_setup_required";
}

function reauthMessageForCode(code) {
  if (code === "session_missing") {
    return "Session cookie is missing. Check HTTP/HTTPS cookie policy, then sign in again.";
  }
  if (code === "mail_secret_required" || code === "mail_auth_missing") {
    return "Mailbox password is missing for this session. Sign in again or unlock mailbox credentials.";
  }
  return "Session is invalid or expired. Sign in again.";
}

function routeToAuthWithMessage(message, code = "") {
  const now = Date.now();
  const shouldAnnounce = now - state.auth.lastUnauthorizedAtMs > 1800 || state.auth.lastUnauthorizedCode !== code;
  state.auth.lastUnauthorizedAtMs = now;
  state.auth.lastUnauthorizedCode = code;
  state.auth.recoveryPromptShownForSession = false;
  state.auth.legacyMFAOfferShownForSession = false;
  state.auth.mfaFlowPromise = null;
  state.user = null;
  clearMailMessageSelection({ render: false });
  clearReaderSelection();
  renderPasskeyCredentials([]);
  renderTrustedDevices([]);
  renderSessions([]);
  closeComposeOverlay(false);
  closeUIModal({ confirmed: false, value: "" });
  closeMFAModal({ action: "cancel", value: "" });
  applyNavVisibility();
  if (!state.setup.required) {
    setActiveTab(el.tabAuth);
    showView("auth");
    setActiveAuthTask("login");
    void loadAuthCapabilities();
    void initCaptchaUI();
  }
  if (shouldAnnounce) {
    setStatus(message, "error");
  }
}

function composeDraftKey() {
  return "despatch.compose.draft.v2";
}

function composeCrashBufferKey(draftID = "") {
  return `despatch.compose.crash.v1.${String(draftID || "new").trim() || "new"}`;
}

function normalizeComposeClientStateJSON(raw) {
  if (typeof raw === "string" && raw.trim() !== "") {
    try {
      return JSON.stringify(JSON.parse(raw));
    } catch {
      return raw;
    }
  }
  if (raw && typeof raw === "object") {
    return JSON.stringify(raw);
  }
  return "";
}

function composeClientStateObject() {
  return {
    cc_visible: !!state.compose.ccVisible,
    bcc_visible: !!state.compose.bccVisible,
    format_tools_visible: !!state.compose.formatToolsVisible,
    to_pending: String(el.composeToInput?.value || ""),
    cc_pending: String(el.composeCcInput?.value || ""),
    bcc_pending: String(el.composeBccInput?.value || ""),
  };
}

function composeClientStateJSON() {
  return normalizeComposeClientStateJSON(composeClientStateObject());
}

function composeComparableDraftPayload(raw = {}) {
  return {
    account_id: String(raw.account_id ?? raw.accountID ?? "").trim(),
    identity_id: String(raw.identity_id ?? raw.identityID ?? "").trim(),
    compose_mode: String(raw.compose_mode ?? raw.composeMode ?? "send").trim().toLowerCase() || "send",
    context_message_id: String(raw.context_message_id ?? raw.contextMessageID ?? "").trim(),
    from_mode: String(raw.from_mode ?? raw.fromMode ?? "default").trim().toLowerCase() || "default",
    from_manual: String(raw.from_manual ?? raw.fromManual ?? "").trim(),
    client_state_json: normalizeComposeClientStateJSON(raw.client_state_json ?? raw.clientStateJSON ?? ""),
    to: String(raw.to ?? raw.to_value ?? raw.toValue ?? "").trim(),
    cc: String(raw.cc ?? raw.cc_value ?? raw.ccValue ?? "").trim(),
    bcc: String(raw.bcc ?? raw.bcc_value ?? raw.bccValue ?? "").trim(),
    subject: String(raw.subject ?? "").trim(),
    body_text: String(raw.body_text ?? raw.bodyText ?? "").trim(),
    body_html: String(raw.body_html ?? raw.bodyHTML ?? "").trim(),
    status: String(raw.status ?? "active").trim().toLowerCase() || "active",
    last_send_error: String(raw.last_send_error ?? raw.lastSendError ?? "").trim(),
  };
}

function composeDraftPayloadJSON(raw) {
  return JSON.stringify(composeComparableDraftPayload(raw));
}

function composeCurrentDraftPayload() {
  syncComposeDraftFields();
  return composeComparableDraftPayload({
    account_id: state.compose.selectedAccountID || "",
    identity_id: state.compose.selectedIdentityID || "",
    compose_mode: state.compose.sendContext?.mode || "send",
    context_message_id: state.compose.sendContext?.messageID || "",
    from_mode: state.compose.fromMode,
    from_manual: String(el.composeFromManualInput?.value || ""),
    client_state_json: composeClientStateJSON(),
    to: serializeComposeRecipients("to"),
    cc: serializeComposeRecipients("cc"),
    bcc: serializeComposeRecipients("bcc"),
    subject: String(el.composeSubjectInput?.value || ""),
    body_text: composeEditorText(),
    body_html: composeEditorHTML(),
    status: composePersistedDraftStatus(),
    last_send_error: composePersistedDraftStatus() === "failed" ? state.compose.lastSendError : "",
  });
}

function composeDraftHasMeaningfulContent(raw = {}) {
  const payload = composeComparableDraftPayload(raw);
  return [
    payload.to,
    payload.cc,
    payload.bcc,
    payload.subject,
    payload.body_text,
    payload.body_html,
  ].some((value) => String(value || "").trim() !== "");
}

function composeHasLiveMedia() {
  return composeAssets().length > 0;
}

function composeAssets() {
  return Array.isArray(state.compose.assets) ? state.compose.assets : [];
}

function composeAssetByID(id) {
  const key = String(id || "").trim();
  if (!key) return null;
  return composeAssets().find((item) => String(item?.id || "") === key) || null;
}

function composeReadyAssets() {
  return composeAssets().filter((item) => String(item?.status || "") === "ready");
}

function composeInlineAssets() {
  return composeAssets().filter((item) => !!item?.inline);
}

function composeAssetUploadInProgress() {
  return composeAssets().some((item) => String(item?.status || "") === "uploading");
}

function composeAssetHasFailed() {
  return composeAssets().some((item) => String(item?.status || "") === "failed");
}

function composePersistedDraftStatus() {
  const current = String(state.compose.draftStatus || "active").trim().toLowerCase();
  if (current === "failed") {
    return state.compose.draftDirty ? "active" : "failed";
  }
  if (current === "scheduled" || current === "retrying" || current === "sent") {
    return current;
  }
  return "active";
}

function composeDraftAttachmentURL(draftID, attachmentID) {
  const resolvedDraftID = String(draftID || state.compose.draftID || "").trim();
  const resolvedAttachmentID = String(attachmentID || "").trim();
  if (!resolvedDraftID || !resolvedAttachmentID) return "";
  return `/api/v2/drafts/${encodeURIComponent(resolvedDraftID)}/attachments/${encodeURIComponent(resolvedAttachmentID)}`;
}

function parseComposeAttachmentRefs(raw) {
  const text = String(raw || "").trim();
  if (!text) return [];
  try {
    const parsed = JSON.parse(text);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter((item) => item && typeof item === "object")
      .map((item, index) => ({
        id: String(item.id || "").trim(),
        name: String(item.filename || item.name || "").trim() || "attachment.bin",
        contentType: String(item.content_type || item.contentType || "application/octet-stream").trim() || "application/octet-stream",
        size: Number(item.size_bytes ?? item.size ?? 0) || 0,
        inline: Boolean(item.inline_part ?? item.inline),
        contentID: String(item.content_id || item.contentID || "").trim(),
        sortOrder: Number(item.sort_order ?? item.sortOrder ?? index) || index,
      }))
      .filter((item) => item.id !== "");
  } catch {
    return [];
  }
}

function composeAssetLabel(asset) {
  const name = String(asset?.name || "attachment.bin");
  const size = Number(asset?.size || 0);
  if (size <= 0) return name;
  const sizeKB = Math.max(1, Math.round(size / 1024));
  return `${name} (${sizeKB} KB)`;
}

function composeAssetStatusLabel(asset) {
  const status = String(asset?.status || "ready");
  if (status === "uploading") return "Uploading";
  if (status === "failed") return "Failed";
  return "Ready";
}

function writeComposeCrashBuffer(draftID = state.compose.draftID || "", options = {}) {
  const overrideUpdatedAtMs = Number(options.updatedAtMs || 0);
  const updatedAtMs = Number.isFinite(overrideUpdatedAtMs) && overrideUpdatedAtMs > 0
    ? overrideUpdatedAtMs
    : Date.now();
  try {
    const payload = {
      ...composeCurrentDraftPayload(),
      draft_id: String(draftID || "").trim(),
      updated_at_ms: updatedAtMs,
    };
    localStorage.setItem(composeCrashBufferKey(draftID), JSON.stringify(payload));
  } catch {
    // Best effort crash recovery only.
  }
}

function readComposeCrashBuffer(draftID = "") {
  try {
    const raw = localStorage.getItem(composeCrashBufferKey(draftID));
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? parsed : null;
  } catch {
    return null;
  }
}

function clearComposeCrashBuffer(draftID = "") {
  try {
    localStorage.removeItem(composeCrashBufferKey(draftID));
    if (!draftID) {
      localStorage.removeItem(composeCrashBufferKey("new"));
      localStorage.removeItem(composeDraftKey());
    }
  } catch {
    // Ignore local cleanup failures.
  }
}

function migrateLegacyComposeDraftToCrashBuffer() {
  try {
    const raw = localStorage.getItem(composeDraftKey());
    if (!raw) return;
    if (!localStorage.getItem(composeCrashBufferKey("new"))) {
      const parsed = JSON.parse(raw);
      const payload = {
        ...composeComparableDraftPayload(parsed),
        updated_at_ms: Date.now(),
      };
      localStorage.setItem(composeCrashBufferKey("new"), JSON.stringify(payload));
    }
    localStorage.removeItem(composeDraftKey());
  } catch {
    localStorage.removeItem(composeDraftKey());
  }
}

function splitComposeRecipients(raw) {
  const seen = new Set();
  return String(raw || "")
    .split(/[,\n;]+/)
    .map((item) => item.trim())
    .filter(Boolean)
    .filter((item) => {
      const key = item.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function composeMissingMediaLabel(kind) {
  return kind === "attachment" ? "[Attachment unavailable after draft restore]" : "[Inline image unavailable after draft restore]";
}

function composeCreateMissingMediaNode(doc, kind) {
  const node = doc.createElement("span");
  node.className = "compose-inline-missing-media";
  node.setAttribute("contenteditable", "false");
  node.dataset.composeMissingMedia = kind;
  node.textContent = composeMissingMediaLabel(kind);
  return node;
}

function composeEditorSnapshot(mode = "live") {
  const root = document.createElement("div");
  root.innerHTML = String(el.composeEditor?.innerHTML || "");
  const inlineByID = new Map(
    composeInlineAssets().map((item) => [String(item.id || ""), item]),
  );

  const chips = Array.from(root.querySelectorAll("[data-compose-attachment-id]"));
  for (const chip of chips) {
    if (mode === "draft") {
      chip.replaceWith(composeCreateMissingMediaNode(root.ownerDocument, "attachment"));
    } else {
      chip.remove();
    }
  }

  const images = Array.from(root.querySelectorAll("img"));
  for (const img of images) {
    const inlineID = String(img.getAttribute("data-compose-inline-image-id") || "");
    const src = String(img.getAttribute("src") || "");
    const isInlineNode = inlineID !== "";
    const inline = isInlineNode ? (inlineByID.get(inlineID) || null) : null;
    if (isInlineNode) {
      if (mode === "send") {
        if (inline && inline.status === "ready" && inline.contentID) {
          img.setAttribute("src", `cid:${inline.contentID}`);
        } else {
          img.replaceWith(composeCreateMissingMediaNode(root.ownerDocument, "image"));
          continue;
        }
      } else if (mode === "draft") {
        if (inline && inline.status === "ready") {
          const remoteURL = composeDraftAttachmentURL(state.compose.draftID, inline.id);
          if (remoteURL) {
            img.setAttribute("src", remoteURL);
          }
        } else {
          img.replaceWith(composeCreateMissingMediaNode(root.ownerDocument, "image"));
          continue;
        }
      }
    } else if (mode === "draft" && (/^blob:/i.test(src) || /^cid:/i.test(src))) {
      img.replaceWith(composeCreateMissingMediaNode(root.ownerDocument, "image"));
      continue;
    }
    if (mode === "draft") {
      if (isInlineNode) {
        img.setAttribute("data-compose-inline-image-id", inlineID);
        if (inline?.contentID) {
          img.setAttribute("data-compose-inline-image-cid", inline.contentID);
        }
        img.setAttribute("contenteditable", "false");
        img.classList.add("compose-inline-image");
      } else {
        img.removeAttribute("data-compose-inline-image-id");
        img.removeAttribute("data-compose-inline-image-cid");
        img.removeAttribute("contenteditable");
        img.classList.remove("compose-inline-image");
      }
    } else {
      img.removeAttribute("data-compose-inline-image-id");
      img.removeAttribute("data-compose-inline-image-cid");
      img.removeAttribute("contenteditable");
      img.classList.remove("compose-inline-image");
    }
  }

  if (mode === "send") {
    for (const node of root.querySelectorAll("[data-compose-missing-media]")) {
      node.remove();
    }
  }

  const html = root.innerHTML.trim();
  const text = String(root.textContent || "").replace(/\u00a0/g, " ").trim();
  const hasMedia = root.querySelector("img") !== null;
  return {
    html,
    text,
    hasContent: text !== "" || hasMedia,
  };
}

function composeEditorHTML() {
  return composeEditorSnapshot("draft").html;
}

function composeEditorText() {
  return composeEditorSnapshot("live").text;
}

function composeEditorHasContent() {
  return composeEditorSnapshot("live").hasContent;
}

function composeAuthEmailValue() {
  return String(state.compose.authEmail || state.user?.email || "").trim();
}

function composeResolvedManualSender() {
  const manual = String(el.composeFromManualInput?.value || "").trim();
  if (manual !== "") return manual;
  return composeAuthEmailValue();
}

function composeRecipientInput(field) {
  if (field === "to") return el.composeToInput;
  if (field === "cc") return el.composeCcInput;
  if (field === "bcc") return el.composeBccInput;
  return null;
}

function composeRecipientChipContainer(field) {
  if (field === "to") return el.composeToChips;
  if (field === "cc") return el.composeCcChips;
  if (field === "bcc") return el.composeBccChips;
  return null;
}

function setComposeDraftState(text, tone = "muted") {
  if (!el.composeDraftState) return;
  el.composeDraftState.textContent = String(text || "Draft");
  el.composeDraftState.dataset.tone = tone === "ok" || tone === "warn" || tone === "error" ? tone : "muted";
}

function setComposeDraftNote(text = "", tone = "muted") {
  if (!el.composeDraftNote) return;
  const msg = String(text || "").trim();
  el.composeDraftNote.classList.remove("compose-inline-note--ok", "compose-inline-note--warn", "compose-inline-note--error");
  if (!msg) {
    el.composeDraftNote.textContent = "";
    el.composeDraftNote.classList.add("hidden");
    return;
  }
  el.composeDraftNote.textContent = msg;
  el.composeDraftNote.classList.remove("hidden");
  if (tone === "ok") {
    el.composeDraftNote.classList.add("compose-inline-note--ok");
  } else if (tone === "warn") {
    el.composeDraftNote.classList.add("compose-inline-note--warn");
  } else if (tone === "error") {
    el.composeDraftNote.classList.add("compose-inline-note--error");
  }
}

function syncComposeServerDraftState(draft, options = {}) {
  const record = draft && typeof draft === "object" ? draft : {};
  const oldDraftID = String(state.compose.draftID || "");
  const nextDraftID = String(record.id || oldDraftID);
  if (nextDraftID) {
    state.compose.draftID = nextDraftID;
    state.compose.draftLoaded = true;
  }
  state.compose.draftSaving = false;
  if (options.keepDirty !== true) {
    state.compose.draftDirty = false;
  }
  state.compose.draftError = "";
  state.compose.draftStatus = String(record.status || state.compose.draftStatus || "active").trim().toLowerCase() || "active";
  state.compose.lastSendError = String(record.last_send_error || "").trim();
  state.compose.draftLastSavedAt = String(record.updated_at || state.compose.draftLastSavedAt || new Date().toISOString());
  if (options.skipBaseline !== true) {
    state.compose.draftBaselineJSON = composeDraftPayloadJSON(record);
  }
  if (state.compose.draftID) {
    const serverUpdatedAtMs = Date.parse(String(record.updated_at || state.compose.draftLastSavedAt || ""));
    writeComposeCrashBuffer(state.compose.draftID, {
      updatedAtMs: Number.isFinite(serverUpdatedAtMs) && serverUpdatedAtMs > 0 ? serverUpdatedAtMs : Date.now(),
    });
  }
  if (!oldDraftID && state.compose.draftID) {
    clearComposeCrashBuffer("");
  }
  if (nextDraftID) {
    upsertLocalDraft(record);
  }
  applyComposeSendFailurePresentation();
}

function applyComposeSendFailurePresentation() {
  if (String(state.compose.draftStatus || "").toLowerCase() === "failed" && state.compose.lastSendError) {
    setComposeDraftNote(`Send failed. ${state.compose.lastSendError}`, "error");
    return;
  }
  if (String(el.composeDraftNote?.textContent || "").trim().toLowerCase().startsWith("send failed.")) {
    setComposeDraftNote("");
  }
}

function setComposeFromNote(text = "", tone = "muted") {
  if (!el.composeFromNote) return;
  const msg = String(text || "").trim();
  el.composeFromNote.classList.remove("compose-inline-note--ok", "compose-inline-note--warn", "compose-inline-note--error");
  if (msg === "") {
    el.composeFromNote.textContent = "";
    el.composeFromNote.classList.add("hidden");
    updateComposeFromRowVisibility();
    return;
  }
  el.composeFromNote.textContent = msg;
  el.composeFromNote.classList.remove("hidden");
  if (tone === "ok") {
    el.composeFromNote.classList.add("compose-inline-note--ok");
  } else if (tone === "warn") {
    el.composeFromNote.classList.add("compose-inline-note--warn");
  } else if (tone === "error") {
    el.composeFromNote.classList.add("compose-inline-note--error");
  }
  updateComposeFromRowVisibility();
}

function updateComposeFromRowVisibility() {
  if (!el.composeFromRow) return;
  const hasManyIdentities = Array.isArray(state.compose.identities) && state.compose.identities.length > 1;
  const hasNote = !!(el.composeFromNote && !el.composeFromNote.classList.contains("hidden"));
  const shouldShow = hasManyIdentities || hasNote;
  el.composeFromRow.classList.toggle("hidden", !shouldShow);
}

function serializeComposeRecipients(field) {
  const rows = Array.isArray(state.compose.recipients[field]) ? state.compose.recipients[field] : [];
  return rows.map((item) => item.value).join(", ");
}

function hydrateComposeRecipientTokens(field, raw) {
  state.compose.recipients[field] = splitComposeRecipients(raw).map((value) => ({
    value,
    valid: validEmail(value),
  }));
  renderComposeRecipientTokens(field);
}

function renderComposeRecipientTokens(field) {
  const wrap = composeRecipientChipContainer(field);
  if (!wrap) return;
  wrap.replaceChildren();
  const rows = Array.isArray(state.compose.recipients[field]) ? state.compose.recipients[field] : [];
  rows.forEach((item, index) => {
    const chip = document.createElement("span");
    chip.className = `compose-token${item.valid ? "" : " compose-token--invalid"}`;
    chip.setAttribute("role", "listitem");
    chip.title = item.valid ? item.value : "Invalid email address";

    const text = document.createElement("span");
    text.textContent = item.value;
    chip.appendChild(text);

    const removeBtn = document.createElement("button");
    removeBtn.type = "button";
    removeBtn.className = "compose-token-remove";
    removeBtn.setAttribute("aria-label", `Remove ${item.value}`);
    removeBtn.textContent = "x";
    removeBtn.addEventListener("click", () => {
      state.compose.recipients[field] = rows.filter((_, i) => i !== index);
      renderComposeRecipientTokens(field);
      syncComposeDraftFields();
      queueComposeDraftSave();
      updateComposeSubmitState();
    });
    chip.appendChild(removeBtn);
    wrap.appendChild(chip);
  });
}

function addComposeRecipientToken(field, rawValue) {
  const value = String(rawValue || "").trim();
  if (!value) return false;
  const list = Array.isArray(state.compose.recipients[field]) ? state.compose.recipients[field] : [];
  const key = value.toLowerCase();
  if (list.some((item) => String(item.value || "").toLowerCase() === key)) return false;
  list.push({ value, valid: validEmail(value) });
  state.compose.recipients[field] = list;
  renderComposeRecipientTokens(field);
  return true;
}

function commitComposeRecipientInput(field) {
  const input = composeRecipientInput(field);
  if (!input) return 0;
  const chunks = splitComposeRecipients(input.value);
  let added = 0;
  for (const item of chunks) {
    if (addComposeRecipientToken(field, item)) added += 1;
  }
  input.value = "";
  return added;
}

function commitComposeAllRecipientInputs() {
  commitComposeRecipientInput("to");
  if (state.compose.ccVisible) commitComposeRecipientInput("cc");
  if (state.compose.bccVisible) commitComposeRecipientInput("bcc");
}

function composeEffectiveRecipients(field) {
  const out = Array.isArray(state.compose.recipients[field]) ? [...state.compose.recipients[field]] : [];
  const input = composeRecipientInput(field);
  const pending = String(input?.value || "").trim();
  if (pending !== "" && !out.some((item) => String(item.value || "").toLowerCase() === pending.toLowerCase())) {
    out.push({ value: pending, valid: validEmail(pending), pending: true });
  }
  return out;
}

function setComposeCcVisible(visible, opts = {}) {
  const clearWhenHidden = opts.clearWhenHidden !== false;
  state.compose.ccVisible = !!visible;
  if (el.composeCcRow) {
    el.composeCcRow.classList.toggle("hidden", !state.compose.ccVisible);
  }
  if (el.composeToggleCc) {
    el.composeToggleCc.textContent = state.compose.ccVisible ? "-Cc" : "+Cc";
    el.composeToggleCc.setAttribute("aria-label", state.compose.ccVisible ? "Hide Cc field" : "Show Cc field");
  }
  if (!state.compose.ccVisible && clearWhenHidden) {
    state.compose.recipients.cc = [];
    if (el.composeCcInput) el.composeCcInput.value = "";
    renderComposeRecipientTokens("cc");
  }
}

function setComposeBccVisible(visible, opts = {}) {
  const clearWhenHidden = opts.clearWhenHidden !== false;
  state.compose.bccVisible = !!visible;
  if (el.composeBccRow) {
    el.composeBccRow.classList.toggle("hidden", !state.compose.bccVisible);
  }
  if (el.composeToggleBcc) {
    el.composeToggleBcc.textContent = state.compose.bccVisible ? "-Bcc" : "+Bcc";
    el.composeToggleBcc.setAttribute("aria-label", state.compose.bccVisible ? "Hide Bcc field" : "Show Bcc field");
  }
  if (!state.compose.bccVisible && clearWhenHidden) {
    state.compose.recipients.bcc = [];
    if (el.composeBccInput) el.composeBccInput.value = "";
    renderComposeRecipientTokens("bcc");
  }
}

function setComposeFormatToolsVisible(visible) {
  state.compose.formatToolsVisible = !!visible;
  if (el.composeEditorTools) {
    el.composeEditorTools.classList.toggle("hidden", !state.compose.formatToolsVisible);
  }
  if (el.composeToggleFormatting) {
    el.composeToggleFormatting.setAttribute("aria-expanded", state.compose.formatToolsVisible ? "true" : "false");
  }
}

function composeHasInvalidRecipients() {
  const fields = ["to"];
  if (state.compose.ccVisible) fields.push("cc");
  if (state.compose.bccVisible) fields.push("bcc");
  return fields.some((field) => composeEffectiveRecipients(field).some((item) => !item.valid));
}

function composeCanSubmit() {
  const toRows = composeEffectiveRecipients("to");
  const toCount = toRows.filter((item) => item.valid).length;
  const subjectOk = String(el.composeSubjectInput?.value || "").trim() !== "";
  const hasBody = composeEditorHasContent();
  if (toCount === 0 || !subjectOk || !hasBody || composeHasInvalidRecipients()) return false;
  if (composeAssetUploadInProgress() || composeAssetHasFailed()) return false;
  if (state.compose.fromMode === "manual") {
    const authEmail = composeAuthEmailValue().toLowerCase();
    const manual = composeResolvedManualSender().toLowerCase();
    if (!authEmail || manual !== authEmail) return false;
  }
  return true;
}

function updateComposeSubmitState() {
  const disabled = state.compose.submitInFlight || !composeCanSubmit();
  if (el.btnComposeSend) {
    el.btnComposeSend.disabled = disabled;
    const retryLabel = String(state.compose.draftStatus || "").toLowerCase() === "failed" ? "Retry send" : "Send";
    el.btnComposeSend.textContent = state.compose.submitInFlight ? "Sending..." : retryLabel;
  }
  const [draftText, draftTone] = composeDraftStatusText();
  if (draftText !== "Draft") {
    setComposeDraftState(draftText, draftTone);
    return;
  }
  if (composeHasInvalidRecipients()) {
    setComposeDraftState("Fix address", "warn");
    return;
  }
  if (composeCanSubmit()) {
    setComposeDraftState("Ready", "ok");
    return;
  }
  setComposeDraftState("Draft", "muted");
}

function updateComposeFromFields() {
  if (el.composeFromModeInput) el.composeFromModeInput.value = state.compose.fromMode;
  if (el.composeIdentityIDInput) el.composeIdentityIDInput.value = state.compose.selectedIdentityID || "";
  if (el.composeAccountIDInput) el.composeAccountIDInput.value = state.compose.selectedAccountID || "";
  if (el.composeFromManualHiddenInput) {
    el.composeFromManualHiddenInput.value = composeResolvedManualSender();
  }
}

function syncComposeBodyFields() {
  const htmlBody = composeEditorHTML();
  const textBody = composeEditorText();
  if (el.composeBodyHTMLInput) el.composeBodyHTMLInput.value = htmlBody;
  if (el.composeBodyTextInput) el.composeBodyTextInput.value = textBody;
}

function syncComposeDraftFields() {
  syncComposeBodyFields();
  updateComposeFromFields();
}

function clearComposeDraftSaveTimer() {
  if (!state.compose.draftSaveTimer) return;
  clearTimeout(state.compose.draftSaveTimer);
  state.compose.draftSaveTimer = 0;
}

function clearComposeDraft() {
  clearComposeDraftSaveTimer();
  state.compose.draftID = "";
  state.compose.draftLoaded = false;
  state.compose.draftDirty = false;
  state.compose.draftSaving = false;
  state.compose.draftError = "";
  state.compose.draftStatus = "draft";
  state.compose.lastSendError = "";
  state.compose.draftLastSavedAt = "";
  state.compose.draftBaselineJSON = "";
  clearComposeCrashBuffer();
}

function applyComposeDraftPayload(payload, opts = {}) {
  const draft = composeComparableDraftPayload(payload);
  let clientState = {};
  if (draft.client_state_json) {
    try {
      clientState = JSON.parse(draft.client_state_json);
    } catch {
      clientState = {};
    }
  }
  state.compose.recipients.to = [];
  state.compose.recipients.cc = [];
  state.compose.recipients.bcc = [];
  renderComposeRecipientTokens("to");
  renderComposeRecipientTokens("cc");
  renderComposeRecipientTokens("bcc");
  if (draft.to) hydrateComposeRecipientTokens("to", draft.to);
  if (draft.cc) hydrateComposeRecipientTokens("cc", draft.cc);
  if (draft.bcc) hydrateComposeRecipientTokens("bcc", draft.bcc);
  if (el.composeToInput) el.composeToInput.value = String(clientState.to_pending || "");
  if (el.composeCcInput) el.composeCcInput.value = String(clientState.cc_pending || "");
  if (el.composeBccInput) el.composeBccInput.value = String(clientState.bcc_pending || "");
  if (el.composeSubjectInput) el.composeSubjectInput.value = draft.subject;
  if (el.composeEditor) {
    if (draft.body_html) {
      el.composeEditor.innerHTML = draft.body_html;
      if (opts.normalizeDraftMedia) normalizeComposeEditorDraftMedia();
    } else if (draft.body_text) {
      const lines = draft.body_text.split(/\r?\n/);
      el.composeEditor.innerHTML = lines.map((line) => `<p>${escapeHtml(line || "")}</p>`).join("");
    } else {
      el.composeEditor.innerHTML = "";
    }
  }
  if (el.composeFromManualInput) {
    el.composeFromManualInput.value = draft.from_manual;
  }
  state.compose.draftStatus = draft.status || "active";
  state.compose.lastSendError = draft.last_send_error || "";
  state.compose.fromMode = draft.from_mode || "default";
  state.compose.selectedIdentityID = draft.identity_id || "";
  state.compose.selectedAccountID = draft.account_id || "";
  setComposeSendContext(draft.compose_mode || "send", draft.context_message_id || "");
  renderComposeFromControls();
  if (state.compose.fromMode === "manual") {
    setComposeFromMode("manual");
  } else if (state.compose.fromMode === "identity" && el.composeFromSelect && state.compose.selectedIdentityID) {
    el.composeFromSelect.value = state.compose.selectedIdentityID;
    const selectedOption = el.composeFromSelect.selectedOptions[0] || null;
    if (selectedOption) {
      state.compose.selectedAccountID = String(selectedOption.dataset.accountId || state.compose.selectedAccountID || "");
    }
    setComposeFromMode("identity");
  } else {
    setComposeFromMode(state.compose.fromMode || "default");
  }
  setComposeCcVisible(Boolean(clientState.cc_visible || draft.cc || String(clientState.cc_pending || "").trim() !== ""), { clearWhenHidden: false });
  setComposeBccVisible(Boolean(clientState.bcc_visible || draft.bcc || String(clientState.bcc_pending || "").trim() !== ""), { clearWhenHidden: false });
  setComposeFormatToolsVisible(Boolean(clientState.format_tools_visible));
  syncComposeDraftFields();
}

function hydrateComposeDraftAssets(draft) {
  const refs = parseComposeAttachmentRefs(draft?.attachments_json || draft?.attachmentsJSON || "");
  mergeComposeAssetRefs(refs);
  const rawHTML = String(draft?.body_html || draft?.bodyHTML || "");
  const hasLegacyMarkers = /data-compose-attachment-id|data-compose-inline-image-id|Attachment unavailable after draft restore|Inline image unavailable after draft restore/i.test(rawHTML);
  return {
    hasAssets: refs.length > 0,
    legacyMissingMedia: refs.length === 0 && hasLegacyMarkers,
  };
}

function restoreComposeDraft(form) {
  if (!form) return false;
  migrateLegacyComposeDraftToCrashBuffer();
  const draft = readComposeCrashBuffer("");
  if (!draft || !composeDraftHasMeaningfulContent(draft)) return false;
  applyComposeDraftPayload(draft, { normalizeDraftMedia: true });
  setComposeDraftNote("Recovered unsynced text from this browser.", "warn");
  state.compose.draftDirty = true;
  syncComposeDraftFields();
  updateComposeSubmitState();
  return true;
}

function upsertLocalDraft(draft) {
  if (!draft || !draft.id) return;
  const items = Array.isArray(state.mail.drafts) ? [...state.mail.drafts] : [];
  const idx = items.findIndex((item) => String(item?.id || "") === String(draft.id || ""));
  if (idx >= 0) items[idx] = draft;
  else items.unshift(draft);
  items.sort((a, b) => new Date(b?.updated_at || 0).getTime() - new Date(a?.updated_at || 0).getTime());
  state.mail.drafts = items.filter((item) => String(item?.status || "").toLowerCase() !== "sent");
  renderMailboxes();
  if (isDraftsMailboxSelected()) {
    renderMessages(state.mail.drafts.map((item) => buildDraftMessageSummary(item)));
  }
}

function removeLocalDraft(draftID) {
  const id = String(draftID || "").trim();
  if (!id) return;
  state.mail.drafts = (Array.isArray(state.mail.drafts) ? state.mail.drafts : []).filter((item) => String(item?.id || "") !== id);
  if (state.mail.selectedDraftID === id) {
    state.mail.selectedDraftID = "";
  }
  renderMailboxes();
  if (isDraftsMailboxSelected()) {
    renderMessages(state.mail.drafts.map((item) => buildDraftMessageSummary(item)));
  }
}

async function createServerDraft(payload) {
  return api("/api/v2/drafts", {
    method: "POST",
    json: {
      account_id: payload.account_id,
      identity_id: payload.identity_id,
      compose_mode: payload.compose_mode,
      context_message_id: payload.context_message_id,
      from_mode: payload.from_mode,
      from_manual: payload.from_manual,
      client_state_json: payload.client_state_json,
      to: payload.to,
      cc: payload.cc,
      bcc: payload.bcc,
      subject: payload.subject,
      body_text: payload.body_text,
      body_html: payload.body_html,
      status: payload.status || "active",
      last_send_error: payload.last_send_error || "",
    },
    logErrors: false,
  });
}

async function updateServerDraft(draftID, payload) {
  return api(`/api/v2/drafts/${encodeURIComponent(draftID)}`, {
    method: "PATCH",
    json: {
      account_id: payload.account_id,
      identity_id: payload.identity_id,
      compose_mode: payload.compose_mode,
      context_message_id: payload.context_message_id,
      from_mode: payload.from_mode,
      from_manual: payload.from_manual,
      client_state_json: payload.client_state_json,
      to: payload.to,
      cc: payload.cc,
      bcc: payload.bcc,
      subject: payload.subject,
      body_text: payload.body_text,
      body_html: payload.body_html,
      status: payload.status || "active",
      last_send_error: payload.last_send_error || "",
    },
    logErrors: false,
  });
}

async function loadComposeDraftByID(draftID) {
  return api(`/api/v2/drafts/${encodeURIComponent(draftID)}`, { logErrors: false });
}

function composeDraftStatusText() {
  if (state.compose.submitInFlight) return ["Sending", "muted"];
  if (composeAssetUploadInProgress()) return ["Uploading", "muted"];
  if (composeAssetHasFailed()) return ["Attachment failed", "error"];
  if (state.compose.draftSaving) return ["Saving", "muted"];
  if (String(state.compose.draftStatus || "").toLowerCase() === "failed") return ["Send failed", "error"];
  if (state.compose.draftError) return ["Save failed", "error"];
  if (state.compose.draftDirty) return ["Unsaved", "warn"];
  if (state.compose.draftLastSavedAt) return ["Saved", "ok"];
  return ["Draft", "muted"];
}

async function flushComposeDraft(options = {}) {
  if (!el.composeForm || state.compose.submitInFlight) return null;
  clearComposeDraftSaveTimer();
  syncComposeDraftFields();
  const payload = composeCurrentDraftPayload();
  const payloadJSON = composeDraftPayloadJSON(payload);
  const forceCreate = options.forceCreate === true;
  writeComposeCrashBuffer(state.compose.draftID || "");

  if (!state.compose.draftID) {
    if (!forceCreate && (!composeDraftHasMeaningfulContent(payload) || payloadJSON === state.compose.draftBaselineJSON)) {
      return null;
    }
  } else if (payloadJSON === state.compose.draftBaselineJSON) {
    state.compose.draftDirty = false;
    updateComposeSubmitState();
    return null;
  }

  state.compose.draftSaving = true;
  state.compose.draftDirty = true;
  state.compose.draftError = "";
  updateComposeSubmitState();
  try {
    const saved = state.compose.draftID
      ? await updateServerDraft(state.compose.draftID, payload)
      : await createServerDraft(payload);
    syncComposeServerDraftState(saved || payload);
    updateComposeSubmitState();
    return saved || payload;
  } catch (err) {
    state.compose.draftSaving = false;
    state.compose.draftDirty = true;
    state.compose.draftError = formatAPIError(err, "Draft save failed.");
    updateComposeSubmitState();
    return null;
  }
}

function queueComposeDraftSave() {
  if (!state.ui.composeOpen || !el.composeForm) return;
  state.compose.draftDirty = true;
  state.compose.draftError = "";
  writeComposeCrashBuffer(state.compose.draftID || "");
  updateComposeSubmitState();
  clearComposeDraftSaveTimer();
  state.compose.draftSaveTimer = window.setTimeout(() => {
    state.compose.draftSaveTimer = 0;
    void flushComposeDraft({ immediate: false });
  }, 900);
}

function composeDraftContextLabel(mode) {
  const normalized = String(mode || "").trim().toLowerCase();
  if (normalized === "reply") return "Reply";
  if (normalized === "forward") return "Forward";
  return "Draft";
}

function stripHTMLToText(rawHTML) {
  const node = document.createElement("div");
  node.innerHTML = String(rawHTML || "");
  return String(node.textContent || "").replace(/\u00a0/g, " ").trim();
}

function draftPrimaryLine(draft) {
  const toValue = String(draft?.to || "").trim();
  if (toValue) return toValue;
  const sender = String(draft?.from_manual || "").trim();
  if (sender) return sender;
  return composeDraftContextLabel(draft?.compose_mode);
}

function buildDraftMessageSummary(draft) {
  const bodyText = String(draft?.body_text || "").trim() || stripHTMLToText(draft?.body_html || "");
  return {
    id: String(draft?.id || ""),
    mailbox: APP_DRAFTS_MAILBOX,
    isDraft: true,
    subject: String(draft?.subject || "").trim() || "(no subject)",
    from: draftPrimaryLine(draft),
    preview: bodyText,
    date: draft?.updated_at || draft?.created_at || new Date().toISOString(),
    seen: true,
    flagged: false,
    answered: false,
    draft_id: String(draft?.id || ""),
    compose_mode: String(draft?.compose_mode || "send"),
    context_badge: composeDraftContextLabel(draft?.compose_mode),
  };
}

function isActionableMailSummary(item) {
  return !!item && !item.isDraft;
}

function visibleActionableMessages() {
  return (Array.isArray(state.messages) ? state.messages : []).filter((item) => isActionableMailSummary(item));
}

function selectedMailMessageIDs() {
  return state.mail.selectedMessageIDs instanceof Set ? state.mail.selectedMessageIDs : new Set();
}

function hasBulkMailSelection() {
  return selectedMailMessageIDs().size > 0;
}

function pruneMailSelectionToVisible() {
  const visible = new Set(visibleActionableMessages().map((item) => String(item?.id || "")).filter(Boolean));
  const next = new Set();
  for (const id of selectedMailMessageIDs()) {
    const key = String(id || "").trim();
    if (visible.has(key)) next.add(key);
  }
  state.mail.selectedMessageIDs = next;
}

function clearMailMessageSelection(options = {}) {
  state.mail.selectedMessageIDs = new Set();
  if (options.render !== false) {
    renderMessages(state.messages);
    return;
  }
  syncMailBulkControls();
  applyMailActionAvailability();
}

function toggleMailMessageSelection(id, selected) {
  const key = String(id || "").trim();
  if (!key) return;
  const next = new Set(selectedMailMessageIDs());
  if (selected) next.add(key);
  else next.delete(key);
  state.mail.selectedMessageIDs = next;
  renderMessages(state.messages);
}

function selectedMailActionIDs() {
  pruneMailSelectionToVisible();
  const bulkIDs = Array.from(selectedMailMessageIDs());
  if (bulkIDs.length > 0) return bulkIDs;
  const currentID = String(state.selectedMessage?.id || "").trim();
  if (currentID && !isDraftsMailboxSelected()) return [currentID];
  return [];
}

function mailActionItemByID(id) {
  const key = String(id || "").trim();
  if (!key) return null;
  return state.messages.find((item) => String(item?.id || "") === key)
    || (state.selectedMessageSummary && String(state.selectedMessageSummary?.id || "") === key ? state.selectedMessageSummary : null)
    || (state.selectedMessage && String(state.selectedMessage?.id || "") === key ? state.selectedMessage : null)
    || null;
}

function selectedMailActionItems() {
  return selectedMailActionIDs()
    .map((id) => mailActionItemByID(id))
    .filter((item) => !!item);
}

function selectedMailActionCount() {
  return selectedMailActionIDs().length;
}

function selectedMailActionReadMode() {
  const items = selectedMailActionItems();
  if (items.length > 0 && items.every((item) => !!item?.seen)) return "unread";
  return "read";
}

function selectedMailActionFlagMode() {
  const items = selectedMailActionItems();
  if (items.length > 0 && items.every((item) => !!item?.flagged)) return "unflag";
  return "flag";
}

function selectableMoveMailboxes() {
  return (Array.isArray(state.mail.mailboxes) ? [...state.mail.mailboxes] : [])
    .filter((item) => normalizeMailboxRole(item?.role, item?.name) !== "drafts")
    .sort((a, b) => {
      const rankDiff = mailboxRoleRank(a?.role || a?.name) - mailboxRoleRank(b?.role || b?.name);
      if (rankDiff !== 0) return rankDiff;
      return String(a?.name || "").localeCompare(String(b?.name || ""));
    });
}

function renderMailMoveTargets() {
  if (!el.mailMoveTarget) return;
  const previous = String(el.mailMoveTarget.value || "");
  const choices = selectableMoveMailboxes()
    .filter((item) => String(item?.name || "").trim() !== String(state.mailbox || "").trim());
  el.mailMoveTarget.replaceChildren();
  const placeholder = document.createElement("option");
  placeholder.value = "";
  placeholder.textContent = "Move to…";
  el.mailMoveTarget.appendChild(placeholder);
  for (const mailbox of choices) {
    const option = document.createElement("option");
    option.value = String(mailbox?.name || "");
    option.textContent = mailboxDisplayLabel(mailbox);
    el.mailMoveTarget.appendChild(option);
  }
  if (choices.some((item) => String(item?.name || "") === previous)) {
    el.mailMoveTarget.value = previous;
  } else {
    el.mailMoveTarget.value = "";
  }
  applyMailActionAvailability();
}

function syncMailBulkControls() {
  if (!el.mailBulkBar || !el.mailSelectionCount || !el.mailCheckAll) return;
  pruneMailSelectionToVisible();
  const visible = visibleActionableMessages();
  const count = selectedMailMessageIDs().size;
  const hasVisible = visible.length > 0;
  const bulkEnabled = !isDraftsMailboxSelected() && hasVisible;
  el.mailBulkBar.classList.toggle("hidden", !bulkEnabled);
  el.mailSelectionCount.textContent = count === 1 ? "1 selected" : `${count} selected`;
  el.mailCheckAll.disabled = !bulkEnabled;
  el.mailCheckAll.checked = bulkEnabled && count > 0 && count === visible.length;
  el.mailCheckAll.indeterminate = bulkEnabled && count > 0 && count < visible.length;
  if (el.btnMailSelectVisible) el.btnMailSelectVisible.disabled = !bulkEnabled || count === visible.length;
  if (el.btnMailClear) el.btnMailClear.disabled = count === 0;
}

function isDraftsMailboxSelected() {
  return String(state.mailbox || "").trim() === APP_DRAFTS_MAILBOX;
}

function isAppDraftMailboxName(name) {
  return String(name || "").trim() === APP_DRAFTS_MAILBOX;
}

async function loadDrafts(opts = {}) {
  if (!state.user) return [];
  const payload = await api("/api/v2/drafts?page=1&page_size=100", { logErrors: opts.logErrors });
  state.mail.drafts = Array.isArray(payload?.items) ? payload.items : [];
  renderMailboxes();
  return state.mail.drafts;
}

function composeID(prefix) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function composeFileLooksInlineImage(file) {
  const type = String(file?.type || "").toLowerCase();
  if (type.startsWith("image/")) return true;
  const name = String(file?.name || "").toLowerCase();
  return /\.(png|jpe?g|gif|webp|bmp|svg|avif|heic|heif)$/i.test(name);
}

function revokeComposeAssetObjectURL(item) {
  const blobURL = String(item?.objectURL || "").trim();
  if (!blobURL) return;
  try {
    URL.revokeObjectURL(blobURL);
  } catch {
    // Best effort cleanup only.
  }
}

function rebindComposeEditorAssetID(oldID, nextAsset) {
  if (!el.composeEditor) return;
  const previousID = String(oldID || "").trim();
  const nextID = String(nextAsset?.id || "").trim();
  if (!previousID || !nextID || previousID === nextID) return;
  const nextCID = String(nextAsset?.contentID || "").trim();
  const nodes = Array.from(el.composeEditor.querySelectorAll("[data-compose-inline-placeholder], [data-compose-inline-image-id], [data-compose-attachment-id]"));
  for (const node of nodes) {
    let matched = false;
    if (String(node.getAttribute("data-compose-inline-placeholder") || "") === previousID) {
      node.setAttribute("data-compose-inline-placeholder", nextID);
      matched = true;
    }
    if (String(node.getAttribute("data-compose-inline-image-id") || "") === previousID) {
      node.setAttribute("data-compose-inline-image-id", nextID);
      matched = true;
    }
    if (String(node.getAttribute("data-compose-attachment-id") || "") === previousID) {
      node.setAttribute("data-compose-attachment-id", nextID);
      matched = true;
    }
    if (matched && nextCID) {
      node.setAttribute("data-compose-inline-image-cid", nextCID);
    }
  }
}

function composeAssetFromDraftRef(ref, draftID = state.compose.draftID || "") {
  return {
    id: String(ref?.id || "").trim(),
    draftID: String(draftID || "").trim(),
    name: String(ref?.name || ref?.filename || "attachment.bin").trim() || "attachment.bin",
    size: Number(ref?.size ?? ref?.size_bytes ?? 0) || 0,
    contentType: String(ref?.contentType || ref?.content_type || "application/octet-stream").trim() || "application/octet-stream",
    inline: Boolean(ref?.inline ?? ref?.inline_part),
    contentID: String(ref?.contentID || ref?.content_id || "").trim(),
    sortOrder: Number(ref?.sortOrder ?? ref?.sort_order ?? 0) || 0,
    status: "ready",
    file: null,
    objectURL: "",
    url: composeDraftAttachmentURL(draftID, ref?.id),
    error: "",
    createdAtMs: Date.now(),
  };
}

function mergeComposeAssetRefs(refs, removedIDs = []) {
  const removed = new Set(removedIDs.map((item) => String(item || "").trim()).filter(Boolean));
  const existing = new Map(composeAssets().map((item) => [String(item?.id || ""), item]));
  const ready = refs
    .map((ref) => composeAssetFromDraftRef(ref))
    .filter((item) => item.id !== "")
    .map((item) => {
      const prev = existing.get(item.id) || null;
      return {
        ...item,
        file: prev?.file || null,
        objectURL: prev?.objectURL || "",
        createdAtMs: Number(prev?.createdAtMs || Date.now()),
      };
    });
  const transient = composeAssets()
    .filter((item) => String(item?.status || "") !== "ready")
    .filter((item) => !removed.has(String(item?.id || "")));
  state.compose.assets = [...ready, ...transient].sort((a, b) => {
    const statusA = String(a?.status || "ready");
    const statusB = String(b?.status || "ready");
    if (statusA === "ready" && statusB === "ready") {
      return Number(a?.sortOrder || 0) - Number(b?.sortOrder || 0);
    }
    if (statusA === "ready") return -1;
    if (statusB === "ready") return 1;
    return Number(a?.createdAtMs || 0) - Number(b?.createdAtMs || 0);
  });
  renderComposeAssets();
  syncComposeInlineAssetNodes();
}

function renderComposeAssets() {
  if (!el.composeAssets || !el.composeAssetsList) return;
  const items = composeAssets();
  el.composeAssets.classList.toggle("hidden", items.length === 0);
  el.composeAssetsList.replaceChildren();
  for (const asset of items) {
    const row = document.createElement("li");
    row.className = "compose-asset-row";
    if (asset.inline) row.classList.add("compose-asset-row--inline");
    if (asset.status === "uploading") row.classList.add("is-uploading");
    if (asset.status === "failed") row.classList.add("is-failed");

    const main = document.createElement("div");
    main.className = "compose-asset-main";

    const name = document.createElement("span");
    name.className = "compose-asset-name";
    name.textContent = composeAssetLabel(asset);
    main.appendChild(name);

    const meta = document.createElement("span");
    meta.className = "compose-asset-meta";
    meta.innerHTML = `<span class="compose-asset-badge">${asset.inline ? "Inline" : "File"}</span><span class="compose-asset-state">${escapeHtml(composeAssetStatusLabel(asset))}</span>`;
    if (asset.status === "failed" && asset.error) {
      const err = document.createElement("span");
      err.className = "compose-asset-error";
      err.textContent = asset.error;
      meta.appendChild(err);
    }
    main.appendChild(meta);
    row.appendChild(main);

    const actions = document.createElement("div");
    actions.className = "compose-asset-actions";
    if (asset.status === "failed" && asset.file) {
      const retry = document.createElement("button");
      retry.type = "button";
      retry.className = "cmd-btn cmd-btn--dense";
      retry.textContent = "Retry";
      retry.dataset.composeAssetRetry = asset.id;
      actions.appendChild(retry);
    }
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "cmd-btn cmd-btn--dense";
    remove.textContent = "Remove";
    remove.dataset.composeAssetRemove = asset.id;
    actions.appendChild(remove);
    row.appendChild(actions);

    el.composeAssetsList.appendChild(row);
  }
}

function insertComposeInlinePlaceholder(asset) {
  const html = `<span class="compose-inline-placeholder is-uploading" contenteditable="false" data-compose-inline-placeholder="${escapeHtml(asset.id)}" data-compose-inline-image-id="${escapeHtml(asset.id)}">Uploading inline image...</span>&nbsp;`;
  insertComposeHTMLAtCaret(html);
}

function syncComposeInlineAssetNodes() {
  if (!el.composeEditor) return;
  const inlineAssets = new Map(composeInlineAssets().map((item) => [String(item?.id || ""), item]));
  const images = Array.from(el.composeEditor.querySelectorAll("img"));
  for (const img of images) {
    const inlineID = String(img.getAttribute("data-compose-inline-image-id") || "");
    const asset = inlineAssets.get(inlineID) || null;
    if (!asset || asset.status !== "ready") {
      continue;
    }
    const remoteURL = composeDraftAttachmentURL(state.compose.draftID, asset.id);
    if (remoteURL) img.setAttribute("src", remoteURL);
    if (asset.contentID) img.setAttribute("data-compose-inline-image-cid", asset.contentID);
    img.classList.add("compose-inline-image");
    img.setAttribute("contenteditable", "false");
  }
  const placeholders = Array.from(el.composeEditor.querySelectorAll("[data-compose-inline-placeholder]"));
  for (const placeholder of placeholders) {
    const inlineID = String(placeholder.getAttribute("data-compose-inline-image-id") || "");
    const asset = inlineAssets.get(inlineID) || null;
    if (!asset) continue;
    placeholder.classList.toggle("is-uploading", asset.status === "uploading");
    placeholder.classList.toggle("is-failed", asset.status === "failed");
    if (asset.status === "ready") {
      const img = document.createElement("img");
      img.className = "compose-inline-image";
      img.setAttribute("src", composeDraftAttachmentURL(state.compose.draftID, asset.id));
      img.setAttribute("alt", asset.name || "inline image");
      img.setAttribute("data-compose-inline-image-id", asset.id);
      img.setAttribute("contenteditable", "false");
      if (asset.contentID) img.setAttribute("data-compose-inline-image-cid", asset.contentID);
      placeholder.replaceWith(img);
      continue;
    }
    placeholder.textContent = asset.status === "failed" ? "Inline image upload failed" : "Uploading inline image...";
  }
}

function normalizeComposeEditorDraftMedia() {
  if (!el.composeEditor) return;
  const chips = Array.from(el.composeEditor.querySelectorAll("[data-compose-attachment-id]"));
  for (const chip of chips) {
    chip.replaceWith(composeCreateMissingMediaNode(document, "attachment"));
  }
  syncComposeInlineAssetNodes();
  const placeholders = Array.from(el.composeEditor.querySelectorAll("[data-compose-inline-placeholder]"));
  for (const placeholder of placeholders) {
    const assetID = String(placeholder.getAttribute("data-compose-inline-image-id") || "");
    const asset = composeAssetByID(assetID);
    if (!asset || asset.status !== "ready") {
      placeholder.replaceWith(composeCreateMissingMediaNode(document, "image"));
    }
  }
  for (const img of Array.from(el.composeEditor.querySelectorAll("img[data-compose-inline-image-id]"))) {
    const inlineID = String(img.getAttribute("data-compose-inline-image-id") || "");
    const asset = composeAssetByID(inlineID);
    if (!asset || asset.status !== "ready") {
      img.replaceWith(composeCreateMissingMediaNode(document, "image"));
      continue;
    }
    const remoteURL = composeDraftAttachmentURL(state.compose.draftID, asset.id);
    if (remoteURL) {
      img.setAttribute("src", remoteURL);
    }
  }
}

function clearComposeAssets(options = {}) {
  const removeEditorNodes = options.removeEditorNodes !== false;
  for (const item of composeAssets()) {
    revokeComposeAssetObjectURL(item);
  }
  state.compose.assets = [];
  renderComposeAssets();
  if (el.composeAttachmentsInput) el.composeAttachmentsInput.value = "";
  if (removeEditorNodes && el.composeEditor) {
    for (const node of el.composeEditor.querySelectorAll("[data-compose-attachment-id], img[data-compose-inline-image-id], [data-compose-inline-placeholder]")) {
      node.remove();
    }
  }
}

async function ensureComposeServerDraft() {
  if (String(state.compose.draftID || "").trim()) return state.compose.draftID;
  state.compose.draftSaving = true;
  state.compose.draftError = "";
  updateComposeSubmitState();
  try {
    const saved = await createServerDraft(composeCurrentDraftPayload());
    syncComposeServerDraftState(saved);
    updateComposeSubmitState();
    return state.compose.draftID;
  } catch (err) {
    state.compose.draftSaving = false;
    state.compose.draftError = formatAPIError(err, "Draft save failed.");
    updateComposeSubmitState();
    throw err;
  }
}

async function uploadComposeAsset(assetID) {
  const current = composeAssetByID(assetID);
  if (!current || !current.file) return;
  let draftID = String(state.compose.draftID || "").trim();
  try {
    draftID = draftID || await ensureComposeServerDraft();
  } catch (err) {
    const message = formatAPIError(err, "Draft save failed.");
    state.compose.assets = composeAssets().map((item) => (item.id === assetID ? { ...item, status: "failed", error: message } : item));
    renderComposeAssets();
    syncComposeInlineAssetNodes();
    updateComposeSubmitState();
    return;
  }
  const asset = composeAssetByID(assetID);
  if (!asset || !asset.file) return;
  const mp = new FormData();
  if (asset.inline) {
    mp.append("inline_images", asset.file, asset.name);
    mp.append("inline_image_cids", asset.contentID || composeID("cid"));
  } else {
    mp.append("attachments", asset.file, asset.name);
  }
  try {
    const res = await api(`/api/v2/drafts/${encodeURIComponent(draftID)}/attachments`, {
      method: "POST",
      body: mp,
      logErrors: false,
    });
    const uploaded = Array.isArray(res?.uploaded) ? res.uploaded : [];
    const match = uploaded[0] || null;
    if (!match || !match.id) {
      throw new Error("Draft attachment upload failed.");
    }
    const readyAsset = composeAssetFromDraftRef(match, draftID);
    readyAsset.createdAtMs = Number(asset.createdAtMs || Date.now());
    rebindComposeEditorAssetID(assetID, readyAsset);
    state.compose.assets = composeAssets().map((item) => (item.id === assetID ? readyAsset : item));
    mergeComposeAssetRefs(Array.isArray(res?.items) ? res.items : [], [assetID]);
    syncComposeServerDraftState(res?.draft || {}, { keepDirty: true });
    setComposeDraftNote("");
    syncComposeDraftFields();
    queueComposeDraftSave();
    updateComposeSubmitState();
  } catch (err) {
    const message = formatAPIError(err, "Attachment upload failed.");
    state.compose.assets = composeAssets().map((item) => (
      item.id === assetID
        ? { ...item, status: "failed", error: message }
        : item
    ));
    renderComposeAssets();
    syncComposeInlineAssetNodes();
    setComposeDraftNote(message, "error");
    updateComposeSubmitState();
  }
}

async function removeComposeAssetByID(id, options = {}) {
  const asset = composeAssetByID(id);
  if (!asset) return;
  const removeEditorNode = options.removeEditorNode !== false;
  const removeLocal = () => {
    revokeComposeAssetObjectURL(asset);
    state.compose.assets = composeAssets().filter((item) => item.id !== id);
    renderComposeAssets();
    if (removeEditorNode && el.composeEditor) {
      for (const node of el.composeEditor.querySelectorAll(`[data-compose-attachment-id="${id}"], img[data-compose-inline-image-id="${id}"], [data-compose-inline-placeholder="${id}"]`)) {
        node.remove();
      }
    }
    syncComposeDraftFields();
    queueComposeDraftSave();
    updateComposeSubmitState();
  };
  if (asset.status !== "ready" || !state.compose.draftID) {
    removeLocal();
    return;
  }
  const res = await api(`/api/v2/drafts/${encodeURIComponent(state.compose.draftID)}/attachments/${encodeURIComponent(id)}`, {
    method: "DELETE",
    json: {},
    logErrors: false,
  });
  revokeComposeAssetObjectURL(asset);
  if (removeEditorNode && el.composeEditor) {
    for (const node of el.composeEditor.querySelectorAll(`[data-compose-attachment-id="${id}"], img[data-compose-inline-image-id="${id}"], [data-compose-inline-placeholder="${id}"]`)) {
      node.remove();
    }
  }
  mergeComposeAssetRefs(Array.isArray(res?.items) ? res.items : [], [id]);
  syncComposeServerDraftState(res?.draft || {}, { keepDirty: true });
  syncComposeDraftFields();
  queueComposeDraftSave();
  updateComposeSubmitState();
}

function retryComposeAssetByID(id) {
  const asset = composeAssetByID(id);
  if (!asset || !asset.file) return;
  state.compose.assets = composeAssets().map((item) => (
    item.id === id
      ? { ...item, status: "uploading", error: "" }
      : item
  ));
  renderComposeAssets();
  syncComposeInlineAssetNodes();
  updateComposeSubmitState();
  void uploadComposeAsset(id);
}

function cleanupComposeInlineReferences() {
  if (!el.composeEditor) return false;
  let changed = false;
  const inlineIDs = new Set(
    Array.from(el.composeEditor.querySelectorAll("img[data-compose-inline-image-id], [data-compose-inline-placeholder]"))
      .map((node) => String(node.getAttribute("data-compose-inline-image-id") || "")),
  );
  for (const item of composeInlineAssets()) {
    if (inlineIDs.has(item.id)) continue;
    changed = true;
    void removeComposeAssetByID(item.id, { removeEditorNode: false });
  }
  return changed;
}

function addComposeFiles(files) {
  if (!files || files.length === 0) return;
  for (const file of Array.from(files)) {
    const item = {
      id: composeID("compose-asset"),
      file,
      name: String(file?.name || "attachment.bin"),
      size: Number(file?.size || 0),
      contentType: String(file?.type || "application/octet-stream"),
      inline: composeFileLooksInlineImage(file),
      contentID: composeID("cid"),
      sortOrder: composeAssets().length,
      status: "uploading",
      objectURL: composeFileLooksInlineImage(file) ? URL.createObjectURL(file) : "",
      error: "",
      createdAtMs: Date.now(),
    };
    state.compose.assets = [...composeAssets(), item];
    if (item.inline) {
      insertComposeInlinePlaceholder(item);
    }
    renderComposeAssets();
    syncComposeInlineAssetNodes();
    void uploadComposeAsset(item.id);
  }
  syncComposeDraftFields();
  queueComposeDraftSave();
  updateComposeSubmitState();
}

function setComposeFromMode(mode) {
  state.compose.fromMode = mode;
  if (el.composeFromManualWrap) {
    el.composeFromManualWrap.classList.toggle("hidden", state.compose.fromMode !== "manual");
  }
  if (el.composeFromManualInput) {
    el.composeFromManualInput.disabled = state.compose.fromMode !== "manual";
    if (state.compose.fromMode === "manual" && String(el.composeFromManualInput.value || "").trim() === "") {
      el.composeFromManualInput.value = composeAuthEmailValue();
    }
  }
  if (state.compose.fromMode === "manual") {
    const authEmail = composeAuthEmailValue().toLowerCase();
    const manual = composeResolvedManualSender().toLowerCase();
    if (manual !== "" && manual !== authEmail) {
      setComposeFromNote("Sender must exactly match your authenticated email.", "error");
    } else if (state.compose.identityLookupError) {
      setComposeFromNote("Identity lookup unavailable. Using authenticated sender.", "warn");
    } else {
      setComposeFromNote("");
    }
  } else if (state.compose.fromMode === "identity") {
    setComposeFromNote("");
  }
  updateComposeFromRowVisibility();
  updateComposeFromFields();
  updateComposeSubmitState();
}

function renderComposeFromControls() {
  if (!el.composeFromSelect) return;
  el.composeFromSelect.replaceChildren();
  const items = Array.isArray(state.compose.identities) ? state.compose.identities : [];
  if (items.length === 0) {
    setComposeFromMode("manual");
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "Authenticated sender";
    el.composeFromSelect.appendChild(option);
    el.composeFromSelect.disabled = true;
    if (state.compose.identityLookupError) {
      setComposeFromNote("Identity lookup unavailable. Using authenticated sender.", "warn");
    } else {
      setComposeFromNote("");
    }
    updateComposeFromFields();
    return;
  }

  el.composeFromSelect.disabled = false;
  for (const item of items) {
    const label = [String(item.identity_display_name || "").trim(), String(item.from_email || "").trim()]
      .filter(Boolean)
      .join(" - ");
    const accountLabel = String(item.account_display_name || item.account_login || "").trim();
    const opt = document.createElement("option");
    opt.value = String(item.identity_id || "");
    opt.textContent = accountLabel ? `${accountLabel}: ${label}` : label;
    opt.dataset.accountId = String(item.account_id || "");
    opt.dataset.fromEmail = String(item.from_email || "");
    if (item.identity_is_default || item.account_is_default) opt.selected = true;
    el.composeFromSelect.appendChild(opt);
  }

  const chosen = state.compose.selectedIdentityID
    ? items.find((item) => String(item.identity_id) === state.compose.selectedIdentityID)
    : items.find((item) => item.identity_is_default || item.account_is_default) || items[0];

  if (chosen) {
    el.composeFromSelect.value = String(chosen.identity_id || "");
    state.compose.selectedIdentityID = String(chosen.identity_id || "");
    state.compose.selectedAccountID = String(chosen.account_id || "");
  }
  setComposeFromMode("identity");
  setComposeFromNote("");
  updateComposeFromRowVisibility();
  updateComposeFromFields();
}

async function loadComposeIdentities() {
  state.compose.authEmail = String(state.user?.email || "").trim();
  state.compose.identities = [];
  state.compose.manualFallbackRequired = false;
  state.compose.identityLookupError = "";
  try {
    const payload = await api("/api/v1/compose/identities");
    state.compose.authEmail = String(payload.auth_email || state.user?.email || "").trim();
    state.compose.identities = Array.isArray(payload.items) ? payload.items : [];
    state.compose.manualFallbackRequired = !!payload.manual_fallback_required;
  } catch (err) {
    state.compose.identities = [];
    state.compose.manualFallbackRequired = true;
    state.compose.identityLookupError = String(err.message || "lookup failed");
  }
  renderComposeFromControls();
}

function resetComposeDraftSession(options = {}) {
  const keepCrash = options.keepCrash === true;
  clearComposeDraftSaveTimer();
  state.compose.submitInFlight = false;
  state.compose.draftID = "";
  state.compose.draftLoaded = false;
  state.compose.draftDirty = false;
  state.compose.draftSaving = false;
  state.compose.draftError = "";
  state.compose.draftStatus = "draft";
  state.compose.lastSendError = "";
  state.compose.draftLastSavedAt = "";
  state.compose.draftBaselineJSON = "";
  state.compose.fromMode = "default";
  state.compose.selectedIdentityID = "";
  state.compose.selectedAccountID = "";
  clearComposeAssets({ removeEditorNodes: false });
  state.mail.selectedDraftID = "";
  if (!keepCrash) {
    clearComposeCrashBuffer("");
  }
}

function mergeCrashBufferIntoCompose(draftID, serverUpdatedAt) {
  const crash = readComposeCrashBuffer(draftID);
  if (!crash) return false;
  const crashAt = Number(crash.updated_at_ms || 0);
  const serverAt = Date.parse(String(serverUpdatedAt || ""));
  if (Number.isFinite(serverAt) && crashAt > 0 && crashAt <= serverAt) {
    return false;
  }
  applyComposeDraftPayload(crash, { normalizeDraftMedia: true });
  state.compose.draftDirty = true;
  setComposeDraftNote("Recovered newer unsynced text from this browser.", "warn");
  return true;
}

async function openComposeDraft(draftID, trigger = null) {
  await openComposeOverlay(trigger, {
    draftID,
    useDraft: false,
    title: composeDraftContextLabel("draft"),
  });
}

function composeFocusableElements() {
  if (!el.composeDialog) return [];
  return Array.from(el.composeDialog.querySelectorAll("button, [href], input, textarea, select, [contenteditable='true'], [tabindex]:not([tabindex='-1'])"))
    .filter((node) => !node.disabled && node.offsetParent !== null);
}

function insertComposeHTMLAtCaret(htmlContent) {
  if (!el.composeEditor) return;
  el.composeEditor.focus();
  if (document.queryCommandSupported && document.queryCommandSupported("insertHTML")) {
    document.execCommand("insertHTML", false, htmlContent);
  } else {
    const selection = window.getSelection();
    const inEditor = selection && el.composeEditor.contains(selection.anchorNode);
    if (!selection || selection.rangeCount === 0 || !inEditor) {
      el.composeEditor.insertAdjacentHTML("beforeend", htmlContent);
    } else {
      const range = selection.getRangeAt(0);
      range.deleteContents();
      const temp = document.createElement("div");
      temp.innerHTML = htmlContent;
      const fragment = document.createDocumentFragment();
      while (temp.firstChild) fragment.appendChild(temp.firstChild);
      range.insertNode(fragment);
      range.collapse(false);
      selection.removeAllRanges();
      selection.addRange(range);
    }
  }
  syncComposeDraftFields();
  updateComposeSubmitState();
}

function runComposeCommand(command, value = null) {
  if (!el.composeEditor) return;
  el.composeEditor.focus();
  document.execCommand(command, false, value);
  syncComposeDraftFields();
  updateComposeSubmitState();
}

function cycleComposeTypographyMode() {
  const next = state.compose.typographyMode === "p" ? "h3" : "p";
  state.compose.typographyMode = next;
  runComposeCommand("formatBlock", `<${next}>`);
}

async function promptComposeLink() {
  const result = await openUIModal({
    title: "Insert Link",
    body: "Enter a URL to attach to selected text.",
    inputLabel: "URL",
    inputType: "url",
    mode: "prompt",
    trigger: el.composeToolLink,
    confirmText: "Insert",
    cancelText: "Cancel",
  });
  if (!result?.confirmed) {
    if (el.composeEditor) el.composeEditor.focus();
    return;
  }
  const url = String(result.value || "").trim();
  if (!url) {
    if (el.composeEditor) el.composeEditor.focus();
    return;
  }
  runComposeCommand("createLink", url);
}

function focusComposeEditorAtEnd() {
  if (!el.composeEditor) return;
  el.composeEditor.focus();
  const selection = window.getSelection();
  if (!selection) return;
  const range = document.createRange();
  range.selectNodeContents(el.composeEditor);
  range.collapse(false);
  selection.removeAllRanges();
  selection.addRange(range);
}

function setComposeSendContext(mode = "send", messageID = "") {
  const normalizedMode = String(mode || "send").trim().toLowerCase();
  state.compose.sendContext = {
    mode: normalizedMode === "reply" || normalizedMode === "forward" ? normalizedMode : "send",
    messageID: String(messageID || "").trim(),
  };
}

function applyComposePrefill(prefill = {}) {
  const to = String(prefill.to || "").trim();
  const cc = String(prefill.cc || "").trim();
  const bcc = String(prefill.bcc || "").trim();
  const subject = String(prefill.subject || "").trim();
  const bodyText = String(prefill.bodyText || "");

  state.compose.recipients.to = [];
  state.compose.recipients.cc = [];
  state.compose.recipients.bcc = [];
  renderComposeRecipientTokens("to");
  renderComposeRecipientTokens("cc");
  renderComposeRecipientTokens("bcc");
  if (to) hydrateComposeRecipientTokens("to", to);
  if (cc) hydrateComposeRecipientTokens("cc", cc);
  if (bcc) hydrateComposeRecipientTokens("bcc", bcc);
  setComposeCcVisible(cc !== "", { clearWhenHidden: cc === "" });
  setComposeBccVisible(bcc !== "", { clearWhenHidden: bcc === "" });

  if (el.composeToInput) el.composeToInput.value = "";
  if (el.composeCcInput) el.composeCcInput.value = "";
  if (el.composeBccInput) el.composeBccInput.value = "";
  if (el.composeSubjectInput) el.composeSubjectInput.value = subject;
  if (el.composeEditor) {
    const normalized = bodyText.replace(/\r\n/g, "\n");
    const lines = normalized.split("\n");
    el.composeEditor.innerHTML = lines.map((line) => `<p>${escapeHtml(line || "")}</p>`).join("");
  }
}

async function openComposeOverlay(trigger = null, opts = {}) {
  if (!el.composeOverlay) return;
  const title = String(opts.title || "New Message").trim() || "New Message";
  const useDraft = opts.useDraft !== false;
  const draftID = String(opts.draftID || "").trim();
  const prefill = opts.prefill && typeof opts.prefill === "object" ? opts.prefill : null;
  const sendContext = opts.sendContext && typeof opts.sendContext === "object" ? opts.sendContext : { mode: "send", messageID: "" };
  state.ui.composeOpen = true;
  state.ui.composeLastTrigger = trigger || document.activeElement || null;
  el.composeOverlay.classList.remove("hidden");
  el.composeOverlay.setAttribute("aria-hidden", "false");
  document.body.style.overflow = "hidden";
  if (el.composeTitle) {
    el.composeTitle.textContent = title;
  }

  resetComposeDraftSession({ keepCrash: true });
  state.compose.authEmail = String(state.user?.email || "").trim();
  setComposeSendContext("send", "");
  state.compose.recipients.to = [];
  state.compose.recipients.cc = [];
  state.compose.recipients.bcc = [];
  if (el.composeForm) el.composeForm.reset();
  if (el.composeToInput) el.composeToInput.value = "";
  if (el.composeCcInput) el.composeCcInput.value = "";
  if (el.composeBccInput) el.composeBccInput.value = "";
  renderComposeRecipientTokens("to");
  renderComposeRecipientTokens("cc");
  renderComposeRecipientTokens("bcc");
  setComposeCcVisible(false);
  clearComposeAssets({ removeEditorNodes: false });
  setComposeBccVisible(false);
  setComposeFormatToolsVisible(false);
  setComposeFromNote("");
  setComposeDraftNote("");
  setComposeDraftState("Draft", "muted");
  if (el.composeEditor) el.composeEditor.innerHTML = "";
  await loadComposeIdentities();
  if (draftID) {
    const draft = await loadComposeDraftByID(draftID);
    applyComposeDraftPayload(draft, { normalizeDraftMedia: false });
    syncComposeServerDraftState(draft, { keepDirty: false });
    const assetInfo = hydrateComposeDraftAssets(draft);
    normalizeComposeEditorDraftMedia();
    state.mail.selectedDraftID = state.compose.draftID;
    const merged = mergeCrashBufferIntoCompose(state.compose.draftID, draft?.updated_at);
    if (!merged && assetInfo.legacyMissingMedia) {
      setComposeDraftNote("This older draft is missing stored attachments or inline images. Re-add them before sending.", "warn");
    }
    if (String(draft?.compose_mode || "").trim()) {
      el.composeTitle.textContent = composeDraftContextLabel(draft.compose_mode);
    }
  } else if (prefill) {
    applyComposePrefill(prefill);
    state.compose.draftBaselineJSON = composeDraftPayloadJSON(composeCurrentDraftPayload());
  } else if (useDraft) {
    const restored = restoreComposeDraft(el.composeForm);
    state.compose.draftBaselineJSON = composeDraftPayloadJSON(restored ? {} : composeCurrentDraftPayload());
  } else {
    state.compose.draftBaselineJSON = composeDraftPayloadJSON(composeCurrentDraftPayload());
  }
  if (!draftID) {
    setComposeSendContext(sendContext.mode, sendContext.messageID);
  }
  applyComposeSendFailurePresentation();
  syncComposeDraftFields();
  updateComposeSubmitState();
  focusComposeEditorAtEnd();
}

function closeComposeOverlay(options = true) {
  if (!el.composeOverlay) return;
  let restoreFocus = true;
  let persistDraft = true;
  if (typeof options === "object" && options !== null) {
    restoreFocus = options.restoreFocus !== false;
    persistDraft = options.persistDraft !== false;
  } else {
    restoreFocus = options !== false;
  }
  state.ui.composeOpen = false;
  if (persistDraft) {
    writeComposeCrashBuffer(state.compose.draftID || "");
    void flushComposeDraft({ immediate: true });
  } else {
    clearComposeDraftSaveTimer();
  }
  el.composeOverlay.classList.add("hidden");
  el.composeOverlay.setAttribute("aria-hidden", "true");
  document.body.style.overflow = state.ui.modalOpen || state.ui.mfaModalOpen ? "hidden" : "";
  state.compose.submitInFlight = false;
  if (!persistDraft) {
    if (el.composeTitle) {
      el.composeTitle.textContent = "New Message";
    }
    clearComposeAssets();
    setComposeDraftNote("");
  }
  if (restoreFocus && state.ui.composeLastTrigger && typeof state.ui.composeLastTrigger.focus === "function") {
    state.ui.composeLastTrigger.focus();
  }
  state.ui.composeLastTrigger = null;
}

function handleComposeOverlayKeydown(event) {
  if (!state.ui.composeOpen) return;
  if (state.ui.modalOpen) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeComposeOverlay(true);
    return;
  }
  if (event.key !== "Tab") return;
  const focusables = composeFocusableElements();
  if (focusables.length === 0) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  }
}

const modalState = {
  resolver: null,
  rejecter: null,
  mode: "",
};

function modalFocusableElements() {
  if (!el.uiModalCard) return [];
  return Array.from(el.uiModalCard.querySelectorAll("button, [href], input, textarea, select, [tabindex]:not([tabindex='-1'])"))
    .filter((node) => !node.disabled && node.offsetParent !== null);
}

function closeUIModal(result = null) {
  if (!el.uiModalOverlay) return;
  state.ui.modalOpen = false;
  el.uiModalOverlay.classList.add("hidden");
  el.uiModalOverlay.setAttribute("aria-hidden", "true");
  document.body.style.overflow = state.ui.composeOpen || state.ui.mfaModalOpen ? "hidden" : "";
  if (modalState.resolver) {
    modalState.resolver(result);
  }
  modalState.resolver = null;
  modalState.rejecter = null;
  modalState.mode = "";
  if (state.ui.modalLastTrigger && typeof state.ui.modalLastTrigger.focus === "function") {
    state.ui.modalLastTrigger.focus();
  }
  state.ui.modalLastTrigger = null;
}

function openUIModal(options) {
  if (!el.uiModalOverlay || !el.uiModalCard) {
    return Promise.resolve({ confirmed: false, value: "" });
  }
  if (state.ui.modalOpen) {
    closeUIModal({ confirmed: false, value: "" });
  }
  const {
    title = "Action",
    body = "",
    confirmText = "Confirm",
    cancelText = "Cancel",
    inputLabel = "Value",
    inputType = "text",
    inputValue = "",
    mode = "confirm",
    trigger = null,
  } = options || {};

  state.ui.modalOpen = true;
  state.ui.modalLastTrigger = trigger || document.activeElement || null;
  modalState.mode = mode;
  el.uiModalTitle.textContent = title;
  el.uiModalBody.textContent = body;
  el.uiModalConfirm.textContent = confirmText;
  el.uiModalCancel.textContent = cancelText;
  el.uiModalInputLabel.textContent = inputLabel;
  el.uiModalInput.type = inputType;
  el.uiModalInput.value = inputValue;
  const hasInput = mode === "prompt";
  el.uiModalInputWrap.classList.toggle("hidden", !hasInput);
  el.uiModalOverlay.classList.remove("hidden");
  el.uiModalOverlay.setAttribute("aria-hidden", "false");
  document.body.style.overflow = "hidden";
  window.setTimeout(() => {
    if (hasInput) el.uiModalInput.focus();
    else el.uiModalConfirm.focus();
  }, 0);

  return new Promise((resolve, reject) => {
    modalState.resolver = resolve;
    modalState.rejecter = reject;
  });
}

async function showPromptModal(opts) {
  const out = await openUIModal({
    mode: "prompt",
    title: opts?.title || "Input required",
    body: opts?.body || "",
    inputLabel: opts?.label || "Value",
    inputType: opts?.inputType || "text",
    inputValue: opts?.defaultValue || "",
    confirmText: opts?.confirmText || "Confirm",
    cancelText: opts?.cancelText || "Cancel",
    trigger: opts?.trigger || null,
  });
  if (!out || !out.confirmed) return null;
  return String(out.value || "");
}

async function showConfirmModal(opts) {
  const out = await openUIModal({
    mode: "confirm",
    title: opts?.title || "Confirm",
    body: opts?.body || "",
    confirmText: opts?.confirmText || "Confirm",
    cancelText: opts?.cancelText || "Cancel",
    trigger: opts?.trigger || null,
  });
  return !!(out && out.confirmed);
}

function closeOpenRowMenus(except = null) {
  const menus = Array.from(document.querySelectorAll(".row-menu[open]"));
  for (const menu of menus) {
    if (except && menu === except) continue;
    menu.removeAttribute("open");
  }
}

function handleUIModalKeydown(event) {
  if (!state.ui.modalOpen) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeUIModal({ confirmed: false, value: "" });
    return;
  }
  if (event.key === "Enter" && modalState.mode !== "prompt") {
    event.preventDefault();
    closeUIModal({ confirmed: true, value: "" });
    return;
  }
  if (event.key !== "Tab") return;
  const focusables = modalFocusableElements();
  if (focusables.length === 0) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  }
}

const mfaModalState = {
  resolver: null,
  rejecter: null,
  mode: "actions",
  collector: null,
};

function mfaModalFocusableElements() {
  if (!el.mfaModalCard) return [];
  return Array.from(el.mfaModalCard.querySelectorAll("button, [href], input, textarea, select, [tabindex]:not([tabindex='-1'])"))
    .filter((node) => !node.disabled && node.offsetParent !== null);
}

function setMFAModalError(text = "") {
  if (!el.mfaModalError) return;
  el.mfaModalError.textContent = String(text || "");
}

function closeMFAModal(result = null) {
  if (!el.mfaModalOverlay) return;
  state.ui.mfaModalOpen = false;
  el.mfaModalOverlay.classList.add("hidden");
  el.mfaModalOverlay.setAttribute("aria-hidden", "true");
  if (el.mfaModalBody) {
    el.mfaModalBody.textContent = "";
  }
  if (el.mfaModalExtra) {
    el.mfaModalExtra.replaceChildren();
    el.mfaModalExtra.classList.add("hidden");
  }
  if (el.mfaModalActions) {
    el.mfaModalActions.replaceChildren();
  }
  setMFAModalError("");
  document.body.style.overflow = state.ui.composeOpen || state.ui.modalOpen ? "hidden" : "";
  if (mfaModalState.resolver) {
    mfaModalState.resolver(result);
  }
  mfaModalState.resolver = null;
  mfaModalState.rejecter = null;
  mfaModalState.mode = "actions";
  mfaModalState.collector = null;
  if (state.ui.mfaModalLastTrigger && typeof state.ui.mfaModalLastTrigger.focus === "function") {
    state.ui.mfaModalLastTrigger.focus();
  }
  state.ui.mfaModalLastTrigger = null;
}

function openMFAModal(options = {}) {
  if (!el.mfaModalOverlay || !el.mfaModalCard) {
    return Promise.resolve({ action: "cancel", value: "" });
  }
  if (state.ui.mfaModalOpen) {
    closeMFAModal({ action: "cancel", value: "" });
  }
  const {
    title = "Multi-Factor Authentication",
    body = "",
    bodyHTML = "",
    inputLabel = "Code",
    inputType = "text",
    inputValue = "",
    showInput = false,
    trigger = null,
    extraContent = null,
    collect = null,
    actions = [],
  } = options;

  const actionDefs = Array.isArray(actions) && actions.length > 0
    ? actions
    : [
      { id: "confirm", label: "Confirm", kind: "primary" },
      { id: "cancel", label: "Cancel", kind: "ghost" },
    ];

  state.ui.mfaModalOpen = true;
  state.ui.mfaModalLastTrigger = trigger || document.activeElement || null;
  mfaModalState.mode = showInput ? "input" : "actions";
  mfaModalState.collector = typeof collect === "function" ? collect : null;

  el.mfaModalTitle.textContent = title;
  if (bodyHTML) {
    el.mfaModalBody.innerHTML = bodyHTML;
  } else {
    el.mfaModalBody.textContent = body;
  }
  if (el.mfaModalExtra) {
    el.mfaModalExtra.replaceChildren();
    if (extraContent instanceof Node) {
      el.mfaModalExtra.appendChild(extraContent);
      el.mfaModalExtra.classList.remove("hidden");
    } else {
      el.mfaModalExtra.classList.add("hidden");
    }
  }
  el.mfaModalInputLabel.textContent = inputLabel;
  el.mfaModalInput.type = inputType;
  el.mfaModalInput.value = inputValue;
  el.mfaModalInputWrap.classList.toggle("hidden", !showInput);
  setMFAModalError("");
  el.mfaModalActions.replaceChildren();

  for (const action of actionDefs) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "cmd-btn";
    if (action.kind === "primary") button.classList.add("cmd-btn--primary");
    if (action.kind === "danger") button.classList.add("cmd-btn--danger");
    button.textContent = String(action.label || action.id || "Action");
    button.dataset.actionId = String(action.id || "action");
    button.addEventListener("click", () => {
      const value = showInput && el.mfaModalInput ? String(el.mfaModalInput.value || "") : "";
      const meta = typeof mfaModalState.collector === "function" ? (mfaModalState.collector() || {}) : {};
      closeMFAModal({ action: button.dataset.actionId, value, meta });
    });
    el.mfaModalActions.appendChild(button);
  }

  el.mfaModalOverlay.classList.remove("hidden");
  el.mfaModalOverlay.setAttribute("aria-hidden", "false");
  document.body.style.overflow = "hidden";
  window.setTimeout(() => {
    if (showInput && el.mfaModalInput) {
      el.mfaModalInput.focus();
      return;
    }
    const firstButton = el.mfaModalActions.querySelector("button");
    if (firstButton) firstButton.focus();
  }, 0);

  return new Promise((resolve, reject) => {
    mfaModalState.resolver = resolve;
    mfaModalState.rejecter = reject;
  });
}

function handleMFAModalKeydown(event) {
  if (!state.ui.mfaModalOpen) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeMFAModal({ action: "cancel", value: "" });
    return;
  }
  if (event.key === "Enter" && mfaModalState.mode !== "actions") {
    event.preventDefault();
    const value = el.mfaModalInput ? String(el.mfaModalInput.value || "") : "";
    const meta = typeof mfaModalState.collector === "function" ? (mfaModalState.collector() || {}) : {};
    closeMFAModal({ action: "confirm", value, meta });
    return;
  }
  if (event.key !== "Tab") return;
  const focusables = mfaModalFocusableElements();
  if (focusables.length === 0) return;
  const first = focusables[0];
  const last = focusables[focusables.length - 1];
  if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  }
}

function authStageFromPayload(payload = {}) {
  return {
    auth_stage: String(payload.auth_stage || "authenticated"),
    mfa_required: !!payload.mfa_required,
    mfa_setup_required: !!payload.mfa_setup_required,
    mfa_setup_method: String(payload.mfa_setup_method || ""),
    mfa_setup_step: String(payload.mfa_setup_step || ""),
    mfa_enrolled: !!payload.mfa_enrolled,
    legacy_mfa_prompt: !!payload.legacy_mfa_prompt,
    mfa_preference: String(payload.mfa_preference || "none"),
    mfa_trusted_supported: payload.mfa_trusted_supported !== false,
  };
}

function requiresMFAStageAuthentication(payload = {}) {
  const stage = authStageFromPayload(payload);
  return stage.auth_stage === "mfa_required" || stage.auth_stage === "mfa_setup_required";
}

function supportsWebAuthn() {
  return !!(window.PublicKeyCredential && navigator.credentials && typeof navigator.credentials.create === "function" && typeof navigator.credentials.get === "function");
}

function isHexString(raw) {
  return /^[0-9a-f]+$/i.test(String(raw || "")) && String(raw || "").length % 2 === 0;
}

function hexToBytes(raw) {
  const value = String(raw || "");
  const out = new Uint8Array(value.length / 2);
  for (let i = 0; i < out.length; i += 1) {
    out[i] = Number.parseInt(value.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

function base64urlToBytes(raw) {
  const value = String(raw || "").replace(/-/g, "+").replace(/_/g, "/");
  const pad = value.length % 4 === 0 ? "" : "=".repeat(4 - (value.length % 4));
  const binary = atob(value + pad);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

function bytesToBase64url(bufferLike) {
  const bytes = bufferLike instanceof Uint8Array ? bufferLike : new Uint8Array(bufferLike);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function decodeCredentialID(raw) {
  const value = String(raw || "").trim();
  if (!value) return new Uint8Array();
  try {
    return base64urlToBytes(value);
  } catch {
    if (isHexString(value)) {
      return hexToBytes(value);
    }
    return new TextEncoder().encode(value);
  }
}

function normalizePublicKeyCreateOptions(beginPayload = {}) {
  const pk = beginPayload.public_key || beginPayload;
  const user = pk.user || beginPayload.user || {};
  const excludeCredentials = Array.isArray(pk.excludeCredentials)
    ? pk.excludeCredentials
    : Array.isArray(beginPayload.exclude_credentials) ? beginPayload.exclude_credentials : [];
  return {
    challenge: decodeCredentialID(pk.challenge || beginPayload.challenge),
    rp: pk.rp || beginPayload.rp || { id: beginPayload.rp_id || window.location.hostname, name: "Despatch" },
    user: {
      id: decodeCredentialID(user.id || ""),
      name: String(user.name || ""),
      displayName: String(user.displayName || user.display_name || user.name || ""),
    },
    pubKeyCredParams: Array.isArray(pk.pubKeyCredParams) ? pk.pubKeyCredParams : (Array.isArray(beginPayload.pub_key_cred_params) ? beginPayload.pub_key_cred_params : [{ type: "public-key", alg: -7 }]),
    timeout: Number(pk.timeout || beginPayload.timeout_ms || 300000),
    attestation: String(pk.attestation || "none"),
    authenticatorSelection: pk.authenticatorSelection || { userVerification: "preferred" },
    excludeCredentials: excludeCredentials.map((item) => ({
      type: "public-key",
      id: decodeCredentialID(item.id),
      transports: Array.isArray(item.transports) ? item.transports : undefined,
    })),
  };
}

function normalizePublicKeyGetOptions(beginPayload = {}) {
  const pk = beginPayload.public_key || beginPayload;
  const allowCredentials = Array.isArray(pk.allowCredentials)
    ? pk.allowCredentials
    : Array.isArray(beginPayload.allow_credentials) ? beginPayload.allow_credentials : [];
  const options = {
    challenge: decodeCredentialID(pk.challenge || beginPayload.challenge),
    rpId: String(pk.rpId || beginPayload.rp_id || window.location.hostname),
    timeout: Number(pk.timeout || beginPayload.timeout_ms || 300000),
    userVerification: String(pk.userVerification || "preferred"),
  };
  if (allowCredentials.length > 0) {
    options.allowCredentials = allowCredentials.map((item) => ({
      type: "public-key",
      id: decodeCredentialID(item.id),
      transports: Array.isArray(item.transports) ? item.transports : undefined,
    }));
  }
  return options;
}

function credentialToPayload(credential) {
  if (!credential || !credential.response) return {};
  const base = {
    id: credential.id,
    rawId: bytesToBase64url(new Uint8Array(credential.rawId)),
    type: credential.type || "public-key",
  };
  if (credential.response.attestationObject) {
    return {
      ...base,
      response: {
        clientDataJSON: bytesToBase64url(new Uint8Array(credential.response.clientDataJSON)),
        attestationObject: bytesToBase64url(new Uint8Array(credential.response.attestationObject)),
      },
      transports: typeof credential.response.getTransports === "function" ? credential.response.getTransports() : [],
    };
  }
  return {
    ...base,
    response: {
      clientDataJSON: bytesToBase64url(new Uint8Array(credential.response.clientDataJSON)),
      authenticatorData: bytesToBase64url(new Uint8Array(credential.response.authenticatorData)),
      signature: bytesToBase64url(new Uint8Array(credential.response.signature)),
      userHandle: credential.response.userHandle ? bytesToBase64url(new Uint8Array(credential.response.userHandle)) : "",
    },
  };
}

async function copyTextToClipboard(text) {
  const value = String(text || "");
  if (!value) return false;
  try {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // fallback below
  }
  const area = document.createElement("textarea");
  area.value = value;
  area.style.position = "fixed";
  area.style.left = "-9999px";
  document.body.appendChild(area);
  area.focus();
  area.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  document.body.removeChild(area);
  return ok;
}

function downloadTextFile(filename, content) {
  const blob = new Blob([String(content || "")], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function printRecoveryCodes(codes) {
  const text = Array.isArray(codes) ? codes.join("\n") : "";
  const popup = window.open("", "_blank", "noopener,noreferrer");
  if (!popup) {
    setStatus("Popup blocked. Use Download instead to save recovery codes.", "error");
    return;
  }
  popup.document.write("<!doctype html><html><head><title>Recovery Codes</title></head><body>");
  popup.document.write("<h1>Recovery Codes</h1>");
  popup.document.write(`<pre>${text.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]))}</pre>`);
  popup.document.write("</body></html>");
  popup.document.close();
  popup.focus();
  popup.print();
}

function createRecoveryCodesPanel(codes, options = {}) {
  const values = Array.isArray(codes) ? codes.map((code) => String(code || "").trim()).filter(Boolean) : [];
  const wrapper = document.createElement("section");
  wrapper.className = "mfa-recovery-card";

  const title = document.createElement("strong");
  title.textContent = "Recovery codes";
  wrapper.appendChild(title);

  const hint = document.createElement("p");
  hint.className = "hint";
  hint.textContent = "Store these codes offline. Each code can be used only once if you lose access to your MFA device.";
  wrapper.appendChild(hint);

  const list = document.createElement("ol");
  list.className = "mfa-recovery-list";
  for (const code of values) {
    const li = document.createElement("li");
    li.textContent = code;
    list.appendChild(li);
  }
  wrapper.appendChild(list);

  const actions = document.createElement("div");
  actions.className = "mfa-recovery-actions";

  const copyBtn = document.createElement("button");
  copyBtn.type = "button";
  copyBtn.className = "cmd-btn cmd-btn--dense";
  copyBtn.textContent = "Copy";
  copyBtn.addEventListener("click", async () => {
    const ok = await copyTextToClipboard(values.join("\n"));
    setStatus(ok ? "Recovery codes copied." : "Failed to copy recovery codes.", ok ? "ok" : "error");
  });
  actions.appendChild(copyBtn);

  const downloadBtn = document.createElement("button");
  downloadBtn.type = "button";
  downloadBtn.className = "cmd-btn cmd-btn--dense";
  downloadBtn.textContent = "Download .txt";
  downloadBtn.addEventListener("click", () => {
    downloadTextFile("despatch-recovery-codes.txt", values.join("\n"));
  });
  actions.appendChild(downloadBtn);

  const printBtn = document.createElement("button");
  printBtn.type = "button";
  printBtn.className = "cmd-btn cmd-btn--dense";
  printBtn.textContent = "Print";
  printBtn.addEventListener("click", () => printRecoveryCodes(values));
  actions.appendChild(printBtn);

  wrapper.appendChild(actions);

  let ackInput = null;
  if (options.requireAck) {
    const ackWrap = document.createElement("label");
    ackWrap.className = "mfa-inline-check";
    ackInput = document.createElement("input");
    ackInput.type = "checkbox";
    ackInput.checked = !!options.ackDefault;
    const ackText = document.createElement("span");
    ackText.textContent = options.ackLabel || "I saved these recovery codes.";
    ackWrap.appendChild(ackInput);
    ackWrap.appendChild(ackText);
    wrapper.appendChild(ackWrap);
  }

  let rememberInput = null;
  if (options.includeRemember) {
    const rememberWrap = document.createElement("label");
    rememberWrap.className = "mfa-inline-check";
    rememberInput = document.createElement("input");
    rememberInput.type = "checkbox";
    rememberInput.checked = options.rememberDefault !== false;
    const rememberText = document.createElement("span");
    rememberText.textContent = "Remember this device for 30 days";
    rememberWrap.appendChild(rememberInput);
    rememberWrap.appendChild(rememberText);
    wrapper.appendChild(rememberWrap);
  }

  return {
    node: wrapper,
    getAck: () => !!(ackInput && ackInput.checked),
    getRemember: () => !!(rememberInput && rememberInput.checked),
    values,
  };
}

function createBackupConfirmationPanel(options = {}) {
  const wrapper = document.createElement("section");
  wrapper.className = "mfa-recovery-card";

  const text = document.createElement("p");
  text.className = "hint";
  text.textContent = options.body || "Confirm that you saved your recovery codes to complete MFA setup.";
  wrapper.appendChild(text);

  const ackWrap = document.createElement("label");
  ackWrap.className = "mfa-inline-check";
  const ackInput = document.createElement("input");
  ackInput.type = "checkbox";
  ackInput.checked = !!options.ackDefault;
  const ackText = document.createElement("span");
  ackText.textContent = options.ackLabel || "I saved my recovery codes.";
  ackWrap.appendChild(ackInput);
  ackWrap.appendChild(ackText);
  wrapper.appendChild(ackWrap);

  const rememberWrap = document.createElement("label");
  rememberWrap.className = "mfa-inline-check";
  const rememberInput = document.createElement("input");
  rememberInput.type = "checkbox";
  rememberInput.checked = options.rememberDefault !== false;
  const rememberText = document.createElement("span");
  rememberText.textContent = "Remember this device for 30 days";
  rememberWrap.appendChild(rememberInput);
  rememberWrap.appendChild(rememberText);
  wrapper.appendChild(rememberWrap);

  return {
    node: wrapper,
    getAck: () => !!ackInput.checked,
    getRemember: () => !!rememberInput.checked,
  };
}

function createRememberDevicePanel(defaultChecked = true) {
  const wrapper = document.createElement("section");
  wrapper.className = "mfa-recovery-card";
  const text = document.createElement("p");
  text.className = "hint";
  text.textContent = "Choose how to verify this login session.";
  wrapper.appendChild(text);
  const rememberWrap = document.createElement("label");
  rememberWrap.className = "mfa-inline-check";
  const rememberInput = document.createElement("input");
  rememberInput.type = "checkbox";
  rememberInput.checked = defaultChecked !== false;
  const rememberText = document.createElement("span");
  rememberText.textContent = "Remember this device for 30 days";
  rememberWrap.appendChild(rememberInput);
  rememberWrap.appendChild(rememberText);
  wrapper.appendChild(rememberWrap);
  return {
    node: wrapper,
    getRemember: () => !!rememberInput.checked,
  };
}

async function fetchCurrentAuthStage() {
  const me = await api("/api/v1/me", {
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
    logErrors: false,
  });
  state.user = me;
  applyNavVisibility();
  return authStageFromPayload(me);
}

async function verifyTOTPCodeFlow(endpoint, title = "Enter Authenticator Code", label = "Authenticator Code", rememberDevice = true) {
  let errorHint = "";
  while (true) {
    const out = await openMFAModal({
      title,
      body: errorHint || (title.includes("Recovery") ? "Enter one of your saved recovery codes." : "Enter the 6-digit code from your authenticator app."),
      showInput: true,
      inputLabel: label,
      inputType: "text",
      actions: [
        { id: "confirm", label: "Verify", kind: "primary" },
        { id: "cancel", label: "Cancel", kind: "ghost" },
      ],
    });
    if (!out || out.action !== "confirm") {
      throw new Error("MFA verification was cancelled");
    }
    const code = String(out.value || "").trim();
    if (!code) {
      errorHint = "Enter a code to continue.";
      continue;
    }
    try {
      await api(endpoint, {
        method: "POST",
        json: { code, remember_device: !!rememberDevice },
        skipUnauthorizedHandling: true,
        skipMFAHandling: true,
      });
      return;
    } catch (err) {
      errorHint = formatAPIError(err, "Verification failed.");
    }
  }
}

async function runTOTPSetupFlow() {
  const enroll = await api("/api/v2/security/mfa/totp/enroll", {
    method: "POST",
    json: {},
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
  });

  const instructions = Array.isArray(enroll.setup_instructions) ? enroll.setup_instructions : [];
  const setupPanel = document.createElement("div");
  setupPanel.className = "mfa-qr-wrap";

  if (String(enroll.qr_png_data_url || "")) {
    const qr = document.createElement("img");
    qr.className = "mfa-qr-image";
    qr.src = String(enroll.qr_png_data_url);
    qr.alt = "Authenticator app QR code";
    setupPanel.appendChild(qr);
  }

  const manualWrap = document.createElement("div");
  manualWrap.className = "mfa-manual-key";
  const keyLabel = document.createElement("span");
  keyLabel.textContent = "Can’t scan?";
  const keyValue = document.createElement("code");
  keyValue.textContent = String(enroll.manual_entry_key || enroll.secret || "");
  const keyCopyBtn = document.createElement("button");
  keyCopyBtn.type = "button";
  keyCopyBtn.className = "cmd-btn cmd-btn--dense";
  keyCopyBtn.textContent = "Copy key";
  keyCopyBtn.addEventListener("click", async () => {
    const ok = await copyTextToClipboard(keyValue.textContent);
    setStatus(ok ? "Manual key copied." : "Failed to copy manual key.", ok ? "ok" : "error");
  });
  manualWrap.appendChild(keyLabel);
  manualWrap.appendChild(keyValue);
  manualWrap.appendChild(keyCopyBtn);
  setupPanel.appendChild(manualWrap);

  const appHint = document.createElement("p");
  appHint.className = "hint";
  appHint.textContent = "Supported apps: Google Authenticator, Microsoft Authenticator, 1Password, Authy.";
  setupPanel.appendChild(appHint);

  const setupIntro = await openMFAModal({
    title: "Set Up Authenticator App",
    body: instructions.length ? instructions.join(" ") : "Scan the QR code in your authenticator app.",
    extraContent: setupPanel,
    actions: [
      { id: "continue", label: "Continue", kind: "primary" },
      { id: "cancel", label: "Cancel", kind: "ghost" },
    ],
  });
  if (!setupIntro || setupIntro.action !== "continue") {
    throw new Error("TOTP setup was cancelled");
  }

  let errorHint = "";
  while (true) {
    const recoveryPanel = createRecoveryCodesPanel(enroll.recovery_codes, {
      requireAck: true,
      includeRemember: true,
      ackDefault: false,
      rememberDefault: true,
      ackLabel: "I saved these recovery codes.",
    });
    const out = await openMFAModal({
      title: "Confirm Authenticator App",
      body: errorHint || "Enter the 6-digit code from your authenticator app, then confirm recovery codes are saved.",
      showInput: true,
      inputLabel: "Authenticator code",
      inputType: "text",
      extraContent: recoveryPanel.node,
      collect: () => ({
        recovery_ack: recoveryPanel.getAck(),
        remember_device: recoveryPanel.getRemember(),
      }),
      actions: [
        { id: "confirm", label: "Enable MFA", kind: "primary" },
        { id: "cancel", label: "Cancel", kind: "ghost" },
      ],
    });
    if (!out || out.action !== "confirm") {
      throw new Error("TOTP setup was cancelled");
    }
    const code = String(out.value || "").trim();
    const ack = !!out.meta?.recovery_ack;
    const remember = !!out.meta?.remember_device;
    if (!ack) {
      errorHint = "Confirm that recovery codes are saved before continuing.";
      continue;
    }
    if (!code) {
      errorHint = "Enter the authenticator code to continue.";
      continue;
    }
    try {
      await api("/api/v2/security/mfa/totp/confirm", {
        method: "POST",
        json: {
          code,
          recovery_codes_ack: true,
          remember_device: remember,
        },
        skipUnauthorizedHandling: true,
        skipMFAHandling: true,
      });
      return;
    } catch (err) {
      errorHint = formatAPIError(err, "Failed to enable authenticator app.");
    }
  }
}

async function runMFARecoveryAckFlow(recoveryCodes = []) {
  let errorHint = "";
  while (true) {
    const useRecoveryCard = Array.isArray(recoveryCodes) && recoveryCodes.length > 0;
    const panel = useRecoveryCard
      ? createRecoveryCodesPanel(recoveryCodes, {
        requireAck: true,
        includeRemember: true,
        ackDefault: false,
        rememberDefault: true,
        ackLabel: "I saved these recovery codes.",
      })
      : createBackupConfirmationPanel({
        body: "Finish MFA setup by confirming your recovery codes are saved.",
        ackDefault: false,
        rememberDefault: true,
      });
    const out = await openMFAModal({
      title: "Finish MFA Setup",
      body: errorHint || "Recovery codes are required as your backup sign-in method.",
      extraContent: panel.node,
      collect: () => ({
        recovery_ack: panel.getAck(),
        remember_device: panel.getRemember(),
      }),
      actions: [
        { id: "confirm", label: "Finish Setup", kind: "primary" },
        { id: "cancel", label: "Cancel", kind: "ghost" },
      ],
    });
    if (!out || out.action !== "confirm") {
      throw new Error("MFA setup was cancelled.");
    }
    if (!out.meta?.recovery_ack) {
      errorHint = "Confirm that recovery codes are saved before continuing.";
      continue;
    }
    try {
      await api("/api/v2/security/mfa/recovery-codes/ack", {
        method: "POST",
        json: {
          recovery_codes_ack: true,
          remember_device: !!out.meta?.remember_device,
        },
        skipUnauthorizedHandling: true,
        skipMFAHandling: true,
      });
      return;
    } catch (err) {
      errorHint = formatAPIError(err, "Failed to confirm recovery codes.");
    }
  }
}

async function runWebAuthnSetupFlow() {
  if (!supportsWebAuthn()) {
    throw new Error("Passkey setup is not supported in this browser or device.");
  }
  const begin = await api("/api/v2/security/mfa/webauthn/register/begin", {
    method: "POST",
    json: {},
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
  });
  const options = normalizePublicKeyCreateOptions(begin);
  const credential = await navigator.credentials.create({ publicKey: options });
  if (!credential) {
    throw new Error("Passkey registration was cancelled.");
  }
  const payload = credentialToPayload(credential);
  payload.challenge = String(begin.challenge || "");
  payload.remember_device = false;
  const finish = await api("/api/v2/security/mfa/webauthn/register/finish", {
    method: "POST",
    json: payload,
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
  });
  await runMFARecoveryAckFlow(Array.isArray(finish.recovery_codes) ? finish.recovery_codes : []);
}

async function runWebAuthnVerifyFlow(rememberDevice = true) {
  if (!supportsWebAuthn()) {
    throw new Error("Passkey verification is not supported in this browser.");
  }
  const begin = await api("/api/v2/mfa/webauthn/begin", {
    method: "POST",
    json: {},
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
  });
  const options = normalizePublicKeyGetOptions(begin);
  const credential = await navigator.credentials.get({ publicKey: options });
  if (!credential) {
    throw new Error("Passkey verification was cancelled.");
  }
  await api("/api/v2/mfa/webauthn/finish", {
    method: "POST",
    json: Object.assign({}, credentialToPayload(credential), { remember_device: !!rememberDevice }),
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
  });
}

async function runPasskeyPrimaryLoginFlow() {
  if (!supportsWebAuthn()) {
    throw new Error("Passkey login is not supported in this browser.");
  }
  const caps = authCapabilities();
  if (!caps.passkey_passwordless_available) {
    throw new Error(authCapabilityReasonMessage(caps.reason, "Passkey login is not available."));
  }
  const begin = await api("/api/v1/login/passkey/begin", {
    method: "POST",
    json: {},
    logErrors: false,
  });
  const options = normalizePublicKeyGetOptions(begin);
  const credential = await navigator.credentials.get({ publicKey: options });
  if (!credential) {
    throw new Error("Passkey login was cancelled.");
  }
  return api("/api/v1/login/passkey/finish", {
    method: "POST",
    json: Object.assign({}, credentialToPayload(credential), {
      challenge_id: String(begin.challenge_id || ""),
      challenge: String(begin.challenge || ""),
    }),
    logErrors: false,
  });
}

function formatPasskeyPrimaryLoginError(err) {
  if (!err || typeof err !== "object") {
    return "Passkey login failed.";
  }
  if (err.code !== "invalid_credentials") {
    return formatAPIError(err, "Passkey login failed.");
  }
  return `No discoverable passkey was accepted on this device. Try again or sign in with email and password.${apiRequestRef(err)}`;
}

async function runMFASetupStage(stage) {
  const method = String(stage.mfa_setup_method || stage.mfa_preference || "").toLowerCase();
  const setupStep = String(stage.mfa_setup_step || "method").toLowerCase();
  if (setupStep === "backup") {
    await runMFARecoveryAckFlow([]);
    return;
  }
  const switchTarget = method === "totp" ? "webauthn" : "totp";
  const switchLabel = method === "totp" ? "Switch To Passkey" : "Switch To Authenticator App";
  while (true) {
    try {
      if (method === "totp") {
        await runTOTPSetupFlow();
      } else if (method === "webauthn") {
        await runWebAuthnSetupFlow();
      } else {
        throw new Error("Unsupported MFA setup method.");
      }
      return;
    } catch (err) {
      const actions = [
        { id: "retry", label: "Retry", kind: "primary" },
        { id: "cancel", label: "Cancel", kind: "ghost" },
      ];
      if (method === "totp" || method === "webauthn") {
        actions.splice(1, 0, { id: "switch", label: switchLabel, kind: "ghost" });
      }
      const out = await openMFAModal({
        title: "MFA Setup Required",
        body: formatAPIError(err, "Setup failed."),
        actions,
      });
      if (!out || out.action === "cancel") {
        throw err;
      }
      if (out.action === "switch") {
        await api("/api/v2/security/mfa/preference", {
          method: "POST",
          json: { preference: switchTarget },
          skipUnauthorizedHandling: true,
          skipMFAHandling: true,
        });
        return;
      }
    }
  }
}

async function runMFAVerificationStage() {
  let lastError = "";
  while (true) {
    const rememberPanel = createRememberDevicePanel(true);
    const actions = [
      { id: "totp", label: "Use Authenticator Code", kind: "primary" },
      { id: "recovery", label: "Use Recovery Code", kind: "ghost" },
      { id: "cancel", label: "Cancel", kind: "ghost" },
    ];
    if (supportsWebAuthn()) {
      actions.unshift({ id: "passkey", label: "Use Passkey", kind: "primary" });
    }
    const out = await openMFAModal({
      title: "MFA Verification Required",
      body: lastError ? `Verification failed: ${lastError}` : "Choose how to verify this login session.",
      extraContent: rememberPanel.node,
      collect: () => ({ remember_device: rememberPanel.getRemember() }),
      actions,
    });
    if (!out || out.action === "cancel") {
      throw new Error("MFA verification was cancelled.");
    }
    const rememberDevice = !!out.meta?.remember_device;
    try {
      if (out.action === "passkey") {
        await runWebAuthnVerifyFlow(rememberDevice);
      } else if (out.action === "totp") {
        await verifyTOTPCodeFlow("/api/v2/mfa/totp/verify", "Enter Authenticator Code", "Authenticator code", rememberDevice);
      } else if (out.action === "recovery") {
        await verifyTOTPCodeFlow("/api/v2/mfa/recovery-code/verify", "Enter Recovery Code", "Recovery code", rememberDevice);
      }
      lastError = "";
      return;
    } catch (err) {
      lastError = formatAPIError(err, "Verification failed.");
    }
  }
}

async function ensureMFAStageAuthenticated(initial = null) {
  if (state.auth.mfaFlowPromise) {
    return state.auth.mfaFlowPromise;
  }
  state.auth.mfaFlowPromise = (async () => {
    let stage = authStageFromPayload(initial || {});
    if (!initial) {
      stage = await fetchCurrentAuthStage();
    }
    while (stage.auth_stage !== "authenticated") {
      if (stage.auth_stage === "mfa_setup_required") {
        await runMFASetupStage(stage);
      } else if (stage.auth_stage === "mfa_required") {
        await runMFAVerificationStage(stage);
      } else {
        throw new Error(`Unsupported authentication stage: ${stage.auth_stage}`);
      }
      stage = await fetchCurrentAuthStage();
    }
    closeMFAModal({ action: "done", value: "" });
    return stage;
  })().finally(() => {
    state.auth.mfaFlowPromise = null;
  });
  return state.auth.mfaFlowPromise;
}

async function promptLegacyMFAIfNeeded() {
  if (!state.user || !state.user.legacy_mfa_prompt || state.auth.legacyMFAOfferShownForSession) {
    return;
  }
  state.auth.legacyMFAOfferShownForSession = true;
  const out = await openMFAModal({
    title: "Secure Your Account",
    body: "Set up multi-factor authentication now for stronger account security.",
    actions: [
      { id: "totp", label: "Set Up Authenticator App", kind: "primary" },
      { id: "passkey", label: "Set Up Passkey", kind: "primary" },
      { id: "later", label: "Not now", kind: "ghost" },
    ],
  });
  if (!out || out.action === "later") {
    await api("/api/v2/security/mfa/legacy-dismiss", {
      method: "POST",
      json: {},
      skipUnauthorizedHandling: true,
      skipMFAHandling: true,
      logErrors: false,
    });
    state.user.legacy_mfa_prompt = false;
    return;
  }
  try {
    if (out.action === "totp") {
      await runTOTPSetupFlow();
    } else if (out.action === "passkey") {
      await runWebAuthnSetupFlow();
    }
    await api("/api/v2/security/mfa/legacy-dismiss", {
      method: "POST",
      json: {},
      skipUnauthorizedHandling: true,
      skipMFAHandling: true,
      logErrors: false,
    });
    state.user.legacy_mfa_prompt = false;
    setStatus("MFA has been enabled for this account.", "ok");
  } catch (err) {
    setStatus(formatAPIError(err, "MFA setup failed."), "error");
  } finally {
    closeMFAModal({ action: "done", value: "" });
  }
}

async function api(path, opts = {}) {
  const method = opts.method || "GET";
  const headers = Object.assign({}, opts.headers || {});
  const init = { method, credentials: "include", headers };

  if (opts.json !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.json);
  } else if (opts.body !== undefined) {
    init.body = opts.body;
  }

  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    const csrf = getCookie("despatch_csrf");
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  const res = await fetch(path, init);
  const text = await res.text();
  const payload = text ? (() => {
    try { return JSON.parse(text); } catch { return {}; }
  })() : {};

  if (!res.ok) {
    const error = new Error(payload.message || payload.code || `HTTP ${res.status}`);
    error.code = payload.code || "request_failed";
    error.retryAfterSec = Number(res.headers.get("Retry-After") || "0");
    error.status = res.status;
    error.requestID = payload.request_id || res.headers.get("X-Request-ID") || "";
    if (opts.logErrors !== false && window && window.console && typeof window.console.error === "function") {
      console.error("API request failed", {
        path,
        method,
        status: error.status,
        code: error.code,
        message: error.message,
        request_id: error.requestID,
      });
    }
    const shouldHandleUnauthorized = error.status === 401
      && isSessionErrorCode(error.code)
      && isProtectedAPIPath(path)
      && !opts.skipUnauthorizedHandling
      && !state.setup.required
      && !!state.user;
    if (shouldHandleUnauthorized) {
      routeToAuthWithMessage(reauthMessageForCode(error.code), error.code);
    }
    const shouldHandleMFAStage = error.status === 401
      && isMFAStageCode(error.code)
      && isProtectedAPIPath(path)
      && !opts.skipMFAHandling
      && !state.setup.required;
    if (shouldHandleMFAStage) {
      try {
        await ensureMFAStageAuthenticated(payload);
        return api(path, Object.assign({}, opts, {
          skipMFAHandling: true,
          skipUnauthorizedHandling: true,
        }));
      } catch (mfaErr) {
        error.message = formatAPIError(mfaErr, "Multi-factor authentication is required.");
      }
    }
    throw error;
  }
  return payload;
}

function ensureSlashSuffix(v) {
  const value = String(v || "").trim();
  if (!value) return "";
  if (value.endsWith("/")) return value;
  return `${value}/`;
}

function setCaptchaToken(token) {
  state.captcha.token = String(token || "").trim();
  if (el.captchaTokenHidden) {
    el.captchaTokenHidden.value = state.captcha.token;
  }
  if (el.captchaManualInput && document.activeElement !== el.captchaManualInput) {
    el.captchaManualInput.value = state.captcha.token;
  }
  syncRegisterSubmitState();
}

function setCaptchaNote(text, type = "info") {
  if (!el.captchaNote) return;
  el.captchaNote.textContent = text || "";
  if (type === "error") el.captchaNote.style.color = "var(--sig-err)";
  else if (type === "ok") el.captchaNote.style.color = "var(--sig-ok)";
  else el.captchaNote.style.color = "var(--fg-muted)";
}

function setCaptchaError(text) {
  state.captcha.widgetError = String(text || "");
  if (!el.captchaError) return;
  el.captchaError.textContent = state.captcha.widgetError;
  el.captchaError.style.color = state.captcha.widgetError ? "var(--sig-err)" : "var(--fg-muted)";
}

function syncRegisterSubmitState() {
  if (!el.registerSubmit) return;
  const cfg = state.captcha.config;
  if (!cfg || !cfg.enabled) {
    el.registerSubmit.disabled = false;
    return;
  }
  const provider = String(cfg.provider || "").toLowerCase();
  if (provider !== "cap") {
    el.registerSubmit.disabled = state.captcha.token.length === 0;
    return;
  }
  if (state.captcha.widgetBlocked) {
    el.registerSubmit.disabled = true;
    return;
  }
  el.registerSubmit.disabled = state.captcha.token.length === 0;
}

function renderCaptchaShell(show) {
  if (!el.captchaShell) return;
  el.captchaShell.classList.toggle("hidden", !show);
}

function showCaptchaManualInput(show) {
  if (!el.captchaManualWrap) return;
  el.captchaManualWrap.classList.toggle("hidden", !show);
}

function clearCaptchaWidget() {
  if (!el.captchaWidgetContainer) return;
  el.captchaWidgetContainer.replaceChildren();
}

function capWidgetPaths(cfg) {
  const endpoint = ensureSlashSuffix(cfg?.widget_api_url || "");
  if (!endpoint) {
    throw new Error("CAPTCHA_WIDGET_API_URL is not set");
  }
  const siteKey = String(cfg?.site_key || "").trim();
  const parsed = new URL(endpoint, window.location.origin);
  let path = parsed.pathname.replace(/\/+$/, "");
  if (siteKey) {
    const suffix = `/${siteKey}`.toLowerCase();
    if (path.toLowerCase().endsWith(suffix)) {
      path = path.slice(0, path.length - suffix.length);
    }
  }
  if (!path) path = "/";
  const assetBase = `${parsed.origin}${path}`.replace(/\/$/, "");
  return {
    endpoint: parsed.toString(),
    scriptURL: `${assetBase}/assets/widget.js`,
    wasmURL: `${assetBase}/assets/cap_wasm_bg.wasm`,
    hashesURL: `${assetBase}/assets/wasm-hashes.min.js`,
  };
}

async function fetchCapAsset(url, expectsBinary) {
  const res = await fetch(url, {
    method: "GET",
    cache: "no-store",
    credentials: "omit",
  });
  if (!res.ok) {
    throw new Error(`asset ${url} returned HTTP ${res.status}`);
  }
  if (expectsBinary) {
    const ctype = String(res.headers.get("content-type") || "").toLowerCase();
    if (ctype.includes("text/html") || ctype.startsWith("text/")) {
      throw new Error(`asset ${url} returned non-binary content-type ${ctype || "<empty>"}`);
    }
    await res.arrayBuffer();
    return;
  }
  const body = await res.text();
  if (!body || body.includes("Failed to resolve the requested file")) {
    throw new Error(`asset ${url} is unresolved in CAP runtime`);
  }
}

async function preflightCapAssets(paths) {
  await fetchCapAsset(paths.scriptURL, false);
  await fetchCapAsset(paths.wasmURL, true);
  let hashesURL = "";
  try {
    await fetchCapAsset(paths.hashesURL, false);
    hashesURL = paths.hashesURL;
  } catch {
    hashesURL = "";
  }
  return { hashesURL };
}

function loadCapWidgetScript(paths, preflight = null) {
  if (state.captcha.scriptLoaded && window.customElements && window.customElements.get("cap-widget")) {
    return Promise.resolve();
  }
  if (state.captcha.scriptPromise) {
    return state.captcha.scriptPromise;
  }
  state.captcha.scriptPromise = new Promise((resolve, reject) => {
    window.CAP_CUSTOM_WASM_URL = paths.wasmURL;
    if (preflight?.hashesURL) {
      window.CAP_CUSTOM_HASHES_URL = preflight.hashesURL;
    } else {
      delete window.CAP_CUSTOM_HASHES_URL;
    }
    const existing = document.querySelector("script[data-cap-widget-script='1']");
    if (existing) {
      existing.addEventListener("load", () => {
        state.captcha.scriptLoaded = true;
        resolve();
      }, { once: true });
      existing.addEventListener("error", () => reject(new Error(`failed to load cap widget script: ${paths.scriptURL}`)), { once: true });
      return;
    }
    const script = document.createElement("script");
    script.src = paths.scriptURL;
    script.defer = true;
    script.dataset.capWidgetScript = "1";
    script.onload = () => {
      state.captcha.scriptLoaded = true;
      resolve();
    };
    script.onerror = () => {
      reject(new Error(`failed to load cap widget script: ${paths.scriptURL}`));
    };
    document.head.appendChild(script);
  }).finally(() => {
    state.captcha.scriptPromise = null;
  });
  return state.captcha.scriptPromise;
}

function mountCapWidget(paths) {
  clearCaptchaWidget();
  if (!el.captchaWidgetContainer) return;
  const widget = document.createElement("cap-widget");
  // cap-widget versions in standalone runtime read `cap-api-endpoint` directly.
  // Keep the data-* attribute for forward compatibility.
  widget.setAttribute("cap-api-endpoint", paths.endpoint);
  widget.setAttribute("data-cap-api-endpoint", paths.endpoint);
  widget.setAttribute("data-cap-hidden-field-name", "cap_internal_token");
  widget.setAttribute("data-cap-language", "en");
  widget.setAttribute("data-cap-max-retries", "3");
  widget.addEventListener("solve", (event) => {
    const detailToken = String(event?.detail?.token || "").trim();
    if (detailToken) {
      setCaptchaToken(detailToken);
      setCaptchaError("");
      setCaptchaNote("Challenge solved. You can submit registration.", "ok");
      return;
    }
    const hidden = el.captchaWidgetContainer.querySelector("input[name='cap_internal_token'], input[name='cap-token']");
    setCaptchaToken(hidden ? hidden.value : "");
    if (state.captcha.token) {
      setCaptchaError("");
      setCaptchaNote("Challenge solved. You can submit registration.", "ok");
    }
  });
  widget.addEventListener("error", () => {
    setCaptchaToken("");
    setCaptchaError("Captcha challenge failed. Retry the challenge.");
    setCaptchaNote("Captcha challenge not solved yet.", "error");
  });
  widget.addEventListener("reset", () => {
    setCaptchaToken("");
    setCaptchaError("");
    setCaptchaNote("Complete the challenge to continue registration.", "info");
  });
  el.captchaWidgetContainer.appendChild(widget);
  state.captcha.widgetActive = true;
  setCaptchaToken("");
}

async function loadCaptchaConfig() {
  try {
    const cfg = await api("/api/v1/public/captcha/config", { logErrors: false });
    state.captcha.config = cfg || null;
  } catch {
    state.captcha.config = null;
  }
  return state.captcha.config;
}

function renderResetCapabilityNote() {
  if (!el.resetCapabilityNote) return;
  const cap = state.auth.resetCapabilities;
  if (!cap) {
    el.resetCapabilityNote.textContent = "Reset token delivery follows current server reset policy.";
    return;
  }
  const enabled = !!cap.self_service_enabled;
  const authMode = String(cap.auth_mode || "sql").toUpperCase();
  const delivery = String(cap.delivery || "disabled");
  const ttl = Number(cap.token_ttl_minutes || 30);
  const senderAddress = String(cap.sender_address || "").trim() || "n/a";
  const senderStatus = String(cap.sender_status || "unknown").trim();
  const senderReason = String(cap.sender_reason || "").trim();
  const reasonText = describeResetSenderReason(senderReason);
  if (!enabled) {
    const suffix = reasonText ? ` ${reasonText}` : "";
    el.resetCapabilityNote.textContent = `Reset is currently unavailable (${authMode} mode, delivery ${delivery}, sender ${senderStatus}).${suffix}`;
    return;
  }
  const mappingNote = cap.requires_mapped_login ? "Mapped mailbox login is required." : "Mapped mailbox login is optional.";
  const senderNote = reasonText ? ` ${reasonText}` : "";
  el.resetCapabilityNote.textContent = `Reset enabled (${authMode}, delivery ${delivery}, token TTL ${ttl} min, sender ${senderAddress}, status ${senderStatus}).${senderNote} ${mappingNote}`.trim();
}

async function loadResetCapabilities() {
  try {
    const data = await api("/api/v1/public/password-reset/capabilities", { logErrors: false });
    state.auth.resetCapabilities = data || null;
  } catch {
    state.auth.resetCapabilities = null;
  }
  renderResetCapabilityNote();
}

function describeResetSenderReason(reason) {
  switch (String(reason || "").trim()) {
    case "log_delivery_disabled":
      return "Log-only delivery is not allowed for public reset.";
    case "external_sender_unconfirmed":
      return "Public reset stays disabled until the external reset sender is explicitly confirmed.";
    case "smtp_unreachable":
      return "The reset sender cannot reach the configured SMTP server.";
    case "smtp_auth_failed":
      return "The reset sender SMTP login was rejected.";
    case "smtp_sender_rejected":
      return "The reset sender address was rejected by the SMTP server policy.";
    case "smtp_probe_failed":
      return "The reset sender SMTP probe failed.";
    case "external_mailbox_required":
      return "An externally managed reset sender mailbox is required.";
    case "sql_provisioner_unconfigured":
      return "The SQL auth provisioner is not configured for the reset sender.";
    case "sender_provision_failed":
      return "Reset sender provisioning failed.";
    case "sender_not_initialized":
      return "Reset sender initialization has not completed.";
    case "sender_state_read_failed":
      return "Reset sender diagnostics could not be loaded.";
    default:
      return "";
  }
}

function captureResetTokenFromLocation() {
  const url = new URL(window.location.href);
  let token = String(url.searchParams.get("token") || "").trim();
  let shouldClear = token !== "";
  const rawHash = String(url.hash || "");
  if (!token && rawHash) {
    const hash = rawHash.startsWith("#") ? rawHash.slice(1) : rawHash;
    const [route, rawQuery = ""] = hash.split("?", 2);
    const normalizedRoute = String(route || "").replace(/^\/+/, "").trim().toLowerCase();
    if (normalizedRoute === "reset") {
      const hashParams = new URLSearchParams(rawQuery);
      token = String(hashParams.get("token") || "").trim();
      shouldClear = token !== "";
    }
  }
  if (shouldClear) {
    url.searchParams.delete("token");
    url.hash = "";
    const nextURL = `${url.pathname}${url.search}`;
    window.history.replaceState({}, document.title, nextURL || "/");
  }
  return token;
}

function applyResetLinkToken(rawToken, options = {}) {
  const token = String(rawToken || "").trim();
  if (!token) return false;
  if (el.resetTokenInput) {
    el.resetTokenInput.value = token;
  }
  setActiveAuthTask("reset");
  if (options.focus && el.resetNewPasswordInput) {
    window.setTimeout(() => {
      el.resetNewPasswordInput.focus();
      el.resetNewPasswordInput.select();
    }, 0);
  }
  return true;
}

async function initCaptchaUI() {
  const cfg = await loadCaptchaConfig();
  if (!cfg || !cfg.enabled) {
    renderCaptchaShell(false);
    showCaptchaManualInput(false);
    setCaptchaToken("");
    state.captcha.widgetBlocked = false;
    state.captcha.widgetActive = false;
    setCaptchaError("");
    return;
  }

  renderCaptchaShell(true);
  state.captcha.widgetBlocked = false;
  state.captcha.widgetActive = false;
  setCaptchaToken("");
  setCaptchaError("");

  const provider = String(cfg.provider || "").toLowerCase();
  if (provider !== "cap") {
    showCaptchaManualInput(true);
    setCaptchaNote("Captcha is enabled. Provide token from configured provider.", "info");
    syncRegisterSubmitState();
    return;
  }

  showCaptchaManualInput(false);
  setCaptchaNote("Complete the challenge to continue registration.", "info");
  let paths = null;
  try {
    paths = capWidgetPaths(cfg);
    const preflight = await preflightCapAssets(paths);
    await loadCapWidgetScript(paths, preflight);
    mountCapWidget(paths);
  } catch (err) {
    state.captcha.widgetBlocked = true;
    setCaptchaToken("");
    const assetHint = paths
      ? `Check self-hosted CAP assets: ${paths.scriptURL} and ${paths.wasmURL}.`
      : "Check CAPTCHA_WIDGET_API_URL and CAP standalone route.";
    const secureHint = !window.isSecureContext
      ? " Browser context is not secure; CAPTCHA worker hashing requires HTTPS."
      : "";
    setCaptchaError("Captcha widget failed to load from self-hosted CAP assets.");
    setCaptchaNote(`CAP asset server unavailable. ${assetHint}${secureHint} (${err.message})`, "error");
  }
  syncRegisterSubmitState();
}

async function resetCaptchaChallenge() {
  if (!state.captcha.config || !state.captcha.config.enabled) {
    setCaptchaToken("");
    return;
  }
  const provider = String(state.captcha.config.provider || "").toLowerCase();
  if (provider !== "cap") {
    setCaptchaToken("");
    return;
  }
  try {
    const paths = capWidgetPaths(state.captcha.config);
    const preflight = await preflightCapAssets(paths);
    if (!state.captcha.scriptLoaded) {
      await loadCapWidgetScript(paths, preflight);
    } else if (preflight?.hashesURL) {
      window.CAP_CUSTOM_HASHES_URL = preflight.hashesURL;
    } else {
      delete window.CAP_CUSTOM_HASHES_URL;
    }
    mountCapWidget(paths);
    setCaptchaError("");
    setCaptchaNote("Complete the challenge to continue registration.", "info");
  } catch {
    state.captcha.widgetBlocked = true;
    setCaptchaToken("");
    setCaptchaError("Captcha widget failed to reset from self-hosted CAP assets.");
  }
  syncRegisterSubmitState();
}

function legacyUpdaterStatus() {
  return {
    enabled: false,
    configured: false,
    current: {
      version: "unknown",
      commit: "",
      build_time: "",
      source_repo: "",
    },
    latest: null,
    last_checked_at: "",
    last_check_error: "",
    update_available: false,
    apply: {
      state: "unsupported",
    },
    legacy_backend: true,
  };
}

function setupRetrySecondsRemaining() {
  const ms = Number(state.setup.retryUntilMs || 0) - Date.now();
  if (ms <= 0) return 0;
  return Math.ceil(ms / 1000);
}

function setSetupCooldown(waitSec) {
  const secs = Math.max(1, Number(waitSec || 1));
  state.setup.retryUntilMs = Date.now() + secs * 1000;
  if (state.setup.retryTimer) {
    clearInterval(state.setup.retryTimer);
    state.setup.retryTimer = 0;
  }
  state.setup.retryTimer = window.setInterval(() => {
    const remaining = setupRetrySecondsRemaining();
    if (remaining <= 0) {
      clearInterval(state.setup.retryTimer);
      state.setup.retryTimer = 0;
      state.setup.retryUntilMs = 0;
      setSetupInlineStatus("You can retry initialization now.", "info");
      OOBEController.refreshNavState();
      return;
    }
    setSetupInlineStatus(`Too many attempts. Retry in ${remaining}s.`, "error");
    OOBEController.refreshNavState();
  }, 250);
}

function setActiveTab(tab) {
  [el.tabSetup, el.tabAuth, el.tabMail, el.tabSettings, el.tabAdmin]
    .filter(Boolean)
    .forEach((btn) => btn.classList.remove("active"));
  if (tab) tab.classList.add("active");
}

function showView(name) {
  el.viewSetup.classList.add("hidden");
  el.viewAuth.classList.add("hidden");
  el.viewSettings.classList.add("hidden");
  el.viewMail.classList.add("hidden");
  el.viewAdmin.classList.add("hidden");
  if (name === "setup") el.viewSetup.classList.remove("hidden");
  if (name === "auth") el.viewAuth.classList.remove("hidden");
  if (name === "settings") el.viewSettings.classList.remove("hidden");
  if (name === "mail") el.viewMail.classList.remove("hidden");
  if (name === "admin") el.viewAdmin.classList.remove("hidden");

  if (!el.appShell) return;
  el.appShell.classList.remove("page-office", "page-machine", "page-oobe-assistant");
  if (name === "setup") {
    el.appShell.classList.add("page-oobe-assistant");
    return;
  }
  if (state.theme === "machine-dark" && (name === "mail" || name === "admin")) {
    el.appShell.classList.add("page-machine");
    return;
  }
  el.appShell.classList.add("page-office");

  if (name === "mail") {
    startMailPolling();
  } else {
    stopMailPolling();
  }
}

function isMobileLayout() {
  return window.matchMedia("(max-width: 980px)").matches;
}

function paneElement(name) {
  if (name === "mailboxes") return el.mailPaneMailboxes;
  if (name === "messages") return el.mailPaneMessages;
  return el.mailPaneReader;
}

function paneFocusElement(name) {
  if (name === "mailboxes") return el.mailboxes || paneElement(name);
  if (name === "messages") return el.messages || paneElement(name);
  return paneElement(name);
}

function focusMailPane(name) {
  const next = ["mailboxes", "messages", "reader"].includes(String(name || "")) ? String(name) : "mailboxes";
  state.ui.activeKeyboardPane = next;
  [el.mailPaneMailboxes, el.mailPaneMessages, el.mailPaneReader]
    .filter(Boolean)
    .forEach((node) => node.classList.remove("is-keyboard-pane"));
  const pane = paneElement(next);
  const focusTarget = paneFocusElement(next);
  if (pane) {
    pane.classList.add("is-keyboard-pane");
  }
  if (focusTarget && typeof focusTarget.focus === "function") {
    focusTarget.focus({ preventScroll: true });
  }
}

function setActiveMailPane(name, opts = {}) {
  if (!el.viewMail) return;
  const next = ["mailboxes", "messages", "reader"].includes(String(name || "")) ? String(name) : "mailboxes";
  state.ui.activeMailPane = next;
  el.viewMail.dataset.mobilePane = next;
  if (el.mailMobileBack) {
    el.mailMobileBack.classList.toggle("hidden", !isMobileLayout() || next === "mailboxes");
  }
  if (el.mailBackToMailboxes) {
    el.mailBackToMailboxes.classList.toggle("hidden", !isMobileLayout() || next !== "messages");
  }
  if (el.mailBackToMessages) {
    el.mailBackToMessages.classList.toggle("hidden", !isMobileLayout() || next !== "reader");
  }
  syncMailboxActiveDescendant();
  syncMessageActiveDescendant();
  if (opts.focus !== false) {
    focusMailPane(next);
  }
}

function cycleMailPane(delta = 1) {
  const panes = ["mailboxes", "messages", "reader"];
  const current = panes.indexOf(state.ui.activeKeyboardPane || "mailboxes");
  const nextIndex = (current + delta + panes.length) % panes.length;
  focusMailPane(panes[nextIndex]);
}

function renderNavItem(node, active) {
  if (!node) return;
  node.classList.toggle("is-active", !!active);
  node.setAttribute("aria-current", active ? "page" : "false");
}

function renderSection(node, visible) {
  if (!node) return;
  node.classList.toggle("hidden", !visible);
}

function renderDomainSidebar(nodesByKey, activeKey) {
  Object.entries(nodesByKey).forEach(([key, node]) => {
    renderNavItem(node, key === activeKey);
  });
}

function renderListItem(opts = {}) {
  const {
    as = "div",
    active = false,
    markerText = "",
    markerClass = "",
    title = "",
    meta = "",
    actionText = "View",
    onSelect = null,
    onAction = null,
    leadingNode = null,
  } = opts;
  const row = document.createElement(as === "button" ? "button" : "div");
  if (row.tagName === "BUTTON") {
    row.type = "button";
  }
  row.className = "setting-list-item";
  if (active) {
    row.classList.add("is-active");
  }
  const canSelect = typeof onSelect === "function";
  if (canSelect) {
    row.classList.add("setting-list-item--interactive");
    if (row.tagName !== "BUTTON") {
      row.tabIndex = 0;
      row.setAttribute("role", "button");
    }
  }
  if (leadingNode) {
    row.appendChild(leadingNode);
  } else if (markerText) {
    const marker = document.createElement("span");
    marker.className = markerClass || "status-chip status-chip--info";
    marker.textContent = markerText;
    row.appendChild(marker);
  }
  const main = document.createElement("span");
  main.className = "setting-list-main";
  const titleNode = document.createElement("span");
  titleNode.className = "setting-list-title";
  titleNode.textContent = String(title || "");
  main.appendChild(titleNode);
  const metaNode = document.createElement("span");
  metaNode.className = "setting-list-meta";
  metaNode.textContent = String(meta || "");
  main.appendChild(metaNode);
  row.appendChild(main);
  const resolvedAction = typeof onAction === "function"
    ? onAction
    : (canSelect ? onSelect : null);
  if (String(actionText || "").trim()) {
    const action = document.createElement("button");
    action.type = "button";
    action.className = "setting-list-action";
    action.textContent = String(actionText || "View");
    if (typeof resolvedAction === "function") {
      action.addEventListener("click", (event) => {
        event.stopPropagation();
        resolvedAction();
      });
    } else {
      action.disabled = true;
    }
    row.appendChild(action);
  }
  if (canSelect) {
    row.addEventListener("click", (event) => {
      if (event.target && event.target.closest("button,a,input,select,textarea,summary")) return;
      onSelect();
    });
    if (row.tagName !== "BUTTON") {
      row.addEventListener("keydown", (event) => {
        if (event.key !== "Enter" && event.key !== " ") return;
        event.preventDefault();
        onSelect();
      });
    }
  }
  return row;
}

function renderDetailView(container, item, renderContent) {
  if (!container) return;
  container.replaceChildren();
  if (!item) {
    container.classList.add("hidden");
    return;
  }
  container.classList.remove("hidden");
  if (typeof renderContent === "function") {
    renderContent(container, item);
  }
}

function createBackToListButton(onClick, label = "Back To List") {
  const back = document.createElement("button");
  back.type = "button";
  back.className = "cmd-btn cmd-btn--dense cmd-btn--ghost";
  back.textContent = label;
  back.addEventListener("click", () => {
    if (typeof onClick === "function") onClick();
  });
  return back;
}

function renderToggleItem(opts = {}) {
  const {
    label = "",
    description = "",
    enabled = false,
    disabled = false,
    onToggle = null,
  } = opts;
  const wrap = document.createElement("div");
  wrap.className = "setting-list-item";
  const status = document.createElement("span");
  status.className = enabled ? "status-chip status-chip--ok" : "status-chip status-chip--warning";
  status.textContent = enabled ? "ON" : "OFF";
  wrap.appendChild(status);
  const main = document.createElement("span");
  main.className = "setting-list-main";
  const title = document.createElement("span");
  title.className = "setting-list-title";
  title.textContent = String(label || "");
  main.appendChild(title);
  const meta = document.createElement("span");
  meta.className = "setting-list-meta";
  meta.textContent = String(description || "");
  main.appendChild(meta);
  wrap.appendChild(main);
  const action = document.createElement("button");
  action.type = "button";
  action.className = enabled ? "cmd-btn cmd-btn--dense cmd-btn--danger" : "cmd-btn cmd-btn--dense cmd-btn--primary";
  action.textContent = enabled ? "Disable" : "Enable";
  action.disabled = !!disabled;
  if (typeof onToggle === "function") {
    action.addEventListener("click", () => {
      onToggle();
    });
  }
  wrap.appendChild(action);
  return wrap;
}

function setActiveSettingsSection(name) {
  const next = ["signin", "devices", "sessions"].includes(String(name || "")) ? String(name) : "signin";
  state.ui.activeSettingsSection = next;
  state.ui.settingsNav.domain = next;
  state.ui.settingsNav.page = "list";
  state.ui.settingsNav.detailId = "";
  const sections = {
    signin: el.settingsSectionSignIn,
    devices: el.settingsSectionDevices,
    sessions: el.settingsSectionSessions,
  };
  const nav = {
    signin: el.settingsNavSignIn,
    devices: el.settingsNavDevices,
    sessions: el.settingsNavSessions,
  };
  Object.entries(sections).forEach(([key, node]) => renderSection(node, key === next));
  renderDomainSidebar(nav, next);
}

function setActiveAdminSection(name) {
  const next = ["system", "registrations", "users", "audit"].includes(String(name || "")) ? String(name) : "system";
  state.ui.activeAdminSection = next;
  state.ui.adminNav.domain = next;
  state.ui.adminNav.page = "list";
  state.ui.adminNav.detailId = "";
  const sections = {
    system: el.adminSectionSystem,
    registrations: el.adminSectionRegistrations,
    users: el.adminSectionUsers,
    audit: el.adminSectionAudit,
  };
  const nav = {
    system: el.adminNavSystem,
    registrations: el.adminNavRegistrations,
    users: el.adminNavUsers,
    audit: el.adminNavAudit,
  };
  Object.entries(sections).forEach(([key, node]) => renderSection(node, key === next));
  renderDomainSidebar(nav, next);
}

function applyNavVisibility() {
  if (el.appShell) {
    el.appShell.classList.toggle("is-setup-required", !!state.setup.required);
  }

  if (state.setup.required) {
    el.tabSetup.style.display = "inline-block";
    el.tabAuth.style.display = "none";
    el.tabMail.style.display = "none";
    el.tabSettings.style.display = "none";
    el.tabAdmin.style.display = "none";
    el.btnTheme.style.display = "none";
    el.btnLogout.style.display = "none";
    return;
  }

  el.tabSetup.style.display = "none";
  el.btnTheme.style.display = "inline-block";
  if (!state.user) {
    el.tabAuth.style.display = "inline-block";
    el.tabMail.style.display = "none";
    el.tabSettings.style.display = "none";
    el.tabAdmin.style.display = "none";
    el.btnLogout.style.display = "none";
    return;
  }
  el.tabAuth.style.display = "none";
  el.tabMail.style.display = "inline-block";
  el.tabSettings.style.display = "inline-block";
  el.tabAdmin.style.display = state.user.role === "admin" ? "inline-block" : "none";
  el.btnLogout.style.display = "inline-block";
}

function renderTrustedDevices(items) {
  if (!el.trustedDevicesList) return;
  const rows = Array.isArray(items) ? items : [];
  state.settings.devices.items = rows;
  if (!rows.some((item) => String(item.id || "") === state.settings.devices.detailId)) {
    state.settings.devices.detailId = "";
    if (state.ui.settingsNav.domain === "devices") {
      state.ui.settingsNav.page = "list";
      state.ui.settingsNav.detailId = "";
    }
  }
  el.trustedDevicesList.replaceChildren();
  if (rows.length === 0) {
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "No trusted devices saved.";
    el.trustedDevicesList.appendChild(empty);
    state.ui.settingsNav.page = "list";
    state.ui.settingsNav.detailId = "";
    renderDetailView(el.settingsDeviceDetail, null);
    return;
  }
  for (const item of rows) {
    const itemID = String(item.id || "");
    const lastUsed = item.last_used_at ? `Last used ${formatDateTimeOrNA(item.last_used_at)}` : "Last used never";
    const expires = item.expires_at ? `Expires ${formatDateTimeOrNA(item.expires_at)}` : "Expires n/a";
    const row = renderListItem({
      active: itemID === state.settings.devices.detailId,
      markerClass: "status-chip status-chip--info",
      markerText: item.is_current ? "Current" : "Trusted",
      title: String(item.display_label || item.device_label || "Trusted device"),
      meta: [lastUsed, expires].join(" • "),
      onSelect: () => {
        state.settings.devices.detailId = itemID;
        state.ui.settingsNav.page = "list";
        state.ui.settingsNav.detailId = itemID;
        renderTrustedDevices(state.settings.devices.items);
      },
      onAction: () => {
        state.settings.devices.detailId = itemID;
        state.ui.settingsNav.page = "detail";
        state.ui.settingsNav.detailId = state.settings.devices.detailId;
        renderTrustedDevices(state.settings.devices.items);
      },
    });
    el.trustedDevicesList.appendChild(row);
  }
  let selected = null;
  if (state.ui.settingsNav.page === "detail") {
    selected = rows.find((item) => String(item.id || "") === state.settings.devices.detailId) || null;
  }
  if (selected) {
    state.ui.settingsNav.page = "detail";
    state.ui.settingsNav.detailId = String(selected.id || "");
  } else {
    state.ui.settingsNav.page = "list";
    state.ui.settingsNav.detailId = "";
  }
  renderDetailView(el.settingsDeviceDetail, selected, (detail, item) => {
    const title = document.createElement("h4");
    title.textContent = String(item.display_label || item.device_label || "Trusted device");
    detail.appendChild(title);
    const summary = document.createElement("p");
    summary.className = "hint";
    summary.textContent = item.is_current ? "Current trusted device." : "Trusted device can skip MFA until it expires.";
    detail.appendChild(summary);
    const created = document.createElement("p");
    created.className = "hint";
    created.textContent = `Created: ${formatDateTimeOrNA(item.created_at)}`;
    detail.appendChild(created);
    const lastUsed = document.createElement("p");
    lastUsed.className = "hint";
    lastUsed.textContent = `Last used: ${formatDateTimeOrNA(item.last_used_at)}`;
    detail.appendChild(lastUsed);
    const expires = document.createElement("p");
    expires.className = "hint";
    expires.textContent = `Expires: ${formatDateTimeOrNA(item.expires_at)}`;
    detail.appendChild(expires);
    const actions = document.createElement("div");
    actions.className = "settings-detail-actions";
    actions.appendChild(createBackToListButton(() => {
      state.ui.settingsNav.page = "list";
      state.ui.settingsNav.detailId = "";
      renderTrustedDevices(state.settings.devices.items);
    }));
    const revoke = document.createElement("button");
    revoke.type = "button";
    revoke.className = "cmd-btn cmd-btn--dense cmd-btn--danger";
    revoke.textContent = "Remove Device";
    revoke.addEventListener("click", async () => {
      const confirmed = await showConfirmModal({
        title: "Remove trusted device?",
        body: "This device will require MFA the next time it signs in.",
        confirmText: "Remove",
        cancelText: "Cancel",
        trigger: revoke,
      });
      if (!confirmed) return;
      try {
        await api(`/api/v2/security/mfa/trusted-devices/${encodeURIComponent(item.id)}/revoke`, {
          method: "POST",
          json: {},
        });
        setStatus("Trusted device revoked.", "ok");
        await loadTrustedDevices();
      } catch (err) {
        setStatus(formatAPIError(err, "Failed to revoke trusted device."), "error");
      }
    });
    actions.appendChild(revoke);
    detail.appendChild(actions);
    const tech = document.createElement("details");
    tech.className = "setting-tech";
    tech.innerHTML = "<summary>Technical details</summary>";
    const techBody = document.createElement("p");
    techBody.className = "hint";
    const browser = String(item.browser || "").trim();
    const os = String(item.os || "").trim();
    const ip = String(item.ip_hint || "").trim();
    techBody.textContent = [
      browser ? `Browser: ${browser}` : "",
      os ? `OS: ${os}` : "",
      ip ? `IP hint: ${ip}` : "",
      item.id ? `Device ID: ${item.id}` : "",
    ].filter(Boolean).join(" | ");
    tech.appendChild(techBody);
    detail.appendChild(tech);
  });
}

function setSessionDetail(item) {
  renderDetailView(el.settingsSessionDetail, item, (detail, selected) => {
    const title = document.createElement("h4");
    title.textContent = String(selected.device_label || selected.ua_summary || "Session");
    detail.appendChild(title);
    const summary = document.createElement("p");
    summary.className = "hint";
    summary.textContent = selected.is_current ? "Current active session." : "Session currently authorized.";
    detail.appendChild(summary);
    const created = document.createElement("p");
    created.className = "hint";
    created.textContent = `Created: ${formatDateTimeOrNA(selected.created_at)}`;
    detail.appendChild(created);
    const lastSeen = document.createElement("p");
    lastSeen.className = "hint";
    lastSeen.textContent = `Last seen: ${formatDateTimeOrNA(selected.last_seen_at)}`;
    detail.appendChild(lastSeen);
    const expires = document.createElement("p");
    expires.className = "hint";
    expires.textContent = `Expires: ${formatDateTimeOrNA(selected.expires_at)}`;
    detail.appendChild(expires);
    const actions = document.createElement("div");
    actions.className = "settings-detail-actions";
    actions.appendChild(createBackToListButton(() => {
      state.ui.settingsNav.page = "list";
      state.ui.settingsNav.detailId = "";
      renderSessions(state.settings.sessions.items);
    }));
    if (!selected.is_current) {
      const revoke = document.createElement("button");
      revoke.type = "button";
      revoke.className = "cmd-btn cmd-btn--dense cmd-btn--danger";
      revoke.textContent = "Revoke Session";
      revoke.addEventListener("click", async () => {
        const confirmed = await showConfirmModal({
          title: "Revoke session?",
          body: "This session will be signed out immediately.",
          confirmText: "Revoke",
          cancelText: "Cancel",
          trigger: revoke,
        });
        if (!confirmed) return;
        try {
          await api(`/api/v2/security/sessions/${encodeURIComponent(String(selected.session_id || ""))}/revoke`, {
            method: "POST",
            json: { reason: "user_initiated" },
          });
          setStatus("Session revoked.", "ok");
          await loadSessions();
        } catch (err) {
          setStatus(formatAPIError(err, "Failed to revoke session."), "error");
        }
      });
      actions.appendChild(revoke);
    }
    detail.appendChild(actions);
    const tech = document.createElement("details");
    tech.className = "setting-tech";
    tech.innerHTML = "<summary>Technical details</summary>";
    const techBody = document.createElement("p");
    techBody.className = "hint";
    const ua = String(selected.ua_summary || "").trim();
    const ip = String(selected.ip_hint || "").trim();
    techBody.textContent = [
      ua ? `User agent: ${ua}` : "",
      ip ? `IP hint: ${ip}` : "",
      selected.session_id ? `Session ID: ${selected.session_id}` : "",
    ].filter(Boolean).join(" | ");
    tech.appendChild(techBody);
    detail.appendChild(tech);
  });
}

async function loadTrustedDevices() {
  if (!state.user || !el.trustedDevicesList) return;
  try {
    const payload = await api("/api/v2/security/mfa/trusted-devices", { logErrors: false });
    renderTrustedDevices(Array.isArray(payload.items) ? payload.items : []);
  } catch (err) {
    renderTrustedDevices([]);
    setStatus(formatAPIError(err, "Failed to load trusted devices."), "error");
  }
}

function renderSessions(items) {
  if (!el.sessionsList) return;
  const rows = Array.isArray(items) ? items : [];
  state.settings.sessions.items = rows;
  if (!rows.some((item) => String(item.session_id || "") === state.settings.sessions.detailId)) {
    state.settings.sessions.detailId = "";
    if (state.ui.settingsNav.domain === "sessions") {
      state.ui.settingsNav.page = "list";
      state.ui.settingsNav.detailId = "";
    }
  }
  el.sessionsList.replaceChildren();
  if (rows.length === 0) {
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "No active sessions found.";
    el.sessionsList.appendChild(empty);
    setSessionDetail(null);
    return;
  }
  for (const item of rows) {
    const sessionID = String(item.session_id || "");
    const row = renderListItem({
      active: sessionID === state.settings.sessions.detailId,
      markerClass: item.is_current ? "status-chip status-chip--ok" : "status-chip status-chip--info",
      markerText: item.is_current ? "Current" : String(item.auth_method || "password"),
      title: String(item.device_label || item.ua_summary || "Session"),
      meta: `Last seen ${formatDateTimeOrNA(item.last_seen_at)} • Expires ${formatDateTimeOrNA(item.expires_at)}`,
      onSelect: () => {
        state.settings.sessions.detailId = sessionID;
        state.ui.settingsNav.page = "list";
        state.ui.settingsNav.detailId = sessionID;
        renderSessions(state.settings.sessions.items);
      },
      onAction: () => {
        state.settings.sessions.detailId = sessionID;
        state.ui.settingsNav.page = "detail";
        state.ui.settingsNav.detailId = state.settings.sessions.detailId;
        renderSessions(state.settings.sessions.items);
      },
    });
    el.sessionsList.appendChild(row);
  }
  let selected = null;
  if (state.ui.settingsNav.page === "detail") {
    selected = rows.find((item) => String(item.session_id || "") === state.settings.sessions.detailId) || null;
  }
  if (selected) {
    state.ui.settingsNav.page = "detail";
    state.ui.settingsNav.detailId = String(selected.session_id || "");
  } else {
    state.ui.settingsNav.page = "list";
    state.ui.settingsNav.detailId = "";
  }
  setSessionDetail(selected);
}

async function loadSessions() {
  if (!state.user || !el.sessionsList) return;
  try {
    const payload = await api("/api/v2/security/sessions", { logErrors: false });
    renderSessions(Array.isArray(payload.items) ? payload.items : []);
  } catch (err) {
    renderSessions([]);
    setStatus(formatAPIError(err, "Failed to load sessions."), "error");
  }
}

function formatDateTimeOrNA(raw) {
  const value = String(raw || "").trim();
  if (!value) return "n/a";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString();
}

function renderJumpResults(container, items, onPick) {
  if (!container) return;
  container.replaceChildren();
  if (!Array.isArray(items) || items.length === 0) {
    container.classList.add("hidden");
    return;
  }
  container.classList.remove("hidden");
  for (const item of items) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "settings-search-result";
    button.textContent = item.subtitle
      ? `${String(item.label)} \u2022 ${String(item.subtitle)}`
      : String(item.label);
    button.addEventListener("click", async () => {
      container.classList.add("hidden");
      container.replaceChildren();
      await onPick(item);
    });
    container.appendChild(button);
  }
}

function buildJumpResults(entries, rawQuery) {
  const query = String(rawQuery || "").trim().toLowerCase();
  if (!query) return [];
  const terms = query.split(/\s+/).filter(Boolean);
  const ranked = [];
  for (const entry of entries) {
    const haystack = String(
      `${entry.label || ""} ${entry.subtitle || ""} ${(entry.keywords || []).join(" ")}`,
    ).toLowerCase();
    if (!terms.every((term) => haystack.includes(term))) {
      continue;
    }
    let score = 0;
    const label = String(entry.label || "").toLowerCase();
    if (label === query) score += 4;
    if (label.startsWith(query)) score += 3;
    if (haystack.startsWith(query)) score += 2;
    score += Math.max(1, 10 - Math.max(0, label.indexOf(terms[0] || "")));
    ranked.push({ ...entry, _score: score });
  }
  ranked.sort((a, b) => b._score - a._score);
  return ranked.slice(0, 10);
}

function settingsSearchEntries() {
  const entries = [
    {
      label: "Sign-In",
      subtitle: "Settings domain",
      keywords: ["settings", "sign in", "login", "passkeys"],
      target: { domain: "signin" },
    },
    {
      label: "Passkeys",
      subtitle: "Sign-In",
      keywords: ["passkey", "webauthn", "security key"],
      target: { domain: "signin" },
    },
    {
      label: "Devices",
      subtitle: "Settings domain",
      keywords: ["settings", "trusted devices", "mfa"],
      target: { domain: "devices" },
    },
    {
      label: "Trusted Devices",
      subtitle: "Devices",
      keywords: ["trusted", "device", "mfa skip"],
      target: { domain: "devices" },
    },
    {
      label: "Sessions",
      subtitle: "Settings domain",
      keywords: ["sessions", "active sessions", "revoke"],
      target: { domain: "sessions" },
    },
    {
      label: "Active Sessions",
      subtitle: "Sessions",
      keywords: ["session", "active", "revoke"],
      target: { domain: "sessions" },
    },
  ];
  for (const item of state.settings.passkeys.items) {
    const id = String(item.id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.name || "Passkey"),
      subtitle: "Passkey detail",
      keywords: ["passkey", "signin", "rename", "delete", formatDateTimeOrNA(item.last_used_at)],
      target: { domain: "signin", type: "passkey", detailId: id },
    });
  }
  for (const item of state.settings.devices.items) {
    const id = String(item.id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.display_label || item.device_label || "Trusted device"),
      subtitle: "Trusted device detail",
      keywords: ["trusted device", "mfa", "revoke", formatDateTimeOrNA(item.last_used_at)],
      target: { domain: "devices", type: "device", detailId: id },
    });
  }
  for (const item of state.settings.sessions.items) {
    const id = String(item.session_id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.device_label || item.ua_summary || "Session"),
      subtitle: "Session detail",
      keywords: ["session", "active session", "revoke", formatDateTimeOrNA(item.last_seen_at)],
      target: { domain: "sessions", type: "session", detailId: id },
    });
  }
  return entries;
}

function adminSearchEntries() {
  const entries = [
    {
      label: "System",
      subtitle: "Admin domain",
      keywords: ["system", "updates", "feature flags"],
      target: { domain: "system" },
    },
    {
      label: "Software Update",
      subtitle: "System",
      keywords: ["update", "version", "release", "apply"],
      target: { domain: "system" },
    },
    {
      label: "Feature Flags",
      subtitle: "System",
      keywords: ["flags", "feature toggle", "runtime"],
      target: { domain: "system" },
    },
    {
      label: "Users",
      subtitle: "Admin domain",
      keywords: ["users", "suspend", "password reset"],
      target: { domain: "users" },
    },
    {
      label: "Registrations",
      subtitle: "Admin domain",
      keywords: ["registrations", "approve", "reject"],
      target: { domain: "registrations" },
    },
    {
      label: "Audit Log",
      subtitle: "Admin domain",
      keywords: ["audit", "events", "security"],
      target: { domain: "audit" },
    },
  ];
  for (const item of state.admin.featureFlags.items) {
    const id = String(item.id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.name || id),
      subtitle: "Feature flag",
      keywords: [String(item.description || ""), String(item.category || ""), id],
      target: { domain: "system", type: "flag", detailId: id },
    });
  }
  for (const item of state.admin.users.items) {
    const id = String(item.id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.email || "User"),
      subtitle: "User detail",
      keywords: [String(item.role || ""), String(item.status || ""), String(item.provision_state || "")],
      target: { domain: "users", type: "user", detailId: id },
    });
  }
  for (const item of state.admin.registrations.items) {
    const id = String(item.id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.email || "Registration"),
      subtitle: "Registration detail",
      keywords: [String(item.status || ""), String(item.reason || ""), formatDate(item.created_at) || ""],
      target: { domain: "registrations", type: "registration", detailId: id },
    });
  }
  for (const item of state.admin.audit.items) {
    const id = String(item.id || "").trim();
    if (!id) continue;
    entries.push({
      label: String(item.summary_text || item.action || "Audit event"),
      subtitle: "Audit detail",
      keywords: [String(item.action || ""), String(item.actor_email || ""), String(item.target_label || item.target || "")],
      target: { domain: "audit", type: "audit", detailId: id },
    });
  }
  return entries;
}

async function loadActiveSettingsSection() {
  if (state.ui.activeSettingsSection === "devices") {
    await loadTrustedDevices();
    return;
  }
  if (state.ui.activeSettingsSection === "sessions") {
    await loadSessions();
    return;
  }
  await loadPasskeyCredentials();
}

async function navigateSettingsTarget(target) {
  if (!target || !target.domain) return;
  setActiveSettingsSection(target.domain);
  state.ui.settingsNav.domain = target.domain;
  state.ui.settingsNav.page = "list";
  state.ui.settingsNav.detailId = "";
  await loadActiveSettingsSection();
  if (target.type === "passkey") {
    state.settings.passkeys.detailId = String(target.detailId || "");
    state.ui.settingsNav.page = "detail";
    state.ui.settingsNav.detailId = state.settings.passkeys.detailId;
    renderPasskeyCredentials(state.settings.passkeys.items);
    return;
  }
  if (target.type === "device") {
    state.settings.devices.detailId = String(target.detailId || "");
    state.ui.settingsNav.page = "detail";
    state.ui.settingsNav.detailId = state.settings.devices.detailId;
    renderTrustedDevices(state.settings.devices.items);
    return;
  }
  if (target.type === "session") {
    state.settings.sessions.detailId = String(target.detailId || "");
    state.ui.settingsNav.page = "detail";
    state.ui.settingsNav.detailId = state.settings.sessions.detailId;
    renderSessions(state.settings.sessions.items);
  }
}

async function navigateAdminTarget(target) {
  if (!target || !target.domain) return;
  setActiveAdminSection(target.domain);
  state.ui.adminNav.domain = target.domain;
  state.ui.adminNav.page = "list";
  state.ui.adminNav.detailId = "";
  await loadActiveAdminSection();
  if (target.type === "flag") {
    state.admin.featureFlags.detailId = String(target.detailId || "");
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.featureFlags.detailId;
    renderAdminFeatureFlagDetail();
    return;
  }
  if (target.type === "user") {
    state.admin.users.detailId = String(target.detailId || "");
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.users.detailId;
    renderAdminUserDetail();
    return;
  }
  if (target.type === "registration") {
    state.admin.registrations.detailId = String(target.detailId || "");
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.registrations.detailId;
    renderAdminRegistrationDetail();
    return;
  }
  if (target.type === "audit") {
    state.admin.audit.detailId = String(target.detailId || "");
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.audit.detailId;
    renderAdminAuditDetail();
  }
}

function renderPasskeyCredentials(items) {
  if (!el.passkeysList) return;
  const rows = Array.isArray(items) ? items : [];
  state.settings.passkeys.items = rows;
  if (!rows.some((item) => String(item.id || "") === state.settings.passkeys.detailId)) {
    state.settings.passkeys.detailId = "";
    if (state.ui.settingsNav.domain === "signin") {
      state.ui.settingsNav.page = "list";
      state.ui.settingsNav.detailId = "";
    }
  }
  el.passkeysList.replaceChildren();
  if (rows.length === 0) {
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "No passkeys enrolled.";
    el.passkeysList.appendChild(empty);
    state.ui.settingsNav.page = "list";
    state.ui.settingsNav.detailId = "";
    renderDetailView(el.settingsPasskeyDetail, null);
    return;
  }
  for (const item of rows) {
    const passkeyID = String(item.id || "");
    const row = renderListItem({
      active: passkeyID === state.settings.passkeys.detailId,
      markerClass: "status-chip status-chip--info",
      markerText: "Passkey",
      title: String(item.name || "Passkey"),
      meta: `Created ${formatDateTimeOrNA(item.created_at)} • Last used ${formatDateTimeOrNA(item.last_used_at)}`,
      onSelect: () => {
        state.settings.passkeys.detailId = passkeyID;
        state.ui.settingsNav.page = "list";
        state.ui.settingsNav.detailId = passkeyID;
        renderPasskeyCredentials(state.settings.passkeys.items);
      },
      onAction: () => {
        state.settings.passkeys.detailId = passkeyID;
        state.ui.settingsNav.page = "detail";
        state.ui.settingsNav.detailId = state.settings.passkeys.detailId;
        renderPasskeyCredentials(state.settings.passkeys.items);
      },
    });
    el.passkeysList.appendChild(row);
  }
  let selected = null;
  if (state.ui.settingsNav.page === "detail") {
    selected = rows.find((item) => String(item.id || "") === state.settings.passkeys.detailId) || null;
  }
  if (selected) {
    state.ui.settingsNav.page = "detail";
    state.ui.settingsNav.detailId = String(selected.id || "");
  } else {
    state.ui.settingsNav.page = "list";
    state.ui.settingsNav.detailId = "";
  }
  renderDetailView(el.settingsPasskeyDetail, selected, (detail, item) => {
    const title = document.createElement("h4");
    title.textContent = String(item.name || "Passkey");
    detail.appendChild(title);
    const created = document.createElement("p");
    created.className = "hint";
    created.textContent = `Created: ${formatDateTimeOrNA(item.created_at)}`;
    detail.appendChild(created);
    const lastUsed = document.createElement("p");
    lastUsed.className = "hint";
    lastUsed.textContent = `Last used: ${formatDateTimeOrNA(item.last_used_at)}`;
    detail.appendChild(lastUsed);
    const actions = document.createElement("div");
    actions.className = "settings-detail-actions";
    actions.appendChild(createBackToListButton(() => {
      state.ui.settingsNav.page = "list";
      state.ui.settingsNav.detailId = "";
      renderPasskeyCredentials(state.settings.passkeys.items);
    }));
    const rename = document.createElement("button");
    rename.type = "button";
    rename.className = "cmd-btn cmd-btn--dense";
    rename.textContent = "Rename";
    rename.addEventListener("click", async () => {
      const nextName = String(await showPromptModal({
        title: "Rename Passkey",
        body: "Set a new label for this passkey.",
        label: "Passkey name",
        defaultValue: String(item.name || ""),
        confirmText: "Save",
        trigger: rename,
      }) || "").trim();
      if (!nextName) return;
      try {
        await api(`/api/v2/security/mfa/webauthn/${encodeURIComponent(item.id)}`, {
          method: "PATCH",
          json: { name: nextName },
        });
        setStatus("Passkey renamed.", "ok");
        await loadPasskeyCredentials();
      } catch (err) {
        setStatus(formatAPIError(err, "Failed to rename passkey."), "error");
      }
    });
    actions.appendChild(rename);
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "cmd-btn cmd-btn--dense cmd-btn--danger";
    remove.textContent = "Delete Passkey";
    remove.addEventListener("click", async () => {
      const confirmed = await showConfirmModal({
        title: "Delete passkey?",
        body: "This passkey will no longer be usable for MFA or passkey sign-in.",
        confirmText: "Delete",
        cancelText: "Cancel",
        trigger: remove,
      });
      if (!confirmed) return;
      try {
        await api(`/api/v2/security/mfa/webauthn/${encodeURIComponent(item.id)}`, {
          method: "DELETE",
        });
        setStatus("Passkey deleted.", "ok");
        await loadPasskeyCredentials();
      } catch (err) {
        setStatus(formatAPIError(err, "Failed to delete passkey."), "error");
      }
    });
    actions.appendChild(remove);
    detail.appendChild(actions);
    const tech = document.createElement("details");
    tech.className = "setting-tech";
    tech.innerHTML = "<summary>Technical details</summary>";
    const techBody = document.createElement("p");
    techBody.className = "hint";
    const credential = String(item.credential_id || "").trim();
    techBody.textContent = credential ? `Credential ID: ${credential}` : "Credential ID unavailable.";
    tech.appendChild(techBody);
    detail.appendChild(tech);
  });
}

async function loadPasskeyCredentials() {
  if (!state.user || !el.passkeysList) return;
  const caps = authCapabilities();
  if (!caps.passkey_mfa_available) {
    renderPasskeyCredentials([]);
    return;
  }
  try {
    const payload = await api("/api/v2/security/mfa/webauthn", { logErrors: false });
    renderPasskeyCredentials(Array.isArray(payload.items) ? payload.items : []);
  } catch (err) {
    renderPasskeyCredentials([]);
    setStatus(formatAPIError(err, "Failed to load passkeys."), "error");
  }
}

function requireSelectedMessage() {
  if (!state.selectedMessage) {
    throw new Error("Select a message first");
  }
}

function normalizeDomain(v) {
  return String(v || "")
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/\/$/, "")
    .replace(/\.$/, "");
}

function domainToDefaultEmail(domain) {
  const d = normalizeDomain(domain);
  if (!d) return "webmaster@example.com";
  return `webmaster@${d}`;
}

function setupThemeLabel(themeName) {
  return themeName === "paper-light" ? "Paper" : "Machine";
}

function setupAutomaticUpdatesLabel(enabled) {
  return enabled === false ? "Manual updates only" : "Automatic updates on";
}

function validDomain(domain) {
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/.test(domain);
}

function validEmail(email) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(String(email || "").trim());
}

function passwordClassCount(password) {
  let classes = 0;
  if (/[a-z]/.test(password)) classes += 1;
  if (/[A-Z]/.test(password)) classes += 1;
  if (/[0-9]/.test(password)) classes += 1;
  if (/[!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~]/.test(password)) classes += 1;
  return classes;
}

async function loadSetupStatus() {
  const data = await api("/api/v1/setup/status");
  state.setup.required = !!data.required;
  state.setup.baseDomain = normalizeDomain(data.base_domain || "example.com");
  state.setup.defaultAdminEmail = String(data.default_admin_email || domainToDefaultEmail(state.setup.baseDomain)).toLowerCase();
  state.setup.automaticUpdatesEnabled = data.automatic_updates_enabled !== false;
  state.setup.passkeyPrimaryEnabled = data.passkey_primary_sign_in_enabled !== false;
  state.setup.authMode = String(data.auth_mode || "sql").toLowerCase();
  state.setup.passwordMinLength = Number(data.password_min_length || 12);
  state.setup.passwordMaxLength = Number(data.password_max_length || 128);
  state.setup.passwordClassMin = Number(data.password_class_min || 3);
  return data;
}

async function completeSetup() {
  const domain = normalizeDomain(el.setupDomain.value);
  const email = String(el.setupAdminEmail.value || "").trim().toLowerCase();
  const recoveryEmail = String(el.setupAdminRecoveryEmail?.value || "").trim().toLowerCase();
  const region = String(el.setupRegion.value || "us-east").trim();
  const password = el.setupPassword.value;
  const mailboxLogin = String(el.setupAdminMailboxLogin?.value || "").trim();
  const passkeyPrimaryEnabled = !!el.setupPasskeyPrimaryEnabled?.checked;
  const automaticUpdatesEnabled = state.setup.automaticUpdatesEnabled !== false;

  const setupPayload = await api("/api/v1/setup/complete", {
    method: "POST",
    json: {
      base_domain: domain,
      admin_email: email,
      admin_recovery_email: recoveryEmail,
      admin_mailbox_login: mailboxLogin,
      admin_password: password,
      region,
      passkey_primary_sign_in_enabled: passkeyPrimaryEnabled,
      automatic_updates_enabled: automaticUpdatesEnabled,
    },
  });

  const setupStage = authStageFromPayload(setupPayload);
  if (setupStage.auth_stage !== "authenticated") {
    await ensureMFAStageAuthenticated(setupStage);
  }

  const session = await refreshSession({
    throwOnFail: true,
    skipUnauthorizedHandling: true,
    skipMFAHandling: true,
  });
  if (!session.ok) {
    throw new Error("Setup completed, but browser session was not established. Check HTTP/HTTPS cookie policy and sign in.");
  }
  const liveStage = authStageFromPayload(session.user || {});
  if (liveStage.auth_stage !== "authenticated") {
    throw new Error("Multi-factor authentication setup is required for the admin account before finishing setup.");
  }
  state.setup.required = false;
  applyNavVisibility();
  showView("setup");
  setStatus("SETUP COMPLETE. SIGNED IN AS WEBMASTER.", "ok");
}

const OOBEController = {
  init() {
    const domain = state.setup.baseDomain || "example.com";
    const email = state.setup.defaultAdminEmail || domainToDefaultEmail(domain);
    el.setupDomain.value = domain;
    el.setupAdminEmail.value = email;
    if (el.setupAdminRecoveryEmail) el.setupAdminRecoveryEmail.value = "";
    el.setupPassword.value = "";
    el.setupPasswordConfirm.value = "";
    if (el.setupAdminMailboxLogin) el.setupAdminMailboxLogin.value = "";
    if (el.setupPasskeyPrimaryEnabled) el.setupPasskeyPrimaryEnabled.checked = state.setup.passkeyPrimaryEnabled !== false;
    el.setupRegion.value = el.setupRegion.value || "us-east";
    if (el.setupCompleteNote) {
      el.setupCompleteNote.textContent = "Auto opening mail in 3 seconds.";
    }
    state.setup.adminEmailTouched = false;
    state.setup.lastAutoAdminEmail = email;
    state.setup.retryUntilMs = 0;
    if (state.setup.retryTimer) {
      clearInterval(state.setup.retryTimer);
      state.setup.retryTimer = 0;
    }
    state.setup.modalType = "";
    state.setup.submitting = false;
    if (state.setup.autoOpenTimer) {
      clearTimeout(state.setup.autoOpenTimer);
      state.setup.autoOpenTimer = 0;
    }
    this.setThemeChoice(ThemeController.getTheme() || state.theme || "machine-dark", { applyTheme: false });
    this.setAutomaticUpdatesChoice(state.setup.automaticUpdatesEnabled !== false);
    setSetupInlineStatus("");
    this.updatePasswordHint();
    this.setStep(0);
    this.updateSummary();
  },

  setThemeChoice(themeName, opts = {}) {
    const next = themeName === "paper-light" ? "paper-light" : "machine-dark";
    if (opts.applyTheme !== false) {
      ThemeController.setTheme(next);
    }
    setSetupChoicePressed(el.setupThemeMachine, next === "machine-dark");
    setSetupChoicePressed(el.setupThemePaper, next === "paper-light");
    this.updateSummary();
    this.refreshNavState();
  },

  setAutomaticUpdatesChoice(enabled) {
    const next = enabled !== false;
    state.setup.automaticUpdatesEnabled = next;
    setSetupChoicePressed(el.setupUpdatesAuto, next);
    setSetupChoicePressed(el.setupUpdatesManual, !next);
    this.updateSummary();
    this.refreshNavState();
  },

  updateProgress() {
    if (!el.setupProgressLabel || !el.setupProgressTitle) return;
    const stepIndex = Math.max(0, Math.min(state.setup.step, setupSteps.length - 1));
    el.setupProgressTitle.textContent = setupStepTitles[stepIndex] || "Setup";
    if (stepIndex === setupCompleteStep) {
      el.setupProgressLabel.textContent = "Setup complete";
      return;
    }
    el.setupProgressLabel.textContent = `Step ${stepIndex + 1} of ${setupSteps.length}`;
  },

  setStep(step) {
    const nextStep = Math.max(0, Math.min(step, setupSteps.length - 1));
    const previousStep = Number(state.setup.step || 0);
    const direction = nextStep >= previousStep ? "forward" : "backward";
    state.setup.step = nextStep;
    for (let i = 0; i < setupSteps.length; i += 1) {
      setupSteps[i].classList.toggle("hidden", i !== state.setup.step);
    }
    const activeStep = setupSteps[state.setup.step];
    if (activeStep) {
      activeStep.classList.remove("is-entering-forward", "is-entering-backward");
      if (nextStep !== previousStep && !prefersReducedMotion()) {
        const enterClass = direction === "backward" ? "is-entering-backward" : "is-entering-forward";
        const clear = () => activeStep.classList.remove("is-entering-forward", "is-entering-backward");
        activeStep.addEventListener("animationend", clear, { once: true });
        window.setTimeout(clear, 260);
        window.requestAnimationFrame(() => {
          activeStep.classList.add(enterClass);
        });
      }
    }
    const isReview = state.setup.step === setupReviewStep;
    const isComplete = state.setup.step === setupCompleteStep;
    const showBack = state.setup.step > 0 && !isComplete;
    const showDiscard = state.setup.step > 0 && !isComplete;
    const retryRemaining = setupRetrySecondsRemaining();

    el.setupBackIcon.classList.toggle("hidden", !showBack);
    el.setupClose.classList.toggle("hidden", !showDiscard);
    el.setupBackIcon.disabled = !showBack || state.setup.submitting || retryRemaining > 0;
    el.setupClose.disabled = !showDiscard || state.setup.submitting || retryRemaining > 0;
    el.setupNext.classList.toggle("hidden", isComplete);
    if (retryRemaining > 0) {
      el.setupNext.textContent = `Retry in ${retryRemaining}s`;
    } else {
      el.setupNext.textContent = state.setup.submitting ? "Initializing..." : isReview ? "Initialize" : "Continue";
    }
    if (!isComplete) setSetupInlineStatus("");
    this.updateProgress();
    this.refreshNavState();
  },

  updateSummary() {
    el.setupSummaryRegion.textContent = el.setupRegion.options[el.setupRegion.selectedIndex]?.text || "-";
    if (el.setupSummaryTheme) {
      el.setupSummaryTheme.textContent = setupThemeLabel(ThemeController.getTheme());
    }
    if (el.setupSummaryUpdates) {
      el.setupSummaryUpdates.textContent = setupAutomaticUpdatesLabel(state.setup.automaticUpdatesEnabled !== false);
    }
    el.setupSummaryDomain.textContent = normalizeDomain(el.setupDomain.value) || "-";
    el.setupSummaryEmail.textContent = String(el.setupAdminEmail.value || "-").trim().toLowerCase();
    if (el.setupSummaryRecoveryEmail) {
      el.setupSummaryRecoveryEmail.textContent = normalizeRecoveryEmailInput(el.setupAdminRecoveryEmail?.value) || "-";
    }
    if (el.setupSummaryPasskey) {
      el.setupSummaryPasskey.textContent = el.setupPasskeyPrimaryEnabled?.checked ? "Enabled" : "Disabled";
    }
  },

  validateStep(stepId) {
    if (stepId === 4) {
      const domain = normalizeDomain(el.setupDomain.value);
      const email = String(el.setupAdminEmail.value || "").trim().toLowerCase();
      const recoveryEmail = normalizeRecoveryEmailInput(el.setupAdminRecoveryEmail?.value);
      if (!validDomain(domain)) {
        throw new Error("Enter a valid domain (example: mail.example.com or example.com)");
      }
      if (!validEmail(email)) {
        throw new Error("Enter a valid admin email");
      }
      if (!email.endsWith(`@${domain}`)) {
        throw new Error(`Admin email must use @${domain}`);
      }
      if (!validEmail(recoveryEmail)) {
        throw new Error("Enter a valid recovery email");
      }
      if (recoveryEmail === email) {
        throw new Error("Recovery email must be different from the admin email");
      }
    }

    if (stepId === 5) {
      const p1 = el.setupPassword.value;
      const p2 = el.setupPasswordConfirm.value;
      if (p1.length === 0) {
        throw new Error("Admin password is required");
      }
      if (p1 !== p2) {
        throw new Error("Password and verify password must match");
      }
      if (state.setup.authMode !== "pam") {
        const minLen = Number(state.setup.passwordMinLength || 12);
        const maxLen = Number(state.setup.passwordMaxLength || 128);
        const classMin = Number(state.setup.passwordClassMin || 3);
        if (p1.length < minLen) {
          throw new Error(`Admin password must be at least ${minLen} characters`);
        }
        if (p1.length > maxLen) {
          throw new Error(`Admin password must be at most ${maxLen} characters`);
        }
        if (passwordClassCount(p1) < classMin) {
          throw new Error(`Password must include at least ${classMin} character classes (lower/upper/number/symbol)`);
        }
      }
    }
  },

  isStepValid(stepId) {
    try {
      this.validateStep(stepId);
      return true;
    } catch {
      return false;
    }
  },

  refreshNavState() {
    const showBack = state.setup.step > 0 && state.setup.step < setupCompleteStep;
    const showDiscard = state.setup.step > 0 && state.setup.step < setupCompleteStep;
    el.setupBackIcon.classList.toggle("hidden", !showBack);
    el.setupClose.classList.toggle("hidden", !showDiscard);
    el.setupNext.classList.toggle("hidden", state.setup.step === setupCompleteStep);

    if (state.setup.step === setupCompleteStep) {
      el.setupBackIcon.disabled = true;
      el.setupClose.disabled = true;
      el.setupNext.disabled = true;
      return;
    }
    if (setupRetrySecondsRemaining() > 0 || state.setup.submitting) {
      el.setupBackIcon.disabled = true;
      el.setupClose.disabled = true;
      el.setupNext.disabled = true;
      return;
    }
    el.setupBackIcon.disabled = !showBack;
    el.setupClose.disabled = !showDiscard;
    el.setupNext.disabled = !this.isStepValid(state.setup.step);
  },

  async next() {
    if (state.setup.submitting) return;
    if (setupRetrySecondsRemaining() > 0) return;
    this.validateStep(state.setup.step);
    if (state.setup.step < setupReviewStep) {
      this.setStep(state.setup.step + 1);
      this.updateSummary();
      return;
    }
    if (state.setup.step === setupReviewStep) {
      state.setup.submitting = true;
      this.refreshNavState();
      setSetupInlineStatus("Initializing setup...", "info");
      try {
        await completeSetup();
        state.setup.submitting = false;
        this.setStep(setupCompleteStep);
        this.scheduleAutoOpenMail();
        setSetupInlineStatus("");
      } catch (err) {
        state.setup.submitting = false;
        this.refreshNavState();
        if (err.code === "rate_limited") {
          const wait = Number(err.retryAfterSec || 60);
          setSetupCooldown(wait);
          const wrapped = new Error(`Too many setup attempts. Wait about ${wait} seconds and try again.`);
          wrapped.code = "rate_limited";
          wrapped.requestID = err.requestID || "";
          throw wrapped;
        }
        throw err;
      }
    }
  },

  updatePasswordHint() {
    if (!el.setupPasswordHint) return;
    if (state.setup.authMode === "pam") {
      el.setupPasswordHint.textContent = "PAM mode: use the mailbox password already managed on this server. Add Mailbox Login if sign-in uses a different local account name.";
      if (el.setupAdminMailboxLoginWrap) {
        el.setupAdminMailboxLoginWrap.classList.remove("hidden");
      }
      return;
    }
    if (el.setupAdminMailboxLoginWrap) {
      el.setupAdminMailboxLoginWrap.classList.add("hidden");
    }
    if (el.setupAdminMailboxLogin) {
      el.setupAdminMailboxLogin.value = "";
    }
    const minLen = Number(state.setup.passwordMinLength || 12);
    const classMin = Number(state.setup.passwordClassMin || 3);
    el.setupPasswordHint.textContent = `Use at least ${minLen} characters and ${classMin} character classes (lower/upper/number/symbol).`;
  },

  back() {
    if (state.setup.step <= 0 || state.setup.step === setupCompleteStep || state.setup.submitting) return;
    this.setStep(state.setup.step - 1);
    this.updateSummary();
  },

  scheduleAutoOpenMail() {
    if (state.setup.autoOpenTimer) clearTimeout(state.setup.autoOpenTimer);
    let ticks = 3;
    if (el.setupCompleteNote) {
      el.setupCompleteNote.textContent = `Auto opening mail in ${ticks} seconds.`;
    }
    const interval = setInterval(() => {
      ticks -= 1;
      if (ticks > 0 && el.setupCompleteNote) {
        el.setupCompleteNote.textContent = `Auto opening mail in ${ticks} seconds.`;
      }
    }, 1000);
    state.setup.autoOpenTimer = window.setTimeout(async () => {
      clearInterval(interval);
      await this.openMail();
    }, 3000);
  },

  async openMail() {
    if (state.setup.autoOpenTimer) {
      clearTimeout(state.setup.autoOpenTimer);
      state.setup.autoOpenTimer = 0;
    }
    setActiveTab(el.tabMail);
    showView("mail");
    setActiveMailPane("messages");
    if (!state.user) {
      routeToAuthWithMessage("Sign in required before opening mailbox.", "session_missing");
      return;
    }
    await loadMailboxes();
    await loadMessages();
  },

  async openAdmin() {
    if (!state.user || state.user.role !== "admin") {
      await this.openMail();
      return;
    }
    if (state.setup.autoOpenTimer) {
      clearTimeout(state.setup.autoOpenTimer);
      state.setup.autoOpenTimer = 0;
    }
    setActiveTab(el.tabAdmin);
    showView("admin");
    setActiveAdminSection(state.ui.activeAdminSection || "system");
    await loadAdmin();
  },

  openConfirm(type) {
    if (type === "cancel" && (state.setup.step < 1 || state.setup.step >= setupCompleteStep)) {
      return;
    }
    state.setup.modalType = type;
    if (type === "cancel") {
      el.setupModalTitle.textContent = "Discard Setup Progress?";
      el.setupModalBody.textContent = "This clears the values entered so far and returns to the welcome step.";
      el.setupModalConfirm.textContent = "Discard";
    } else {
      el.setupModalTitle.textContent = "Reset Entered Values?";
      el.setupModalBody.textContent = "This removes all values entered in the assistant and returns to the welcome step.";
      el.setupModalConfirm.textContent = "Reset";
    }
    el.setupModalOverlay.classList.remove("hidden");
    el.setupModalOverlay.setAttribute("aria-hidden", "false");
    el.setupModalCancel.focus();
  },

  closeConfirm() {
    el.setupModalOverlay.classList.add("hidden");
    el.setupModalOverlay.setAttribute("aria-hidden", "true");
    if (!el.setupBackIcon.classList.contains("hidden") && !el.setupBackIcon.disabled) {
      el.setupBackIcon.focus();
    } else if (!el.setupNext.classList.contains("hidden") && !el.setupNext.disabled) {
      el.setupNext.focus();
    } else if (el.setupOpenMail && !el.setupOpenMail.classList.contains("hidden")) {
      el.setupOpenMail.focus();
    } else {
      el.setupClose.focus();
    }
  },

  async confirm() {
    const type = state.setup.modalType || "cancel";
    this.closeConfirm();
    this.init();
    if (type === "cancel") {
      setStatus("SETUP PROGRESS DISCARDED.", "info");
      return;
    }
    setStatus("SETUP FORM RESET", "info");
  },
};

async function enterSetupIfRequired() {
  const status = await loadSetupStatus();
  if (!status.required) return false;
  state.setup.required = true;
  OOBEController.init();
  applyNavVisibility();
  setActiveTab(el.tabSetup);
  showView("setup");
  setStatus("FIRST-RUN SETUP REQUIRED", "info");
  return true;
}

async function unlockMailSecretForSession() {
  let errorHint = "";
  while (true) {
    const password = await showPromptModal({
      title: "Unlock Mailbox Password",
      body: errorHint || "Enter your mailbox password once to unlock message access for this passkey session.",
      label: "Mailbox Password",
      inputType: "password",
      confirmText: "Unlock",
      cancelText: "Cancel",
    });
    if (!password) {
      throw new Error("Mailbox password is required before mail can be loaded.");
    }
    try {
      await api("/api/v1/session/mail-secret/unlock", {
        method: "POST",
        json: { password },
        skipUnauthorizedHandling: true,
        skipMFAHandling: true,
      });
      setStatus("Mailbox password unlocked for this session.", "ok");
      return;
    } catch (err) {
      if (err.code === "invalid_credentials") {
        errorHint = "Mailbox password was not accepted. Try again.";
        continue;
      }
      throw err;
    }
  }
}

async function ensureMailSecretUnlocked(payload = null) {
  if (!payload || payload.mail_secret_required !== true) {
    return;
  }
  await unlockMailSecretForSession();
}

async function finalizePrimaryLogin(loginPayload) {
  const loginStage = authStageFromPayload(loginPayload);
  if (loginStage.auth_stage !== "authenticated") {
    await ensureMFAStageAuthenticated(loginStage);
  }
  await ensureMailSecretUnlocked(loginPayload);
  let session;
  try {
    session = await refreshSession({ throwOnFail: true, skipUnauthorizedHandling: true, skipMFAHandling: true });
  } catch (err) {
    if (isSessionErrorCode(err.code)) {
      routeToAuthWithMessage("Login accepted but browser session cookie was not established. Check HTTP/HTTPS cookie policy.", err.code);
      return false;
    }
    throw err;
  }
  if (session?.user?.mail_secret_required === true) {
    await unlockMailSecretForSession();
    session = await refreshSession({ throwOnFail: true, skipUnauthorizedHandling: true, skipMFAHandling: true });
  }
  clearReaderSelection();
  await loadMailboxes();
  await loadMessages();
  setActiveTab(el.tabMail);
  showView("mail");
  setActiveMailPane("messages");
  return true;
}

async function refreshSession(opts = {}) {
  try {
    const me = await api("/api/v1/me", {
      skipUnauthorizedHandling: !!opts.skipUnauthorizedHandling,
      skipMFAHandling: !!opts.skipMFAHandling,
      logErrors: !opts.skipUnauthorizedHandling,
    });
    state.user = me;
    setStatus(`Signed in as ${me.email}.`, "ok");
    applyNavVisibility();
    await promptRecoveryEmailIfNeeded();
    await promptLegacyMFAIfNeeded();
    await loadTrustedDevices();
    await loadPasskeyCredentials();
    await loadSessions();
    return { ok: true, user: me };
  } catch (err) {
    state.auth.recoveryPromptShownForSession = false;
    state.auth.legacyMFAOfferShownForSession = false;
    state.user = null;
    renderSessions([]);
    applyNavVisibility();
    if (opts.throwOnFail) throw err;
    return { ok: false, error: err };
  }
}

function userNeedsRecoveryEmail(user) {
  if (!user || typeof user !== "object") {
    return false;
  }
  if (user.needs_recovery_email === true) {
    return true;
  }
  const login = String(user.email || "").trim().toLowerCase();
  const recovery = String(user.recovery_email || "").trim().toLowerCase();
  if (!recovery || !validEmail(recovery)) {
    return true;
  }
  return login !== "" && recovery === login;
}

async function promptRecoveryEmailIfNeeded() {
  if (!state.user || state.auth.recoveryPromptShownForSession || !userNeedsRecoveryEmail(state.user)) {
    return;
  }
  state.auth.recoveryPromptShownForSession = true;

  while (state.user && userNeedsRecoveryEmail(state.user)) {
    const seed = String(state.user.recovery_email || state.user.email || "").trim();
    const input = await showPromptModal({
      title: "Set Recovery Email",
      body: "Password reset stays unavailable for this account until a recovery email different from your login email is saved.",
      label: "Recovery Email",
      inputType: "email",
      defaultValue: seed,
      confirmText: "Save",
      cancelText: "Skip For Now",
    });
    if (input === null) {
      setStatus("Recovery email is missing. Password reset delivery stays disabled until it is set.", "info");
      return;
    }
    const candidate = String(input || "").trim().toLowerCase();
    const login = String(state.user.email || "").trim().toLowerCase();
    if (!candidate || !validEmail(candidate)) {
      setStatus("Enter a valid recovery email address.", "error");
      continue;
    }
    if (candidate === login) {
      setStatus("Recovery email must be different from login email.", "error");
      continue;
    }
    try {
      const res = await api("/api/v1/me/recovery-email", {
        method: "POST",
        json: { recovery_email: candidate },
      });
      const next = String(res.recovery_email || candidate).trim().toLowerCase();
      state.user.recovery_email = next;
      state.user.needs_recovery_email = false;
      setStatus("Recovery email saved.", "ok");
      return;
    } catch (err) {
      setStatus(formatAPIError(err, "Failed to save recovery email."), "error");
    }
  }
}

function normalizeMailboxRole(rawRole, mailboxName = "") {
  const role = String(rawRole || "").trim().toLowerCase();
  if (role) return role;
  const key = String(mailboxName || "").trim().toLowerCase();
  if (key === "inbox" || key.endsWith("/inbox")) return "inbox";
  if (key === "drafts" || key.includes("draft")) return "drafts";
  if (key === "sent" || key === "sent messages" || key.includes("sent")) return "sent";
  if (key === "trash" || key === "deleted messages" || key.includes("trash") || key.includes("deleted")) return "trash";
  if (key === "archive" || key.includes("archive") || key.includes("all mail")) return "archive";
  if (key === "junk" || key === "spam" || key.includes("junk") || key.includes("spam")) return "junk";
  return "";
}

function mailboxRoleRank(role) {
  const ranks = { inbox: 0, drafts: 1, sent: 2, trash: 3, archive: 4, junk: 5 };
  return Number(ranks[normalizeMailboxRole(role)] ?? 999);
}

function mailboxDisplayLabel(mailbox) {
  const role = normalizeMailboxRole(mailbox?.role, mailbox?.name);
  if (role === "inbox") return "Inbox";
  if (role === "drafts") return "Drafts";
  if (role === "sent") return "Sent";
  if (role === "trash") return "Trash";
  if (role === "archive") return "Archive";
  if (role === "junk") return "Junk";
  return String(mailbox?.name || "Mailbox");
}

function mailboxRecordByName(name) {
  const target = String(name || "").trim();
  if (!target) return null;
  return state.mail.mailboxes.find((item) => String(item?.name || "") === target) || null;
}

function mailboxNameForRole(role) {
  const target = normalizeMailboxRole(role);
  if (!target) return "";
  const match = state.mail.mailboxes.find((item) => normalizeMailboxRole(item?.role, item?.name) === target);
  return String(match?.name || "");
}

function renderMailboxes() {
  const items = Array.isArray(state.mail.mailboxes) ? state.mail.mailboxes : [];
  const system = [];
  const folders = [];
  for (const mb of items) {
    const role = normalizeMailboxRole(mb?.role, mb?.name);
    if (role === "drafts") continue;
    if (role) system.push(mb);
    else folders.push(mb);
  }
  system.push({
    name: APP_DRAFTS_MAILBOX,
    role: "drafts",
    count: Array.isArray(state.mail.drafts) ? state.mail.drafts.length : 0,
  });
  system.sort((a, b) => {
    const ra = mailboxRoleRank(a?.role || a?.name);
    const rb = mailboxRoleRank(b?.role || b?.name);
    if (ra !== rb) return ra - rb;
    return String(a?.name || "").localeCompare(String(b?.name || ""));
  });
  folders.sort((a, b) => String(a?.name || "").localeCompare(String(b?.name || "")));

  el.mailboxes.innerHTML = "";
  let optionIndex = 0;
  const appendSection = (title, list) => {
    if (!Array.isArray(list) || list.length === 0) return;
    const header = document.createElement("li");
    header.className = "mailbox-section";
    header.innerHTML = `<div class="mailbox-section-title">${escapeHtml(title)}</div>`;
    el.mailboxes.appendChild(header);

    for (const mb of list) {
      const role = normalizeMailboxRole(mb?.role, mb?.name);
      const unread = role === "drafts"
        ? Math.max(0, Number(mb?.count || 0))
        : Math.max(0, Number(mb?.unread || 0));
      const li = document.createElement("li");
      li.className = "mailbox-row";
      const btn = document.createElement("button");
      btn.innerHTML = `<span class="mailbox-name">${escapeHtml(mailboxDisplayLabel(mb))}</span><span class="mailbox-meta">${unread > 0 ? `(${unread})` : ""}</span>`;
      btn.className = String(mb.name || "") === String(state.mailbox || "") ? "active" : "";
      btn.dataset.mailboxName = mb.name;
      btn.dataset.mailboxRole = role;
      btn.id = safeDomID("mailbox-option-", mb.name, `${optionIndex}`);
      btn.type = "button";
      btn.setAttribute("role", "option");
      btn.setAttribute("aria-selected", String(mb.name || "") === String(state.mailbox || "") ? "true" : "false");
      btn.tabIndex = -1;
      btn.onclick = async () => {
        state.mailbox = mb.name;
        state.mail.selectedDraftID = "";
        clearMailMessageSelection({ render: false });
        clearReaderSelection();
        await loadMessages();
        await loadMailboxes({ quiet: true });
        setActiveMailPane("messages");
      };
      optionIndex += 1;
      li.appendChild(btn);
      el.mailboxes.appendChild(li);
    }
  };

  appendSection("System", system);
  appendSection("Folders", folders);
  if (system.length === 0 && folders.length === 0) {
    const empty = document.createElement("li");
    empty.className = "message-empty";
    empty.textContent = "No mailboxes available.";
    el.mailboxes.appendChild(empty);
  }
  syncMailboxActiveDescendant();
}

async function loadMailboxes(opts = {}) {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  const [mailboxes] = await Promise.all([
    api("/api/v1/mailboxes", { logErrors: opts.logErrors }),
    loadDrafts({ logErrors: false }).catch(() => state.mail.drafts),
  ]);
  state.mail.mailboxes = Array.isArray(mailboxes) ? mailboxes : [];
  renderMailboxes();
  renderMailMoveTargets();
}

function updateLocalMailboxUnread(mailboxName, delta) {
  const target = String(mailboxName || state.mailbox || "").trim();
  if (!target || !delta) return;
  for (const mailbox of state.mail.mailboxes) {
    if (String(mailbox?.name || "") !== target) continue;
    const next = Math.max(0, Number(mailbox?.unread || 0) + Number(delta || 0));
    mailbox.unread = next;
    break;
  }
  renderMailboxes();
}

function applyLocalMessagePatch(messageID, patch) {
  const id = String(messageID || "").trim();
  if (!id || !patch || typeof patch !== "object") return;
  const summary = state.messages.find((item) => String(item?.id || "") === id) || null;
  const selectedSummary = state.selectedMessageSummary && String(state.selectedMessageSummary.id || "") === id ? state.selectedMessageSummary : null;
  const selectedMessage = state.selectedMessage && String(state.selectedMessage.id || "") === id ? state.selectedMessage : null;
  const threadItem = Array.isArray(state.thread?.items) ? state.thread.items.find((item) => String(item?.id || "") === id) : null;
  const mailboxName = String(patch.mailbox || summary?.mailbox || selectedSummary?.mailbox || selectedMessage?.mailbox || state.mailbox || "").trim();
  const prevSeen = summary?.seen ?? selectedSummary?.seen ?? selectedMessage?.seen;
  const nextSeen = Object.prototype.hasOwnProperty.call(patch, "seen") ? !!patch.seen : prevSeen;
  if (prevSeen !== undefined && nextSeen !== undefined && prevSeen !== nextSeen) {
    updateLocalMailboxUnread(mailboxName, nextSeen ? -1 : 1);
  }
  [summary, selectedSummary, selectedMessage, threadItem].forEach((item) => {
    if (!item) return;
    Object.assign(item, patch);
  });
  renderMessages(state.messages);
  if (selectedMessage) {
    renderSelectedMessageChrome(selectedMessage);
    renderReaderBody(selectedMessage);
  }
  renderThreadContext();
  if (!selectedMessage) {
    applyMailActionAvailability();
  }
}

function renderMessages(items) {
  el.messages.innerHTML = "";
  state.messages = Array.isArray(items) ? items : [];
  pruneMailSelectionToVisible();
  if (state.messages.length === 0) {
    const empty = document.createElement("li");
    empty.className = "message-empty";
    empty.textContent = "No messages to display.";
    el.messages.appendChild(empty);
    syncMailBulkControls();
    applyMailActionAvailability();
    return;
  }
  for (const m of state.messages) {
    const li = document.createElement("li");
    li.dataset.messageId = String(m?.id || "");
    const checked = selectedMailMessageIDs().has(String(m?.id || ""));
    const isActive = m.isDraft
      ? String(state.mail.selectedDraftID || "") === String(m.id || "")
      : !!(state.selectedMessage && state.selectedMessage.id === m.id);
    li.className = "message-row";
    if (isActive) li.classList.add("active");
    if (checked) li.classList.add("is-selected");
    if (!m.seen) li.classList.add("is-unread");
    if (m.flagged) li.classList.add("is-flagged");
    if (m.answered) li.classList.add("is-answered");
    if (m.isDraft) li.classList.add("is-draft");

    if (!m.isDraft) {
      const selectWrap = document.createElement("label");
      selectWrap.className = "message-row-select";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.className = "message-row-check";
      checkbox.checked = checked;
      checkbox.dataset.messageSelect = String(m.id || "");
      checkbox.setAttribute("aria-label", `Select ${String(m.subject || "(no subject)")}`);
      checkbox.addEventListener("click", (event) => {
        event.stopPropagation();
      });
      checkbox.addEventListener("change", () => {
        toggleMailMessageSelection(String(m.id || ""), checkbox.checked);
      });
      selectWrap.appendChild(checkbox);
      li.appendChild(selectWrap);
    }

    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "message-row-btn";
    btn.dataset.messageId = m.id;
    btn.dataset.threadId = String(m.thread_id || "");
    btn.id = safeDomID("message-option-", m.id, "message");
    btn.setAttribute("role", "option");
    btn.setAttribute("aria-selected", isActive ? "true" : "false");
    btn.tabIndex = -1;
    const sender = formatSenderDisplayName(m.from);
    const previewText = String(m.preview || "").trim();
    const previewMarkup = previewText ? escapeHtml(previewText) : "&nbsp;";
    const previewClass = previewText ? "message-preview" : "message-preview message-preview--empty";
    const contextBadge = m.isDraft && String(m.context_badge || "").trim()
      ? `<span class="message-context-badge">${escapeHtml(m.context_badge)}</span>`
      : "";
    btn.innerHTML = `<span class="message-mark" aria-hidden="true"></span>
      <span class="message-from">${escapeHtml(sender)}</span>
      <span class="message-subject">${escapeHtml(m.subject || "(no subject)")}${contextBadge}</span>
      <span class="${previewClass}">${previewMarkup}</span>
      <span class="message-date">${escapeHtml(formatListDate(m.date))}</span>`;
    btn.onclick = () => {
      if (m.isDraft) {
        clearReaderSelection();
        state.mail.selectedDraftID = String(m.id || "");
        renderMessages(state.messages);
        void openComposeDraft(m.id, btn);
        return;
      }
      state.mail.selectedDraftID = "";
      void openMessage(m.id, m);
    };
    li.appendChild(btn);
    el.messages.appendChild(li);
  }
  syncMessageActiveDescendant();
  syncMailBulkControls();
  applyMailActionAvailability();
}

async function loadMessages(opts = {}) {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  if (isDraftsMailboxSelected()) {
    const query = String(opts.query || "").trim().toLowerCase();
    if (!Array.isArray(state.mail.drafts) || state.mail.drafts.length === 0 || opts.refreshDrafts) {
      await loadDrafts({ logErrors: false });
    }
    let drafts = Array.isArray(state.mail.drafts) ? [...state.mail.drafts] : [];
    if (query) {
      drafts = drafts.filter((item) => {
        const haystack = [
          item?.to,
          item?.cc,
          item?.bcc,
          item?.subject,
          item?.body_text,
          item?.body_html,
          composeDraftContextLabel(item?.compose_mode),
        ].join("\n").toLowerCase();
        return haystack.includes(query);
      });
    }
    renderMessages(drafts.map((item) => buildDraftMessageSummary(item)));
    if (!opts.quiet) {
      setStatus(`Drafts loaded (${drafts.length}).`, "ok");
    }
    return;
  }
  const data = await api(`/api/v1/messages?mailbox=${encodeURIComponent(state.mailbox)}&page=1&page_size=40`);
  const selectedID = String(state.selectedMessage?.id || "");
  renderMessages(data.items || []);
  if (selectedID) {
    state.selectedMessageSummary = state.messages.find((item) => String(item?.id || "") === selectedID) || state.selectedMessageSummary;
  }
  if (!opts.quiet) {
    setStatus(`Mailbox ${state.mailbox} loaded.`, "ok");
  }
}

function renderMessageMeta(rows) {
  el.meta.innerHTML = "";
  for (const [label, rawValue] of rows) {
    const row = document.createElement("div");
    const labelNode = document.createElement("span");
    labelNode.className = "meta-label";
    labelNode.textContent = String(label || "-");

    const valueNode = document.createElement("span");
    valueNode.className = "meta-value";
    const valueText = String(rawValue || "-");
    valueNode.textContent = valueText;
    valueNode.title = valueText;

    row.appendChild(labelNode);
    row.appendChild(valueNode);
    el.meta.appendChild(row);
  }
}

function readerStatusText(message) {
  const states = [];
  if (!message?.seen) states.push("Unread");
  if (message?.flagged) states.push("Important");
  if (message?.answered) states.push("Replied");
  return states.join(" · ");
}

function renderSelectedMessageChrome(message = state.selectedMessage) {
  if (!message) {
    if (el.messageSubjectAnchor) el.messageSubjectAnchor.textContent = "(no subject)";
    renderMessageMeta([]);
    applyMailActionAvailability();
    return;
  }
  if (el.messageSubjectAnchor) {
    el.messageSubjectAnchor.textContent = message.subject || "(no subject)";
  }
  const metaRows = [
    ["From", message.from || "-"],
    ["To", (message.to || []).join(", ") || "-"],
    ["Date", formatDate(message.date) || "-"],
  ];
  const status = readerStatusText(message);
  if (status) {
    metaRows.push(["Status", status]);
  }
  renderMessageMeta(metaRows);
  applyMailActionAvailability();
}

function messageHasHTML(message) {
  return typeof message?.body_html === "string" && message.body_html.trim() !== "";
}

function buildReaderHTMLSrcdoc(rawHTML) {
  const bodyHTML = String(rawHTML || "");
  const csp = [
    "default-src 'none'",
    "img-src 'self' data: blob:",
    "media-src 'self' data: blob:",
    "style-src 'unsafe-inline'",
    "font-src data:",
    "script-src 'none'",
    "connect-src 'none'",
    "object-src 'none'",
    "frame-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
  ].join("; ");
  const baseStyle = [
    ":root{color-scheme:light;}",
    "html,body{margin:0;padding:0;background:#ffffff;color:#000000;}",
    "body{padding:12px;word-break:break-word;overflow-wrap:anywhere;-webkit-text-size-adjust:100%;}",
    "img,svg,video,canvas{max-width:100%;height:auto;}",
    "blockquote{margin:0 0 0 8px;padding-left:10px;}",
    "pre{white-space:pre-wrap;word-break:break-word;overflow-wrap:anywhere;}",
  ].join("");
  return `<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"color-scheme\" content=\"light\"><meta http-equiv=\"Content-Security-Policy\" content=\"${csp}\"><style>${baseStyle}</style></head><body>${bodyHTML}</body></html>`;
}

function renderReaderBody(message = state.selectedMessage) {
  const hasMessage = !!message;
  const hasHTML = hasMessage && messageHasHTML(message);
  const mode = hasMessage && hasHTML && state.ui.readerViewMode === "html" ? "html" : "plain";

  if (el.btnReaderViewHTML) {
    el.btnReaderViewHTML.disabled = !hasHTML;
    el.btnReaderViewHTML.classList.toggle("is-active", hasHTML && mode === "html");
    el.btnReaderViewHTML.setAttribute("aria-pressed", hasHTML && mode === "html" ? "true" : "false");
  }
  if (el.btnReaderViewPlain) {
    el.btnReaderViewPlain.disabled = !hasMessage;
    el.btnReaderViewPlain.classList.toggle("is-active", hasMessage && mode === "plain");
    el.btnReaderViewPlain.setAttribute("aria-pressed", hasMessage && mode === "plain" ? "true" : "false");
  }

  if (!hasMessage) {
    state.ui.readerViewMode = "plain";
    if (el.bodyHTMLWrap) el.bodyHTMLWrap.classList.add("hidden");
    if (el.bodyHTMLFrame) el.bodyHTMLFrame.srcdoc = "";
    if (el.bodyPlain) {
      el.bodyPlain.classList.remove("hidden");
      el.bodyPlain.textContent = "Select a message.";
    }
    return;
  }

  if (mode === "html") {
    if (el.bodyHTMLWrap) el.bodyHTMLWrap.classList.remove("hidden");
    if (el.bodyHTMLFrame) {
      el.bodyHTMLFrame.srcdoc = buildReaderHTMLSrcdoc(message.body_html || "");
    }
    if (el.bodyPlain) el.bodyPlain.classList.add("hidden");
    return;
  }

  if (el.bodyHTMLWrap) el.bodyHTMLWrap.classList.add("hidden");
  if (el.bodyHTMLFrame) el.bodyHTMLFrame.srcdoc = "";
  if (el.bodyPlain) {
    el.bodyPlain.classList.remove("hidden");
    el.bodyPlain.textContent = formatReaderPlainBody(String(message.body || "(empty)"));
  }
}

function applyMailActionAvailability() {
  const bulkSelected = hasBulkMailSelection();
  const actionCount = selectedMailActionCount();
  const hasReaderSelection = !!state.selectedMessage && !bulkSelected;
  const hasActionSelection = actionCount > 0;
  const hasArchiveMailbox = String(mailboxNameForRole("archive") || "").trim() !== "";
  const hasTrashMailbox = String(mailboxNameForRole("trash") || "").trim() !== "";
  const hasMoveTarget = String(el.mailMoveTarget?.value || "").trim() !== "";
  [el.btnReply, el.btnForward].forEach((node) => {
    if (!node) return;
    node.disabled = !hasReaderSelection;
  });
  [el.btnFlag, el.btnSeen].forEach((node) => {
    if (!node) return;
    node.disabled = !hasActionSelection;
  });
  if (el.btnArchive) el.btnArchive.disabled = !hasActionSelection || !hasArchiveMailbox;
  if (el.btnMove) el.btnMove.disabled = !hasActionSelection || !hasMoveTarget;
  if (el.btnTrash) el.btnTrash.disabled = !hasActionSelection || !hasTrashMailbox;
  if (el.btnFlag) {
    el.btnFlag.textContent = selectedMailActionFlagMode() === "unflag" ? "Unflag" : "Flag";
  }
  if (el.btnSeen) {
    el.btnSeen.textContent = selectedMailActionReadMode() === "unread" ? "Mark Unread" : "Mark Read";
  }
}

function clearReaderSelection() {
  const expanded = state.thread?.expanded !== false;
  state.selectedMessage = null;
  state.selectedMessageSummary = null;
  state.mail.selectedDraftID = "";
  state.thread = createThreadState({ expanded });
  if (el.messageSubjectAnchor) el.messageSubjectAnchor.textContent = "(no subject)";
  renderMessageMeta([]);
  renderReaderBody(null);
  if (el.attachments) el.attachments.textContent = "";
  renderThreadContext();
  applyMailActionAvailability();
}

function threadRailExpanded(thread = state.thread) {
  if (isMobileLayout()) return true;
  return thread?.expanded !== false;
}

function threadCanCollapse(thread = state.thread) {
  const items = Array.isArray(thread?.items) ? thread.items : [];
  return !isMobileLayout() && items.length > 1;
}

function setThreadExpanded(expanded) {
  state.thread = createThreadState({
    ...state.thread,
    expanded: !!expanded,
  });
  renderThreadContext();
}

function toggleThreadExpanded() {
  if (!threadCanCollapse(state.thread)) return;
  setThreadExpanded(!threadRailExpanded(state.thread));
}

function formatThreadCountLabel(thread, hasSelection) {
  const items = Array.isArray(thread?.items) ? thread.items : [];
  const hasThread = !!thread?.id && items.length > 0 && Number(thread?.index ?? -1) >= 0;
  if (!hasSelection) return "Conversation: -";
  if (!hasThread || items.length <= 1) return "Conversation · 1 message";
  return `Conversation · ${items.length} messages`;
}

function formatThreadSelectionLabel(thread, hasSelection) {
  const items = Array.isArray(thread?.items) ? thread.items : [];
  const index = Number(thread?.index ?? -1);
  const hasThread = !!thread?.id && items.length > 0 && index >= 0;
  if (!hasSelection) return "";
  if (!hasThread || items.length <= 1) return "Viewing current message";
  return `Viewing ${index + 1} of ${items.length}`;
}

function threadMailboxBadgeLabel(item, currentMailbox = "") {
  const mailbox = String(item?.mailbox || "").trim();
  const current = String(currentMailbox || "").trim();
  if (!mailbox || !current || mailbox.toLowerCase() === current.toLowerCase()) {
    return "";
  }
  const record = mailboxRecordByName(mailbox);
  return mailboxDisplayLabel(record || { name: mailbox });
}

function syncThreadActiveDescendant() {
  if (!el.threadList) return;
  const buttons = Array.from(el.threadList.querySelectorAll(".thread-row-btn"));
  const selectedID = String(state.selectedMessage?.id || "");
  const active = buttons.find((node) => String(node.dataset.messageId || "") === selectedID) || null;
  for (const button of buttons) {
    const isActive = button === active;
    const row = button.closest(".thread-row");
    if (row) row.classList.toggle("active", isActive);
    button.setAttribute("aria-selected", isActive ? "true" : "false");
    button.setAttribute("aria-current", isActive ? "true" : "false");
    button.tabIndex = -1;
  }
  if (active) {
    if (!active.id) {
      active.id = safeDomID("thread-option-", active.dataset.messageId || "", `${buttons.indexOf(active)}`);
    }
    el.threadList.setAttribute("aria-activedescendant", active.id);
  } else {
    el.threadList.removeAttribute("aria-activedescendant");
  }
}

function renderThreadList() {
  if (!el.threadList || !el.threadListWrap) return;
  el.threadList.innerHTML = "";
  const thread = state.thread || createThreadState();
  const items = Array.isArray(thread.items) ? thread.items : [];
  const hasThread = !!thread.id && items.length > 0 && Number(thread.index ?? -1) >= 0;
  const expanded = hasThread && items.length > 1 && threadRailExpanded(thread);
  el.threadListWrap.classList.toggle("hidden", !expanded);
  if (!expanded) {
    el.threadList.removeAttribute("aria-activedescendant");
    return;
  }
  const currentMailbox = String(thread.mailbox || state.mailbox || "").trim();
  for (const item of items) {
    const row = document.createElement("li");
    row.className = "thread-row";
    if (!item?.seen) row.classList.add("is-unread");
    if (item?.flagged) row.classList.add("is-flagged");
    if (item?.answered) row.classList.add("is-answered");

    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "thread-row-btn";
    btn.dataset.messageId = String(item?.id || "");
    btn.setAttribute("role", "option");
    btn.tabIndex = -1;
    const sender = formatSenderDisplayName(item?.from);
    const previewText = String(item?.preview || "").trim();
    const previewClass = previewText ? "thread-row-preview" : "thread-row-preview thread-row-preview--empty";
    const mailboxLabel = threadMailboxBadgeLabel(item, currentMailbox);
    const mailboxChip = mailboxLabel
      ? `<span class="thread-row-mailbox">${escapeHtml(mailboxLabel)}</span>`
      : "";
    btn.innerHTML = `<span class="thread-row-mark" aria-hidden="true"></span>
      <span class="thread-row-main">
        <span class="thread-row-top">
          <span class="thread-row-topline"><span class="thread-row-from">${escapeHtml(sender)}</span>${mailboxChip}</span>
          <span class="thread-row-date">${escapeHtml(formatListDate(item?.date))}</span>
        </span>
        <span class="thread-row-subject">${escapeHtml(item?.subject || "(no subject)")}</span>
        <span class="${previewClass}">${previewText ? escapeHtml(previewText) : "No preview available."}</span>
      </span>`;
    btn.addEventListener("click", () => {
      focusMailPane("reader");
      void openMessage(item.id, item).catch((err) => {
        setStatus(err.message, "error");
      });
    });
    row.appendChild(btn);
    el.threadList.appendChild(row);
  }
  syncThreadActiveDescendant();
}

function renderThreadContext() {
  if (
    !el.threadStrip
    || !el.threadPosition
    || !el.threadSelectionStatus
    || !el.threadTruncated
    || !el.btnThreadCollapse
    || !el.btnThreadPrev
    || !el.btnThreadNext
  ) return;
  const thread = state.thread || createThreadState();
  const items = Array.isArray(thread.items) ? thread.items : [];
  const hasThread = !!thread.id && items.length > 0 && Number(thread.index ?? -1) >= 0;
  const hasSelection = !!state.selectedMessage;
  const hasMultiple = hasThread && items.length > 1;
  const expanded = hasMultiple && threadRailExpanded(thread);
  const canCollapse = threadCanCollapse(thread);
  el.threadStrip.classList.toggle("hidden", !hasSelection);
  el.threadStrip.classList.toggle("is-expanded", expanded);
  el.threadStrip.classList.toggle("is-collapsed", hasMultiple && !expanded);
  el.threadPosition.textContent = formatThreadCountLabel(thread, hasSelection);
  const selectionLabel = formatThreadSelectionLabel(thread, hasSelection);
  el.threadSelectionStatus.textContent = selectionLabel;
  el.threadSelectionStatus.classList.toggle("hidden", selectionLabel === "");
  el.threadTruncated.classList.toggle("hidden", !(hasMultiple && thread.truncated));
  el.btnThreadCollapse.classList.toggle("hidden", !canCollapse);
  el.btnThreadCollapse.setAttribute("aria-expanded", expanded ? "true" : "false");
  el.btnThreadCollapse.textContent = expanded ? "Collapse" : "Expand";
  el.btnThreadPrev.disabled = !hasMultiple || thread.index <= 0;
  el.btnThreadNext.disabled = !hasMultiple || thread.index >= items.length - 1;
  renderThreadList();
}

async function loadThreadContext(summary, messageID, mailboxHint = "") {
  const expanded = state.thread?.expanded !== false;
  const threadID = String(summary?.thread_id || "").trim();
  if (!threadID) {
    state.thread = createThreadState({ expanded });
    renderThreadContext();
    return;
  }
  const baseMailbox = String(mailboxHint || summary?.mailbox || state.mailbox || "INBOX").trim() || "INBOX";
  try {
    const payload = await api(`/api/v1/threads/${encodeURIComponent(threadID)}/messages?mailbox=${encodeURIComponent(baseMailbox)}&scope=conversation&page=1&page_size=100`, { logErrors: false });
    const items = Array.isArray(payload?.items) ? [...payload.items] : [];
    if (!items.some((it) => String(it?.id || "") === String(messageID || "")) && summary?.id && String(summary.id) === String(messageID || "")) {
      items.push(summary);
    }
    items.sort((a, b) => {
      const da = new Date(a?.date || 0).getTime();
      const db = new Date(b?.date || 0).getTime();
      return da - db;
    });
    let index = items.findIndex((it) => String(it?.id || "") === String(messageID || ""));
    if (index < 0 && items.length > 0) {
      index = items.length - 1;
    }
    state.thread = createThreadState({
      id: threadID,
      items,
      index,
      truncated: !!payload?.truncated,
      mailbox: baseMailbox,
      expanded,
    });
  } catch {
    state.thread = createThreadState({ expanded });
  }
  renderThreadContext();
}

async function openThreadIndex(index) {
  const items = Array.isArray(state.thread?.items) ? state.thread.items : [];
  if (items.length === 0) return;
  const next = Number(index);
  if (!Number.isFinite(next) || next < 0 || next >= items.length) return;
  const target = items[next];
  if (!target?.id) return;
  if (String(target.id || "") === String(state.selectedMessage?.id || "")) {
    focusMailPane("reader");
    return;
  }
  await openMessage(target.id, target);
}

async function openThreadNeighbor(delta) {
  const items = Array.isArray(state.thread?.items) ? state.thread.items : [];
  const current = Number(state.thread?.index ?? -1);
  if (items.length === 0 || current < 0) return;
  await openThreadIndex(current + delta);
}

async function openThreadBoundary(position) {
  const items = Array.isArray(state.thread?.items) ? state.thread.items : [];
  if (items.length === 0) return;
  await openThreadIndex(position === "start" ? 0 : items.length - 1);
}

async function refreshSelectedThreadContext(opts = {}) {
  const message = state.selectedMessage;
  const summary = state.selectedMessageSummary || message || null;
  if (!message || !summary) {
    renderThreadContext();
    return;
  }
  const mailboxHint = String(summary?.mailbox || message?.mailbox || state.mailbox || "INBOX").trim() || "INBOX";
  await loadThreadContext(summary, message.id, mailboxHint);
  if (!opts.quiet) {
    renderThreadContext();
  }
}

async function refreshMailView(opts = {}) {
  if (!state.user) return;
  const preservePane = opts.preservePane !== false;
  const activePane = state.ui.activeMailPane;
  await Promise.all([
    loadMailboxes({ quiet: true, logErrors: false }),
    loadMessages({ quiet: true }),
  ]);
  await refreshSelectedThreadContext({ quiet: true });
  if (preservePane) {
    setActiveMailPane(activePane || (state.selectedMessage ? "reader" : "messages"), { focus: false });
  }
}

function stopMailPolling() {
  if (!state.mail.pollTimer) return;
  clearInterval(state.mail.pollTimer);
  state.mail.pollTimer = 0;
}

async function pollMailView() {
  if (!state.user) return;
  if (state.mail.refreshInFlight) return;
  if (document.visibilityState !== "visible") return;
  if (el.viewMail?.classList.contains("hidden")) return;
  state.mail.refreshInFlight = true;
  try {
    await refreshMailView({ preservePane: true });
  } catch {
    // Polling failures should not disrupt current mail state.
  } finally {
    state.mail.refreshInFlight = false;
  }
}

function startMailPolling() {
  stopMailPolling();
  if (!state.user) return;
  state.mail.pollTimer = window.setInterval(() => {
    void pollMailView();
  }, 20000);
}

function extractPrimaryEmailAddress(raw) {
  const value = String(raw || "").trim();
  if (!value) return "";
  const bracket = value.match(/<([^>]+)>/);
  if (bracket && validEmail(bracket[1])) {
    return bracket[1].trim().toLowerCase();
  }
  const direct = value.match(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i);
  if (direct && validEmail(direct[0])) {
    return direct[0].trim().toLowerCase();
  }
  return "";
}

function formatSenderDisplayName(raw) {
  const value = String(raw || "").trim();
  if (!value) return "(unknown sender)";
  const named = value.match(/^"?([^"<]+?)"?\s*<[^>]+>$/);
  if (named && named[1] && named[1].trim() !== "") {
    return named[1].trim();
  }
  const addr = value.match(/<([^>]+)>/);
  if (addr && validEmail(addr[1])) {
    return sanitizeSenderFallback(addr[1]);
  }
  if (validEmail(value)) {
    return sanitizeSenderFallback(value);
  }
  return value;
}

function sanitizeSenderFallback(emailLike) {
  const cleaned = String(emailLike || "").trim().toLowerCase();
  if (!validEmail(cleaned)) return cleaned || "(unknown sender)";
  const [local] = cleaned.split("@");
  if (!local) return cleaned;
  return local.replace(/[._-]+/g, " ").trim() || cleaned;
}

function formatReaderPlainBody(raw) {
  const text = String(raw || "");
  if (text === "") return "";
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const out = [];
  let inForwardHeader = false;
  for (const line of lines) {
    const trimmed = line.trimStart();
    if (/^>\s*(begin\s+forwarded\s+message:?|forwarded message:?)$/i.test(trimmed)) {
      out.push("──────── Forwarded message ────────");
      inForwardHeader = true;
      continue;
    }
    if (inForwardHeader) {
      if (trimmed === "") {
        out.push("");
        inForwardHeader = false;
        continue;
      }
      out.push(line.replace(/^\s*>\s?/, ""));
      continue;
    }
    out.push(line);
  }
  return out.join("\n");
}

function withPrefixSubject(prefix, value) {
  const subject = String(value || "").trim();
  if (subject === "") {
    return `${prefix}: (no subject)`;
  }
  const re = new RegExp(`^${prefix}\\s*:`, "i");
  if (re.test(subject)) {
    return subject;
  }
  return `${prefix}: ${subject}`;
}

function quoteMessageBody(body) {
  return String(body || "")
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((line) => `> ${line}`)
    .join("\n");
}

function buildReplyBodyPrefill(message) {
  const from = String(message?.from || "sender").trim() || "sender";
  const date = formatDate(message?.date) || "an earlier time";
  const body = quoteMessageBody(message?.body || "");
  return `\n\nOn ${date}, ${from} wrote:\n${body}`;
}

function buildForwardBodyPrefill(message) {
  const from = String(message?.from || "-");
  const to = Array.isArray(message?.to) ? message.to.join(", ") : "-";
  const subject = String(message?.subject || "(no subject)");
  const date = formatDate(message?.date) || "-";
  const body = String(message?.body || "");
  return `----- Forwarded message -----\nFrom: ${from}\nDate: ${date}\nSubject: ${subject}\nTo: ${to}\n\n${body}`;
}

function pluralizeMessages(count) {
  return count === 1 ? "message" : "messages";
}

async function runMailAction(action, options = {}) {
  const ids = selectedMailActionIDs();
  if (ids.length === 0) {
    setStatus("Select at least one message.", "error");
    return;
  }
  const targetMailbox = String(options.mailbox || "").trim();
  if ((action === "move" || action === "archive" || action === "trash") && !targetMailbox) {
    setStatus("Choose a destination mailbox first.", "error");
    return;
  }
  const failed = [];
  let succeeded = 0;
  for (const id of ids) {
    try {
      if (action === "flag") {
        await api(`/api/v1/messages/${encodeURIComponent(id)}/flags`, {
          method: "POST",
          json: { add: ["\\Flagged"] },
        });
      } else if (action === "unflag") {
        await api(`/api/v1/messages/${encodeURIComponent(id)}/flags`, {
          method: "POST",
          json: { remove: ["\\Flagged"] },
        });
      } else if (action === "read") {
        await api(`/api/v1/messages/${encodeURIComponent(id)}/flags`, {
          method: "POST",
          json: { add: ["\\Seen"] },
        });
      } else if (action === "unread") {
        await api(`/api/v1/messages/${encodeURIComponent(id)}/flags`, {
          method: "POST",
          json: { remove: ["\\Seen"] },
        });
      } else if (action === "move" || action === "archive" || action === "trash") {
        await api(`/api/v1/messages/${encodeURIComponent(id)}/move`, {
          method: "POST",
          json: { mailbox: targetMailbox },
        });
      } else {
        throw new Error("Unsupported mail action.");
      }
      succeeded += 1;
    } catch (err) {
      failed.push({ id, message: err.message });
    }
  }

  if (succeeded > 0) {
    if (action === "move" || action === "archive" || action === "trash") {
      clearMailMessageSelection({ render: false });
      clearReaderSelection();
      await Promise.all([
        loadMailboxes({ quiet: true, logErrors: false }),
        loadMessages({ quiet: true }),
      ]);
      setActiveMailPane("messages");
    } else {
      await refreshMailView({ preservePane: true });
    }
  }

  if (failed.length > 0) {
    setStatus(`${options.statusVerb || "Updated"} ${succeeded} ${pluralizeMessages(succeeded)}, ${failed.length} failed.`, "error");
    return;
  }
  setStatus(`${options.statusVerb || "Updated"} ${succeeded} ${pluralizeMessages(succeeded)}.`, "ok");
}

async function openReplyCompose() {
  requireSelectedMessage();
  const message = state.selectedMessage;
  const target = extractPrimaryEmailAddress(message?.from);
  if (!target) {
    setStatus("Cannot determine sender address for reply.", "error");
    return;
  }
  await openComposeOverlay(el.btnReply || el.btnComposeOpen, {
    title: "Reply",
    useDraft: false,
    sendContext: { mode: "reply", messageID: message.id },
    prefill: {
      to: target,
      subject: withPrefixSubject("Re", message.subject),
      bodyText: buildReplyBodyPrefill(message),
    },
  });
}

async function openForwardCompose() {
  requireSelectedMessage();
  const message = state.selectedMessage;
  await openComposeOverlay(el.btnForward || el.btnComposeOpen, {
    title: "Forward",
    useDraft: false,
    sendContext: { mode: "forward", messageID: message.id },
    prefill: {
      subject: withPrefixSubject("Fwd", message.subject),
      bodyText: buildForwardBodyPrefill(message),
    },
  });
}

function composeEndpointForContext() {
  const mode = String(state.compose.sendContext?.mode || "send").trim().toLowerCase();
  const messageID = String(state.compose.sendContext?.messageID || "").trim();
  if ((mode === "reply" || mode === "forward") && messageID) {
    return `/api/v1/messages/${encodeURIComponent(messageID)}/${mode}`;
  }
  return "/api/v1/messages/send";
}

async function openMessage(id, summary = null) {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  state.mail.selectedDraftID = "";
  const knownSummary = summary || state.messages.find((item) => String(item?.id || "") === String(id || "")) || null;
  state.selectedMessageSummary = knownSummary;
  const m = await api(`/api/v1/messages/${encodeURIComponent(id)}`);
  state.selectedMessage = m;
  renderSelectedMessageChrome(m);
  state.ui.readerViewMode = messageHasHTML(m) ? "html" : "plain";
  renderReaderBody(m);

  el.attachments.innerHTML = "";
  for (const a of (m.attachments || [])) {
    const link = document.createElement("a");
    link.href = `/api/v1/attachments/${encodeURIComponent(a.id)}`;
    link.textContent = `${a.filename || "attachment"} (${Math.round((a.size || 0) / 1024)} KB)`;
    link.target = "_blank";
    el.attachments.appendChild(link);
  }
  const threadMailbox = String(knownSummary?.mailbox || m.mailbox || state.mailbox || "INBOX").trim() || "INBOX";
  await loadThreadContext(knownSummary, m.id, threadMailbox);
  if (knownSummary && !knownSummary.seen) {
    applyLocalMessagePatch(m.id, { seen: true });
    void api(`/api/v1/messages/${encodeURIComponent(m.id)}/flags`, {
      method: "POST",
      json: { add: ["\\Seen"] },
      logErrors: false,
    }).then(async () => {
      await Promise.all([
        loadMailboxes({ quiet: true, logErrors: false }),
        loadMessages({ quiet: true }),
      ]);
      await refreshSelectedThreadContext({ quiet: true });
    }).catch(() => {
      applyLocalMessagePatch(m.id, { seen: false });
      setStatus("Failed to mark message as read.", "error");
    });
  }
  renderMessages(state.messages);
  syncMessageActiveDescendant();
  setActiveMailPane("reader");
}

async function searchMessages() {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  const q = el.searchInput.value.trim();
  clearMailMessageSelection({ render: false });
  if (isDraftsMailboxSelected()) {
    clearReaderSelection();
    await loadMessages({ quiet: true, query: q });
    setStatus(`Draft search complete (${state.messages.length} results).`, "ok");
    setActiveMailPane("messages");
    return;
  }
  const data = await api(`/api/v1/search?mailbox=${encodeURIComponent(state.mailbox)}&q=${encodeURIComponent(q)}&page=1&page_size=40`);
  clearReaderSelection();
  renderMessages(data.items || []);
  setStatus(`Search complete (${(data.items || []).length} results).`, "ok");
  setActiveMailPane("messages");
}

function composeShouldPersistSendFailure(err) {
  const status = Number(err?.status || 0);
  if (!status) return true;
  return status >= 500;
}

async function markComposeDraftSendFailed(err) {
  const message = formatAPIError(err, "Send failed.");
  state.compose.draftStatus = "failed";
  state.compose.lastSendError = message;
  applyComposeSendFailurePresentation();
  updateComposeSubmitState();
  const draftID = String(state.compose.draftID || "").trim();
  if (!draftID) return;
  try {
    const saved = await api(`/api/v2/drafts/${encodeURIComponent(draftID)}`, {
      method: "PATCH",
      json: { status: "failed", last_send_error: message },
      logErrors: false,
    });
    syncComposeServerDraftState(saved || {}, { keepDirty: true });
  } catch {
    // Best effort only; keep the local failed state visible.
  }
}

async function sendCompose(form) {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  commitComposeAllRecipientInputs();
  cleanupComposeInlineReferences();
  syncComposeDraftFields();
  await flushComposeDraft({ immediate: true, forceCreate: true });
  const draftID = String(state.compose.draftID || "").trim();
  if (!draftID) {
    throw new Error("Draft save failed.");
  }
  const result = await api(`/api/v2/drafts/${encodeURIComponent(draftID)}/send`, {
    method: "POST",
    json: {},
    logErrors: false,
  });
  clearComposeCrashBuffer(draftID);
  removeLocalDraft(draftID);
  clearComposeDraft();
  setComposeSendContext("send", "");
  return result;
}

async function discardComposeDraft() {
  const draftID = String(state.compose.draftID || "").trim();
  const hadContent = !!draftID || composeHasLiveMedia() || composeDraftHasMeaningfulContent(composeCurrentDraftPayload());
  clearComposeDraftSaveTimer();
  if (draftID) {
    await api(`/api/v2/drafts/${encodeURIComponent(draftID)}`, {
      method: "DELETE",
      logErrors: false,
    });
    clearComposeCrashBuffer(draftID);
    removeLocalDraft(draftID);
  } else {
    clearComposeCrashBuffer("");
  }
  resetComposeDraftSession();
  setComposeSendContext("send", "");
  setComposeDraftNote("");
  closeComposeOverlay({ restoreFocus: true, persistDraft: false });
  if (hadContent) {
    setStatus("Draft discarded.", "ok");
  }
}

function setUpdateNote(text, type = "info") {
  if (!el.updateNote) return;
  el.updateNote.textContent = text || "";
  el.updateNote.classList.remove("update-note--ok", "update-note--error", "update-note--info");
  if (type === "error") el.updateNote.classList.add("update-note--error");
  else if (type === "ok") el.updateNote.classList.add("update-note--ok");
  else el.updateNote.classList.add("update-note--info");
}

function updateConfigDiagnosticMessage(status) {
  const diag = status && typeof status === "object" ? status.config_diagnostic : null;
  if (!diag || typeof diag !== "object") {
    return "Updater is not configured on this host. Install despatch-updater systemd units to enable one-click updates.";
  }
  const reason = String(diag.reason || "").trim().toLowerCase();
  const detail = String(diag.detail || "").trim();
  const repair = String(diag.repair_hint || "").trim();
  let headline = "Updater is not configured on this host.";
  if (reason === "updater_unit_missing") {
    headline = "Updater units are missing on this host.";
  } else if (reason === "updater_service_missing") {
    headline = "Updater service unit is missing or not loaded by systemd.";
  } else if (reason === "updater_path_trigger_limited") {
    headline = "Updater request watcher hit a systemd trigger limit and stopped watching for requests.";
  } else if (reason === "updater_path_inactive") {
    headline = "Updater path unit is installed but not active.";
  } else if (reason === "updater_worker_missing") {
    headline = "Updater service does not resolve to a working updater executable.";
  } else if (reason === "updater_runtime_probe_failed") {
    headline = "Updater runtime could not be inspected through systemd.";
  } else if (reason === "request_dir_unwritable" || reason === "status_dir_unwritable") {
    headline = "Updater request/status directories are not writable by the despatch service user.";
  } else if (reason === "request_probe_failed" || reason === "status_probe_failed") {
    headline = "Updater write-probe failed due to permissions or ownership mismatch.";
  }
  const parts = [headline];
  if (detail) parts.push(detail);
  if (repair) parts.push(`Fix: ${repair}`);
  return parts.join(" ");
}

function applyUpdateControls(status) {
  if (!el.btnUpdateCheck || !el.btnUpdateApply || !el.btnUpdateAuto) return;
  const st = status || state.update.lastStatus || {};
  const applyState = String(st.apply?.state || "idle");
  const autoState = String(st.auto_update?.state || "idle");
  const busy = applyState === "queued" || applyState === "in_progress" || autoState === "preparing" || autoState === "applying";
  const checkSupported = !st.legacy_backend;
  const assetMissing = String(st.last_check_error || "").toLowerCase().includes("release asset");
  const scheduled = autoState === "scheduled";
  const prepared = autoState === "downloaded";
  el.btnUpdateCheck.disabled = state.update.checking || state.update.autoSaving || state.update.cancelingScheduled || !checkSupported || busy;
  const canApply = !!st.enabled && !!st.configured && !!st.update_available && !busy && !state.update.applying && !assetMissing;
  el.btnUpdateApply.disabled = !canApply;
  el.btnUpdateApply.textContent = scheduled || prepared ? "Install Now" : "Install Update";
  el.btnUpdateApply.classList.toggle("hidden", !busy && !scheduled && !prepared && !st.update_available);
  el.btnUpdateAuto.disabled = state.update.autoSaving || state.update.checking || busy || !st.enabled || st.legacy_backend;
  el.btnUpdateAuto.textContent = st.auto_update?.enabled === false ? "Off" : "On";
  if (el.btnUpdateCancelScheduled) {
    el.btnUpdateCancelScheduled.disabled = state.update.cancelingScheduled || !scheduled;
    el.btnUpdateCancelScheduled.classList.toggle("hidden", !scheduled);
  }
}

function renderUpdateStatus(status) {
  state.update.lastStatus = status || null;
  if (!status) {
    if (el.updateCurrentVersion) el.updateCurrentVersion.textContent = "-";
    if (el.updateCurrentCommit) el.updateCurrentCommit.textContent = "-";
    if (el.updateLatestVersion) el.updateLatestVersion.textContent = "-";
    if (el.updateLatestPublished) el.updateLatestPublished.textContent = "-";
    if (el.updateAvailable) el.updateAvailable.textContent = "-";
    if (el.updateLastChecked) el.updateLastChecked.textContent = "-";
    if (el.updateApplyState) el.updateApplyState.textContent = "Idle";
    if (el.updateScheduledFor) el.updateScheduledFor.textContent = "-";
    if (el.updateAutoState) el.updateAutoState.textContent = "Automatic updates on";
    if (el.updateHeroHeadline) el.updateHeroHeadline.textContent = "Software update status unavailable";
    if (el.updateHeroSubline) el.updateHeroSubline.textContent = "Despatch could not load updater status from the backend.";
    if (el.updateHeroIcon) el.updateHeroIcon.textContent = "!";
    if (el.updateHeroCard) el.updateHeroCard.dataset.state = "attention";
    if (el.adminSystemBadge) el.adminSystemBadge.classList.add("hidden");
    setUpdateNote("Update status unavailable.", "error");
    applyUpdateControls();
    return;
  }
  if (el.updateCurrentVersion) el.updateCurrentVersion.textContent = status.current?.version || "-";
  if (el.updateCurrentCommit) el.updateCurrentCommit.textContent = status.current?.commit ? `commit ${status.current.commit}` : "-";
  if (el.updateLatestVersion) el.updateLatestVersion.textContent = status.latest?.tag_name || "-";
  if (el.updateLatestPublished) el.updateLatestPublished.textContent = formatDate(status.latest?.published_at) || "-";
  if (el.updateAvailable) el.updateAvailable.textContent = status.update_available ? "Available" : "Up to date";
  if (el.updateLastChecked) el.updateLastChecked.textContent = formatDate(status.last_checked_at) || "-";
  if (el.updateApplyState) el.updateApplyState.textContent = String(status.apply?.state || "idle").replaceAll("_", " ");
  if (el.updateScheduledFor) {
    const autoState = String(status.auto_update?.state || "idle");
    const scheduledFor = formatDate(status.auto_update?.scheduled_for);
    el.updateScheduledFor.textContent = autoState === "scheduled" ? (scheduledFor || "Tonight at 02:00") : "-";
  }
  if (el.updateAutoState) {
    const autoEnabled = status.auto_update?.enabled !== false;
    const autoState = String(status.auto_update?.state || "idle");
    let autoLabel = autoEnabled ? "Automatic updates on" : "Automatic updates off";
    if (autoEnabled && autoState === "scheduled" && status.auto_update?.target_version) {
      autoLabel = `Scheduled for ${status.auto_update.target_version}`;
    } else if (autoEnabled && autoState === "preparing" && status.auto_update?.target_version) {
      autoLabel = `Preparing ${status.auto_update.target_version}`;
    } else if (autoEnabled && autoState === "downloaded" && status.auto_update?.target_version) {
      autoLabel = `${status.auto_update.target_version} downloaded`;
    } else if (autoState === "failed") {
      autoLabel = "Automatic update needs attention";
    }
    el.updateAutoState.textContent = autoLabel;
  }
  if (el.updateSourceLink) {
    el.updateSourceLink.href = status.latest?.html_url || "https://github.com/2high4schooltoday/despatch/releases";
  }

  const applyState = String(status.apply?.state || "idle");
  const applyError = String(status.apply?.error || "").trim();
  const autoState = String(status.auto_update?.state || "idle");
  const autoError = String(status.auto_update?.error || "").trim();
  let heroState = "ready";
  let heroIcon = "OK";
  let heroHeadline = "Your server is up to date";
  let heroSubline = status.current?.version ? `Despatch ${status.current.version} is currently installed.` : "This server is already on the latest available release.";

  if (status.legacy_backend) {
    heroState = "attention";
    heroIcon = "!";
    heroHeadline = "Backend update API is not available";
    heroSubline = "This build does not expose the updater endpoints yet. Upgrade the backend manually once, then reopen Admin.";
    setUpdateNote("This server build does not expose updater API endpoints yet (HTTP 404). Upgrade backend binary manually to a newer release, then reopen Admin.", "error");
  } else if (!status.enabled) {
    heroState = "attention";
    heroIcon = "!";
    heroHeadline = "Software update is disabled";
    heroSubline = "UPDATE_ENABLED is off in configuration, so Despatch will not check, prepare, or install updates.";
    setUpdateNote("Software update feature is disabled in configuration (UPDATE_ENABLED=false).", "info");
  } else if (!status.configured) {
    heroState = "attention";
    heroIcon = "!";
    heroHeadline = "One-click updates need attention";
    heroSubline = "The updater runtime is not healthy enough to stage or install releases from Admin.";
    setUpdateNote(updateConfigDiagnosticMessage(status), "error");
  } else if (autoState === "scheduled" && status.auto_update?.target_version) {
    heroState = "scheduled";
    heroIcon = "OK";
    heroHeadline = "Update scheduled";
    heroSubline = `${status.auto_update.target_version} is downloaded and will install at ${formatDate(status.auto_update?.scheduled_for) || "02:00 server time"}.`;
    setUpdateNote("The next verified release is ready and scheduled for the nightly maintenance window.", "ok");
  } else if (autoState === "preparing" && status.auto_update?.target_version) {
    heroState = "busy";
    heroIcon = "…";
    heroHeadline = "Preparing update";
    heroSubline = `${status.auto_update.target_version} is being downloaded, verified, and staged for automatic install.`;
    setUpdateNote("Despatch is downloading and verifying the latest release.", "info");
  } else if (applyState === "queued" || applyState === "in_progress" || autoState === "applying") {
    heroState = "busy";
    heroIcon = "…";
    heroHeadline = "Installing update";
    heroSubline = status.apply?.target_version
      ? `${status.apply.target_version} is being applied on this server now.`
      : "The updater is currently installing the staged release.";
    setUpdateNote("The updater is actively applying the release.", "info");
  } else if ((applyState === "failed" || applyState === "rolled_back") && applyError) {
    heroState = "failed";
    heroIcon = "!";
    heroHeadline = "Last update needs attention";
    heroSubline = applyError;
    if (applyError.toLowerCase().includes("mailsec")) {
      setUpdateNote(`Last update failed due to mailsec dependency checks: ${applyError}`, "error");
    } else {
      setUpdateNote(`Last update failed: ${applyError}`, "error");
    }
  } else if (autoState === "failed" && autoError) {
    heroState = "failed";
    heroIcon = "!";
    heroHeadline = "Automatic update needs attention";
    heroSubline = autoError;
    setUpdateNote(`Automatic update preparation failed: ${autoError}`, "error");
  } else if (status.last_check_error) {
    heroState = "attention";
    heroIcon = "!";
    heroHeadline = "Latest release check failed";
    heroSubline = status.last_check_error;
    if (String(status.last_check_error).toLowerCase().includes("release asset")) {
      setUpdateNote(`Release packaging issue detected for this CPU architecture: ${status.last_check_error}`, "error");
    } else {
      setUpdateNote(`Latest check failed: ${status.last_check_error}`, "error");
    }
  } else if (status.update_available && status.latest?.tag_name) {
    heroState = "ready";
    heroIcon = "↑";
    heroHeadline = "Update available";
    heroSubline = `${status.latest.tag_name} is available to install now${status.auto_update?.enabled === false ? "." : " or let Despatch stage it for tonight."}`;
    setUpdateNote(`New release available: ${status.latest.tag_name}`, "ok");
  } else {
    setUpdateNote("No update currently available.", "info");
  }
  if (el.updateHeroCard) el.updateHeroCard.dataset.state = heroState;
  if (el.updateHeroIcon) el.updateHeroIcon.textContent = heroIcon;
  if (el.updateHeroHeadline) el.updateHeroHeadline.textContent = heroHeadline;
  if (el.updateHeroSubline) el.updateHeroSubline.textContent = heroSubline;
  if (el.adminSystemBadge) {
    const needsBadge = !status.configured || autoState === "scheduled" || autoState === "failed" || applyState === "failed" || applyState === "rolled_back";
    el.adminSystemBadge.classList.toggle("hidden", !needsBadge);
    if (needsBadge) el.adminSystemBadge.textContent = autoState === "scheduled" ? "1" : "!";
  }
  applyUpdateControls(status);
}

function isUpdateStateBusy(status) {
  const stateName = String(status?.apply?.state || "");
  const autoState = String(status?.auto_update?.state || "");
  return stateName === "queued" || stateName === "in_progress" || autoState === "preparing" || autoState === "applying";
}

function handleUpdaterStatusUnavailable(err) {
  if (!err || Number(err.status) !== 503) return false;
  const last = state.update.lastStatus;
  if (last && !last.configured) {
    setUpdateNote(updateConfigDiagnosticMessage(last), "error");
    return true;
  }
  if (isUpdateStateBusy(last)) {
    setUpdateNote("Updater temporarily unavailable (HTTP 503). Waiting for service recovery.", "info");
    return true;
  }
  setUpdateNote("Updater temporarily unavailable (HTTP 503).", "error");
  return true;
}

function stopUpdatePolling() {
  if (!state.update.pollTimer) return;
  clearInterval(state.update.pollTimer);
  state.update.pollTimer = 0;
}

function startUpdatePolling() {
  stopUpdatePolling();
  state.update.pollTimer = window.setInterval(async () => {
    try {
      const status = await api("/api/v1/admin/system/update/status", { logErrors: false });
      renderUpdateStatus(status);
      if (!isUpdateStateBusy(status)) {
        stopUpdatePolling();
        if (status.apply?.state === "completed") {
          setStatus(`UPDATE COMPLETED: ${status.apply?.to_version || "new version active"}`, "ok");
        } else if (status.apply?.state === "rolled_back" || status.apply?.state === "failed") {
          setStatus(`UPDATE FAILED: ${status.apply?.error || "rolled back"}`, "error");
        }
      }
    } catch (err) {
      if (err.status === 404) {
        state.update.apiMissing = true;
        stopUpdatePolling();
        renderUpdateStatus(legacyUpdaterStatus());
        return;
      }
      if (handleUpdaterStatusUnavailable(err)) {
        return;
      }
      setUpdateNote(`Failed to refresh update status: ${err.message}`, "error");
    }
  }, 2500);
}

async function loadUpdateStatus(forceCheck = false) {
  if (!state.user || state.user.role !== "admin") {
    throw new Error("admin role required");
  }
  if (state.update.apiMissing) {
    const legacy = legacyUpdaterStatus();
    renderUpdateStatus(legacy);
    return legacy;
  }
  let status;
  try {
    status = forceCheck
      ? await api("/api/v1/admin/system/update/check", { method: "POST", json: {}, logErrors: false })
      : await api("/api/v1/admin/system/update/status", { logErrors: false });
  } catch (err) {
    if (err.status === 404) {
      state.update.apiMissing = true;
      const legacy = legacyUpdaterStatus();
      renderUpdateStatus(legacy);
      return legacy;
    }
    if (handleUpdaterStatusUnavailable(err) && state.update.lastStatus) {
      if (isUpdateStateBusy(state.update.lastStatus)) {
        startUpdatePolling();
      }
      return state.update.lastStatus;
    }
    throw err;
  }
  renderUpdateStatus(status);
  if (isUpdateStateBusy(status)) {
    startUpdatePolling();
  } else {
    stopUpdatePolling();
  }
  return status;
}

async function loadAdmin() {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  await loadActiveAdminSection();
}

async function loadActiveAdminSection() {
  if (state.ui.activeAdminSection === "system") {
    try {
      await loadUpdateStatus(false);
    } catch (err) {
      renderUpdateStatus(null);
      setUpdateNote(`Unable to load updater status: ${err.message}`, "error");
    }
    await loadAdminFeatureFlags();
    return;
  }
  if (state.ui.activeAdminSection === "registrations") {
    await loadAdminRegistrations();
    return;
  }
  if (state.ui.activeAdminSection === "users") {
    await loadAdminUsers();
    return;
  }
  if (state.ui.activeAdminSection === "audit") {
    await loadAdminAudit();
  }
}

function adminQuery(params) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    const v = String(value ?? "").trim();
    if (!v) return;
    query.set(key, v);
  });
  return query.toString();
}

function syncAdminCheckAll() {
  if (el.adminRegCheckAll) {
    const boxes = Array.from(document.querySelectorAll(".admin-reg-check"));
    el.adminRegCheckAll.checked = boxes.length > 0 && boxes.every((node) => node.checked);
  }
  if (el.adminUserCheckAll) {
    const boxes = Array.from(document.querySelectorAll(".admin-user-check"));
    el.adminUserCheckAll.checked = boxes.length > 0 && boxes.every((node) => node.checked);
  }
}

async function loadAdminRegistrations() {
  const f = state.admin.registrations;
  const query = adminQuery({
    status: f.status,
    q: f.q,
    sort: f.sort,
    order: f.order,
    page: 1,
    page_size: 100,
  });
  const regs = await api(`/api/v1/admin/registrations?${query}`);
  const rows = Array.isArray(regs.items) ? regs.items : [];
  state.admin.registrations.items = rows;
  if (!rows.some((item) => String(item.id || "") === state.admin.registrations.detailId)) {
    state.admin.registrations.detailId = "";
    if (state.ui.adminNav.domain === "registrations") {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
  }

  el.adminRegs.replaceChildren();
  if (rows.length === 0) {
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "No registrations match the current filters.";
    el.adminRegs.appendChild(empty);
  }
  for (const item of rows) {
    const regID = String(item.id || "").trim();
    const regEmail = String(item.email || "").trim();
    const checked = state.admin.registrations.selected.has(regID);
    const row = document.createElement("div");
    row.className = "setting-list-item";
    row.classList.add("setting-list-item--interactive");
    if (regID === state.admin.registrations.detailId) {
      row.classList.add("is-active");
    }
    row.tabIndex = 0;
    row.setAttribute("role", "button");
    row.addEventListener("click", (event) => {
      if (event.target && event.target.closest("input,button")) return;
      state.admin.registrations.detailId = regID;
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = regID;
      renderAdminRegistrationDetail();
    });
    row.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      state.admin.registrations.detailId = regID;
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = regID;
      renderAdminRegistrationDetail();
    });

    const checkWrap = document.createElement("span");
    const check = document.createElement("input");
    check.type = "checkbox";
    check.className = "admin-reg-check";
    check.dataset.id = regID;
    check.checked = checked;
    check.addEventListener("change", () => {
      if (check.checked) state.admin.registrations.selected.add(regID);
      else state.admin.registrations.selected.delete(regID);
      syncAdminCheckAll();
    });
    checkWrap.appendChild(check);
    row.appendChild(checkWrap);

    const main = document.createElement("span");
    main.className = "setting-list-main";
    const title = document.createElement("span");
    title.className = "setting-list-title";
    title.textContent = regEmail || "Registration";
    main.appendChild(title);
    const meta = document.createElement("span");
    meta.className = "setting-list-meta";
    meta.textContent = `${String(item.status || "pending").toUpperCase()} • Created ${formatDate(item.created_at) || "n/a"}`;
    main.appendChild(meta);
    row.appendChild(main);

    const view = document.createElement("button");
    view.type = "button";
    view.className = "setting-list-action";
    view.textContent = "View";
    view.addEventListener("click", () => {
      state.admin.registrations.detailId = regID;
      state.ui.adminNav.page = "detail";
      state.ui.adminNav.detailId = regID;
      renderAdminRegistrationDetail();
    });
    row.appendChild(view);
    el.adminRegs.appendChild(row);
  }
  renderAdminRegistrationDetail();
  if (state.ui.adminNav.page === "detail" && state.admin.registrations.detailId) {
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.registrations.detailId;
  } else {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
  }
  syncAdminCheckAll();
}

async function loadAdminUsers() {
  const f = state.admin.users;
  const query = adminQuery({
    q: f.q,
    status: f.status,
    role: f.role,
    provision_state: f.provision,
    sort: f.sort,
    order: f.order,
    page: 1,
    page_size: 100,
  });
  const users = await api(`/api/v1/admin/users?${query}`);
  const rows = (Array.isArray(users.items) ? users.items : []).filter((u) => String(u.status || "").trim().toLowerCase() !== "rejected");
  state.admin.users.items = rows;
  if (!rows.some((item) => String(item.id || "") === state.admin.users.detailId)) {
    state.admin.users.detailId = "";
    if (state.ui.adminNav.domain === "users") {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
  }
  el.adminUsers.replaceChildren();
  if (rows.length === 0) {
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "No users match the current filters.";
    el.adminUsers.appendChild(empty);
  }
  for (const item of rows) {
    const userID = String(item.id || "").trim();
    const checked = state.admin.users.selected.has(userID);
    const row = document.createElement("div");
    row.className = "setting-list-item";
    row.classList.add("setting-list-item--interactive");
    if (userID === state.admin.users.detailId) {
      row.classList.add("is-active");
    }
    row.tabIndex = 0;
    row.setAttribute("role", "button");
    row.addEventListener("click", (event) => {
      if (event.target && event.target.closest("input,button")) return;
      state.admin.users.detailId = userID;
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = userID;
      renderAdminUserDetail();
    });
    row.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      state.admin.users.detailId = userID;
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = userID;
      renderAdminUserDetail();
    });
    const checkWrap = document.createElement("span");
    const check = document.createElement("input");
    check.type = "checkbox";
    check.className = "admin-user-check";
    check.dataset.id = userID;
    check.checked = checked;
    check.addEventListener("change", () => {
      if (check.checked) state.admin.users.selected.add(userID);
      else state.admin.users.selected.delete(userID);
      syncAdminCheckAll();
    });
    checkWrap.appendChild(check);
    row.appendChild(checkWrap);
    const main = document.createElement("span");
    main.className = "setting-list-main";
    const title = document.createElement("span");
    title.className = "setting-list-title";
    title.textContent = String(item.email || "User");
    main.appendChild(title);
    const meta = document.createElement("span");
    meta.className = "setting-list-meta";
    meta.textContent = `${String(item.role || "user").toUpperCase()} • ${String(item.status || "active").toUpperCase()} • ${String(item.provision_state || "ok").toUpperCase()}`;
    main.appendChild(meta);
    row.appendChild(main);
    const view = document.createElement("button");
    view.type = "button";
    view.className = "setting-list-action";
    view.textContent = "View";
    view.addEventListener("click", () => {
      state.admin.users.detailId = userID;
      state.ui.adminNav.page = "detail";
      state.ui.adminNav.detailId = userID;
      renderAdminUserDetail();
    });
    row.appendChild(view);
    el.adminUsers.appendChild(row);
  }
  renderAdminUserDetail();
  if (state.ui.adminNav.page === "detail" && state.admin.users.detailId) {
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.users.detailId;
  } else {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
  }
  syncAdminCheckAll();
}

async function loadAdminAudit() {
  const f = state.admin.audit;
  const query = adminQuery({
    q: f.q,
    action: f.action,
    actor: f.actor,
    target: f.target,
    from: f.from,
    to: f.to,
    sort: f.sort,
    order: f.order,
    page: 1,
    page_size: 100,
  });
  const audit = await api(`/api/v1/admin/audit-log?${query}`);
  const rows = Array.isArray(audit.items) ? audit.items : [];
  state.admin.audit.items = rows;
  if (!rows.some((item) => String(item.id || "") === state.admin.audit.detailId)) {
    state.admin.audit.detailId = "";
    if (state.ui.adminNav.domain === "audit") {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
  }
  el.adminAudit.replaceChildren();
  if (rows.length === 0) {
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "No audit entries match the current filters.";
    el.adminAudit.appendChild(empty);
  }
  for (const item of rows) {
    const auditID = String(item.id || "");
    const row = document.createElement("div");
    row.className = "setting-list-item";
    row.classList.add("setting-list-item--interactive");
    if (auditID === state.admin.audit.detailId) {
      row.classList.add("is-active");
    }
    row.tabIndex = 0;
    row.setAttribute("role", "button");
    row.addEventListener("click", (event) => {
      if (event.target && event.target.closest("button,input,select,textarea,a")) return;
      state.admin.audit.detailId = auditID;
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = state.admin.audit.detailId;
      renderAdminAuditDetail();
    });
    row.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      state.admin.audit.detailId = auditID;
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = state.admin.audit.detailId;
      renderAdminAuditDetail();
    });
    const sev = document.createElement("span");
    sev.className = `status-chip status-chip--${String(item.severity || "info").toLowerCase()}`;
    sev.textContent = String(item.severity || "info").toUpperCase();
    row.appendChild(sev);
    const main = document.createElement("span");
    main.className = "setting-list-main";
    const title = document.createElement("span");
    title.className = "setting-list-title";
    title.textContent = String(item.summary_text || item.action || "Audit event");
    main.appendChild(title);
    const meta = document.createElement("span");
    meta.className = "setting-list-meta";
    meta.textContent = `${formatDate(item.created_at) || "n/a"} • ${String(item.actor_email || "-")} • ${String(item.target_label || item.target || "-")}`;
    main.appendChild(meta);
    row.appendChild(main);
    const view = document.createElement("button");
    view.type = "button";
    view.className = "setting-list-action";
    view.textContent = "View";
    view.addEventListener("click", () => {
      state.admin.audit.detailId = auditID;
      state.ui.adminNav.page = "detail";
      state.ui.adminNav.detailId = state.admin.audit.detailId;
      renderAdminAuditDetail();
    });
    row.appendChild(view);
    el.adminAudit.appendChild(row);
  }
  renderAdminAuditDetail();
  if (state.ui.adminNav.page === "detail" && state.admin.audit.detailId) {
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = state.admin.audit.detailId;
  } else {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
  }
}

function renderAdminRegistrationDetail() {
  if (!el.adminRegsDetail) return;
  el.adminRegsDetail.replaceChildren();
  const inDetailMode = state.ui.adminNav.page === "detail";
  const item = inDetailMode
    ? state.admin.registrations.items.find((it) => String(it.id || "") === state.admin.registrations.detailId)
    : null;
  if (!item) {
    if (inDetailMode) {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
    state.ui.adminNav.page = "list";
    el.adminRegsDetail.classList.add("hidden");
    return;
  }
  state.ui.adminNav.page = "detail";
  state.ui.adminNav.detailId = String(item.id || "");
  el.adminRegsDetail.classList.remove("hidden");
  const title = document.createElement("h4");
  title.textContent = String(item.email || "Registration");
  el.adminRegsDetail.appendChild(title);
  const status = document.createElement("p");
  status.className = "hint";
  status.textContent = `Status: ${String(item.status || "pending")}`;
  el.adminRegsDetail.appendChild(status);
  const created = document.createElement("p");
  created.className = "hint";
  created.textContent = `Created: ${formatDate(item.created_at) || "n/a"}`;
  el.adminRegsDetail.appendChild(created);
  if (item.decided_at) {
    const decided = document.createElement("p");
    decided.className = "hint";
    decided.textContent = `Decided: ${formatDate(item.decided_at)}`;
    el.adminRegsDetail.appendChild(decided);
  }
  if (item.reason) {
    const reason = document.createElement("p");
    reason.className = "hint";
    reason.textContent = `Reason: ${String(item.reason)}`;
    el.adminRegsDetail.appendChild(reason);
  }
  const actions = document.createElement("div");
  actions.className = "settings-detail-actions";
  actions.appendChild(createBackToListButton(() => {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
    renderAdminRegistrationDetail();
  }));
  if (String(item.status || "").toLowerCase() === "pending") {
    const approve = document.createElement("button");
    approve.type = "button";
    approve.className = "cmd-btn cmd-btn--dense cmd-btn--primary";
    approve.textContent = "Approve";
    approve.addEventListener("click", async () => {
      try {
        await api(`/api/v1/admin/registrations/${encodeURIComponent(item.id)}/approve`, { method: "POST", json: {} });
        state.admin.registrations.selected.delete(String(item.id || ""));
        await loadAdminRegistrations();
        setStatus(`Approved ${item.email}.`, "ok");
      } catch (err) {
        presentAPIError(err, "Failed to approve registration");
      }
    });
    actions.appendChild(approve);
    const reject = document.createElement("button");
    reject.type = "button";
    reject.className = "cmd-btn cmd-btn--dense cmd-btn--danger";
    reject.textContent = "Reject";
    reject.addEventListener("click", async () => {
      try {
        const reasonInput = await showPromptModal({
          title: "Reject Registration",
          body: `Provide rejection reason for ${item.email}.`,
          label: "Reason",
          defaultValue: "Rejected by admin",
          confirmText: "Reject",
          cancelText: "Cancel",
          trigger: reject,
        });
        if (!reasonInput) return;
        await api(`/api/v1/admin/registrations/${encodeURIComponent(item.id)}/reject`, {
          method: "POST",
          json: { reason: reasonInput.trim() || "Rejected" },
        });
        state.admin.registrations.selected.delete(String(item.id || ""));
        await loadAdminRegistrations();
        setStatus(`Rejected ${item.email}.`, "ok");
      } catch (err) {
        presentAPIError(err, "Failed to reject registration");
      }
    });
    actions.appendChild(reject);
  }
  el.adminRegsDetail.appendChild(actions);
}

function renderAdminUserDetail() {
  if (!el.adminUsersDetail) return;
  el.adminUsersDetail.replaceChildren();
  const inDetailMode = state.ui.adminNav.page === "detail";
  const item = inDetailMode
    ? state.admin.users.items.find((it) => String(it.id || "") === state.admin.users.detailId)
    : null;
  if (!item) {
    if (inDetailMode) {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
    state.ui.adminNav.page = "list";
    el.adminUsersDetail.classList.add("hidden");
    return;
  }
  state.ui.adminNav.page = "detail";
  state.ui.adminNav.detailId = String(item.id || "");
  el.adminUsersDetail.classList.remove("hidden");
  const title = document.createElement("h4");
  title.textContent = String(item.email || "User");
  el.adminUsersDetail.appendChild(title);
  const role = document.createElement("p");
  role.className = "hint";
  role.textContent = `Role: ${String(item.role || "user")}`;
  el.adminUsersDetail.appendChild(role);
  const status = document.createElement("p");
  status.className = "hint";
  status.textContent = `Status: ${String(item.status || "active")}`;
  el.adminUsersDetail.appendChild(status);
  const provision = document.createElement("p");
  provision.className = "hint";
  provision.textContent = `Provision: ${String(item.provision_state || "ok")}`;
  el.adminUsersDetail.appendChild(provision);
  const actions = document.createElement("div");
  actions.className = "settings-detail-actions";
  actions.appendChild(createBackToListButton(() => {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
    renderAdminUserDetail();
  }));
  const userStatus = String(item.status || "").toLowerCase();
  if (userStatus === "active") {
    const suspend = document.createElement("button");
    suspend.type = "button";
    suspend.className = "cmd-btn cmd-btn--dense cmd-btn--danger";
    suspend.textContent = "Suspend";
    suspend.addEventListener("click", async () => {
      try {
        const confirmed = await showConfirmModal({
          title: "Suspend user?",
          body: `${String(item.email || "This user")} will lose access until unsuspended.`,
          confirmText: "Suspend",
          cancelText: "Cancel",
          trigger: suspend,
        });
        if (!confirmed) return;
        await api(`/api/v1/admin/users/${encodeURIComponent(item.id)}/suspend`, { method: "POST", json: {} });
        state.admin.users.selected.delete(String(item.id || ""));
        await loadAdminUsers();
        setStatus(`Suspended ${item.email}.`, "ok");
      } catch (err) {
        presentAPIError(err, "Failed to suspend user");
      }
    });
    actions.appendChild(suspend);
  } else if (userStatus === "suspended") {
    const unsuspend = document.createElement("button");
    unsuspend.type = "button";
    unsuspend.className = "cmd-btn cmd-btn--dense cmd-btn--primary";
    unsuspend.textContent = "Unsuspend";
    unsuspend.addEventListener("click", async () => {
      try {
        await api(`/api/v1/admin/users/${encodeURIComponent(item.id)}/unsuspend`, { method: "POST", json: {} });
        state.admin.users.selected.delete(String(item.id || ""));
        await loadAdminUsers();
        setStatus(`Unsuspended ${item.email}.`, "ok");
      } catch (err) {
        presentAPIError(err, "Failed to unsuspend user");
      }
    });
    actions.appendChild(unsuspend);
  }
  const reset = document.createElement("button");
  reset.type = "button";
  reset.className = "cmd-btn cmd-btn--dense";
  reset.textContent = "Reset Password";
  reset.addEventListener("click", async () => {
    try {
      const pw = await showPromptModal({
        title: "Reset Password",
        body: `Set a new password for ${String(item.email || "")}.`,
        label: "New password",
        inputType: "password",
        defaultValue: "",
        confirmText: "Apply",
        cancelText: "Cancel",
        trigger: reset,
      });
      if (!pw) return;
      await api(`/api/v1/admin/users/${encodeURIComponent(item.id)}/reset-password`, {
        method: "POST",
        json: { new_password: String(pw).trim() },
      });
      setStatus(`Password reset for ${item.email}.`, "ok");
    } catch (err) {
      presentAPIError(err, "Failed to reset password");
    }
  });
  actions.appendChild(reset);
  if (String(item.provision_state || "").toLowerCase() === "error") {
    const retry = document.createElement("button");
    retry.type = "button";
    retry.className = "cmd-btn cmd-btn--dense";
    retry.textContent = "Retry Provision";
    retry.addEventListener("click", async () => {
      try {
        await api(`/api/v1/admin/users/${encodeURIComponent(item.id)}/retry-provision`, { method: "POST", json: {} });
        await loadAdminUsers();
        setStatus(`Provision retry queued for ${item.email}.`, "ok");
      } catch (err) {
        presentAPIError(err, "Failed to retry provision");
      }
    });
    actions.appendChild(retry);
  }
  el.adminUsersDetail.appendChild(actions);
}

function renderAdminAuditDetail() {
  if (!el.adminAuditDetail) return;
  el.adminAuditDetail.replaceChildren();
  const inDetailMode = state.ui.adminNav.page === "detail";
  const item = inDetailMode
    ? state.admin.audit.items.find((it) => String(it.id || "") === state.admin.audit.detailId)
    : null;
  if (!item) {
    if (inDetailMode) {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
    state.ui.adminNav.page = "list";
    el.adminAuditDetail.classList.add("hidden");
    return;
  }
  state.ui.adminNav.page = "detail";
  state.ui.adminNav.detailId = String(item.id || "");
  el.adminAuditDetail.classList.remove("hidden");
  const title = document.createElement("h4");
  title.textContent = String(item.summary_text || item.action || "Audit event");
  el.adminAuditDetail.appendChild(title);
  const when = document.createElement("p");
  when.className = "hint";
  when.textContent = `When: ${formatDate(item.created_at) || "n/a"}`;
  el.adminAuditDetail.appendChild(when);
  const actor = document.createElement("p");
  actor.className = "hint";
  actor.textContent = `Actor: ${String(item.actor_email || "-")}`;
  el.adminAuditDetail.appendChild(actor);
  const target = document.createElement("p");
  target.className = "hint";
  target.textContent = `Target: ${String(item.target_label || item.target || "-")}`;
  el.adminAuditDetail.appendChild(target);
  const severity = document.createElement("p");
  severity.className = "hint";
  severity.textContent = `Severity: ${String(item.severity || "info")}`;
  el.adminAuditDetail.appendChild(severity);
  const actions = document.createElement("div");
  actions.className = "settings-detail-actions";
  actions.appendChild(createBackToListButton(() => {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
    renderAdminAuditDetail();
  }));
  el.adminAuditDetail.appendChild(actions);
  if (String(item.metadata_json || "").trim()) {
    const tech = document.createElement("details");
    tech.className = "setting-tech";
    tech.innerHTML = "<summary>Technical details</summary>";
    const pre = document.createElement("pre");
    pre.className = "hint";
    pre.textContent = String(item.metadata_json || "");
    tech.appendChild(pre);
    el.adminAuditDetail.appendChild(tech);
  }
}

async function loadAdminFeatureFlags() {
  if (!el.adminFeatureFlags) return;
  try {
    const payload = await api("/api/v1/admin/system/feature-flags", { logErrors: false });
    const rows = Array.isArray(payload.items) ? payload.items : [];
    state.admin.featureFlags.items = rows;
    if (!rows.some((item) => String(item.id || "") === state.admin.featureFlags.detailId)) {
      state.admin.featureFlags.detailId = "";
      if (state.ui.adminNav.domain === "system") {
        state.ui.adminNav.page = "list";
        state.ui.adminNav.detailId = "";
      }
    }
    el.adminFeatureFlags.replaceChildren();
    if (rows.length === 0) {
      const empty = document.createElement("p");
      empty.className = "settings-list-empty";
      empty.textContent = "No feature flags available.";
      el.adminFeatureFlags.appendChild(empty);
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
    }
    for (const item of rows) {
      const row = renderListItem({
        active: String(item.id || "") === state.admin.featureFlags.detailId,
        markerClass: item.enabled ? "status-chip status-chip--ok" : "status-chip status-chip--warning",
        markerText: item.enabled ? "ON" : "OFF",
        title: String(item.name || item.id || "Feature flag"),
        meta: `${String(item.category || "General")} • ${item.editable ? "Editable" : "Read-only"}`,
        onSelect: () => {
          state.admin.featureFlags.detailId = String(item.id || "");
          state.ui.adminNav.page = "list";
          state.ui.adminNav.detailId = state.admin.featureFlags.detailId;
          renderAdminFeatureFlagDetail();
        },
        onAction: () => {
          state.admin.featureFlags.detailId = String(item.id || "");
          state.ui.adminNav.page = "detail";
          state.ui.adminNav.detailId = state.admin.featureFlags.detailId;
          renderAdminFeatureFlagDetail();
        },
      });
      el.adminFeatureFlags.appendChild(row);
    }
    renderAdminFeatureFlagDetail();
  } catch (err) {
    state.admin.featureFlags.items = [];
    state.admin.featureFlags.detailId = "";
    el.adminFeatureFlags.replaceChildren();
    const empty = document.createElement("p");
    empty.className = "settings-list-empty";
    empty.textContent = "Unable to load feature flags.";
    el.adminFeatureFlags.appendChild(empty);
    renderDetailView(el.adminFeatureFlagsDetail, null);
    throw err;
  }
}

function renderAdminFeatureFlagDetail() {
  const inDetailMode = state.ui.adminNav.page === "detail";
  const item = inDetailMode
    ? state.admin.featureFlags.items.find((it) => String(it.id || "") === state.admin.featureFlags.detailId)
    : null;
  if (item) {
    state.ui.adminNav.page = "detail";
    state.ui.adminNav.detailId = String(item.id || "");
  } else {
    state.ui.adminNav.page = "list";
    state.ui.adminNav.detailId = "";
  }
  renderDetailView(el.adminFeatureFlagsDetail, item, (detail, selected) => {
    const title = document.createElement("h4");
    title.textContent = String(selected.name || selected.id || "Feature flag");
    detail.appendChild(title);
    const description = document.createElement("p");
    description.className = "hint";
    description.textContent = String(selected.description || "");
    detail.appendChild(description);
    const stateNote = document.createElement("p");
    stateNote.className = "hint";
    stateNote.textContent = `State: ${selected.enabled ? "Enabled" : "Disabled"} • Source: ${String(selected.source || "default")}`;
    detail.appendChild(stateNote);
    const navActions = document.createElement("div");
    navActions.className = "settings-detail-actions";
    navActions.appendChild(createBackToListButton(() => {
      state.ui.adminNav.page = "list";
      state.ui.adminNav.detailId = "";
      renderAdminFeatureFlagDetail();
    }));
    detail.appendChild(navActions);
    if (selected.note) {
      const note = document.createElement("p");
      note.className = "hint";
      note.textContent = String(selected.note);
      detail.appendChild(note);
    }
    if (selected.editable) {
      const toggle = renderToggleItem({
        label: "Enabled",
        description: selected.enabled
          ? "This feature is currently active."
          : "This feature is currently disabled.",
        enabled: !!selected.enabled,
        disabled: false,
        onToggle: async () => {
          try {
            await api(`/api/v1/admin/system/feature-flags/${encodeURIComponent(String(selected.id || ""))}`, {
              method: "POST",
              json: { enabled: !selected.enabled },
            });
            await loadAdminFeatureFlags();
            await loadAuthCapabilities();
            setStatus(`${selected.name || selected.id} updated.`, "ok");
          } catch (err) {
            presentAPIError(err, "Failed to update feature flag");
          }
        },
      });
      detail.appendChild(toggle);
      const actions = document.createElement("div");
      actions.className = "settings-detail-actions";
      const reset = document.createElement("button");
      reset.type = "button";
      reset.className = "cmd-btn cmd-btn--dense";
      reset.textContent = "Reset To Default";
      reset.addEventListener("click", async () => {
        try {
          await api(`/api/v1/admin/system/feature-flags/${encodeURIComponent(String(selected.id || ""))}/reset`, {
            method: "POST",
            json: {},
          });
          await loadAdminFeatureFlags();
          await loadAuthCapabilities();
          setStatus(`${selected.name || selected.id} reset to default.`, "ok");
        } catch (err) {
          presentAPIError(err, "Failed to reset feature flag");
        }
      });
      actions.appendChild(reset);
      detail.appendChild(actions);
      return;
    }
    const note = document.createElement("p");
    note.className = "hint";
    note.textContent = selected.requires_restart
      ? "This flag is startup-managed and may require service restart after configuration changes."
      : "This flag is managed by server configuration.";
    detail.appendChild(note);
  });
}

async function runBulkRegistrationDecision(decision) {
  const ids = Array.from(state.admin.registrations.selected);
  if (ids.length === 0) {
    setStatus("Select at least one registration.", "error");
    return;
  }
  let reason = "";
  if (decision === "reject") {
    const input = await showPromptModal({
      title: "Reject Selected Registrations",
      body: `Provide rejection reason for ${ids.length} selected item(s).`,
      label: "Reason",
      defaultValue: "Rejected by admin",
      confirmText: "Reject",
      cancelText: "Cancel",
      trigger: el.btnRegReject,
    });
    if (!input) return;
    reason = input.trim() || "Rejected";
  }
  const payload = {
    ids,
    decision,
    reason,
  };
  const out = await api("/api/v1/admin/registrations/bulk/decision", { method: "POST", json: payload });
  state.admin.registrations.selected.clear();
  await loadAdminRegistrations();
  const appliedCount = Array.isArray(out.applied) ? out.applied.length : 0;
  const failed = Array.isArray(out.failed) ? out.failed : [];
  const failedCount = failed.length;
  if (failedCount > 0) {
    const preview = failed
      .slice(0, 2)
      .map((item) => `${item.id || "?"}:${item.code || "action_failed"}`)
      .join(", ");
    setStatus(
      `${decision === "approve" ? "Approved" : "Rejected"} ${appliedCount} registration(s), ${failedCount} failed (${preview})`,
      "error",
    );
    return;
  }
  setStatus(`${decision === "approve" ? "Approved" : "Rejected"} ${appliedCount} registration(s).`, "ok");
}

async function runBulkUserAction(action) {
  const ids = Array.from(state.admin.users.selected);
  if (ids.length === 0) {
    setStatus("Select at least one user.", "error");
    return;
  }
  if (action === "suspend") {
    const confirmed = await showConfirmModal({
      title: "Suspend selected users?",
      body: `${ids.length} selected user(s) will lose access until unsuspended.`,
      confirmText: "Suspend",
      cancelText: "Cancel",
      trigger: el.btnUserSuspend,
    });
    if (!confirmed) return;
  }
  const out = await api("/api/v1/admin/users/bulk/action", {
    method: "POST",
    json: { ids, action },
  });
  state.admin.users.selected.clear();
  await loadAdminUsers();
  const appliedCount = Array.isArray(out.applied) ? out.applied.length : 0;
  const failed = Array.isArray(out.failed) ? out.failed : [];
  const failedCount = failed.length;
  if (failedCount > 0) {
    const preview = failed
      .slice(0, 2)
      .map((item) => `${item.id || "?"}:${item.code || "action_failed"}`)
      .join(", ");
    setStatus(`${action === "suspend" ? "Suspended" : "Unsuspended"} ${appliedCount} user(s), ${failedCount} failed (${preview})`, "error");
    return;
  }
  setStatus(`${action === "suspend" ? "Suspended" : "Unsuspended"} ${appliedCount} user(s).`, "ok");
}

function mailboxesButtons() {
  return Array.from(el.mailboxes.querySelectorAll("button[role='option']"));
}

function messageButtons() {
  return Array.from(el.messages.querySelectorAll(".message-row-btn[role='option']"));
}

async function moveMailboxSelection(delta) {
  const buttons = mailboxesButtons();
  if (buttons.length === 0) return;
  const index = Math.max(0, buttons.findIndex((node) => String(node.dataset.mailboxName || "") === state.mailbox));
  const next = Math.max(0, Math.min(buttons.length - 1, index + delta));
  const nextBtn = buttons[next];
  if (nextBtn) {
    if (el.mailboxes && typeof el.mailboxes.focus === "function") {
      el.mailboxes.focus({ preventScroll: true });
    }
    nextBtn.click();
    syncMailboxActiveDescendant();
  }
}

async function moveMessageSelection(delta) {
  const buttons = messageButtons();
  if (buttons.length === 0) return;
  const currentID = String(state.selectedMessage?.id || "");
  let index = buttons.findIndex((node) => String(node.dataset.messageId || "") === currentID);
  if (index < 0) index = 0;
  const next = Math.max(0, Math.min(buttons.length - 1, index + delta));
  const nextBtn = buttons[next];
  if (nextBtn) {
    if (el.messages && typeof el.messages.focus === "function") {
      el.messages.focus({ preventScroll: true });
    }
    nextBtn.click();
    syncMessageActiveDescendant();
  }
}

async function handleMailKeyboard(event) {
  if (el.viewMail.classList.contains("hidden")) return;
  if (state.ui.composeOpen || state.ui.modalOpen) return;
  const target = event.target;
  const isEditable = target && (
    target.tagName === "INPUT"
    || target.tagName === "TEXTAREA"
    || target.tagName === "SELECT"
    || target.isContentEditable
  );
  if (isEditable && event.key !== "Escape") return;
  const k = event.key.toLowerCase();

  if (event.key === "Tab") {
    event.preventDefault();
    cycleMailPane(event.shiftKey ? -1 : 1);
    return;
  }

  if (event.key === "/" && !event.shiftKey) {
    event.preventDefault();
    el.searchInput.focus();
    return;
  }

  if (k === "c") {
    event.preventDefault();
    void openComposeOverlay(el.btnComposeOpen);
    return;
  }

  if (k === "r") {
    event.preventDefault();
    if (state.selectedMessage && el.btnReply) el.btnReply.click();
    return;
  }

  if (event.shiftKey && k === "f") {
    event.preventDefault();
    if (state.selectedMessage && el.btnForward) el.btnForward.click();
    return;
  }

  if (k === "f") {
    event.preventDefault();
    if (state.selectedMessage) el.btnFlag.click();
    return;
  }

  if (k === "s") {
    event.preventDefault();
    if (state.selectedMessage) el.btnSeen.click();
    return;
  }

  if (event.key === "Delete") {
    event.preventDefault();
    if (state.selectedMessage) el.btnTrash.click();
    return;
  }

  if (event.key === "Enter") {
    if (state.ui.activeKeyboardPane === "mailboxes") {
      event.preventDefault();
      const activeID = el.mailboxes?.getAttribute("aria-activedescendant") || "";
      const active = activeID ? document.getElementById(activeID) : null;
      const current = active || mailboxesButtons().find((node) => String(node.dataset.mailboxName || "") === state.mailbox);
      if (current) current.click();
      return;
    }
    if (state.ui.activeKeyboardPane === "messages") {
      event.preventDefault();
      const activeID = el.messages?.getAttribute("aria-activedescendant") || "";
      const active = activeID ? document.getElementById(activeID) : null;
      const current = active || messageButtons().find((node) => String(node.dataset.messageId || "") === String(state.selectedMessage?.id || ""));
      if (current) current.click();
    }
  }

  if (event.key === "Escape") {
    if (isMobileLayout() && state.ui.activeMailPane === "reader") {
      event.preventDefault();
      setActiveMailPane("messages");
      return;
    }
    if (isMobileLayout() && state.ui.activeMailPane === "messages") {
      event.preventDefault();
      setActiveMailPane("mailboxes");
    }
    return;
  }

  if (state.ui.activeKeyboardPane === "mailboxes" && (k === "j" || event.key === "ArrowDown")) {
    event.preventDefault();
    await moveMailboxSelection(1);
    return;
  }
  if (state.ui.activeKeyboardPane === "mailboxes" && (k === "k" || event.key === "ArrowUp")) {
    event.preventDefault();
    await moveMailboxSelection(-1);
    return;
  }
  if (state.ui.activeKeyboardPane === "messages" && (k === "j" || event.key === "ArrowDown")) {
    event.preventDefault();
    await moveMessageSelection(1);
    return;
  }
  if (state.ui.activeKeyboardPane === "messages" && (k === "k" || event.key === "ArrowUp")) {
    event.preventDefault();
    await moveMessageSelection(-1);
    return;
  }
  if (state.ui.activeKeyboardPane === "reader" && (k === "j" || event.key === "ArrowDown")) {
    event.preventDefault();
    await openThreadNeighbor(1);
    return;
  }
  if (state.ui.activeKeyboardPane === "reader" && (k === "k" || event.key === "ArrowUp")) {
    event.preventDefault();
    await openThreadNeighbor(-1);
    return;
  }
  if (state.ui.activeKeyboardPane === "reader" && event.key === "Home") {
    event.preventDefault();
    await openThreadBoundary("start");
    return;
  }
  if (state.ui.activeKeyboardPane === "reader" && event.key === "End") {
    event.preventDefault();
    await openThreadBoundary("end");
    return;
  }
}

function formatDate(value) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return d.toLocaleString();
}

function formatListDate(value) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return String(value);
  const now = new Date();
  const dayStart = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const nowStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const deltaDays = Math.round((nowStart.getTime() - dayStart.getTime()) / 86400000);
  if (deltaDays === 0) {
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }
  if (deltaDays === 1) {
    return "Yesterday";
  }
  if (deltaDays > 1 && deltaDays < 7) {
    return d.toLocaleDateString([], { weekday: "short" });
  }
  if (d.getFullYear() === now.getFullYear()) {
    return d.toLocaleDateString([], { month: "short", day: "numeric" });
  }
  return d.toLocaleDateString([], { month: "short", day: "numeric", year: "numeric" });
}

function escapeHtml(s) {
  return String(s || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function bindSetupUI() {
  el.setupBackIcon.onclick = () => OOBEController.back();
  el.setupClose.onclick = async () => {
    if (state.setup.step === setupCompleteStep && !state.setup.required) {
      await OOBEController.openMail();
      return;
    }
    OOBEController.openConfirm("cancel");
  };

  el.setupForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (state.setup.step >= setupCompleteStep) return;
    try {
      await OOBEController.next();
      setSetupInlineStatus("");
    } catch (err) {
      if (err.code === "setup_already_complete") {
        try {
          const status = await loadSetupStatus();
          if (!status.required) {
            state.setup.required = false;
            applyNavVisibility();
            setActiveTab(el.tabAuth);
            showView("auth");
            setActiveAuthTask("login");
            setStatus("SETUP ALREADY COMPLETED. SIGN IN WITH ADMIN ACCOUNT.", "info");
            setSetupInlineStatus("");
            return;
          }
        } catch {
          // fallback to normal error rendering below
        }
      }
      if (err.code === "pam_credentials_invalid") {
        if (!/attempted logins:/i.test(String(err.message || ""))) {
          err.message = "PAM mode is enabled. The password or mailbox login identity is invalid. Try the optional Mailbox Login field if IMAP login differs from email.";
        }
      } else if (err.code === "pam_verifier_unavailable") {
        err.message = "Cannot validate PAM credentials right now because IMAP connectivity failed. Check IMAP host/port/TLS and try again.";
      } else if (err.code === "recovery_email_required") {
        err.message = "A recovery email is required before setup can finish.";
      } else if (err.code === "recovery_email_matches_login") {
        err.message = "Recovery email must be different from the admin email.";
      } else if (err.code === "invalid_recovery_email") {
        err.message = "Enter a valid recovery email address.";
      } else if (isSessionErrorCode(err.code)) {
        err.message = "Setup was accepted, but browser session cookie was not established. Check HTTP/HTTPS cookie policy and then sign in from Login.";
      }
      const requestRef = err.requestID ? ` (request ${err.requestID})` : "";
      const detail = err.code && err.code !== "request_failed" ? `${err.message} [${err.code}]${requestRef}` : `${err.message}${requestRef}`;
      setStatus(detail, "error");
      setSetupInlineStatus(detail, "error");
    }
  });

  el.setupOpenMail.onclick = async () => {
    try {
      await OOBEController.openMail();
    } catch (err) {
      setStatus(err.message, "error");
    }
  };

  el.setupOpenAdmin.onclick = async () => {
    try {
      await OOBEController.openAdmin();
    } catch (err) {
      setStatus(err.message, "error");
    }
  };

  el.setupModalCancel.onclick = () => OOBEController.closeConfirm();
  el.setupModalConfirm.onclick = async () => {
    try {
      await OOBEController.confirm();
    } catch (err) {
      setStatus(err.message, "error");
    }
  };

  el.setupModalOverlay.addEventListener("click", (event) => {
    if (event.target === el.setupModalOverlay) {
      OOBEController.closeConfirm();
    }
  });

  el.setupDomain.addEventListener("input", () => {
    const domain = normalizeDomain(el.setupDomain.value);
    const autoEmail = domainToDefaultEmail(domain);
    if (!state.setup.adminEmailTouched || String(el.setupAdminEmail.value).trim().toLowerCase() === state.setup.lastAutoAdminEmail) {
      el.setupAdminEmail.value = autoEmail;
      state.setup.lastAutoAdminEmail = autoEmail;
    }
    OOBEController.updateSummary();
    OOBEController.refreshNavState();
  });

  el.setupAdminEmail.addEventListener("input", () => {
    const email = String(el.setupAdminEmail.value || "").trim().toLowerCase();
    state.setup.adminEmailTouched = email !== state.setup.lastAutoAdminEmail;
    OOBEController.updateSummary();
    OOBEController.refreshNavState();
  });
  if (el.setupAdminRecoveryEmail) {
    el.setupAdminRecoveryEmail.addEventListener("input", () => {
      OOBEController.updateSummary();
      OOBEController.refreshNavState();
    });
  }

  el.setupRegion.addEventListener("change", () => {
    OOBEController.updateSummary();
    OOBEController.refreshNavState();
  });
  if (el.setupThemeMachine) {
    el.setupThemeMachine.addEventListener("click", () => OOBEController.setThemeChoice("machine-dark"));
  }
  if (el.setupThemePaper) {
    el.setupThemePaper.addEventListener("click", () => OOBEController.setThemeChoice("paper-light"));
  }
  if (el.setupUpdatesAuto) {
    el.setupUpdatesAuto.addEventListener("click", () => OOBEController.setAutomaticUpdatesChoice(true));
  }
  if (el.setupUpdatesManual) {
    el.setupUpdatesManual.addEventListener("click", () => OOBEController.setAutomaticUpdatesChoice(false));
  }
  if (el.setupPasskeyPrimaryEnabled) {
    el.setupPasskeyPrimaryEnabled.addEventListener("change", () => {
      OOBEController.updateSummary();
      OOBEController.refreshNavState();
    });
  }
  el.setupPassword.addEventListener("input", () => OOBEController.refreshNavState());
  el.setupPasswordConfirm.addEventListener("input", () => OOBEController.refreshNavState());

  document.addEventListener("keydown", async (event) => {
    if (!state.setup.required || el.viewSetup.classList.contains("hidden")) return;
    if (!el.setupModalOverlay.classList.contains("hidden")) {
      if (event.key === "Tab") {
        event.preventDefault();
        const focusables = [el.setupModalCancel, el.setupModalConfirm];
        const currentIndex = focusables.findIndex((node) => node === document.activeElement);
        const nextIndex = (currentIndex + (event.shiftKey ? -1 : 1) + focusables.length) % focusables.length;
        focusables[nextIndex].focus();
      }
      if (event.key === "Escape") {
        event.preventDefault();
        OOBEController.closeConfirm();
      }
      if (event.key === "Enter") {
        event.preventDefault();
        await OOBEController.confirm();
      }
      return;
    }

    if (event.key === "Escape" && state.setup.step >= 1 && state.setup.step < setupCompleteStep) {
      event.preventDefault();
      OOBEController.openConfirm("cancel");
    }
  });
}

function bindUI() {
  bindSetupUI();
  setActiveAuthTask("login");
  setActiveSettingsSection(state.ui.activeSettingsSection || "signin");
  setActiveAdminSection(state.ui.activeAdminSection || "system");
  setActiveMailPane(state.ui.activeMailPane || "mailboxes", { focus: false });
  if (el.authModeLogin) {
    el.authModeLogin.onclick = () => {
      setActiveAuthTask("login");
      void loadAuthCapabilities();
    };
  }
  if (el.authModeRegister) {
    el.authModeRegister.onclick = () => {
      setActiveAuthTask("register");
      void initCaptchaUI();
    };
  }
  if (el.authModeReset) {
    el.authModeReset.onclick = () => {
      setActiveAuthTask("reset");
      void loadResetCapabilities();
    };
  }
  if (el.captchaManualInput) {
    el.captchaManualInput.addEventListener("input", () => {
      setCaptchaToken(el.captchaManualInput.value);
      if (state.captcha.token) {
        setCaptchaError("");
        setCaptchaNote("Captcha token captured.", "ok");
      } else {
        setCaptchaNote("Captcha is enabled. Provide token from configured provider.", "info");
      }
    });
  }
  if (el.btnTheme) {
    el.btnTheme.onclick = () => {
      const next = ThemeController.getTheme() === "paper-light" ? "machine-dark" : "paper-light";
      ThemeController.setTheme(next);
      if (state.setup.required) {
        showView("setup");
      } else if (!state.user) {
        showView("auth");
        setActiveAuthTask(state.ui.activeAuthTask || "login");
      } else if (!el.viewAdmin.classList.contains("hidden")) {
        showView("admin");
      } else if (!el.viewSettings.classList.contains("hidden")) {
        showView("settings");
      } else {
        showView("mail");
      }
    };
  }

  const loadCurrentSettingsSection = async () => {
    if (el.viewSettings.classList.contains("hidden")) return;
    try {
      await loadActiveSettingsSection();
    } catch (err) {
      presentAPIError(err, "Failed to load settings data");
    }
  };

  const loadCurrentAdminSection = async () => {
    if (el.viewAdmin.classList.contains("hidden")) return;
    try {
      await loadActiveAdminSection();
    } catch (err) {
      presentAPIError(err, "Failed to load admin data");
    }
  };

  const runSettingsSearch = () => {
    state.settings.searchQuery = String(el.settingsSearchInput?.value || "").trim();
    const results = buildJumpResults(settingsSearchEntries(), state.settings.searchQuery);
    renderJumpResults(el.settingsSearchResults, results, async (entry) => {
      await navigateSettingsTarget(entry.target || {});
    });
  };

  const runAdminSearch = () => {
    const query = String(el.adminSearchInput?.value || "").trim();
    const results = buildJumpResults(adminSearchEntries(), query);
    renderJumpResults(el.adminSearchResults, results, async (entry) => {
      await navigateAdminTarget(entry.target || {});
    });
  };

  if (el.settingsSearchInput) {
    el.settingsSearchInput.addEventListener("input", runSettingsSearch);
    el.settingsSearchInput.addEventListener("focus", runSettingsSearch);
  }
  if (el.adminSearchInput) {
    el.adminSearchInput.addEventListener("input", runAdminSearch);
    el.adminSearchInput.addEventListener("focus", runAdminSearch);
  }

  if (el.settingsNavSignIn) {
    el.settingsNavSignIn.onclick = async () => {
      setActiveSettingsSection("signin");
      await loadCurrentSettingsSection();
    };
  }
  if (el.settingsNavDevices) {
    el.settingsNavDevices.onclick = async () => {
      setActiveSettingsSection("devices");
      await loadCurrentSettingsSection();
    };
  }
  if (el.settingsNavSessions) {
    el.settingsNavSessions.onclick = async () => {
      setActiveSettingsSection("sessions");
      await loadCurrentSettingsSection();
    };
  }

  if (el.adminNavSystem) {
    el.adminNavSystem.onclick = async () => {
      setActiveAdminSection("system");
      await loadCurrentAdminSection();
    };
  }
  if (el.adminNavRegistrations) {
    el.adminNavRegistrations.onclick = async () => {
      setActiveAdminSection("registrations");
      await loadCurrentAdminSection();
    };
  }
  if (el.adminNavUsers) {
    el.adminNavUsers.onclick = async () => {
      setActiveAdminSection("users");
      await loadCurrentAdminSection();
    };
  }
  if (el.adminNavAudit) {
    el.adminNavAudit.onclick = async () => {
      setActiveAdminSection("audit");
      await loadCurrentAdminSection();
    };
  }

  if (el.btnAdminRegApply) {
    el.btnAdminRegApply.onclick = async () => {
      state.admin.registrations.q = String(el.adminRegQ?.value || "").trim();
      state.admin.registrations.status = String(el.adminRegStatus?.value || "pending").trim();
      state.admin.registrations.sort = String(el.adminRegSort?.value || "created_at").trim();
      state.admin.registrations.order = String(el.adminRegOrder?.value || "desc").trim();
      await loadCurrentAdminSection();
    };
  }
  if (el.btnAdminUserApply) {
    el.btnAdminUserApply.onclick = async () => {
      state.admin.users.q = String(el.adminUserQ?.value || "").trim();
      state.admin.users.status = String(el.adminUserStatus?.value || "all").trim();
      state.admin.users.role = String(el.adminUserRole?.value || "all").trim();
      state.admin.users.provision = String(el.adminUserProvision?.value || "all").trim();
      state.admin.users.sort = String(el.adminUserSort?.value || "created_at").trim();
      state.admin.users.order = String(el.adminUserOrder?.value || "desc").trim();
      await loadCurrentAdminSection();
    };
  }
  if (el.btnAdminAuditApply) {
    el.btnAdminAuditApply.onclick = async () => {
      state.admin.audit.q = String(el.adminAuditQ?.value || "").trim();
      state.admin.audit.action = String(el.adminAuditAction?.value || "all").trim();
      state.admin.audit.actor = String(el.adminAuditActor?.value || "").trim();
      state.admin.audit.target = String(el.adminAuditTarget?.value || "").trim();
      state.admin.audit.from = String(el.adminAuditFrom?.value || "").trim();
      state.admin.audit.to = String(el.adminAuditTo?.value || "").trim();
      state.admin.audit.sort = String(el.adminAuditSort?.value || "created_at").trim();
      state.admin.audit.order = String(el.adminAuditOrder?.value || "desc").trim();
      await loadCurrentAdminSection();
    };
  }

  if (el.btnRegSelectAll) {
    el.btnRegSelectAll.onclick = () => {
      Array.from(document.querySelectorAll(".admin-reg-check")).forEach((node) => {
        node.checked = true;
        const regID = String(node.dataset.id || "");
        if (regID) state.admin.registrations.selected.add(regID);
      });
      syncAdminCheckAll();
    };
  }
  if (el.btnRegClear) {
    el.btnRegClear.onclick = () => {
      state.admin.registrations.selected.clear();
      Array.from(document.querySelectorAll(".admin-reg-check")).forEach((node) => {
        node.checked = false;
      });
      syncAdminCheckAll();
    };
  }
  if (el.btnRegApprove) {
    el.btnRegApprove.onclick = async () => {
      try {
        await runBulkRegistrationDecision("approve");
      } catch (err) {
        presentAPIError(err, "Failed to approve selected registrations");
      }
    };
  }
  if (el.btnRegReject) {
    el.btnRegReject.onclick = async () => {
      try {
        await runBulkRegistrationDecision("reject");
      } catch (err) {
        presentAPIError(err, "Failed to reject selected registrations");
      }
    };
  }
  if (el.adminRegCheckAll) {
    el.adminRegCheckAll.addEventListener("change", () => {
      const checked = el.adminRegCheckAll.checked;
      Array.from(document.querySelectorAll(".admin-reg-check")).forEach((node) => {
        node.checked = checked;
        const id = String(node.dataset.id || "");
        if (!id) return;
        if (checked) state.admin.registrations.selected.add(id);
        else state.admin.registrations.selected.delete(id);
      });
      syncAdminCheckAll();
    });
  }

  if (el.btnUserSelectAll) {
    el.btnUserSelectAll.onclick = () => {
      Array.from(document.querySelectorAll(".admin-user-check")).forEach((node) => {
        node.checked = true;
        const id = String(node.dataset.id || "");
        if (id) state.admin.users.selected.add(id);
      });
      syncAdminCheckAll();
    };
  }
  if (el.btnUserClear) {
    el.btnUserClear.onclick = () => {
      state.admin.users.selected.clear();
      Array.from(document.querySelectorAll(".admin-user-check")).forEach((node) => {
        node.checked = false;
      });
      syncAdminCheckAll();
    };
  }
  if (el.btnUserSuspend) {
    el.btnUserSuspend.onclick = async () => {
      try {
        await runBulkUserAction("suspend");
      } catch (err) {
        presentAPIError(err, "Failed to suspend selected users");
      }
    };
  }
  if (el.btnUserUnsuspend) {
    el.btnUserUnsuspend.onclick = async () => {
      try {
        await runBulkUserAction("unsuspend");
      } catch (err) {
        presentAPIError(err, "Failed to unsuspend selected users");
      }
    };
  }
  if (el.adminUserCheckAll) {
    el.adminUserCheckAll.addEventListener("change", () => {
      const checked = el.adminUserCheckAll.checked;
      Array.from(document.querySelectorAll(".admin-user-check")).forEach((node) => {
        node.checked = checked;
        const id = String(node.dataset.id || "");
        if (!id) return;
        if (checked) state.admin.users.selected.add(id);
        else state.admin.users.selected.delete(id);
      });
      syncAdminCheckAll();
    });
  }

  if (el.mailMobileBack) {
    el.mailMobileBack.onclick = () => {
      if (state.ui.activeMailPane === "reader") setActiveMailPane("messages");
      else if (state.ui.activeMailPane === "messages") setActiveMailPane("mailboxes");
    };
  }
  if (el.mailBackToMailboxes) {
    el.mailBackToMailboxes.onclick = () => setActiveMailPane("mailboxes");
  }
  if (el.mailBackToMessages) {
    el.mailBackToMessages.onclick = () => setActiveMailPane("messages");
  }
  [el.mailPaneMailboxes, el.mailPaneMessages, el.mailPaneReader].forEach((pane) => {
    if (!pane) return;
    pane.addEventListener("focus", () => {
      if (pane === el.mailPaneMailboxes) focusMailPane("mailboxes");
      if (pane === el.mailPaneMessages) focusMailPane("messages");
      if (pane === el.mailPaneReader) focusMailPane("reader");
    });
    pane.addEventListener("click", () => {
      if (pane === el.mailPaneMailboxes) focusMailPane("mailboxes");
      if (pane === el.mailPaneMessages) focusMailPane("messages");
      if (pane === el.mailPaneReader) focusMailPane("reader");
    });
  });
  [el.mailboxes, el.messages].forEach((list) => {
    if (!list) return;
    list.addEventListener("focus", () => {
      if (list === el.mailboxes) focusMailPane("mailboxes");
      if (list === el.messages) focusMailPane("messages");
    });
    list.addEventListener("click", () => {
      if (list === el.mailboxes) focusMailPane("mailboxes");
      if (list === el.messages) focusMailPane("messages");
    });
  });
  window.addEventListener("resize", () => {
    setActiveMailPane(state.ui.activeMailPane, { focus: false });
    renderThreadContext();
  });
  document.addEventListener("keydown", (event) => {
    void handleMailKeyboard(event);
  });
  document.addEventListener("click", (event) => {
    if (event.target && event.target.closest && event.target.closest(".row-menu")) return;
    const target = event.target;
    if (el.settingsSearchResults && el.settingsSearchInput) {
      const inSettingsSearch = !!(target && target.closest && target.closest("#settings-search-input"));
      const inSettingsResults = !!(target && target.closest && target.closest("#settings-search-results"));
      if (!inSettingsSearch && !inSettingsResults) {
        el.settingsSearchResults.classList.add("hidden");
      }
    }
    if (el.adminSearchResults && el.adminSearchInput) {
      const inAdminSearch = !!(target && target.closest && target.closest("#admin-search-input"));
      const inAdminResults = !!(target && target.closest && target.closest("#admin-search-results"));
      if (!inAdminSearch && !inAdminResults) {
        el.adminSearchResults.classList.add("hidden");
      }
    }
    closeOpenRowMenus(null);
  });

  if (el.loginForm) {
    el.loginForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      const fd = new FormData(e.target);
      try {
        const loginPayload = await api("/api/v1/login", {
          method: "POST",
          json: {
            email: fd.get("email"),
            password: fd.get("password"),
          },
        });
        await finalizePrimaryLogin(loginPayload);
      } catch (err) {
        if (err.code === "setup_required") {
          await enterSetupIfRequired();
          return;
        }
        if (err.code === "pam_verifier_unavailable") {
          const requestRef = err.requestID ? ` (request ${err.requestID})` : "";
          setStatus(`PAM/IMAP auth backend is unreachable. Check IMAP connectivity or switch local dev to SQL auth mode.${requestRef}`, "error");
          return;
        }
        setStatus(err.message, "error");
      }
    });
  }
  if (el.btnPasskeyLogin) {
    el.btnPasskeyLogin.addEventListener("click", async () => {
      try {
        const loginPayload = await runPasskeyPrimaryLoginFlow();
        await finalizePrimaryLogin(loginPayload);
      } catch (err) {
        if (err.code === "setup_required") {
          await enterSetupIfRequired();
          return;
        }
        setStatus(formatPasskeyPrimaryLoginError(err), "error");
      }
    });
  }

  document.getElementById("form-register").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const loginEmail = String(fd.get("email") || "").trim().toLowerCase();
    const recoveryEmail = String(fd.get("recovery_email") || "").trim().toLowerCase();
    if (!validEmail(recoveryEmail)) {
      setStatus("Provide a valid recovery email.", "error");
      return;
    }
    if (loginEmail !== "" && loginEmail === recoveryEmail) {
      setStatus("Recovery email must be different from login email.", "error");
      return;
    }
    const captchaToken = String(fd.get("captcha_token") || "").trim();
    const captchaEnabled = !!state.captcha.config?.enabled;
    if (captchaEnabled && !captchaToken) {
      setCaptchaError("Complete captcha challenge before submitting registration.");
      setCaptchaNote("Captcha challenge not solved yet.", "error");
      setStatus("Complete captcha challenge before submitting registration.", "error");
      syncRegisterSubmitState();
      return;
    }
    try {
      await api("/api/v1/register", {
        method: "POST",
        json: {
          email: loginEmail,
          recovery_email: recoveryEmail,
          password: fd.get("password"),
          captcha_token: captchaToken,
          mfa_preference: fd.get("mfa_preference") || "none",
        },
      });
      setStatus("Registration submitted. Wait for approval.", "ok");
      e.target.reset();
      await resetCaptchaChallenge();
    } catch (err) {
      if (err.code === "setup_required") {
        await enterSetupIfRequired();
        return;
      }
      if (err.code === "captcha_required") {
        setCaptchaError("Captcha validation failed. Please solve a fresh challenge and retry.");
        setCaptchaNote("Captcha challenge failed verification.", "error");
        setStatus("Captcha validation failed. Solve challenge again and retry.", "error");
        await resetCaptchaChallenge();
        return;
      }
      if (err.code === "captcha_unavailable") {
        const requestRef = err.requestID ? ` (request ${err.requestID})` : "";
        setCaptchaError("Captcha verification service is currently unavailable.");
        setCaptchaNote(`Verification service unavailable${requestRef}.`, "error");
        setStatus(`Captcha verification service is unavailable. Please retry shortly.${requestRef}`, "error");
        await resetCaptchaChallenge();
        return;
      }
      setStatus(err.message, "error");
      if (captchaEnabled) {
        await resetCaptchaChallenge();
      }
    }
  });

  if (el.registerMFAPreference) {
    el.registerMFAPreference.addEventListener("change", updateRegisterMFAHelp);
  }
  updateRegisterMFAHelp();

  if (el.btnPasskeysRefresh) {
    el.btnPasskeysRefresh.addEventListener("click", async () => {
      await loadPasskeyCredentials();
      setStatus("Passkeys refreshed.", "ok");
    });
  }
  if (el.btnPasskeysAdd) {
    el.btnPasskeysAdd.addEventListener("click", async () => {
      if (!state.user) return;
      if (!supportsWebAuthn()) {
        setStatus("Passkey enrollment is not supported in this browser.", "error");
        return;
      }
      try {
        await runWebAuthnSetupFlow();
        await refreshSession({
          throwOnFail: true,
          skipUnauthorizedHandling: true,
          skipMFAHandling: true,
        });
        await loadPasskeyCredentials();
        setStatus("Passkey enrolled.", "ok");
      } catch (err) {
        setStatus(formatAPIError(err, "Failed to enroll passkey."), "error");
      }
    });
  }
  if (el.btnTrustedDevicesRefresh) {
    el.btnTrustedDevicesRefresh.addEventListener("click", async () => {
      await loadTrustedDevices();
      setStatus("Trusted devices refreshed.", "ok");
    });
  }
  if (el.btnTrustedDevicesRevokeAll) {
    el.btnTrustedDevicesRevokeAll.addEventListener("click", async () => {
      if (!state.user) return;
      const confirmed = await showConfirmModal({
        title: "Revoke all trusted devices?",
        body: "All trusted devices will require MFA again on the next login.",
        confirmText: "Revoke All",
        cancelText: "Cancel",
      });
      if (!confirmed) return;
      try {
        await api("/api/v2/security/mfa/trusted-devices/revoke-all", {
          method: "POST",
          json: {},
        });
        setStatus("All trusted devices were revoked.", "ok");
        await loadTrustedDevices();
      } catch (err) {
        setStatus(formatAPIError(err, "Failed to revoke trusted devices."), "error");
      }
    });
  }
  if (el.btnSessionsRefresh) {
    el.btnSessionsRefresh.addEventListener("click", async () => {
      await loadSessions();
      setStatus("Sessions refreshed.", "ok");
    });
  }

  document.getElementById("form-reset-request").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      await api("/api/v1/password/reset/request", { method: "POST", json: { email: fd.get("email") } });
      setStatus("If the account or recovery email exists, reset instructions were sent.", "ok");
    } catch (err) {
      if (err.code === "setup_required") {
        await enterSetupIfRequired();
        return;
      }
      setStatus(err.message, "error");
    }
  });

  document.getElementById("form-reset-confirm").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      await api("/api/v1/password/reset/confirm", { method: "POST", json: { token: fd.get("token"), new_password: fd.get("new_password") } });
      setStatus("Password updated.", "ok");
      e.target.reset();
    } catch (err) {
      if (err.code === "setup_required") {
        await enterSetupIfRequired();
        return;
      }
      setStatus(err.message, "error");
    }
  });

  if (el.composeForm) {
    el.composeForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (state.compose.submitInFlight) return;
      commitComposeAllRecipientInputs();
      cleanupComposeInlineReferences();
      syncComposeDraftFields();
      if (!composeCanSubmit()) {
        if (composeHasInvalidRecipients()) {
          setStatus("Fix invalid recipient addresses before sending.", "error");
        } else {
          setStatus("Compose is incomplete. Fill To, Subject, and message body before sending.", "error");
        }
        return;
      }
      state.compose.submitInFlight = true;
      updateComposeSubmitState();
      try {
        const sendMode = String(state.compose.sendContext?.mode || "send").toLowerCase();
        const result = await sendCompose(e.target);
        let sentMessage = "Message sent.";
        if (sendMode === "reply") sentMessage = "Reply sent.";
        else if (sendMode === "forward") sentMessage = "Forward sent.";
        if (String(result?.warning || "").trim()) {
          setStatus(`${sentMessage} ${String(result.warning).trim()}`, "info");
        } else {
          setStatus(sentMessage, "ok");
        }
        const sentDraftID = String(state.compose.draftID || "").trim();
        e.target.reset();
        state.compose.recipients.to = [];
        state.compose.recipients.cc = [];
        state.compose.recipients.bcc = [];
        renderComposeRecipientTokens("to");
        renderComposeRecipientTokens("cc");
        renderComposeRecipientTokens("bcc");
        if (el.composeEditor) el.composeEditor.innerHTML = "";
        if (el.composeFromManualInput) el.composeFromManualInput.value = "";
        setComposeCcVisible(false);
        setComposeBccVisible(false);
        setComposeFormatToolsVisible(false);
        setComposeFromNote("");
        setComposeDraftNote("");
        setComposeDraftState("Draft", "muted");
        clearComposeAssets();
        if (sentDraftID) {
          clearComposeCrashBuffer(sentDraftID);
          removeLocalDraft(sentDraftID);
        } else {
          clearComposeCrashBuffer("");
        }
        resetComposeDraftSession();
        closeComposeOverlay({ restoreFocus: true, persistDraft: false });
        await refreshMailView({ preservePane: true });
      } catch (err) {
        if (err.code === "smtp_sender_rejected") {
          const requestRef = err.requestID ? ` (request ${err.requestID})` : "";
          setComposeDraftNote("SMTP sender policy rejected this message.", "error");
          setStatus(`SMTP sender policy rejected this message. On Ubuntu, check Postfix sender-login policy and users.mail_login mapping.${requestRef}`, "error");
          return;
        }
        if (err.code === "invalid_sender_manual") {
          setComposeDraftNote("Manual sender must exactly match your authenticated account email.", "error");
          setStatus("Manual sender must exactly match your authenticated account email.", "error");
          return;
        }
        if (composeShouldPersistSendFailure(err)) {
          await markComposeDraftSendFailed(err);
          setStatus(formatAPIError(err, "Send failed."), "error");
          return;
        }
        setComposeDraftNote(formatAPIError(err, "Send failed."), "error");
        setStatus(err.message, "error");
      } finally {
        state.compose.submitInFlight = false;
        updateComposeSubmitState();
      }
    });
  }

  if (el.btnUpdateCheck) {
    el.btnUpdateCheck.onclick = async () => {
      if (!state.user || state.user.role !== "admin") return;
      state.update.checking = true;
      applyUpdateControls();
      try {
        await loadUpdateStatus(true);
        setStatus("Update check complete.", "ok");
      } catch (err) {
        setUpdateNote(`Update check failed: ${err.message}`, "error");
        setStatus(err.message, "error");
      } finally {
        state.update.checking = false;
        applyUpdateControls();
      }
    };
  }

  if (el.btnUpdateApply) {
    el.btnUpdateApply.onclick = async () => {
      if (!state.user || state.user.role !== "admin") return;
      state.update.applying = true;
      applyUpdateControls();
      try {
        const targetVersion = state.update.lastStatus?.latest?.tag_name || "";
        await api("/api/v1/admin/system/update/apply", {
          method: "POST",
          json: targetVersion ? { target_version: targetVersion } : {},
        });
        setStatus(`UPDATE QUEUED${targetVersion ? ` (${targetVersion})` : ""}`, "info");
        await loadUpdateStatus(false);
        startUpdatePolling();
      } catch (err) {
        if (err.code === "updater_not_configured") {
          setUpdateNote(updateConfigDiagnosticMessage(state.update.lastStatus), "error");
        } else if (err.code === "update_in_progress") {
          setUpdateNote("An update is already running. Waiting for completion.", "info");
          startUpdatePolling();
        } else {
          setUpdateNote(`Update request failed: ${err.message}`, "error");
        }
        setStatus(err.message, "error");
      } finally {
        state.update.applying = false;
        applyUpdateControls();
      }
    };
  }

  if (el.btnUpdateAuto) {
    el.btnUpdateAuto.onclick = async () => {
      if (!state.user || state.user.role !== "admin") return;
      const enabled = state.update.lastStatus?.auto_update?.enabled !== false;
      state.update.autoSaving = true;
      applyUpdateControls();
      try {
        await api("/api/v1/admin/system/update/automatic", {
          method: "POST",
          json: { enabled: !enabled },
        });
        await loadUpdateStatus(false);
        setStatus(`Automatic updates turned ${enabled ? "off" : "on"}.`, "ok");
      } catch (err) {
        setUpdateNote(`Automatic update setting failed: ${err.message}`, "error");
        setStatus(err.message, "error");
      } finally {
        state.update.autoSaving = false;
        applyUpdateControls();
      }
    };
  }

  if (el.btnUpdateCancelScheduled) {
    el.btnUpdateCancelScheduled.onclick = async () => {
      if (!state.user || state.user.role !== "admin") return;
      state.update.cancelingScheduled = true;
      applyUpdateControls();
      try {
        await api("/api/v1/admin/system/update/cancel-scheduled", {
          method: "POST",
          json: {},
        });
        await loadUpdateStatus(false);
        setStatus("Scheduled update canceled for this release.", "ok");
      } catch (err) {
        setUpdateNote(`Cancel scheduled update failed: ${err.message}`, "error");
        setStatus(err.message, "error");
      } finally {
        state.update.cancelingScheduled = false;
        applyUpdateControls();
      }
    };
  }

  if (el.composeForm) {
    const persistDraft = () => {
      syncComposeDraftFields();
      queueComposeDraftSave();
      updateComposeSubmitState();
    };
    el.composeForm.addEventListener("input", persistDraft);
    el.composeForm.addEventListener("change", persistDraft);
    el.composeForm.addEventListener("focusout", (event) => {
      const next = event.relatedTarget;
      if (next instanceof Node && el.composeForm.contains(next)) return;
      void flushComposeDraft({ immediate: true });
    });
  }

  const popLastComposeRecipientToken = (field) => {
    const rows = Array.isArray(state.compose.recipients[field]) ? state.compose.recipients[field] : [];
    if (rows.length === 0) return false;
    rows.pop();
    state.compose.recipients[field] = rows;
    renderComposeRecipientTokens(field);
    syncComposeDraftFields();
    queueComposeDraftSave();
    updateComposeSubmitState();
    return true;
  };

  const bindComposeRecipientInput = (field, input) => {
    if (!input) return;
    input.addEventListener("input", () => {
      const value = String(input.value || "").trim();
      const chunks = splitComposeRecipients(value);
      const single = chunks.length === 1 ? chunks[0] : "";
      input.classList.toggle("compose-input-invalid", single !== "" && !validEmail(single));
      queueComposeDraftSave();
      updateComposeSubmitState();
    });
    input.addEventListener("keydown", (event) => {
      if (event.key === "," || event.key === ";" || event.key === "Enter") {
        event.preventDefault();
        commitComposeRecipientInput(field);
        input.classList.remove("compose-input-invalid");
        syncComposeDraftFields();
        queueComposeDraftSave();
        updateComposeSubmitState();
        return;
      }
      if (event.key === "Tab") {
        commitComposeRecipientInput(field);
        input.classList.remove("compose-input-invalid");
        syncComposeDraftFields();
        queueComposeDraftSave();
        updateComposeSubmitState();
        return;
      }
      if (event.key === "Backspace" && String(input.value || "").trim() === "") {
        popLastComposeRecipientToken(field);
      }
    });
    input.addEventListener("blur", () => {
      commitComposeRecipientInput(field);
      input.classList.remove("compose-input-invalid");
      syncComposeDraftFields();
      void flushComposeDraft({ immediate: true });
      updateComposeSubmitState();
    });
    input.addEventListener("paste", (event) => {
      const text = event.clipboardData?.getData("text/plain") || "";
      if (!text) return;
      event.preventDefault();
      splitComposeRecipients(text).forEach((item) => addComposeRecipientToken(field, item));
      input.classList.remove("compose-input-invalid");
      syncComposeDraftFields();
      queueComposeDraftSave();
      updateComposeSubmitState();
    });
  };

  bindComposeRecipientInput("to", el.composeToInput);
  bindComposeRecipientInput("cc", el.composeCcInput);
  bindComposeRecipientInput("bcc", el.composeBccInput);

  if (el.composeEditor) {
    el.composeEditor.addEventListener("input", () => {
      cleanupComposeInlineReferences();
      syncComposeDraftFields();
      queueComposeDraftSave();
      updateComposeSubmitState();
    });
    el.composeEditor.addEventListener("paste", (event) => {
      event.preventDefault();
      const text = event.clipboardData?.getData("text/plain") || "";
      if (text) {
        insertComposeHTMLAtCaret(escapeHtml(text).replace(/\n/g, "<br>"));
      }
      queueComposeDraftSave();
    });
  }

  if (el.composeToggleCc) {
    el.composeToggleCc.addEventListener("click", () => {
      setComposeCcVisible(!state.compose.ccVisible);
      queueComposeDraftSave();
      updateComposeSubmitState();
      if (state.compose.ccVisible && el.composeCcInput) el.composeCcInput.focus();
    });
  }

  if (el.composeToggleBcc) {
    el.composeToggleBcc.addEventListener("click", () => {
      setComposeBccVisible(!state.compose.bccVisible);
      queueComposeDraftSave();
      updateComposeSubmitState();
      if (state.compose.bccVisible && el.composeBccInput) el.composeBccInput.focus();
    });
  }

  if (el.composeToggleFormatting) {
    el.composeToggleFormatting.addEventListener("click", () => {
      setComposeFormatToolsVisible(!state.compose.formatToolsVisible);
      queueComposeDraftSave();
      updateComposeSubmitState();
      if (state.compose.formatToolsVisible && el.composeEditor) {
        el.composeEditor.focus();
      }
    });
  }

  if (el.composeFromSelect) {
    el.composeFromSelect.addEventListener("change", () => {
      const selectedOption = el.composeFromSelect.selectedOptions[0] || null;
      state.compose.selectedIdentityID = String(selectedOption?.value || "");
      state.compose.selectedAccountID = String(selectedOption?.dataset.accountId || "");
      setComposeFromMode("identity");
      setComposeFromNote("");
      queueComposeDraftSave();
      updateComposeSubmitState();
    });
  }

  if (el.composeFromManualInput) {
    el.composeFromManualInput.addEventListener("input", () => {
      if (state.compose.fromMode !== "manual") {
        setComposeFromMode("manual");
      }
      const authEmail = composeAuthEmailValue().toLowerCase();
      const manualRaw = String(el.composeFromManualInput.value || "").trim().toLowerCase();
      if (manualRaw === "" || manualRaw === authEmail) {
        setComposeFromNote("");
      } else {
        setComposeFromNote("Sender must exactly match your authenticated email.", "error");
      }
      queueComposeDraftSave();
      updateComposeSubmitState();
    });
  }

  if (el.composeToolUndo) {
    el.composeToolUndo.addEventListener("click", () => runComposeCommand("undo"));
  }
  if (el.composeToolTypography) {
    el.composeToolTypography.addEventListener("click", () => cycleComposeTypographyMode());
  }
  if (el.composeToolBold) {
    el.composeToolBold.addEventListener("click", () => runComposeCommand("bold"));
  }
  if (el.composeToolItalic) {
    el.composeToolItalic.addEventListener("click", () => runComposeCommand("italic"));
  }
  if (el.composeToolUnderline) {
    el.composeToolUnderline.addEventListener("click", () => runComposeCommand("underline"));
  }
  if (el.composeToolList) {
    el.composeToolList.addEventListener("click", (event) => {
      if (event.altKey) {
        runComposeCommand("formatBlock", "<blockquote>");
        return;
      }
      if (event.shiftKey) {
        runComposeCommand("insertOrderedList");
        return;
      }
      runComposeCommand("insertUnorderedList");
    });
  }
  if (el.composeToolLink) {
    el.composeToolLink.addEventListener("click", () => {
      void promptComposeLink();
    });
  }
  if (el.composeToolClear) {
    el.composeToolClear.addEventListener("click", () => {
      runComposeCommand("removeFormat");
      runComposeCommand("unlink");
    });
  }
  if (el.composeToolAttach && el.composeAttachmentsInput) {
    el.composeToolAttach.addEventListener("click", () => el.composeAttachmentsInput.click());
  }
  if (el.composeAttachmentsInput) {
    el.composeAttachmentsInput.addEventListener("change", (event) => {
      addComposeFiles(event.target.files || []);
      event.target.value = "";
    });
  }
  if (el.composeAssetsList) {
    el.composeAssetsList.addEventListener("click", (event) => {
      const target = event.target instanceof Element ? event.target : null;
      if (!target) return;
      const retry = target.closest("[data-compose-asset-retry]");
      if (retry) {
        retryComposeAssetByID(String(retry.getAttribute("data-compose-asset-retry") || ""));
        return;
      }
      const remove = target.closest("[data-compose-asset-remove]");
      if (!remove) return;
      void removeComposeAssetByID(String(remove.getAttribute("data-compose-asset-remove") || "")).catch((err) => {
        setComposeDraftNote(formatAPIError(err, "Attachment removal failed."), "error");
        setStatus(err.message, "error");
      });
    });
  }
  renderComposeRecipientTokens("to");
  renderComposeRecipientTokens("cc");
  renderComposeRecipientTokens("bcc");
  setComposeCcVisible(state.compose.ccVisible, { clearWhenHidden: false });
  setComposeBccVisible(state.compose.bccVisible, { clearWhenHidden: false });
  setComposeFormatToolsVisible(state.compose.formatToolsVisible);
  setComposeDraftState("Draft", "muted");
  updateComposeSubmitState();

  if (el.btnComposeOpen) {
    el.btnComposeOpen.onclick = () => {
      if (!state.user || state.setup.required) return;
      void openComposeOverlay(el.btnComposeOpen);
    };
  }
  if (el.btnComposeClose) {
    el.btnComposeClose.onclick = () => closeComposeOverlay(true);
  }
  if (el.btnComposeDiscard) {
    el.btnComposeDiscard.onclick = async () => {
      try {
        await discardComposeDraft();
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }
  if (el.composeOverlay) {
    el.composeOverlay.addEventListener("click", (event) => {
      if (event.target === el.composeOverlay) {
        closeComposeOverlay(true);
      }
    });
  }
  document.addEventListener("keydown", handleComposeOverlayKeydown);
  if (el.uiModalCancel) {
    el.uiModalCancel.onclick = () => closeUIModal({ confirmed: false, value: "" });
  }
  if (el.uiModalConfirm) {
    el.uiModalConfirm.onclick = () => {
      const value = el.uiModalInput ? el.uiModalInput.value : "";
      closeUIModal({ confirmed: true, value });
    };
  }
  if (el.uiModalOverlay) {
    el.uiModalOverlay.addEventListener("click", (event) => {
      if (event.target === el.uiModalOverlay) {
        closeUIModal({ confirmed: false, value: "" });
      }
    });
  }
  if (el.mfaModalOverlay) {
    el.mfaModalOverlay.addEventListener("click", (event) => {
      if (event.target === el.mfaModalOverlay) {
        closeMFAModal({ action: "cancel", value: "" });
      }
    });
  }
  document.addEventListener("keydown", handleUIModalKeydown);
  document.addEventListener("keydown", handleMFAModalKeydown);

  el.tabSetup.onclick = () => {
    if (!state.setup.required) return;
    setActiveTab(el.tabSetup);
    showView("setup");
  };

  el.tabAuth.onclick = () => {
    if (state.setup.required) return;
    closeComposeOverlay(false);
    setActiveTab(el.tabAuth);
    showView("auth");
    setActiveAuthTask("login");
    void loadAuthCapabilities();
    void initCaptchaUI();
  };

  el.tabMail.onclick = async () => {
    if (!state.user || state.setup.required) return;
    if (requiresMFAStageAuthentication(state.user)) {
      try {
        await ensureMFAStageAuthenticated(state.user);
        await refreshSession({
          throwOnFail: true,
          skipUnauthorizedHandling: true,
          skipMFAHandling: true,
        });
      } catch (err) {
        presentAPIError(err, "Multi-factor authentication setup is required before opening mail.");
        return;
      }
    }
    if (state.user.mail_secret_required === true) {
      try {
        await unlockMailSecretForSession();
        const refreshed = await refreshSession({
          throwOnFail: true,
          skipUnauthorizedHandling: true,
          skipMFAHandling: true,
        });
        if (!refreshed.ok || refreshed.user?.mail_secret_required === true) {
          routeToAuthWithMessage("Mailbox password unlock failed. Sign in again.", "mail_secret_required");
          return;
        }
      } catch (err) {
        presentAPIError(err, "Mailbox password is required before opening mail.");
        return;
      }
    }
    closeComposeOverlay(false);
    setActiveTab(el.tabMail);
    showView("mail");
    setActiveMailPane(state.selectedMessage ? "reader" : "messages");
    try {
      await loadMailboxes();
      await loadMessages();
    } catch (err) {
      presentAPIError(err, "Failed to load mail");
    }
  };

  if (el.tabSettings) {
    el.tabSettings.onclick = async () => {
      if (!state.user || state.setup.required) return;
      if (requiresMFAStageAuthentication(state.user)) {
        try {
          await ensureMFAStageAuthenticated(state.user);
          await refreshSession({
            throwOnFail: true,
            skipUnauthorizedHandling: true,
            skipMFAHandling: true,
          });
        } catch (err) {
          presentAPIError(err, "Multi-factor authentication is required before opening Settings.");
          return;
        }
      }
      closeComposeOverlay(false);
      setActiveTab(el.tabSettings);
      showView("settings");
      setActiveSettingsSection(state.ui.activeSettingsSection || "signin");
      if (el.settingsSearchResults) {
        el.settingsSearchResults.classList.add("hidden");
      }
      try {
        await Promise.all([
          loadPasskeyCredentials(),
          loadTrustedDevices(),
          loadSessions(),
        ]);
      } catch (err) {
        presentAPIError(err, "Failed to load settings security data");
      }
    };
  }

  el.tabAdmin.onclick = async () => {
    if (!state.user || state.user.role !== "admin" || state.setup.required) return;
    if (requiresMFAStageAuthentication(state.user)) {
      try {
        await ensureMFAStageAuthenticated(state.user);
        await refreshSession({
          throwOnFail: true,
          skipUnauthorizedHandling: true,
          skipMFAHandling: true,
        });
      } catch (err) {
        presentAPIError(err, "Multi-factor authentication setup is required before opening admin.");
        return;
      }
    }
    closeComposeOverlay(false);
    setActiveTab(el.tabAdmin);
    showView("admin");
    setActiveAdminSection(state.ui.activeAdminSection || "system");
    if (el.adminSearchResults) {
      el.adminSearchResults.classList.add("hidden");
    }
    try {
      await loadAdmin();
    } catch (err) {
      presentAPIError(err, "Failed to load admin data");
    }
  };

  el.btnLogout.onclick = async () => {
    try {
      await api("/api/v1/logout", { method: "POST", json: {} });
    } catch {
      // ignore
    }
    stopUpdatePolling();
    state.auth.recoveryPromptShownForSession = false;
    state.auth.legacyMFAOfferShownForSession = false;
    state.auth.mfaFlowPromise = null;
    state.user = null;
    clearReaderSelection();
    renderPasskeyCredentials([]);
    renderTrustedDevices([]);
    renderSessions([]);
    closeComposeOverlay(false);
    closeMFAModal({ action: "cancel", value: "" });
    applyNavVisibility();
    setActiveTab(el.tabAuth);
    showView("auth");
    setActiveAuthTask("login");
    void initCaptchaUI();
    setStatus("Signed out.", "ok");
  };

  el.btnSearch.onclick = async () => {
    try {
      await searchMessages();
    } catch (err) {
      setStatus(err.message, "error");
    }
  };

  if (el.btnReaderViewHTML) {
    el.btnReaderViewHTML.onclick = () => {
      state.ui.readerViewMode = "html";
      renderReaderBody(state.selectedMessage);
    };
  }

  if (el.btnReaderViewPlain) {
    el.btnReaderViewPlain.onclick = () => {
      state.ui.readerViewMode = "plain";
      renderReaderBody(state.selectedMessage);
    };
  }

  if (el.btnReply) {
    el.btnReply.onclick = async () => {
      try {
        await openReplyCompose();
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnForward) {
    el.btnForward.onclick = async () => {
      try {
        await openForwardCompose();
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnFlag) {
    el.btnFlag.onclick = async () => {
      try {
        const mode = selectedMailActionFlagMode();
        await runMailAction(mode, { statusVerb: mode === "unflag" ? "Unflagged" : "Flagged" });
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnSeen) {
    el.btnSeen.onclick = async () => {
      try {
        const mode = selectedMailActionReadMode();
        await runMailAction(mode, { statusVerb: mode === "unread" ? "Marked unread" : "Marked read" });
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnArchive) {
    el.btnArchive.onclick = async () => {
      try {
        const archiveMailbox = mailboxNameForRole("archive");
        if (!archiveMailbox) {
          setStatus("No archive mailbox is available.", "error");
          return;
        }
        await runMailAction("archive", { mailbox: archiveMailbox, statusVerb: "Archived" });
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnMove) {
    el.btnMove.onclick = async () => {
      try {
        const mailbox = String(el.mailMoveTarget?.value || "").trim();
        await runMailAction("move", { mailbox, statusVerb: "Moved" });
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnTrash) {
    el.btnTrash.onclick = async () => {
      try {
        const trashMailbox = mailboxNameForRole("trash") || "Trash";
        await runMailAction("trash", { mailbox: trashMailbox, statusVerb: "Moved to trash" });
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.mailMoveTarget) {
    el.mailMoveTarget.addEventListener("change", () => {
      applyMailActionAvailability();
    });
  }

  if (el.btnMailSelectVisible) {
    el.btnMailSelectVisible.onclick = () => {
      const next = new Set(visibleActionableMessages().map((item) => String(item?.id || "")).filter(Boolean));
      state.mail.selectedMessageIDs = next;
      renderMessages(state.messages);
    };
  }

  if (el.btnMailClear) {
    el.btnMailClear.onclick = () => {
      clearMailMessageSelection();
    };
  }

  if (el.mailCheckAll) {
    el.mailCheckAll.addEventListener("change", () => {
      if (el.mailCheckAll.checked) {
        state.mail.selectedMessageIDs = new Set(visibleActionableMessages().map((item) => String(item?.id || "")).filter(Boolean));
      } else {
        state.mail.selectedMessageIDs = new Set();
      }
      renderMessages(state.messages);
    });
  }

  if (el.btnThreadCollapse) {
    el.btnThreadCollapse.onclick = () => {
      toggleThreadExpanded();
    };
  }

  if (el.btnThreadPrev) {
    el.btnThreadPrev.onclick = async () => {
      try {
        await openThreadNeighbor(-1);
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  if (el.btnThreadNext) {
    el.btnThreadNext.onclick = async () => {
      try {
        await openThreadNeighbor(1);
      } catch (err) {
        setStatus(err.message, "error");
      }
    };
  }

  applyMailActionAvailability();
  renderThreadContext();
  renderReaderBody(null);
}

async function bootstrap() {
  ThemeController.initTheme();
  bindUI();
  window.addEventListener("beforeunload", () => {
    if (!state.ui.composeOpen || !el.composeForm) return;
    syncComposeDraftFields();
    writeComposeCrashBuffer(state.compose.draftID || "");
    void flushComposeDraft({ immediate: true });
  });
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && !el.viewMail?.classList.contains("hidden")) {
      startMailPolling();
      void pollMailView();
      return;
    }
    if (document.visibilityState !== "visible") {
      stopMailPolling();
    }
  });
  const resetLinkToken = captureResetTokenFromLocation();
  if (resetLinkToken) {
    applyResetLinkToken(resetLinkToken);
  }
  await initCaptchaUI();
  await loadResetCapabilities();
  await loadAuthCapabilities();

  try {
    if (await enterSetupIfRequired()) {
      return;
    }
  } catch (err) {
    setStatus(err.message, "error");
    return;
  }

  const session = await refreshSession({ skipUnauthorizedHandling: true });
  if (!session.ok) {
    setActiveTab(el.tabAuth);
    showView("auth");
    setActiveAuthTask(el.resetTokenInput && String(el.resetTokenInput.value || "").trim() ? "reset" : "login");
    await loadAuthCapabilities();
    await initCaptchaUI();
    if (el.resetTokenInput && String(el.resetTokenInput.value || "").trim()) {
      applyResetLinkToken(el.resetTokenInput.value, { focus: true });
    }
    setStatus("Authentication required.");
    return;
  }

  if (requiresMFAStageAuthentication(session.user || {})) {
    try {
      await ensureMFAStageAuthenticated(session.user);
      const refreshed = await refreshSession({
        skipUnauthorizedHandling: true,
        throwOnFail: true,
        skipMFAHandling: true,
      });
      if (!refreshed.ok) {
        routeToAuthWithMessage("Session refresh failed after multi-factor setup. Sign in again.", "session_invalid");
        return;
      }
    } catch (err) {
      routeToAuthWithMessage(formatAPIError(err, "Multi-factor authentication is required to continue."), "mfa_required");
      return;
    }
  }

  if (state.user && state.user.mail_secret_required === true) {
    try {
      await unlockMailSecretForSession();
      const refreshed = await refreshSession({
        skipUnauthorizedHandling: true,
        throwOnFail: true,
        skipMFAHandling: true,
      });
      if (!refreshed.ok || refreshed.user?.mail_secret_required === true) {
        routeToAuthWithMessage("Mailbox password unlock failed. Sign in again.", "mail_secret_required");
        return;
      }
    } catch (err) {
      routeToAuthWithMessage(formatAPIError(err, "Mailbox password is required to continue."), "mail_secret_required");
      return;
    }
  }

  setActiveTab(el.tabMail);
  showView("mail");
  setActiveMailPane("messages", { focus: false });
  try {
    await loadMailboxes();
    await loadMessages();
  } catch (err) {
    setStatus(err.message, "error");
  }
}

bootstrap();
