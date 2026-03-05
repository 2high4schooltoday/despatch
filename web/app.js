const state = {
  user: null,
  mailbox: "INBOX",
  messages: [],
  selectedMessage: null,
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
  loginForm: document.getElementById("form-login"),
  passkeyEmailInput: document.getElementById("passkey-email"),
  btnPasskeyLogin: document.getElementById("btn-passkey-login"),
  passkeyLoginHint: document.getElementById("passkey-login-hint"),
  resetCapabilityNote: document.getElementById("reset-capability-note"),
  mailboxes: document.getElementById("mailboxes"),
  messages: document.getElementById("messages"),
  meta: document.getElementById("message-meta"),
  body: document.getElementById("message-body"),
  attachments: document.getElementById("attachment-list"),
  searchInput: document.getElementById("search-input"),
  btnSearch: document.getElementById("btn-search"),
  btnFlag: document.getElementById("btn-flag"),
  btnSeen: document.getElementById("btn-mark-seen"),
  btnTrash: document.getElementById("btn-trash"),
  btnComposeOpen: document.getElementById("btn-compose-open"),
  btnComposeClose: document.getElementById("btn-compose-close"),
  composeOverlay: document.getElementById("compose-overlay"),
  composeDialog: document.getElementById("compose-dialog"),
  composeForm: document.getElementById("form-compose"),
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
  updateCurrentVersion: document.getElementById("update-current-version"),
  updateCurrentCommit: document.getElementById("update-current-commit"),
  updateLatestVersion: document.getElementById("update-latest-version"),
  updateAvailable: document.getElementById("update-available"),
  updateLastChecked: document.getElementById("update-last-checked"),
  updateApplyState: document.getElementById("update-apply-state"),
  updateNote: document.getElementById("update-note"),
  btnUpdateCheck: document.getElementById("btn-update-check"),
  btnUpdateApply: document.getElementById("btn-update-apply"),
  setupBackIcon: document.getElementById("setup-back-icon"),
  setupClose: document.getElementById("setup-close"),
  setupForm: document.getElementById("form-setup"),
  setupBack: document.getElementById("setup-back"),
  setupNext: document.getElementById("setup-next"),
  setupOpenMail: document.getElementById("setup-open-mail"),
  setupOpenAdmin: document.getElementById("setup-open-admin"),
  setupRegion: document.getElementById("setup-region"),
  setupDomain: document.getElementById("setup-domain"),
  setupAdminEmail: document.getElementById("setup-admin-email"),
  setupAdminMailboxLogin: document.getElementById("setup-admin-mailbox-login"),
  setupAdminMailboxLoginWrap: document.getElementById("setup-mailbox-login-wrap"),
  setupPassword: document.getElementById("setup-password"),
  setupPasswordConfirm: document.getElementById("setup-password-confirm"),
  setupSummaryRegion: document.getElementById("setup-summary-region"),
  setupSummaryDomain: document.getElementById("setup-summary-domain"),
  setupSummaryEmail: document.getElementById("setup-summary-email"),
  setupPasswordHint: document.getElementById("setup-password-hint"),
  setupInlineStatus: document.getElementById("setup-inline-status"),
  setupCompleteNote: document.getElementById("setup-complete-note"),
  setupModalOverlay: document.getElementById("setup-modal-overlay"),
  setupModalTitle: document.getElementById("setup-modal-title"),
  setupModalBody: document.getElementById("setup-modal-body"),
  setupModalCancel: document.getElementById("setup-modal-cancel"),
  setupModalConfirm: document.getElementById("setup-modal-confirm"),
};

const setupSteps = [
  document.getElementById("setup-step-0"),
  document.getElementById("setup-step-1"),
  document.getElementById("setup-step-2"),
  document.getElementById("setup-step-3"),
  document.getElementById("setup-step-4"),
  document.getElementById("setup-step-5"),
];

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

function passkeyLoginEmailValue() {
  return String(el.passkeyEmailInput?.value || "").trim().toLowerCase();
}

function renderPasskeyLoginUI() {
  if (!el.btnPasskeyLogin || !el.passkeyLoginHint) {
    return;
  }
  const caps = authCapabilities();
  const browserSupported = supportsWebAuthn();
  const available = browserSupported && !!caps.passkey_passwordless_available;
  const usernameless = caps.passkey_usernameless_enabled !== false;
  const hasEmail = passkeyLoginEmailValue() !== "";
  const emailRequiredButMissing = available && !usernameless && !hasEmail;
  el.btnPasskeyLogin.disabled = !available || emailRequiredButMissing;
  if (available) {
    if (!usernameless) {
      el.passkeyLoginHint.textContent = "Email is required for passkey sign-in.";
      return;
    }
    el.passkeyLoginHint.textContent = hasEmail
      ? "Use a passkey scoped to this email."
      : "Use a passkey on this device; account discovery is automatic.";
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
  lines.push(caps.passkey_usernameless_enabled === false
    ? "Username-less account discovery is disabled; email is required for passkey sign-in."
    : "Username-less account discovery is enabled; email is optional for passkey sign-in.");
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
  return "despatch.compose.draft.v1";
}

function saveComposeDraft(form) {
  if (!form) return;
  const fd = new FormData(form);
  const payload = {
    to: String(fd.get("to") || ""),
    subject: String(fd.get("subject") || ""),
    body: String(fd.get("body") || ""),
  };
  localStorage.setItem(composeDraftKey(), JSON.stringify(payload));
}

function clearComposeDraft() {
  localStorage.removeItem(composeDraftKey());
}

function restoreComposeDraft(form) {
  if (!form) return;
  const raw = localStorage.getItem(composeDraftKey());
  if (!raw) return;
  try {
    const draft = JSON.parse(raw);
    if (typeof draft.to === "string") form.elements.to.value = draft.to;
    if (typeof draft.subject === "string") form.elements.subject.value = draft.subject;
    if (typeof draft.body === "string") form.elements.body.value = draft.body;
  } catch {
    localStorage.removeItem(composeDraftKey());
  }
}

function composeFocusableElements() {
  if (!el.composeDialog) return [];
  return Array.from(el.composeDialog.querySelectorAll("button, [href], input, textarea, select, [tabindex]:not([tabindex='-1'])"))
    .filter((node) => !node.disabled && node.offsetParent !== null);
}

function openComposeOverlay(trigger = null) {
  if (!el.composeOverlay) return;
  state.ui.composeOpen = true;
  state.ui.composeLastTrigger = trigger || document.activeElement || null;
  el.composeOverlay.classList.remove("hidden");
  el.composeOverlay.setAttribute("aria-hidden", "false");
  document.body.style.overflow = "hidden";
  if (el.composeForm) {
    restoreComposeDraft(el.composeForm);
    const toInput = el.composeForm.elements.to;
    if (toInput && typeof toInput.focus === "function") {
      toInput.focus();
    }
  }
}

function closeComposeOverlay(restoreFocus = true) {
  if (!el.composeOverlay) return;
  state.ui.composeOpen = false;
  el.composeOverlay.classList.add("hidden");
  el.composeOverlay.setAttribute("aria-hidden", "true");
  document.body.style.overflow = state.ui.modalOpen || state.ui.mfaModalOpen ? "hidden" : "";
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
  if (!out || !out.confirmed) return "";
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
  return {
    challenge: decodeCredentialID(pk.challenge || beginPayload.challenge),
    rpId: String(pk.rpId || beginPayload.rp_id || window.location.hostname),
    timeout: Number(pk.timeout || beginPayload.timeout_ms || 300000),
    userVerification: String(pk.userVerification || "preferred"),
    allowCredentials: allowCredentials.map((item) => ({
      type: "public-key",
      id: decodeCredentialID(item.id),
      transports: Array.isArray(item.transports) ? item.transports : undefined,
    })),
  };
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
  const emailValue = passkeyLoginEmailValue();
  const begin = await api("/api/v1/login/passkey/begin", {
    method: "POST",
    json: emailValue ? { email: emailValue } : {},
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
  const scopedEmail = passkeyLoginEmailValue();
  const caps = authCapabilities();
  const usernameless = caps.passkey_usernameless_enabled !== false;
  if (scopedEmail) {
    return `No matching passkey was accepted for ${scopedEmail}. Try another email or continue with password.${apiRequestRef(err)}`;
  }
  if (usernameless) {
    return `No discoverable passkey was accepted on this device. Try again or sign in with email and password.${apiRequestRef(err)}`;
  }
  return `Email is required for passkey sign-in on this server.${apiRequestRef(err)}`;
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
  if (!enabled) {
    const suffix = senderReason ? ` (${senderReason})` : "";
    el.resetCapabilityNote.textContent = `Reset is currently unavailable (${authMode} mode, delivery ${delivery}, sender ${senderStatus}${suffix}).`;
    return;
  }
  const mappingNote = cap.requires_mapped_login ? "Mapped mailbox login is required." : "Mapped mailbox login is optional.";
  el.resetCapabilityNote.textContent = `Reset enabled (${authMode}, delivery ${delivery}, token TTL ${ttl} min, sender ${senderAddress}, status ${senderStatus}). ${mappingNote}`;
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
  state.setup.authMode = String(data.auth_mode || "sql").toLowerCase();
  state.setup.passwordMinLength = Number(data.password_min_length || 12);
  state.setup.passwordMaxLength = Number(data.password_max_length || 128);
  state.setup.passwordClassMin = Number(data.password_class_min || 3);
  return data;
}

async function completeSetup() {
  const domain = normalizeDomain(el.setupDomain.value);
  const email = String(el.setupAdminEmail.value || "").trim().toLowerCase();
  const region = String(el.setupRegion.value || "us-east").trim();
  const password = el.setupPassword.value;
  const mailboxLogin = String(el.setupAdminMailboxLogin?.value || "").trim();

  const setupPayload = await api("/api/v1/setup/complete", {
    method: "POST",
    json: {
      base_domain: domain,
      admin_email: email,
      admin_mailbox_login: mailboxLogin,
      admin_password: password,
      region,
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
    el.setupPassword.value = "";
    el.setupPasswordConfirm.value = "";
    if (el.setupAdminMailboxLogin) el.setupAdminMailboxLogin.value = "";
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
    setSetupInlineStatus("");
    this.updatePasswordHint();
    this.setStep(0);
    this.updateSummary();
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
    const isFirst = state.setup.step === 0;
    const isReview = state.setup.step === 4;
    const isComplete = state.setup.step === 5;
    const showFooterBack = state.setup.step >= 1 && state.setup.step <= 4;
    const showDiscard = state.setup.step >= 2 && state.setup.step <= 4;

    el.setupBack.classList.toggle("hidden", !showFooterBack);
    el.setupClose.classList.toggle("hidden", !showDiscard);
    // Footer-only back navigation: keep top-left control hidden for DOM compatibility only.
    el.setupBackIcon.classList.add("hidden");

    el.setupBack.disabled = !showFooterBack || isComplete || state.setup.submitting;
    el.setupBackIcon.disabled = true;
    el.setupClose.disabled = !showDiscard || state.setup.submitting;
    el.setupNext.disabled = isComplete || state.setup.submitting;
    const retryRemaining = setupRetrySecondsRemaining();
    if (retryRemaining > 0) {
      el.setupNext.textContent = `Retry in ${retryRemaining}s`;
    } else {
      el.setupNext.textContent = state.setup.submitting ? "Initializing..." : isReview ? "Initialize" : "Continue";
    }
    if (!isComplete) setSetupInlineStatus("");
    this.refreshNavState();
  },

  updateSummary() {
    el.setupSummaryRegion.textContent = el.setupRegion.options[el.setupRegion.selectedIndex]?.text || "-";
    el.setupSummaryDomain.textContent = normalizeDomain(el.setupDomain.value) || "-";
    el.setupSummaryEmail.textContent = String(el.setupAdminEmail.value || "-").trim().toLowerCase();
  },

  validateStep(stepId) {
    if (stepId === 2) {
      const domain = normalizeDomain(el.setupDomain.value);
      const email = String(el.setupAdminEmail.value || "").trim().toLowerCase();
      if (!validDomain(domain)) {
        throw new Error("Enter a valid domain (example: mail.example.com or example.com)");
      }
      if (!validEmail(email)) {
        throw new Error("Enter a valid admin email");
      }
      if (!email.endsWith(`@${domain}`)) {
        throw new Error(`Admin email must use @${domain}`);
      }
    }

    if (stepId === 3) {
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
    const showFooterBack = state.setup.step >= 1 && state.setup.step <= 4;
    const showDiscard = state.setup.step >= 2 && state.setup.step <= 4;
    el.setupBack.classList.toggle("hidden", !showFooterBack);
    el.setupClose.classList.toggle("hidden", !showDiscard);
    el.setupBackIcon.classList.add("hidden");

    if (state.setup.step === 5) {
      el.setupNext.disabled = true;
      el.setupBack.disabled = true;
      el.setupClose.disabled = true;
      return;
    }
    if (setupRetrySecondsRemaining() > 0) {
      el.setupNext.disabled = true;
      el.setupBack.disabled = true;
      el.setupClose.disabled = true;
      return;
    }
    if (state.setup.submitting) {
      el.setupNext.disabled = true;
      el.setupBack.disabled = true;
      el.setupClose.disabled = true;
      return;
    }
    el.setupBack.disabled = !showFooterBack;
    el.setupClose.disabled = !showDiscard;
    el.setupNext.disabled = !this.isStepValid(state.setup.step);
  },

  async next() {
    if (state.setup.submitting) return;
    if (setupRetrySecondsRemaining() > 0) return;
    this.validateStep(state.setup.step);
    if (state.setup.step < 4) {
      this.setStep(state.setup.step + 1);
      this.updateSummary();
      return;
    }
    if (state.setup.step === 4) {
      state.setup.submitting = true;
      this.refreshNavState();
      setSetupInlineStatus("Initializing setup...", "info");
      try {
        await completeSetup();
        state.setup.submitting = false;
        this.setStep(5);
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
      return;
    }
  },

  updatePasswordHint() {
    if (!el.setupPasswordHint) return;
    if (state.setup.authMode === "pam") {
      el.setupPasswordHint.textContent = "PAM mode: enter the current mailbox password. If login differs from email, provide Mailbox Login.";
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
    if (state.setup.step <= 0 || state.setup.step === 5) return;
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
    if (type === "cancel" && (state.setup.step < 2 || state.setup.step >= 5)) {
      return;
    }
    state.setup.modalType = type;
    if (type === "cancel") {
      el.setupModalTitle.textContent = "Discard Setup Progress?";
      el.setupModalBody.textContent = "If you close setup now, initialization stays incomplete and login remains blocked.";
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
    if (!el.setupBack.classList.contains("hidden") && !el.setupBack.disabled) {
      el.setupBack.focus();
    } else {
      el.setupNext.focus();
    }
  },

  async confirm() {
    const type = state.setup.modalType || "cancel";
    this.closeConfirm();
    if (type === "cancel") {
      setStatus("SETUP CANCELLED. COMPLETE SETUP TO LOG IN.", "info");
      return;
    }
    this.init();
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
      body: "Password reset tokens are sent to your recovery email. Use an address different from your login email.",
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

async function loadMailboxes() {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  const data = await api("/api/v1/mailboxes");
  el.mailboxes.innerHTML = "";
  for (const [index, mb] of data.entries()) {
    const li = document.createElement("li");
    li.className = "mailbox-row";
    const btn = document.createElement("button");
    btn.innerHTML = `<span class="mailbox-name">${escapeHtml(mb.name)}</span><span class="mailbox-count">${Number(mb.unread || 0)}/${Number(mb.messages || 0)}</span>`;
    btn.className = mb.name === state.mailbox ? "active" : "";
    btn.dataset.mailboxName = mb.name;
    btn.id = safeDomID("mailbox-option-", mb.name, `${index}`);
    btn.type = "button";
    btn.setAttribute("role", "option");
    btn.setAttribute("aria-selected", mb.name === state.mailbox ? "true" : "false");
    btn.tabIndex = -1;
    btn.onclick = async () => {
      state.mailbox = mb.name;
      state.selectedMessage = null;
      el.body.textContent = "Select a message.";
      el.meta.innerHTML = "";
      el.attachments.textContent = "";
      await loadMessages();
      await loadMailboxes();
      setActiveMailPane("messages");
    };
    li.appendChild(btn);
    el.mailboxes.appendChild(li);
  }
  syncMailboxActiveDescendant();
}

function renderMessages(items) {
  el.messages.innerHTML = "";
  state.messages = items;
  if (!Array.isArray(items) || items.length === 0) {
    const empty = document.createElement("li");
    empty.className = "message-empty";
    empty.textContent = "No messages to display.";
    el.messages.appendChild(empty);
    return;
  }
  for (const m of items) {
    const li = document.createElement("li");
    const isActive = state.selectedMessage && state.selectedMessage.id === m.id;
    li.className = "message-row";
    if (isActive) li.classList.add("active");
    if (!m.seen) li.classList.add("is-unread");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "message-row-btn";
    btn.dataset.messageId = m.id;
    btn.id = safeDomID("message-option-", m.id, "message");
    btn.setAttribute("role", "option");
    btn.setAttribute("aria-selected", isActive ? "true" : "false");
    btn.tabIndex = -1;
    btn.innerHTML = `<span class="message-mark" aria-hidden="true"></span>
      <span class="message-from">${escapeHtml(m.from || "(unknown sender)")}</span>
      <span class="message-subject">${escapeHtml(m.subject || "(no subject)")}</span>
      <span class="message-date">${escapeHtml(formatDate(m.date))}</span>`;
    btn.onclick = () => openMessage(m.id);
    li.appendChild(btn);
    el.messages.appendChild(li);
  }
  syncMessageActiveDescendant();
}

async function loadMessages() {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  const data = await api(`/api/v1/messages?mailbox=${encodeURIComponent(state.mailbox)}&page=1&page_size=40`);
  renderMessages(data.items || []);
  setStatus(`Mailbox ${state.mailbox} loaded.`, "ok");
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

async function openMessage(id) {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  const m = await api(`/api/v1/messages/${encodeURIComponent(id)}`);
  state.selectedMessage = m;
  const metaRows = [
    ["From", m.from || "-"],
    ["To", (m.to || []).join(", ") || "-"],
    ["Subject", m.subject || "-"],
    ["Date", formatDate(m.date) || "-"],
  ];
  renderMessageMeta(metaRows);
  el.body.textContent = m.body || "(empty)";

  el.attachments.innerHTML = "";
  for (const a of (m.attachments || [])) {
    const link = document.createElement("a");
    link.href = `/api/v1/attachments/${encodeURIComponent(a.id)}`;
    link.textContent = `${a.filename || "attachment"} (${Math.round((a.size || 0) / 1024)} KB)`;
    link.target = "_blank";
    el.attachments.appendChild(link);
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
  const data = await api(`/api/v1/search?mailbox=${encodeURIComponent(state.mailbox)}&q=${encodeURIComponent(q)}&page=1&page_size=40`);
  renderMessages(data.items || []);
  setStatus(`Search complete (${(data.items || []).length} results).`, "ok");
  setActiveMailPane("messages");
}

async function sendCompose(form) {
  if (!state.user) {
    throw new Error("Sign in required");
  }
  saveComposeDraft(form);
  const fd = new FormData(form);
  const files = form.querySelector("input[name='attachments']").files;

  if (files && files.length > 0) {
    const mp = new FormData();
    mp.append("to", fd.get("to"));
    mp.append("subject", fd.get("subject"));
    mp.append("body", fd.get("body"));
    for (const f of files) mp.append("attachments", f);
    await api("/api/v1/messages/send", { method: "POST", body: mp });
  } else {
    await api("/api/v1/messages/send", {
      method: "POST",
      json: {
        to: String(fd.get("to")).split(",").map((s) => s.trim()).filter(Boolean),
        subject: String(fd.get("subject") || ""),
        body: String(fd.get("body") || ""),
      },
    });
  }
  clearComposeDraft();
}

function setUpdateNote(text, type = "info") {
  if (!el.updateNote) return;
  el.updateNote.textContent = text || "";
  if (type === "error") el.updateNote.style.color = "var(--sig-err)";
  else if (type === "ok") el.updateNote.style.color = "var(--sig-ok)";
  else el.updateNote.style.color = "var(--fg-muted)";
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
  if (!el.btnUpdateCheck || !el.btnUpdateApply) return;
  const st = status || state.update.lastStatus || {};
  const applyState = String(st.apply?.state || "idle");
  const busy = applyState === "queued" || applyState === "in_progress";
  const checkSupported = !st.legacy_backend;
  const assetMissing = String(st.last_check_error || "").toLowerCase().includes("release asset");
  el.btnUpdateCheck.disabled = state.update.checking || !checkSupported;
  const canApply = !!st.enabled && !!st.configured && !!st.update_available && !busy && !state.update.applying && !assetMissing;
  el.btnUpdateApply.disabled = !canApply;
}

function renderUpdateStatus(status) {
  state.update.lastStatus = status || null;
  if (!status) {
    if (el.updateCurrentVersion) el.updateCurrentVersion.textContent = "-";
    if (el.updateCurrentCommit) el.updateCurrentCommit.textContent = "-";
    if (el.updateLatestVersion) el.updateLatestVersion.textContent = "-";
    if (el.updateAvailable) el.updateAvailable.textContent = "-";
    if (el.updateLastChecked) el.updateLastChecked.textContent = "-";
    if (el.updateApplyState) el.updateApplyState.textContent = "idle";
    setUpdateNote("Update status unavailable.", "error");
    applyUpdateControls();
    return;
  }
  if (el.updateCurrentVersion) el.updateCurrentVersion.textContent = status.current?.version || "-";
  if (el.updateCurrentCommit) el.updateCurrentCommit.textContent = status.current?.commit || "-";
  if (el.updateLatestVersion) el.updateLatestVersion.textContent = status.latest?.tag_name || "-";
  if (el.updateAvailable) el.updateAvailable.textContent = status.update_available ? "YES" : "NO";
  if (el.updateLastChecked) el.updateLastChecked.textContent = formatDate(status.last_checked_at) || "-";
  if (el.updateApplyState) el.updateApplyState.textContent = String(status.apply?.state || "idle");

  const applyState = String(status.apply?.state || "idle");
  const applyError = String(status.apply?.error || "").trim();

  if (status.legacy_backend) {
    setUpdateNote("This server build does not expose updater API endpoints yet (HTTP 404). Upgrade backend binary manually to a newer release, then reopen Admin.", "error");
  } else if (!status.enabled) {
    setUpdateNote("Software update feature is disabled in configuration (UPDATE_ENABLED=false).", "info");
  } else if (!status.configured) {
    setUpdateNote(updateConfigDiagnosticMessage(status), "error");
  } else if ((applyState === "failed" || applyState === "rolled_back") && applyError) {
    if (applyError.toLowerCase().includes("mailsec")) {
      setUpdateNote(`Last update failed due to mailsec dependency checks: ${applyError}`, "error");
    } else {
      setUpdateNote(`Last update failed: ${applyError}`, "error");
    }
  } else if (status.last_check_error) {
    if (String(status.last_check_error).toLowerCase().includes("release asset")) {
      setUpdateNote(`Release packaging issue detected for this CPU architecture: ${status.last_check_error}`, "error");
    } else {
      setUpdateNote(`Latest check failed: ${status.last_check_error}`, "error");
    }
  } else if (status.update_available && status.latest?.tag_name) {
    setUpdateNote(`New release available: ${status.latest.tag_name}`, "ok");
  } else {
    setUpdateNote("No update currently available.", "info");
  }
  applyUpdateControls(status);
}

function isUpdateStateBusy(status) {
  const stateName = String(status?.apply?.state || "");
  return stateName === "queued" || stateName === "in_progress";
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

  if (event.key.toLowerCase() === "c") {
    event.preventDefault();
    openComposeOverlay(el.btnComposeOpen);
    return;
  }

  if (event.key.toLowerCase() === "f") {
    event.preventDefault();
    if (state.selectedMessage) el.btnFlag.click();
    return;
  }

  if (event.key.toLowerCase() === "s") {
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

  const k = event.key.toLowerCase();
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
}

function formatDate(value) {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return String(value);
  return d.toLocaleString();
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
  el.setupBack.onclick = () => OOBEController.back();
  el.setupBackIcon.onclick = () => OOBEController.back();
  el.setupClose.onclick = async () => {
    if (state.setup.step === 5 && !state.setup.required) {
      await OOBEController.openMail();
      return;
    }
    OOBEController.openConfirm("cancel");
  };

  el.setupForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    if (state.setup.step >= 5) return;
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

  el.setupRegion.addEventListener("change", () => {
    OOBEController.updateSummary();
    OOBEController.refreshNavState();
  });
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

    if (event.key === "Escape" && state.setup.step >= 2 && state.setup.step < 5) {
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
  if (el.passkeyEmailInput) {
    el.passkeyEmailInput.addEventListener("input", () => {
      renderPasskeyLoginUI();
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
  window.addEventListener("resize", () => setActiveMailPane(state.ui.activeMailPane, { focus: false }));
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
      setStatus("If the account exists, reset instructions were sent.", "ok");
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

  document.getElementById("form-compose").addEventListener("submit", async (e) => {
    e.preventDefault();
    try {
      await sendCompose(e.target);
      setStatus("Message sent.", "ok");
      e.target.reset();
      clearComposeDraft();
      closeComposeOverlay(true);
      await loadMessages();
    } catch (err) {
      if (err.code === "smtp_sender_rejected") {
        const requestRef = err.requestID ? ` (request ${err.requestID})` : "";
        setStatus(`SMTP sender policy rejected this message. On Ubuntu, check Postfix sender-login policy and users.mail_login mapping.${requestRef}`, "error");
        return;
      }
      setStatus(err.message, "error");
    }
  });

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

  if (el.composeForm) {
    restoreComposeDraft(el.composeForm);
    const persistDraft = () => saveComposeDraft(el.composeForm);
    el.composeForm.addEventListener("input", persistDraft);
    el.composeForm.addEventListener("change", persistDraft);
  }

  if (el.btnComposeOpen) {
    el.btnComposeOpen.onclick = () => {
      if (!state.user || state.setup.required) return;
      openComposeOverlay(el.btnComposeOpen);
    };
  }
  if (el.btnComposeClose) {
    el.btnComposeClose.onclick = () => closeComposeOverlay(true);
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
    state.selectedMessage = null;
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

  el.btnFlag.onclick = async () => {
    try {
      requireSelectedMessage();
      await api(`/api/v1/messages/${encodeURIComponent(state.selectedMessage.id)}/flags`, { method: "POST", json: { flags: ["\\Flagged", "\\Seen"] } });
      await loadMessages();
      setStatus("Message flagged.", "ok");
    } catch (err) {
      setStatus(err.message, "error");
    }
  };

  el.btnSeen.onclick = async () => {
    try {
      requireSelectedMessage();
      await api(`/api/v1/messages/${encodeURIComponent(state.selectedMessage.id)}/flags`, { method: "POST", json: { flags: ["\\Seen"] } });
      await loadMessages();
      setStatus("Message marked seen.", "ok");
    } catch (err) {
      setStatus(err.message, "error");
    }
  };

  el.btnTrash.onclick = async () => {
    try {
      requireSelectedMessage();
      await api(`/api/v1/messages/${encodeURIComponent(state.selectedMessage.id)}/move`, { method: "POST", json: { mailbox: "Trash" } });
      state.selectedMessage = null;
      el.body.textContent = "Select a message.";
      await loadMessages();
      setStatus("Message moved to trash.", "ok");
      setActiveMailPane("messages");
    } catch (err) {
      setStatus(err.message, "error");
    }
  };
}

async function bootstrap() {
  ThemeController.initTheme();
  bindUI();
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
    setActiveAuthTask("login");
    await loadAuthCapabilities();
    await initCaptchaUI();
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
