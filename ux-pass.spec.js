const { test, expect } = require('@playwright/test');

async function dismissRecoveryPromptIfPresent(page) {
  const overlay = page.locator('#ui-modal-overlay');
  if (!(await overlay.isVisible())) return;
  const title = (await page.locator('#ui-modal-title').textContent() || '').trim();
  if (!/set recovery email/i.test(title)) return;
  await page.click('#ui-modal-cancel');
  await expect(overlay).toHaveClass(/hidden/);
}

async function skipIfMFANavigationIsBlocked(page) {
  const overlay = page.locator('#mfa-modal-overlay');
  if (!(await overlay.isVisible().catch(() => false))) return;
  const title = (await page.locator('#mfa-modal-title').textContent().catch(() => '') || '').trim();
  if (!/authenticator|multi-factor authentication/i.test(title)) return;
  test.skip(true, 'Server requires MFA setup before the main workspace becomes reachable.');
}

async function expectNoHorizontalOverflow(page, selector) {
  const metrics = await page.locator(selector).evaluate((node) => ({
    clientWidth: Math.round(node.clientWidth || 0),
    scrollWidth: Math.round(node.scrollWidth || 0),
  }));
  expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth + 1);
  return metrics;
}

async function readHealthToggleMetrics(page) {
  return page.locator('#btn-mail-health-toggle').evaluate((node) => {
    const toggleRect = node.getBoundingClientRect();
    const copy = node.querySelector('.mail-health-toggle-copy');
    const caret = node.querySelector('#mail-health-toggle-caret');
    const copyRect = copy?.getBoundingClientRect();
    const caretRect = caret?.getBoundingClientRect();
    return {
      clientWidth: Math.round(node.clientWidth || 0),
      scrollWidth: Math.round(node.scrollWidth || 0),
      copyRight: copyRect ? Math.round(copyRect.right - toggleRect.left) : 0,
      caretLeft: caretRect ? Math.round(caretRect.left - toggleRect.left) : 0,
    };
  });
}

async function readReaderActionLayout(page) {
  return page.locator('#reader-action-controls').evaluate((node) => {
    const style = window.getComputedStyle(node);
    return {
      borderLeftWidth: style.borderLeftWidth,
      marginLeft: style.marginLeft,
      clientWidth: Math.round(node.clientWidth || 0),
      scrollWidth: Math.round(node.scrollWidth || 0),
    };
  });
}

async function readBackgroundLuminance(page, selector) {
  return page.locator(selector).evaluate((node) => {
    const color = window.getComputedStyle(node).backgroundColor || '';
    const match = color.match(/[\d.]+/g) || [];
    let [red = 0, green = 0, blue = 0] = match.map(Number);
    if (Math.max(red, green, blue) <= 1.2) {
      red *= 255;
      green *= 255;
      blue *= 255;
    }
    return {
      color,
      luminance: Math.round((red * 299 + green * 587 + blue * 114) / 1000),
    };
  });
}

async function readLocatorBackgroundLuminance(locator) {
  return locator.evaluate((node) => {
    const color = window.getComputedStyle(node).backgroundColor || '';
    const match = color.match(/[\d.]+/g) || [];
    let [red = 0, green = 0, blue = 0] = match.map(Number);
    if (Math.max(red, green, blue) <= 1.2) {
      red *= 255;
      green *= 255;
      blue *= 255;
    }
    return {
      color,
      luminance: Math.round((red * 299 + green * 587 + blue * 114) / 1000),
    };
  });
}

async function openMailFilters(page) {
  await page.locator('#mail-view-menu > summary').click();
  await page.click('#btn-mail-filters');
  await page.waitForTimeout(150);
  return page.locator('#mail-filter-advanced').evaluate((node) => !node.classList.contains('hidden'));
}

function threadedMailFixture() {
  const threadItems = [
    {
      id: 'm0',
      mailbox: 'Sent Messages',
      from: 'OpenAI <noreply@email.openai.com>',
      subject: 'Updates to OpenAI Privacy Policy',
      date: '2026-03-06T05:21:16.000Z',
      seen: true,
      answered: false,
      flagged: false,
      preview: 'Initial outbound note with policy update context.',
      thread_id: 'thread-1',
    },
    {
      id: 'm1',
      mailbox: 'INBOX',
      from: 'webmaster@2h4s2d.ru',
      subject: 'Fwd: Updates to OpenAI Privacy Policy',
      date: '2026-03-06T08:34:00.000Z',
      seen: true,
      answered: true,
      flagged: false,
      preview: 'On 06/03/2026, 08:21:16, \"KanzlerRoss\" <uristishko82@gmail.com> wrote: >>> Begin forwarded message...',
      thread_id: 'thread-1',
    },
    {
      id: 'm2',
      mailbox: 'INBOX',
      from: 'webmaster@2h4s2d.ru',
      subject: 'Re: Updates to OpenAI Privacy Policy',
      date: '2026-03-07T06:10:00.000Z',
      seen: true,
      answered: true,
      flagged: false,
      preview: 'Following up on the forwarded copy from yesterday.',
      thread_id: 'thread-1',
    },
    {
      id: 'm3',
      mailbox: 'Archive',
      from: 'OpenAI <noreply@email.openai.com>',
      subject: 'Updates to OpenAI Privacy Policy',
      date: '2026-03-08T09:55:00.000Z',
      seen: true,
      answered: false,
      flagged: true,
      preview: 'Final archived copy with the original sender retained.',
      thread_id: 'thread-1',
    },
  ];

  const messageDetails = {
    m0: {
      id: 'm0',
      mailbox: 'Sent Messages',
      from: 'OpenAI <noreply@email.openai.com>',
      to: ['webmaster@2h4s2d.ru'],
      subject: 'Updates to OpenAI Privacy Policy',
      date: '2026-03-06T05:21:16.000Z',
      seen: true,
      flagged: false,
      answered: false,
      body: 'Initial outbound note with policy update context.',
      body_html: '',
      attachments: [],
    },
    m1: {
      id: 'm1',
      mailbox: 'INBOX',
      from: 'webmaster@2h4s2d.ru',
      to: ['team@example.com'],
      subject: 'Fwd: Updates to OpenAI Privacy Policy',
      date: '2026-03-06T08:34:00.000Z',
      seen: true,
      flagged: false,
      answered: true,
      body: 'Forwarded copy of the original policy update.',
      body_html: '',
      attachments: [],
    },
    m2: {
      id: 'm2',
      mailbox: 'INBOX',
      from: 'webmaster@2h4s2d.ru',
      to: ['team@example.com'],
      subject: 'Re: Updates to OpenAI Privacy Policy',
      date: '2026-03-07T06:10:00.000Z',
      seen: true,
      flagged: false,
      answered: true,
      body: 'Following up on the forwarded copy from yesterday.',
      body_html: '',
      attachments: [],
    },
    m3: {
      id: 'm3',
      mailbox: 'Archive',
      from: 'OpenAI <noreply@email.openai.com>',
      to: ['webmaster@2h4s2d.ru'],
      subject: 'Updates to OpenAI Privacy Policy',
      date: '2026-03-08T09:55:00.000Z',
      seen: true,
      flagged: true,
      answered: false,
      body: 'Final archived copy with the original sender retained.',
      body_html: '',
      attachments: [],
    },
  };

  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    mailboxes: [
      { name: 'INBOX', role: 'inbox', unread: 1, messages: 8 },
      { name: 'Sent Messages', role: 'sent', unread: 0, messages: 4 },
      { name: 'Archive', role: 'archive', unread: 0, messages: 12 },
    ],
    mailboxItems: [threadItems[1], threadItems[2]],
    threadItems,
    messageDetails,
  };
}

function composeReliabilityFixture() {
  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    mailboxes: [
      { name: 'INBOX', role: 'inbox', unread: 1, messages: 1 },
      { name: 'Sent', role: 'sent', unread: 0, messages: 0 },
      { name: 'Drafts', role: 'drafts', unread: 0, messages: 0 },
    ],
    mailboxItems: [
      {
        id: 'm-reply-1',
        mailbox: 'INBOX',
        from: 'sender@example.com',
        subject: 'Draft reliability check',
        date: '2026-03-08T09:55:00.000Z',
        seen: true,
        answered: false,
        flagged: false,
        preview: 'Please reply with the latest numbers.',
        thread_id: 'thread-reply-1',
      },
    ],
    messageDetails: {
      'm-reply-1': {
        id: 'm-reply-1',
        mailbox: 'INBOX',
        from: 'sender@example.com',
        to: ['admin@example.com'],
        subject: 'Draft reliability check',
        date: '2026-03-08T09:55:00.000Z',
        seen: true,
        flagged: false,
        answered: false,
        body: 'Please reply with the latest numbers.',
        body_html: '',
        attachments: [],
      },
    },
  };
}

function indexedLockedFixture() {
  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: true,
    },
    account: {
      id: 'acct-indexed',
      user_id: 'user-admin',
      display_name: 'Indexed Mail',
      login: 'indexed@example.com',
      is_default: true,
      status: 'active',
      last_sync_at: '2026-03-10T09:00:00.000Z',
      last_error: '',
    },
    identities: [
      {
        account_id: '',
        account_display_name: 'Session sender',
        account_login: 'admin@example.com',
        account_is_default: true,
        identity_id: 'session-admin',
        identity_display_name: 'Admin Session',
        from_email: 'admin@example.com',
        reply_to: '',
        signature_text: '',
        signature_html: '',
        identity_is_default: true,
        is_default: true,
        is_session: true,
      },
      {
        account_id: 'acct-indexed',
        account_display_name: 'Indexed Mail',
        account_login: 'indexed@example.com',
        account_is_default: true,
        identity_id: 'ident-indexed',
        identity_display_name: 'Indexed Sender',
        from_email: 'indexed@example.com',
        reply_to: '',
        signature_text: '',
        signature_html: '<p>Indexed Signature</p>',
        identity_is_default: true,
        is_default: true,
        is_session: false,
      },
    ],
    mailboxes: [
      { name: 'INBOX', role: 'inbox', unread: 1, messages: 1 },
    ],
    messageSummary: {
      id: 'idx-msg-1',
      mailbox: 'INBOX',
      from: 'Sender <sender@example.com>',
      subject: 'Indexed reply target',
      date: '2026-03-10T08:00:00.000Z',
      seen: false,
      answered: false,
      flagged: false,
      preview: 'Please send the latest indexed update.',
      thread_id: 'idx-thread-1',
      account_id: 'acct-indexed',
    },
    messageDetail: {
      id: 'idx-msg-1',
      account_id: 'acct-indexed',
      mailbox: 'INBOX',
      uid: 11,
      thread_id: 'idx-thread-1',
      from: 'Sender <sender@example.com>',
      to: 'indexed@example.com',
      cc: '',
      bcc: '',
      subject: 'Indexed reply target',
      date: '2026-03-10T08:00:00.000Z',
      seen: false,
      flagged: false,
      answered: false,
      body: 'Please send the latest indexed update.',
      body_html: '',
    },
  };
}

function mailIdentityFixture() {
  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    sessionProfile: {
      id: 'session-admin',
      user_id: 'user-admin',
      from_email: 'admin@example.com',
      display_name: 'Admin Session',
      reply_to: 'session-reply@example.com',
      signature_text: '-- \nSession Signature',
      signature_html: '<p>Session Signature</p>',
      created_at: '2026-03-09T09:00:00.000Z',
      updated_at: '2026-03-09T09:00:00.000Z',
    },
    accounts: [
      {
        id: 'acct-support',
        user_id: 'user-admin',
        display_name: 'Support Account',
        login: 'support-login@example.com',
        imap_host: 'imap.example.com',
        imap_port: 993,
        imap_tls: true,
        imap_starttls: false,
        smtp_host: 'smtp.example.com',
        smtp_port: 587,
        smtp_tls: false,
        smtp_starttls: true,
        is_default: false,
      },
    ],
    identities: {
      'acct-support': [
        {
          id: 'ident-support',
          account_id: 'acct-support',
          display_name: 'Support Team',
          from_email: 'support@example.com',
          reply_to: 'support-reply@example.com',
          signature_text: '-- \nSupport Signature',
          signature_html: '<p>Support Signature</p>',
          is_default: true,
        },
      ],
    },
    inboxMessage: {
      id: 'm-ident-1',
      mailbox: 'INBOX',
      from: 'alice@example.com',
      to: ['admin@example.com'],
      subject: 'Identity workflow check',
      date: '2026-03-09T12:00:00.000Z',
      seen: false,
      flagged: false,
      answered: false,
      preview: 'Please reply from the right team alias.',
      body: 'Please reply from the right team alias.',
      body_html: '',
      attachments: [],
      thread_id: 'thread-ident-1',
    },
  };
}

function reliableMailboxStateFixture() {
  const original = {
    id: 'm-live-1',
    mailbox: 'INBOX',
    from: 'sender@example.com',
    to: ['admin@example.com'],
    subject: 'Reliable mailbox state',
    date: '2026-03-09T09:00:00.000Z',
    seen: false,
    flagged: false,
    answered: false,
    preview: 'Please send the latest update and keep the thread intact.',
    body: 'Please send the latest update and keep the thread intact.',
    body_html: '',
    attachments: [],
    thread_id: 'thread-live-1',
  };
  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    mailboxes: [
      { name: 'INBOX', role: 'inbox' },
      { name: 'Projects', role: '' },
    ],
    messages: [original],
  };
}

function mailActionFixture() {
  const messages = [
    {
      id: 'm1',
      mailbox: 'INBOX',
      from: 'alice@example.com',
      to: ['admin@example.com'],
      subject: 'Action one',
      date: '2026-03-08T09:00:00.000Z',
      seen: true,
      flagged: false,
      answered: false,
      preview: 'First message preview.',
      body: 'First message body.',
      body_html: '',
      attachments: [],
      thread_id: 'bulk-1',
    },
    {
      id: 'm2',
      mailbox: 'INBOX',
      from: 'bob@example.com',
      to: ['admin@example.com'],
      subject: 'Action two',
      date: '2026-03-08T09:30:00.000Z',
      seen: true,
      flagged: false,
      answered: false,
      preview: 'Second message preview.',
      body: 'Second message body.',
      body_html: '',
      attachments: [],
      thread_id: 'bulk-2',
    },
    {
      id: 'm3',
      mailbox: 'INBOX',
      from: 'carol@example.com',
      to: ['admin@example.com'],
      subject: 'Action three',
      date: '2026-03-08T10:00:00.000Z',
      seen: false,
      flagged: false,
      answered: false,
      preview: 'Third message preview.',
      body: 'Third message body.',
      body_html: '',
      attachments: [],
      thread_id: 'bulk-3',
    },
    {
      id: 'm4',
      mailbox: 'INBOX',
      from: 'dave@example.com',
      to: ['admin@example.com'],
      subject: 'Action four',
      date: '2026-03-08T10:30:00.000Z',
      seen: true,
      flagged: false,
      answered: false,
      preview: 'Fourth message preview.',
      body: 'Fourth message body.',
      body_html: '',
      attachments: [],
      thread_id: 'bulk-4',
    },
  ];

  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    mailboxes: [
      { name: 'INBOX', role: 'inbox' },
      { name: 'Projects', role: '' },
    ],
    messages,
  };
}

function denseMailActionFixture(count = 28) {
  const base = mailActionFixture();
  const messages = [];
  for (let index = 0; index < count; index += 1) {
    const seq = index + 1;
    const minute = String(seq).padStart(2, '0');
    messages.push({
      id: `mx-${seq}`,
      mailbox: 'INBOX',
      from: `sender${seq}@example.com`,
      to: ['admin@example.com'],
      subject: `Dense row ${seq}`,
      date: `2026-03-08T10:${minute}:00.000Z`,
      seen: seq % 3 !== 0,
      flagged: seq % 5 === 0,
      answered: false,
      preview: `Preview text for dense row ${seq}.`,
      body: `Body for dense row ${seq}.`,
      body_html: '',
      attachments: [],
      thread_id: `dense-${seq}`,
    });
  }
  return {
    ...base,
    messages,
  };
}

function parseMultipartRequest(request) {
  const header = request.headers()['content-type'] || '';
  const match = header.match(/boundary=(.+)$/i);
  if (!match) {
    return { files: [], fields: {} };
  }
  const boundary = `--${match[1]}`;
  const raw = request.postDataBuffer() || Buffer.alloc(0);
  const text = raw.toString('latin1');
  const parts = text.split(boundary).slice(1, -1);
  const files = [];
  const fields = {};
  for (const part of parts) {
    const trimmed = part.replace(/^\r\n/, '').replace(/\r\n$/, '');
    const sep = trimmed.indexOf('\r\n\r\n');
    if (sep < 0) continue;
    const headers = trimmed.slice(0, sep);
    const body = trimmed.slice(sep + 4).replace(/\r\n$/, '');
    const nameMatch = headers.match(/name="([^"]+)"/i);
    if (!nameMatch) continue;
    const field = nameMatch[1];
    const filenameMatch = headers.match(/filename="([^"]*)"/i);
    const typeMatch = headers.match(/content-type:\s*([^\r\n]+)/i);
    if (filenameMatch) {
      files.push({
        field,
        filename: filenameMatch[1],
        contentType: typeMatch ? typeMatch[1].trim() : 'application/octet-stream',
        data: Buffer.from(body, 'latin1'),
      });
      continue;
    }
    if (!fields[field]) fields[field] = [];
    fields[field].push(body);
  }
  return { files, fields };
}

async function mockComposeReliabilityScenario(page, options = {}) {
  const fixture = composeReliabilityFixture();
  const opts = {
    delayFirstPatchMs: 0,
    ...options,
  };
  const drafts = new Map();
  const attachmentBodies = new Map();
  let draftSeq = 1;
  let attachmentSeq = 1;
  let sendAttempts = 0;
  let patchCount = 0;
  const runtime = {
    lastSendDraft: null,
  };

  const draftList = () => Array
    .from(drafts.values())
    .filter((item) => String(item.status || '').toLowerCase() !== 'sent')
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime());

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/mail/senders') {
      return ok({
        items: [{
          id: 'sender-primary',
          kind: 'primary',
          name: 'Admin',
          from_email: fixture.user.email,
          reply_to: '',
          signature_text: '',
          signature_html: '',
          account_id: 'acct-primary',
          account_label: 'Primary Account',
          account_login: fixture.user.email,
          is_default: true,
          is_primary: true,
          can_delete: false,
          can_schedule: true,
          status: 'ok',
        }],
      });
    }
    if (path === '/api/v1/mailboxes') {
      const draftCount = draftList().length;
      return ok(fixture.mailboxes.map((item) => (
        item.role === 'drafts'
          ? { ...item, messages: draftCount }
          : item
      )));
    }
    if (path === '/api/v1/messages') return ok({ items: fixture.mailboxItems });
    if (path === '/api/v1/threads/thread-reply-1/messages') {
      return ok({
        thread_id: 'thread-reply-1',
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: fixture.mailboxItems,
      });
    }
    if (path === '/api/v1/compose/identities') return ok({ items: [] });
    if (path === '/api/v2/drafts') {
      if (route.request().method() === 'GET') {
        return ok({ items: draftList(), page: 1, page_size: 100, total: draftList().length });
      }
      if (route.request().method() === 'POST') {
        const payload = route.request().postDataJSON();
        const draft = {
          id: `draft-${draftSeq++}`,
          account_id: payload.account_id || '',
          sender_profile_id: payload.sender_profile_id || '',
          identity_id: payload.identity_id || '',
          compose_mode: payload.compose_mode || 'send',
          context_message_id: payload.context_message_id || '',
          context_account_id: payload.context_account_id || '',
          from_mode: payload.from_mode || 'default',
          from_manual: payload.from_manual || '',
          client_state_json: payload.client_state_json || '',
          to: payload.to || '',
          cc: payload.cc || '',
          bcc: payload.bcc || '',
          subject: payload.subject || '',
          body_text: payload.body_text || '',
          body_html: payload.body_html || '',
          attachments_json: '[]',
          send_mode: payload.send_mode || '',
          scheduled_for: payload.scheduled_for || '',
          status: payload.status || 'active',
          last_send_error: payload.last_send_error || '',
          created_at: '2026-03-09T10:00:00.000Z',
          updated_at: new Date().toISOString(),
        };
        drafts.set(draft.id, draft);
        return ok(draft, { status: 201 });
      }
    }
    const draftAttachmentGet = path.match(/^\/api\/v2\/drafts\/([^/]+)\/attachments\/([^/]+)$/);
    if (draftAttachmentGet && route.request().method() === 'GET') {
      const draftID = decodeURIComponent(draftAttachmentGet[1]);
      const attachmentID = decodeURIComponent(draftAttachmentGet[2]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      const items = JSON.parse(draft.attachments_json || '[]');
      const item = items.find((entry) => entry.id === attachmentID);
      if (!item) return ok({ error: 'draft_attachment_not_found' }, { status: 404 });
      const body = attachmentBodies.get(`${draftID}:${attachmentID}`) || Buffer.alloc(0);
      return ok(null, {
        status: 200,
        contentType: item.content_type || 'application/octet-stream',
        rawBody: body,
      });
    }
    const draftAttachmentDelete = path.match(/^\/api\/v2\/drafts\/([^/]+)\/attachments\/([^/]+)$/);
    if (draftAttachmentDelete && route.request().method() === 'DELETE') {
      const draftID = decodeURIComponent(draftAttachmentDelete[1]);
      const attachmentID = decodeURIComponent(draftAttachmentDelete[2]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      const items = JSON.parse(draft.attachments_json || '[]').filter((entry) => entry.id !== attachmentID);
      draft.attachments_json = JSON.stringify(items);
      draft.updated_at = new Date().toISOString();
      draft.status = 'active';
      draft.last_send_error = '';
      attachmentBodies.delete(`${draftID}:${attachmentID}`);
      drafts.set(draftID, draft);
      return ok({ draft, items });
    }
    const draftAttachmentPost = path.match(/^\/api\/v2\/drafts\/([^/]+)\/attachments$/);
    if (draftAttachmentPost && route.request().method() === 'POST') {
      const draftID = decodeURIComponent(draftAttachmentPost[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      const parsed = parseMultipartRequest(route.request());
      const current = JSON.parse(draft.attachments_json || '[]');
      const uploaded = [];
      let inlineIndex = 0;
      for (const file of parsed.files) {
        const inline = file.field === 'inline_images';
        const contentID = inline ? (parsed.fields.inline_image_cids?.[inlineIndex++] || `cid-${attachmentSeq}`) : '';
        const nextID = `att-${attachmentSeq++}`;
        const next = {
          id: nextID,
          filename: file.filename,
          content_type: file.contentType,
          size_bytes: file.data.length,
          inline_part: inline,
          content_id: contentID,
          sort_order: current.length,
        };
        attachmentBodies.set(`${draftID}:${nextID}`, Buffer.from(file.data));
        current.push(next);
        uploaded.push(next);
      }
      draft.attachments_json = JSON.stringify(current);
      draft.updated_at = new Date().toISOString();
      draft.status = 'active';
      draft.last_send_error = '';
      drafts.set(draftID, draft);
      return ok({ draft, items: current, uploaded }, { status: 201 });
    }
    const draftMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)$/);
    if (draftMatch) {
      const draftID = decodeURIComponent(draftMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      if (route.request().method() === 'GET') return ok(draft);
      if (route.request().method() === 'PATCH') {
        const payload = route.request().postDataJSON();
        patchCount += 1;
        if (opts.delayFirstPatchMs > 0 && patchCount === 1) {
          await new Promise((resolve) => setTimeout(resolve, opts.delayFirstPatchMs));
        }
        Object.assign(draft, payload, { updated_at: new Date().toISOString() });
        drafts.set(draftID, draft);
        return ok(draft);
      }
      if (route.request().method() === 'DELETE') {
        drafts.delete(draftID);
        return ok({ status: 'deleted' });
      }
    }
    const draftSendMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)\/send$/);
    if (draftSendMatch) {
      const draftID = decodeURIComponent(draftSendMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      runtime.lastSendDraft = { ...draft };
      if (String(draft.send_mode || '').trim().toLowerCase() === 'scheduled' && String(draft.scheduled_for || '').trim()) {
        draft.status = 'scheduled';
        draft.last_send_error = '';
        draft.updated_at = new Date().toISOString();
        drafts.set(draftID, draft);
        return ok({ status: 'scheduled', scheduled_for: draft.scheduled_for, draft_id: draft.id });
      }
      sendAttempts += 1;
      if (sendAttempts === 1) {
        draft.status = 'failed';
        draft.last_send_error = 'temporary smtp outage';
        draft.updated_at = new Date().toISOString();
        drafts.set(draftID, draft);
        return ok({ code: 'send_failed', message: 'temporary smtp outage' }, { status: 502 });
      }
      draft.status = 'sent';
      draft.last_send_error = '';
      draft.updated_at = new Date().toISOString();
      drafts.set(draftID, draft);
      return ok({ status: 'sent', saved_copy: true, saved_copy_mailbox: 'Sent' });
    }
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const messageID = decodeURIComponent(messageMatch[1]);
      const message = fixture.messageDetails[messageID];
      if (message) return ok(message);
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });

  return runtime;
}

async function mockIndexedAccountLockedScenario(page) {
  const fixture = indexedLockedFixture();
  const drafts = new Map();
  const counters = {
    unlockCalls: 0,
    legacyMailboxCalls: 0,
    legacyMessageCalls: 0,
    accountSpecialCalls: 0,
  };
  let draftSeq = 1;
  let sentConfigured = false;

  const draftList = () => Array
    .from(drafts.values())
    .filter((item) => String(item.status || '').toLowerCase() !== 'sent')
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime());

  const accountMailboxes = () => {
    const items = fixture.mailboxes.map((item) => ({ ...item }));
    if (sentConfigured && !items.some((item) => item.name === 'Sent')) {
      items.push({ name: 'Sent', role: 'sent', unread: 0, messages: 0 });
    }
    return items;
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/mail/senders') {
      return ok({
        items: [
          {
            id: 'session-admin',
            kind: 'primary',
            name: 'Admin Session',
            from_email: fixture.user.email,
            reply_to: '',
            signature_text: '',
            signature_html: '',
            account_id: '',
            account_label: 'Session sender',
            account_login: fixture.user.email,
            is_default: false,
            is_primary: true,
            can_delete: false,
            can_schedule: true,
            status: 'ok',
          },
          {
            id: 'ident-indexed',
            kind: 'identity',
            name: 'Indexed Sender',
            from_email: 'indexed@example.com',
            reply_to: '',
            signature_text: '',
            signature_html: '<p>Indexed Signature</p>',
            account_id: fixture.account.id,
            account_label: fixture.account.display_name,
            account_login: fixture.account.login,
            is_default: true,
            is_primary: false,
            account_is_default: true,
            can_delete: false,
            can_schedule: true,
            status: 'ok',
          },
        ],
      });
    }
    if (path === '/api/v2/accounts') return ok({ items: [fixture.account] });
    if (path === '/api/v2/saved-searches') return ok({ items: [] });
    if (path === '/api/v1/compose/identities') {
      return ok({
        auth_email: fixture.user.email,
        manual_fallback_required: false,
        items: fixture.identities,
      });
    }
    if (path === '/api/v1/session/mail-secret/unlock') {
      counters.unlockCalls += 1;
      return ok({ error: 'unlock should not be required' }, { status: 500 });
    }
    if (path === '/api/v1/mailboxes') {
      counters.legacyMailboxCalls += 1;
      return ok({ error: 'legacy mailbox path should not be used' }, { status: 500 });
    }
    if (path === '/api/v1/messages' || path === '/api/v1/search') {
      counters.legacyMessageCalls += 1;
      return ok({ error: 'legacy message path should not be used' }, { status: 500 });
    }
    if (path === `/api/v2/accounts/${fixture.account.id}/mailboxes`) return ok(accountMailboxes());
    if (path === `/api/v2/accounts/${fixture.account.id}/mailboxes/special/sent`) {
      counters.accountSpecialCalls += 1;
      sentConfigured = true;
      return ok({
        status: 'ok',
        role: 'sent',
        mailbox_name: 'Sent',
        created: true,
        items: [{ role: 'sent', mailbox_name: 'Sent' }],
        mailboxes: accountMailboxes(),
      });
    }
    if (path === '/api/v2/messages') {
      return ok({ items: [fixture.messageSummary], total: 1 });
    }
    if (path === `/api/v2/messages/${fixture.messageSummary.id}`) {
      return ok({ message: fixture.messageDetail, attachments: [] });
    }
    if (path === '/api/v2/messages/bulk') {
      return ok({ status: 'ok', applied: [fixture.messageSummary.id], failed: [] });
    }
    if (path === `/api/v2/threads/${fixture.messageSummary.thread_id}`) {
      return ok({ id: fixture.messageSummary.thread_id, items: [fixture.messageSummary] });
    }
    if (path === '/api/v2/drafts' && route.request().method() === 'GET') {
      return ok({ items: draftList(), page: 1, page_size: 100, total: draftList().length });
    }
    if (path === '/api/v2/drafts' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON();
      const draft = {
        id: `draft-${draftSeq++}`,
        account_id: payload.account_id || '',
        identity_id: payload.identity_id || '',
        compose_mode: payload.compose_mode || 'send',
        context_message_id: payload.context_message_id || '',
        context_account_id: payload.context_account_id || '',
        from_mode: payload.from_mode || 'default',
        from_manual: payload.from_manual || '',
        client_state_json: payload.client_state_json || '',
        to: payload.to || '',
        cc: payload.cc || '',
        bcc: payload.bcc || '',
        subject: payload.subject || '',
        body_text: payload.body_text || '',
        body_html: payload.body_html || '',
        attachments_json: '[]',
        status: payload.status || 'active',
        last_send_error: '',
        created_at: '2026-03-10T09:00:00.000Z',
        updated_at: new Date().toISOString(),
      };
      drafts.set(draft.id, draft);
      return ok(draft, { status: 201 });
    }
    const draftMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)$/);
    if (draftMatch && route.request().method() === 'PATCH') {
      const draftID = decodeURIComponent(draftMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      Object.assign(draft, route.request().postDataJSON(), { updated_at: new Date().toISOString() });
      drafts.set(draftID, draft);
      return ok(draft);
    }
    const draftSendMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)\/send$/);
    if (draftSendMatch) {
      const draftID = decodeURIComponent(draftSendMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      draft.status = 'sent';
      draft.updated_at = new Date().toISOString();
      drafts.set(draftID, draft);
      sentConfigured = true;
      return ok({ status: 'sent', saved_copy: true, saved_copy_mailbox: 'Sent' });
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });

  return counters;
}

async function mockIndexedSentRefreshLagScenario(page) {
  const fixture = {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    account: {
      id: 'acct-sent-lag',
      user_id: 'user-admin',
      display_name: 'Laggy Indexed Mail',
      login: 'indexed@example.com',
      is_default: true,
      status: 'active',
      last_sync_at: '2026-03-10T09:00:00.000Z',
      last_error: '',
    },
    senders: [{
      id: 'sender-indexed',
      kind: 'primary',
      name: 'Indexed Sender',
      from_email: 'indexed@example.com',
      reply_to: '',
      signature_text: '',
      signature_html: '',
      account_id: 'acct-sent-lag',
      account_label: 'Laggy Indexed Mail',
      account_login: 'indexed@example.com',
      is_default: true,
      is_primary: true,
      can_delete: false,
      can_schedule: true,
      status: 'ok',
    }],
    mailboxes: [
      { name: 'INBOX', role: 'inbox', unread: 1, messages: 1 },
      { name: 'Sent', role: 'sent', unread: 0, messages: 0 },
    ],
    inboxSummary: {
      id: 'idx-lag-1',
      mailbox: 'INBOX',
      from: 'Sender <sender@example.com>',
      subject: 'Laggy sent refresh target',
      date: '2026-03-10T08:00:00.000Z',
      seen: false,
      answered: false,
      flagged: false,
      preview: 'Watch the Sent view while the index catches up.',
      thread_id: 'idx-lag-thread-1',
      account_id: 'acct-sent-lag',
    },
    inboxDetail: {
      id: 'idx-lag-1',
      account_id: 'acct-sent-lag',
      mailbox: 'INBOX',
      uid: 11,
      thread_id: 'idx-lag-thread-1',
      from: 'Sender <sender@example.com>',
      to: 'indexed@example.com',
      cc: '',
      bcc: '',
      subject: 'Laggy sent refresh target',
      date: '2026-03-10T08:00:00.000Z',
      seen: false,
      flagged: false,
      answered: false,
      body: 'Watch the Sent view while the index catches up.',
      body_html: '',
    },
  };
  const drafts = new Map();
  let draftSeq = 1;

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/accounts') return ok({ items: [fixture.account] });
    if (path === '/api/v2/saved-searches') return ok({ items: [] });
    if (path === '/api/v2/accounts/health') return ok({ summary: null, items: [] });
    if (path === '/api/v2/mail/senders') return ok({ items: fixture.senders });
    if (path === `/api/v2/accounts/${fixture.account.id}/mailboxes`) return ok(fixture.mailboxes);
    if (path === '/api/v2/messages') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      if (mailbox === 'Sent') return ok({ items: [], total: 0 });
      return ok({ items: [fixture.inboxSummary], total: 1 });
    }
    if (path === `/api/v2/messages/${fixture.inboxSummary.id}`) {
      return ok({ message: fixture.inboxDetail, attachments: [] });
    }
    if (path === '/api/v2/drafts' && route.request().method() === 'GET') {
      const items = Array.from(drafts.values()).filter((item) => String(item.status || '').toLowerCase() !== 'sent');
      return ok({ items, page: 1, page_size: 100, total: items.length });
    }
    if (path === '/api/v2/drafts' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON();
      const draft = {
        id: `draft-sent-lag-${draftSeq++}`,
        account_id: payload.account_id || fixture.account.id,
        identity_id: payload.identity_id || '',
        sender_profile_id: payload.sender_profile_id || 'sender-indexed',
        compose_mode: payload.compose_mode || 'send',
        context_message_id: payload.context_message_id || '',
        context_account_id: payload.context_account_id || '',
        from_mode: payload.from_mode || 'sender',
        from_manual: payload.from_manual || '',
        client_state_json: payload.client_state_json || '',
        to: payload.to || '',
        cc: payload.cc || '',
        bcc: payload.bcc || '',
        subject: payload.subject || '',
        body_text: payload.body_text || '',
        body_html: payload.body_html || '',
        attachments_json: '[]',
        status: payload.status || 'active',
        last_send_error: '',
        created_at: '2026-03-10T09:00:00.000Z',
        updated_at: new Date().toISOString(),
      };
      drafts.set(draft.id, draft);
      return ok(draft, { status: 201 });
    }
    const draftMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)$/);
    if (draftMatch && route.request().method() === 'PATCH') {
      const draftID = decodeURIComponent(draftMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      Object.assign(draft, route.request().postDataJSON(), { updated_at: new Date().toISOString() });
      drafts.set(draftID, draft);
      return ok(draft);
    }
    const draftSendMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)\/send$/);
    if (draftSendMatch) {
      const draftID = decodeURIComponent(draftSendMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      draft.status = 'sent';
      draft.updated_at = new Date().toISOString();
      drafts.set(draftID, draft);
      return ok({ status: 'sent', saved_copy: true, saved_copy_mailbox: 'Sent' });
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockMailIdentityScenario(page) {
  const fixture = mailIdentityFixture();
  const drafts = new Map();
  const sentMessages = [];
  let draftSeq = 1;
  let sentSeq = 1;

  const composeIdentityItems = () => {
    const items = [{
      account_id: '',
      account_display_name: 'Session sender',
      account_login: fixture.sessionProfile.from_email,
      account_is_default: true,
      identity_id: fixture.sessionProfile.id,
      identity_display_name: fixture.sessionProfile.display_name,
      from_email: fixture.sessionProfile.from_email,
      reply_to: fixture.sessionProfile.reply_to,
      signature_text: fixture.sessionProfile.signature_text,
      signature_html: fixture.sessionProfile.signature_html,
      identity_is_default: true,
      is_default: true,
      is_session: true,
    }];
    for (const account of fixture.accounts) {
      for (const identity of fixture.identities[account.id] || []) {
        items.push({
          account_id: account.id,
          account_display_name: account.display_name,
          account_login: account.login,
          account_is_default: !!account.is_default,
          identity_id: identity.id,
          identity_display_name: identity.display_name,
          from_email: identity.from_email,
          reply_to: identity.reply_to,
          signature_text: identity.signature_text,
          signature_html: identity.signature_html,
          identity_is_default: !!identity.is_default,
          is_default: !!identity.is_default,
          is_session: false,
        });
      }
    }
    return items;
  };

  const draftList = () => Array
    .from(drafts.values())
    .filter((item) => String(item.status || '').toLowerCase() !== 'sent')
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime());

  const mailboxItems = (mailbox) => {
    if (mailbox === 'Sent') return [...sentMessages];
    if (mailbox === 'Drafts') return [];
    if (!mailbox || mailbox === 'INBOX') return [fixture.inboxMessage];
    return [];
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/mail/session-profile') {
      if (route.request().method() === 'GET') return ok(fixture.sessionProfile);
      if (route.request().method() === 'PATCH') {
        const payload = route.request().postDataJSON();
        fixture.sessionProfile = {
          ...fixture.sessionProfile,
          display_name: payload.display_name ?? fixture.sessionProfile.display_name,
          reply_to: payload.reply_to ?? fixture.sessionProfile.reply_to,
          signature_html: payload.signature_html ?? fixture.sessionProfile.signature_html,
          signature_text: payload.signature_text ?? fixture.sessionProfile.signature_text,
          updated_at: new Date().toISOString(),
        };
        if (!payload.signature_text && payload.signature_html) {
          const plain = String(payload.signature_html).replace(/<[^>]+>/g, '').trim();
          fixture.sessionProfile.signature_text = plain ? `-- \n${plain}` : '';
        }
        return ok(fixture.sessionProfile);
      }
    }
    if (path === '/api/v2/accounts') {
      if (route.request().method() === 'GET') return ok({ items: fixture.accounts });
    }
    const accountIdentities = path.match(/^\/api\/v2\/accounts\/([^/]+)\/identities$/);
    if (accountIdentities) {
      const accountID = decodeURIComponent(accountIdentities[1]);
      if (route.request().method() === 'GET') {
        return ok({ items: fixture.identities[accountID] || [] });
      }
    }
    if (path === '/api/v1/compose/identities') {
      return ok({
        auth_email: fixture.user.email,
        manual_fallback_required: false,
        items: composeIdentityItems(),
      });
    }
    if (path === '/api/v1/mailboxes') {
      return ok([
        { name: 'INBOX', role: 'inbox', unread: fixture.inboxMessage.seen ? 0 : 1, messages: 1 },
        { name: 'Sent', role: 'sent', unread: 0, messages: sentMessages.length },
        { name: 'Drafts', role: 'drafts', unread: 0, messages: draftList().length },
      ]);
    }
    if (path === '/api/v1/messages') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      return ok({ items: mailboxItems(mailbox) });
    }
    if (path === '/api/v1/threads/thread-ident-1/messages') {
      return ok({
        thread_id: 'thread-ident-1',
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: [fixture.inboxMessage, ...sentMessages.filter((item) => item.thread_id === 'thread-ident-1')],
      });
    }
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const messageID = decodeURIComponent(messageMatch[1]);
      const message = [fixture.inboxMessage, ...sentMessages].find((item) => item.id === messageID);
      if (message) return ok(message);
    }
    const messageFlagsMatch = path.match(/^\/api\/v1\/messages\/([^/]+)\/flags$/);
    if (messageFlagsMatch && route.request().method() === 'POST') {
      const messageID = decodeURIComponent(messageFlagsMatch[1]);
      const payload = route.request().postDataJSON() || {};
      if (messageID === fixture.inboxMessage.id) {
        const add = Array.isArray(payload.add) ? payload.add : [];
        const remove = Array.isArray(payload.remove) ? payload.remove : [];
        if (add.includes('\\Seen')) fixture.inboxMessage.seen = true;
        if (remove.includes('\\Seen')) fixture.inboxMessage.seen = false;
      }
      return ok({ status: 'ok' });
    }
    if (path === '/api/v2/drafts') {
      if (route.request().method() === 'GET') {
        return ok({ items: draftList(), page: 1, page_size: 100, total: draftList().length });
      }
      if (route.request().method() === 'POST') {
        const payload = route.request().postDataJSON();
        const draft = {
          id: `draft-ident-${draftSeq++}`,
          account_id: payload.account_id || '',
          identity_id: payload.identity_id || '',
          compose_mode: payload.compose_mode || 'send',
          context_message_id: payload.context_message_id || '',
          from_mode: payload.from_mode || 'default',
          from_manual: payload.from_manual || '',
          client_state_json: payload.client_state_json || '',
          to: payload.to || '',
          cc: payload.cc || '',
          bcc: payload.bcc || '',
          subject: payload.subject || '',
          body_text: payload.body_text || '',
          body_html: payload.body_html || '',
          attachments_json: '[]',
          status: payload.status || 'active',
          last_send_error: '',
          created_at: '2026-03-09T12:30:00.000Z',
          updated_at: new Date().toISOString(),
        };
        drafts.set(draft.id, draft);
        return ok(draft, { status: 201 });
      }
    }
    const draftMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)$/);
    if (draftMatch) {
      const draftID = decodeURIComponent(draftMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      if (route.request().method() === 'GET') return ok(draft);
      if (route.request().method() === 'PATCH') {
        const payload = route.request().postDataJSON();
        Object.assign(draft, payload, { updated_at: new Date().toISOString(), status: 'active', last_send_error: '' });
        drafts.set(draftID, draft);
        return ok(draft);
      }
      if (route.request().method() === 'DELETE') {
        drafts.delete(draftID);
        return ok({ status: 'deleted' });
      }
    }
    const draftSendMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)\/send$/);
    if (draftSendMatch) {
      const draftID = decodeURIComponent(draftSendMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      const sender = composeIdentityItems().find((item) => item.identity_id === draft.identity_id)
        || composeIdentityItems().find((item) => item.is_session)
        || composeIdentityItems()[0];
      sentMessages.unshift({
        id: `sent-ident-${sentSeq++}`,
        mailbox: 'Sent',
        from: sender.identity_display_name ? `${sender.identity_display_name} <${sender.from_email}>` : sender.from_email,
        to: String(draft.to || '').split(',').map((item) => item.trim()).filter(Boolean),
        subject: draft.subject || '(no subject)',
        date: new Date().toISOString(),
        seen: true,
        flagged: false,
        answered: draft.compose_mode === 'reply',
        preview: String(draft.body_text || '').trim() || 'Sent message.',
        body: String(draft.body_text || '').trim() || 'Sent message.',
        body_html: draft.body_html || '',
        attachments: [],
        thread_id: draft.context_message_id ? fixture.inboxMessage.thread_id : `thread-sent-${sentSeq}`,
      });
      draft.status = 'sent';
      draft.updated_at = new Date().toISOString();
      drafts.set(draftID, draft);
      return ok({ status: 'sent', saved_copy: true, saved_copy_mailbox: 'Sent' });
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockMailActionScenario(page, options = {}) {
  const fixture = options.fixture || mailActionFixture();
  const messages = new Map(fixture.messages.map((item) => [item.id, { ...item }]));
  const specialMappings = {};

  const summarize = (item) => ({
    id: item.id,
    mailbox: item.mailbox,
    from: item.from,
    subject: item.subject,
    date: item.date,
    seen: item.seen,
    answered: item.answered,
    flagged: item.flagged,
    preview: item.preview,
    thread_id: item.thread_id,
  });

  const mailboxPayload = () => fixture.mailboxes.map((mailbox) => {
    let role = mailbox.role || '';
    if (specialMappings.archive === mailbox.name) role = 'archive';
    if (specialMappings.trash === mailbox.name) role = 'trash';
    const mailboxMessages = Array.from(messages.values()).filter((item) => item.mailbox === mailbox.name);
    return {
      name: mailbox.name,
      role,
      unread: mailboxMessages.filter((item) => !item.seen).length,
      messages: mailboxMessages.length,
    };
  });

  const mailboxItems = (mailboxName) => Array
    .from(messages.values())
    .filter((item) => item.mailbox === mailboxName)
    .sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime())
    .map(summarize);

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v1/mailboxes') return ok(mailboxPayload());
    if (path === '/api/v1/mailboxes/special') {
      return ok({
        items: Object.entries(specialMappings).map(([role, mailbox_name]) => ({ role, mailbox_name })),
      });
    }
    if (path === '/api/v2/drafts') return ok({ items: [], page: 1, page_size: 100, total: 0 });
    if (path === '/api/v1/compose/identities') return ok({ items: [] });
    if (path === '/api/v1/search') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      const q = String(url.searchParams.get('q') || '').toLowerCase();
      return ok({
        items: mailboxItems(mailbox).filter((item) => (
          [item.from, item.subject, item.preview].join('\n').toLowerCase().includes(q)
        )),
      });
    }
    if (path === '/api/v1/messages') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      return ok({ items: mailboxItems(mailbox) });
    }

    const messageFlagsMatch = path.match(/^\/api\/v1\/messages\/([^/]+)\/flags$/);
    if (messageFlagsMatch && route.request().method() === 'POST') {
      const message = messages.get(decodeURIComponent(messageFlagsMatch[1]));
      if (!message) return ok({ error: 'not_found' }, { status: 404 });
      const payload = route.request().postDataJSON();
      const add = Array.isArray(payload.add) ? payload.add : [];
      const remove = Array.isArray(payload.remove) ? payload.remove : [];
      if (add.includes('\\Seen')) message.seen = true;
      if (remove.includes('\\Seen')) message.seen = false;
      if (add.includes('\\Flagged')) message.flagged = true;
      if (remove.includes('\\Flagged')) message.flagged = false;
      messages.set(message.id, message);
      return ok({ status: 'ok' });
    }

    const messageMoveMatch = path.match(/^\/api\/v1\/messages\/([^/]+)\/move$/);
    if (messageMoveMatch && route.request().method() === 'POST') {
      const message = messages.get(decodeURIComponent(messageMoveMatch[1]));
      if (!message) return ok({ error: 'not_found' }, { status: 404 });
      const payload = route.request().postDataJSON();
      message.mailbox = String(payload.mailbox || message.mailbox);
      messages.set(message.id, message);
      return ok({ status: 'ok' });
    }

    const specialMailboxMatch = path.match(/^\/api\/v1\/mailboxes\/special\/([^/]+)$/);
    if (specialMailboxMatch && route.request().method() === 'POST') {
      const role = decodeURIComponent(specialMailboxMatch[1]);
      const payload = route.request().postDataJSON();
      const mailboxName = String(payload.mailbox_name || '').trim();
      if (!mailboxName) return ok({ error: 'mailbox_name_required' }, { status: 400 });
      if (!fixture.mailboxes.some((item) => item.name === mailboxName)) {
        fixture.mailboxes.push({ name: mailboxName, role: '' });
      }
      specialMappings[role] = mailboxName;
      return ok({
        status: 'ok',
        role,
        mailbox_name: mailboxName,
        created: true,
        items: Object.entries(specialMappings).map(([entryRole, entryMailboxName]) => ({ role: entryRole, mailbox_name: entryMailboxName })),
        mailboxes: mailboxPayload(),
      });
    }

    const threadMatch = path.match(/^\/api\/v1\/threads\/([^/]+)\/messages$/);
    if (threadMatch) {
      const threadID = decodeURIComponent(threadMatch[1]);
      return ok({
        thread_id: threadID,
        mailbox: url.searchParams.get('mailbox') || 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: Array.from(messages.values()).filter((item) => item.thread_id === threadID).map(summarize),
      });
    }

    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const message = messages.get(decodeURIComponent(messageMatch[1]));
      if (message) return ok(message);
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

function mailTriageFixture() {
  return {
    user: {
      email: 'admin@example.com',
      role: 'admin',
      recovery_email: 'recovery@example.net',
      needs_recovery_email: false,
      auth_stage: 'authenticated',
      mail_secret_required: false,
    },
    mailboxes: [
      { name: 'INBOX', role: 'inbox' },
      { name: 'Archive', role: 'archive' },
    ],
    messages: [
      {
        id: 'tri-1-msg-1',
        mailbox: 'INBOX',
        from: 'alice@example.com',
        to: ['admin@example.com'],
        subject: 'Budget review',
        date: '2026-03-13T08:00:00.000Z',
        seen: false,
        flagged: false,
        answered: false,
        preview: 'Need follow-up on the budget review conversation.',
        body: 'Need follow-up on the budget review conversation.',
        body_html: '',
        attachments: [],
        thread_id: 'tri-thread-1',
      },
      {
        id: 'tri-2-msg-1',
        mailbox: 'INBOX',
        from: 'bob@example.com',
        to: ['admin@example.com'],
        subject: 'Vendor invoice',
        date: '2026-03-13T07:10:00.000Z',
        seen: true,
        flagged: false,
        answered: false,
        preview: 'Invoice needs categorisation and tags.',
        body: 'Invoice needs categorisation and tags.',
        body_html: '',
        attachments: [],
        thread_id: 'tri-thread-2',
      },
    ],
  };
}

async function mockMailTriageScenario(page, options = {}) {
  const fixture = options.fixture || mailTriageFixture();
  const messages = new Map(fixture.messages.map((item) => [item.id, { ...item }]));
  const categories = new Map();
  const tags = new Map();
  const triageByThread = new Map();
  let categorySeq = 1;
  let tagSeq = 1;
  let nowMs = Date.parse(options.now || '2026-03-13T09:00:00.000Z');

  const control = {
    advanceTime(ms) {
      nowMs += Number(ms || 0);
    },
    setTime(value) {
      nowMs = Date.parse(value);
    },
  };

  const sortByName = (items) => items.slice().sort((a, b) => String(a.name || '').localeCompare(String(b.name || '')));
  const nowISO = () => new Date(nowMs).toISOString();
  const normalizeName = (value) => String(value || '').trim().replace(/\s+/g, ' ');

  const categoryList = () => sortByName(Array.from(categories.values()));
  const tagList = () => sortByName(Array.from(tags.values()));

  const ensureCategory = (id, name) => {
    const trimmedID = String(id || '').trim();
    if (trimmedID && categories.has(trimmedID)) {
      return trimmedID;
    }
    const normalized = normalizeName(name);
    if (!normalized) return '';
    const existing = categoryList().find((item) => item.name.toLowerCase() === normalized.toLowerCase());
    if (existing) return existing.id;
    const next = {
      id: `cat-${categorySeq++}`,
      name: normalized,
      created_at: nowISO(),
      updated_at: nowISO(),
    };
    categories.set(next.id, next);
    return next.id;
  };

  const ensureTag = (id, name, createMissing = true) => {
    const trimmedID = String(id || '').trim();
    if (trimmedID && tags.has(trimmedID)) {
      return trimmedID;
    }
    const normalized = normalizeName(name);
    if (!normalized) return '';
    const existing = tagList().find((item) => item.name.toLowerCase() === normalized.toLowerCase());
    if (existing) return existing.id;
    if (!createMissing) return '';
    const next = {
      id: `tag-${tagSeq++}`,
      name: normalized,
      created_at: nowISO(),
      updated_at: nowISO(),
    };
    tags.set(next.id, next);
    return next.id;
  };

  const triageKeyForThread = (threadID) => `triage-${threadID}`;

  const triageStateForThread = (threadID) => {
    const raw = triageByThread.get(threadID) || {};
    const snoozedUntil = String(raw.snoozed_until || '').trim();
    const reminderAt = String(raw.reminder_at || '').trim();
    const category = raw.category_id ? categories.get(raw.category_id) : null;
    const tagRefs = (Array.isArray(raw.tag_ids) ? raw.tag_ids : [])
      .map((id) => tags.get(id))
      .filter(Boolean)
      .map((item) => ({ id: item.id, name: item.name }));
    return {
      snoozed_until: snoozedUntil,
      reminder_at: reminderAt,
      category: category ? { id: category.id, name: category.name } : null,
      tags: tagRefs,
      is_snoozed: !!snoozedUntil && Date.parse(snoozedUntil) > nowMs,
      is_follow_up_due: !!reminderAt && Date.parse(reminderAt) <= nowMs,
    };
  };

  const withTriage = (item) => ({
    ...item,
    triage_key: triageKeyForThread(item.thread_id),
    triage: triageStateForThread(item.thread_id),
  });

  const summarize = (item) => {
    const enriched = withTriage(item);
    return {
      id: enriched.id,
      mailbox: enriched.mailbox,
      from: enriched.from,
      subject: enriched.subject,
      date: enriched.date,
      seen: enriched.seen,
      answered: enriched.answered,
      flagged: enriched.flagged,
      preview: enriched.preview,
      thread_id: enriched.thread_id,
      triage_key: enriched.triage_key,
      triage: enriched.triage,
    };
  };

  const collapseLatestByThread = (items) => {
    const byThread = new Map();
    for (const item of items) {
      const threadID = String(item.thread_id || '').trim();
      if (!threadID) continue;
      const current = byThread.get(threadID);
      if (!current || new Date(item.date).getTime() > new Date(current.date).getTime()) {
        byThread.set(threadID, item);
      }
    }
    return Array.from(byThread.values());
  };

  const filteredMailboxItems = (url) => {
    const mailbox = url.searchParams.get('mailbox') || 'INBOX';
    const query = String(url.searchParams.get('q') || '').trim().toLowerCase();
    const followUp = url.searchParams.get('follow_up') === '1' || url.searchParams.get('view') === 'follow_up';
    const snoozed = url.searchParams.get('snoozed') === '1' || url.searchParams.get('view') === 'snoozed';
    const categoryID = String(url.searchParams.get('category_id') || '').trim();
    const tagIDs = url.searchParams.getAll('tag_id').map((value) => String(value || '').trim()).filter(Boolean);
    const triageOnly = followUp || snoozed || !!categoryID || tagIDs.length > 0;

    let items = Array.from(messages.values())
      .filter((item) => item.mailbox === mailbox)
      .map((item) => withTriage(item));

    if (query) {
      items = items.filter((item) => (
        [item.from, item.subject, item.preview, item.body].join('\n').toLowerCase().includes(query)
      ));
    }
    if (!triageOnly) {
      items = items.filter((item) => !item.triage.is_snoozed);
    }
    if (followUp) {
      items = items.filter((item) => !!item.triage.reminder_at);
    }
    if (snoozed) {
      items = items.filter((item) => !!item.triage.is_snoozed);
    }
    if (categoryID) {
      items = items.filter((item) => String(item.triage.category?.id || '') === categoryID);
    }
    if (tagIDs.length > 0) {
      items = items.filter((item) => item.triage.tags.some((tag) => tagIDs.includes(String(tag.id || ''))));
    }
    if (triageOnly) {
      items = collapseLatestByThread(items);
    }
    items.sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime());
    return items.map(summarize);
  };

  const responseTriageState = (target) => {
    const threadID = String(target.thread_id || '').trim();
    return {
      target: {
        source: String(target.source || 'live').trim() || 'live',
        account_id: String(target.account_id || '').trim(),
        thread_id: threadID,
        mailbox: String(target.mailbox || '').trim(),
        subject: String(target.subject || '').trim(),
        from: String(target.from || '').trim(),
      },
      triage_key: triageKeyForThread(threadID),
      triage: triageStateForThread(threadID),
    };
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/accounts') return ok({ items: [] });
    if (path === '/api/v2/saved-searches') return ok({ items: [] });
    if (path === '/api/v2/accounts/health') return ok({ summary: {}, items: [] });
    if (path === '/api/v2/mail/senders') return ok({ items: [] });
    if (path === '/api/v2/drafts') return ok({ items: [], page: 1, page_size: 100, total: 0 });
    if (path === '/api/v1/compose/identities') return ok({ items: [] });

    if (path === '/api/v2/mail-triage/catalog') {
      return ok({ categories: categoryList(), tags: tagList() });
    }

    if (path === '/api/v2/mail-triage/reminders/due') {
      const items = [];
      for (const [threadID, state] of triageByThread.entries()) {
        const reminderAt = String(state.reminder_at || '').trim();
        if (!reminderAt || Date.parse(reminderAt) > nowMs) continue;
        if (String(state.last_notified_at || '') === reminderAt) continue;
        const message = Array.from(messages.values()).find((item) => item.thread_id === threadID) || null;
        if (!message) continue;
        state.last_notified_at = reminderAt;
        triageByThread.set(threadID, state);
        items.push({
          triage_key: triageKeyForThread(threadID),
          source: 'live',
          account_id: '',
          thread_id: threadID,
          mailbox: message.mailbox,
          subject: message.subject,
          from: message.from,
          reminder_at: reminderAt,
        });
      }
      return ok({ items });
    }

    if (path === '/api/v2/mail-triage/actions' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON();
      const targets = Array.isArray(payload.targets) ? payload.targets : [];
      const responseItems = [];
      for (const rawTarget of targets) {
        const threadID = String(rawTarget.thread_id || '').trim();
        if (!threadID) continue;
        const current = {
          ...(triageByThread.get(threadID) || {
            category_id: '',
            tag_ids: [],
            snoozed_until: '',
            reminder_at: '',
            last_notified_at: '',
          }),
        };
        if (payload.clear_snooze) current.snoozed_until = '';
        else if (payload.snoozed_until) current.snoozed_until = String(payload.snoozed_until);

        if (payload.clear_reminder) {
          current.reminder_at = '';
          current.last_notified_at = '';
        } else if (payload.reminder_at) {
          current.reminder_at = String(payload.reminder_at);
          current.last_notified_at = '';
        }

        if (payload.clear_category) current.category_id = '';
        else if (payload.category_id || payload.category_name) {
          current.category_id = ensureCategory(payload.category_id, payload.category_name);
        }

        if (payload.clear_tags) {
          current.tag_ids = [];
        } else {
          const nextTagIDs = new Set(Array.isArray(current.tag_ids) ? current.tag_ids : []);
          for (const id of Array.isArray(payload.add_tag_ids) ? payload.add_tag_ids : []) {
            const tagID = ensureTag(id, '', true);
            if (tagID) nextTagIDs.add(tagID);
          }
          for (const name of Array.isArray(payload.add_tag_names) ? payload.add_tag_names : []) {
            const tagID = ensureTag('', name, true);
            if (tagID) nextTagIDs.add(tagID);
          }
          for (const id of Array.isArray(payload.remove_tag_ids) ? payload.remove_tag_ids : []) {
            nextTagIDs.delete(String(id || '').trim());
          }
          for (const name of Array.isArray(payload.remove_tag_names) ? payload.remove_tag_names : []) {
            const existing = tagList().find((item) => item.name.toLowerCase() === String(name || '').trim().toLowerCase());
            if (existing) nextTagIDs.delete(existing.id);
          }
          current.tag_ids = Array.from(nextTagIDs);
        }

        triageByThread.set(threadID, current);
        responseItems.push(responseTriageState(rawTarget));
      }
      return ok({ status: 'ok', items: responseItems });
    }

    const categoryMatch = path.match(/^\/api\/v2\/mail-triage\/categories\/([^/]+)$/);
    if (categoryMatch && route.request().method() === 'PATCH') {
      const categoryID = decodeURIComponent(categoryMatch[1]);
      const category = categories.get(categoryID);
      if (!category) return ok({ code: 'mail_triage_category_not_found' }, { status: 404 });
      const payload = route.request().postDataJSON();
      category.name = normalizeName(payload.name);
      category.updated_at = nowISO();
      categories.set(categoryID, category);
      return ok(category);
    }
    if (categoryMatch && route.request().method() === 'DELETE') {
      const categoryID = decodeURIComponent(categoryMatch[1]);
      categories.delete(categoryID);
      for (const [threadID, state] of triageByThread.entries()) {
        if (state.category_id === categoryID) {
          state.category_id = '';
          triageByThread.set(threadID, state);
        }
      }
      return ok({ status: 'ok' });
    }
    if (path === '/api/v2/mail-triage/categories' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON();
      const categoryID = ensureCategory('', payload.name);
      return ok(categories.get(categoryID), { status: 201 });
    }

    const tagMatch = path.match(/^\/api\/v2\/mail-triage\/tags\/([^/]+)$/);
    if (tagMatch && route.request().method() === 'PATCH') {
      const tagID = decodeURIComponent(tagMatch[1]);
      const tag = tags.get(tagID);
      if (!tag) return ok({ code: 'mail_triage_tag_not_found' }, { status: 404 });
      const payload = route.request().postDataJSON();
      tag.name = normalizeName(payload.name);
      tag.updated_at = nowISO();
      tags.set(tagID, tag);
      return ok(tag);
    }
    if (tagMatch && route.request().method() === 'DELETE') {
      const tagID = decodeURIComponent(tagMatch[1]);
      tags.delete(tagID);
      for (const [threadID, state] of triageByThread.entries()) {
        state.tag_ids = (Array.isArray(state.tag_ids) ? state.tag_ids : []).filter((item) => item !== tagID);
        triageByThread.set(threadID, state);
      }
      return ok({ status: 'ok' });
    }
    if (path === '/api/v2/mail-triage/tags' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON();
      const tagID = ensureTag('', payload.name, true);
      return ok(tags.get(tagID), { status: 201 });
    }

    if (path === '/api/v1/mailboxes') {
      return ok(fixture.mailboxes.map((mailbox) => {
        const mailboxMessages = filteredMailboxItems(new URL(`http://local.test/api/v1/messages?mailbox=${encodeURIComponent(mailbox.name)}`));
        return {
          name: mailbox.name,
          role: mailbox.role || '',
          unread: mailboxMessages.filter((item) => !item.seen).length,
          messages: mailboxMessages.length,
        };
      }));
    }
    if (path === '/api/v1/messages' || path === '/api/v1/search') {
      return ok({ items: filteredMailboxItems(url) });
    }

    const threadMatch = path.match(/^\/api\/v1\/threads\/([^/]+)\/messages$/);
    if (threadMatch) {
      const threadID = decodeURIComponent(threadMatch[1]);
      const items = Array.from(messages.values())
        .filter((item) => item.thread_id === threadID)
        .sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime())
        .map((item) => summarize(item));
      return ok({
        thread_id: threadID,
        mailbox: url.searchParams.get('mailbox') || 'INBOX',
        scope: 'conversation',
        truncated: false,
        items,
      });
    }

    const messageFlagsMatch = path.match(/^\/api\/v1\/messages\/([^/]+)\/flags$/);
    if (messageFlagsMatch && route.request().method() === 'POST') {
      const message = messages.get(decodeURIComponent(messageFlagsMatch[1]));
      if (!message) return ok({ error: 'not_found' }, { status: 404 });
      const payload = route.request().postDataJSON();
      const add = Array.isArray(payload.add) ? payload.add : [];
      const remove = Array.isArray(payload.remove) ? payload.remove : [];
      if (add.includes('\\Seen')) message.seen = true;
      if (remove.includes('\\Seen')) message.seen = false;
      if (add.includes('\\Flagged')) message.flagged = true;
      if (remove.includes('\\Flagged')) message.flagged = false;
      messages.set(message.id, message);
      return ok({ status: 'ok' });
    }

    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const message = messages.get(decodeURIComponent(messageMatch[1]));
      if (!message) return ok({ error: 'not_found' }, { status: 404 });
      return ok(withTriage(message));
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });

  return control;
}

async function mockReliableMailboxStateScenario(page) {
  const fixture = reliableMailboxStateFixture();
  const messages = new Map(fixture.messages.map((item) => [item.id, { ...item }]));
  const drafts = new Map();
  const specialMappings = {};
  let draftSeq = 1;
  let sentSeq = 1;

  const summarize = (item) => ({
    id: item.id,
    mailbox: item.mailbox,
    from: item.from,
    subject: item.subject,
    date: item.date,
    seen: item.seen,
    answered: item.answered,
    flagged: item.flagged,
    preview: item.preview,
    thread_id: item.thread_id,
  });

  const mailboxPayload = () => fixture.mailboxes.map((mailbox) => {
    let role = mailbox.role || '';
    if (specialMappings.sent === mailbox.name) role = 'sent';
    if (specialMappings.archive === mailbox.name) role = 'archive';
    if (specialMappings.trash === mailbox.name) role = 'trash';
    const mailboxMessages = Array.from(messages.values()).filter((item) => item.mailbox === mailbox.name);
    return {
      name: mailbox.name,
      role,
      unread: mailboxMessages.filter((item) => !item.seen).length,
      messages: mailboxMessages.length,
    };
  });

  const mailboxItems = (mailboxName) => Array
    .from(messages.values())
    .filter((item) => item.mailbox === mailboxName)
    .sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime())
    .map(summarize);

  const draftList = () => Array
    .from(drafts.values())
    .filter((item) => String(item.status || '').toLowerCase() !== 'sent')
    .sort((a, b) => new Date(b.updated_at || b.created_at || 0).getTime() - new Date(a.updated_at || a.created_at || 0).getTime());

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/mail/senders') {
      return ok({
        items: [{
          id: 'sender-primary',
          kind: 'primary',
          name: 'Admin',
          from_email: fixture.user.email,
          reply_to: '',
          signature_text: '',
          signature_html: '',
          account_id: '',
          account_label: 'Built-in sender',
          account_login: fixture.user.email,
          is_default: true,
          is_primary: true,
          can_delete: false,
          can_schedule: true,
          status: 'ok',
        }],
      });
    }
    if (path === '/api/v1/mailboxes') return ok(mailboxPayload());
    if (path === '/api/v1/mailboxes/special') {
      return ok({
        items: Object.entries(specialMappings).map(([role, mailbox_name]) => ({ role, mailbox_name })),
      });
    }
    if (path === '/api/v1/compose/identities') return ok({ items: [] });
    if (path === '/api/v2/drafts' && route.request().method() === 'GET') {
      return ok({ items: draftList(), page: 1, page_size: 100, total: draftList().length });
    }
    if (path === '/api/v2/drafts' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON();
      const draft = {
        id: `draft-${draftSeq++}`,
        account_id: payload.account_id || '',
        identity_id: payload.identity_id || '',
        compose_mode: payload.compose_mode || 'send',
        context_message_id: payload.context_message_id || '',
        from_mode: payload.from_mode || 'default',
        from_manual: payload.from_manual || '',
        client_state_json: payload.client_state_json || '',
        to: payload.to || '',
        cc: payload.cc || '',
        bcc: payload.bcc || '',
        subject: payload.subject || '',
        body_text: payload.body_text || '',
        body_html: payload.body_html || '',
        attachments_json: '[]',
        status: payload.status || 'active',
        last_send_error: '',
        created_at: '2026-03-09T10:00:00.000Z',
        updated_at: new Date().toISOString(),
      };
      drafts.set(draft.id, draft);
      return ok(draft, { status: 201 });
    }

    const draftMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)$/);
    if (draftMatch && route.request().method() === 'PATCH') {
      const draftID = decodeURIComponent(draftMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      Object.assign(draft, route.request().postDataJSON(), { updated_at: new Date().toISOString() });
      drafts.set(draftID, draft);
      return ok(draft);
    }

    const draftSendMatch = path.match(/^\/api\/v2\/drafts\/([^/]+)\/send$/);
    if (draftSendMatch) {
      const draftID = decodeURIComponent(draftSendMatch[1]);
      const draft = drafts.get(draftID);
      if (!draft) return ok({ error: 'draft_not_found' }, { status: 404 });
      const savedCopyMailbox = specialMappings.sent || 'Sent';
      if (!fixture.mailboxes.some((item) => item.name === savedCopyMailbox)) {
        fixture.mailboxes.push({ name: savedCopyMailbox, role: '' });
      }
      const messageID = `sent-${sentSeq++}`;
      const sentMessage = {
        id: messageID,
        mailbox: savedCopyMailbox,
        from: fixture.user.email,
        to: String(draft.to || '').split(',').map((item) => item.trim()).filter(Boolean),
        subject: draft.subject || '(no subject)',
        date: new Date().toISOString(),
        seen: true,
        flagged: false,
        answered: String(draft.compose_mode || '').toLowerCase() === 'reply',
        preview: String(draft.body_text || '').replace(/\s+/g, ' ').trim().slice(0, 140),
        body: draft.body_text || '',
        body_html: draft.body_html || '',
        attachments: [],
        thread_id: draft.context_message_id ? 'thread-live-1' : `thread-sent-${messageID}`,
      };
      messages.set(messageID, sentMessage);
      if (draft.context_message_id && messages.has(draft.context_message_id)) {
        const original = messages.get(draft.context_message_id);
        original.answered = true;
        messages.set(original.id, original);
      }
      draft.status = 'sent';
      drafts.set(draftID, draft);
      return ok({ status: 'sent', saved_copy: true, saved_copy_mailbox: savedCopyMailbox });
    }

    if (path === '/api/v1/messages') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      return ok({ items: mailboxItems(mailbox) });
    }
    if (path === '/api/v1/search') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      const q = String(url.searchParams.get('q') || '').toLowerCase();
      return ok({
        items: mailboxItems(mailbox).filter((item) => (
          [item.from, item.subject, item.preview].join('\n').toLowerCase().includes(q)
        )),
      });
    }

    const messageFlagsMatch = path.match(/^\/api\/v1\/messages\/([^/]+)\/flags$/);
    if (messageFlagsMatch && route.request().method() === 'POST') {
      const message = messages.get(decodeURIComponent(messageFlagsMatch[1]));
      if (!message) return ok({ error: 'not_found' }, { status: 404 });
      const payload = route.request().postDataJSON();
      const add = Array.isArray(payload.add) ? payload.add : [];
      const remove = Array.isArray(payload.remove) ? payload.remove : [];
      if (add.includes('\\Seen')) message.seen = true;
      if (remove.includes('\\Seen')) message.seen = false;
      messages.set(message.id, message);
      return ok({ status: 'ok' });
    }

    const specialMailboxMatch = path.match(/^\/api\/v1\/mailboxes\/special\/([^/]+)$/);
    if (specialMailboxMatch && route.request().method() === 'POST') {
      const role = decodeURIComponent(specialMailboxMatch[1]);
      const payload = route.request().postDataJSON();
      const mailboxName = String(payload.mailbox_name || '').trim();
      if (!mailboxName) return ok({ error: 'mailbox_name_required' }, { status: 400 });
      if (!fixture.mailboxes.some((item) => item.name === mailboxName)) {
        fixture.mailboxes.push({ name: mailboxName, role: '' });
      }
      specialMappings[role] = mailboxName;
      return ok({
        status: 'ok',
        role,
        mailbox_name: mailboxName,
        created: true,
        items: Object.entries(specialMappings).map(([entryRole, entryMailboxName]) => ({ role: entryRole, mailbox_name: entryMailboxName })),
        mailboxes: mailboxPayload(),
      });
    }

    const threadMatch = path.match(/^\/api\/v1\/threads\/([^/]+)\/messages$/);
    if (threadMatch) {
      const threadID = decodeURIComponent(threadMatch[1]);
      return ok({
        thread_id: threadID,
        mailbox: url.searchParams.get('mailbox') || 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: Array.from(messages.values()).filter((item) => item.thread_id === threadID).map(summarize),
      });
    }

    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const message = messages.get(decodeURIComponent(messageMatch[1]));
      if (message) return ok(message);
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockLiveRefreshScenario(page, options = {}) {
  const fixture = threadedMailFixture();
  const messages = new Map(fixture.threadItems.map((item) => [item.id, {
    ...fixture.messageDetails[item.id],
    preview: item.preview,
    thread_id: item.thread_id,
  }]));
  const duplicateSummaryIDs = new Set(
    (Array.isArray(options?.duplicateSummaryIDs) ? options.duplicateSummaryIDs : [])
      .map((value) => String(value || '').trim())
      .filter(Boolean),
  );

  const summarize = (item) => ({
    id: item.id,
    mailbox: item.mailbox,
    from: item.from,
    subject: item.subject,
    date: item.date,
    seen: item.seen,
    answered: item.answered,
    flagged: item.flagged,
    preview: item.preview,
    thread_id: item.thread_id,
  });

  const mailboxSummaries = (mailbox) => {
    const items = Array.from(messages.values())
      .filter((item) => item.mailbox === mailbox)
      .sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime())
      .map(summarize);
    for (const duplicateID of duplicateSummaryIDs) {
      const match = items.find((item) => item.id === duplicateID);
      if (match && match.mailbox === mailbox) {
        items.push({ ...match });
      }
    }
    return items;
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/drafts') return ok({ items: [], page: 1, page_size: 100, total: 0 });
    if (path === '/api/v1/compose/identities') return ok({ items: [] });
    if (path === '/api/v1/mailboxes') {
      const inboxMessages = Array.from(messages.values()).filter((item) => item.mailbox === 'INBOX');
      return ok([
        { name: 'INBOX', role: 'inbox', unread: inboxMessages.filter((item) => !item.seen).length, messages: inboxMessages.length },
        { name: 'Sent Messages', role: 'sent', unread: 0, messages: Array.from(messages.values()).filter((item) => item.mailbox === 'Sent Messages').length },
        { name: 'Archive', role: 'archive', unread: 0, messages: Array.from(messages.values()).filter((item) => item.mailbox === 'Archive').length },
      ]);
    }
    if (path === '/api/v1/messages') {
      const mailbox = url.searchParams.get('mailbox') || 'INBOX';
      return ok({ items: mailboxSummaries(mailbox) });
    }
    if (path === '/api/v1/threads/thread-1/messages') {
      return ok({
        thread_id: 'thread-1',
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: Array.from(messages.values()).filter((item) => item.thread_id === 'thread-1').map(summarize),
      });
    }
    if (/^\/api\/v1\/messages\/[^/]+\/flags$/.test(path)) {
      return ok({ status: 'ok' });
    }
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const message = messages.get(decodeURIComponent(messageMatch[1]));
      if (message) return ok(message);
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });

  return {
    addIncomingMessage() {
      messages.set('m4', {
        id: 'm4',
        mailbox: 'INBOX',
        from: 'alerts@example.com',
        to: ['admin@example.com'],
        subject: 'New incoming status',
        date: '2026-03-09T11:20:00.000Z',
        seen: false,
        flagged: false,
        answered: false,
        preview: 'A fresh message arrived while you were reading.',
        body: 'A fresh message arrived while you were reading.',
        body_html: '',
        attachments: [],
        thread_id: 'thread-4',
      });
    },
    duplicateSummary(id) {
      const value = String(id || '').trim();
      if (value) duplicateSummaryIDs.add(value);
    },
  };
}

async function mockThreadedMailScenario(page) {
  const fixture = threadedMailFixture();
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v1/mailboxes') return ok(fixture.mailboxes);
    if (path === '/api/v1/messages') return ok({ items: fixture.mailboxItems });
    if (path === '/api/v1/threads/thread-1/messages') {
      return ok({
        thread_id: 'thread-1',
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: true,
        items: fixture.threadItems,
      });
    }
    if (/^\/api\/v1\/messages\/[^/]+\/flags$/.test(path)) return ok({ ok: true });
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const messageID = decodeURIComponent(messageMatch[1]);
      const message = fixture.messageDetails[messageID];
      if (message) return ok(message);
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockSingletonThreadScenario(page) {
  const fixture = threadedMailFixture();
  fixture.mailboxItems = [
    {
      id: 'single-1',
      mailbox: 'INBOX',
      from: 'mailer-daemon@example.com',
      subject: 'Delivery report',
      date: '2026-03-10T08:00:00.000Z',
      seen: true,
      answered: false,
      flagged: false,
      preview: 'Single thread item.',
      thread_id: 'thread-single-1',
    },
  ];
  fixture.messageDetails = {
    'single-1': {
      id: 'single-1',
      mailbox: 'INBOX',
      from: 'mailer-daemon@example.com',
      to: ['admin@example.com'],
      subject: 'Delivery report',
      date: '2026-03-10T08:00:00.000Z',
      seen: true,
      flagged: false,
      answered: false,
      body: 'Single thread item.',
      body_html: '',
      attachments: [],
      thread_id: 'thread-single-1',
    },
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body) => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v1/mailboxes') return ok(fixture.mailboxes);
    if (path === '/api/v1/messages') return ok({ items: fixture.mailboxItems });
    if (path === '/api/v1/threads/thread-single-1/messages') {
      return ok({
        thread_id: 'thread-single-1',
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: fixture.mailboxItems,
      });
    }
    if (/^\/api\/v1\/messages\/[^/]+\/flags$/.test(path)) return ok({ ok: true });
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const messageID = decodeURIComponent(messageMatch[1]);
      const message = fixture.messageDetails[messageID];
      if (message) return ok(message);
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockThreadedSummaryFallbackScenario(page) {
  const fixture = threadedMailFixture();
  fixture.mailboxItems = fixture.mailboxItems.map((item, index) => (
    index === 0 ? { ...item, thread_id: '' } : item
  ));
  fixture.messageDetails.m1 = {
    ...fixture.messageDetails.m1,
    thread_id: 'thread-1',
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body) => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') return ok(fixture.user);
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v1/mailboxes') return ok(fixture.mailboxes);
    if (path === '/api/v1/messages') return ok({ items: fixture.mailboxItems });
    if (path === '/api/v1/threads/thread-1/messages') {
      return ok({
        thread_id: 'thread-1',
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: fixture.threadItems,
      });
    }
    if (/^\/api\/v1\/messages\/[^/]+\/flags$/.test(path)) return ok({ ok: true });
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const messageID = decodeURIComponent(messageMatch[1]);
      const message = fixture.messageDetails[messageID];
      if (message) return ok(message);
    }
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockReaderHTMLSizingScenario(page) {
  const longHTML = [
    '<div style="font-family: Georgia, serif; font-size: 18px; line-height: 1.5;">',
    '<p><strong>Delivery report</strong> with a long quoted reply chain.</p>',
    ...Array.from({ length: 20 }, (_, index) => (
      `<p>Paragraph ${index + 1}: This reply contains enough HTML body content to exceed the visible reader pane and must stay fully reachable without clipping or a black void underneath the iframe.</p>`
    )),
    '<blockquote><p>Quoted message block with multiple lines to emulate a real forwarded or bounced message preview in HTML mode.</p><p>Additional quoted context keeps the document height non-trivial.</p></blockquote>',
    '<p>Final paragraph at the bottom of the message body.</p>',
    '</div>',
  ].join('');
  const shortHTML = '<div style="font-family: Georgia, serif; font-size: 18px; line-height: 1.5;"><p>Short HTML message.</p></div>';

  const messages = new Map([
    ['html-long', {
      id: 'html-long',
      mailbox: 'INBOX',
      from: 'Mail Delivery System <mailer-daemon@example.com>',
      to: ['admin@example.com'],
      subject: 'Re: Long HTML preview check',
      date: '2026-03-12T09:25:03.000Z',
      seen: true,
      answered: false,
      flagged: false,
      preview: 'Long HTML preview content for iframe sizing regression.',
      body: 'Long HTML preview content for iframe sizing regression.',
      body_html: longHTML,
      attachments: [],
      thread_id: 'thread-html-long',
    }],
    ['html-short', {
      id: 'html-short',
      mailbox: 'INBOX',
      from: 'Support <support@example.com>',
      to: ['admin@example.com'],
      subject: 'Short HTML preview check',
      date: '2026-03-11T08:10:00.000Z',
      seen: true,
      answered: false,
      flagged: false,
      preview: 'Short HTML preview content for iframe sizing regression.',
      body: 'Short HTML preview content for iframe sizing regression.',
      body_html: shortHTML,
      attachments: [],
      thread_id: 'thread-html-short',
    }],
  ]);

  const summarize = (item) => ({
    id: item.id,
    mailbox: item.mailbox,
    from: item.from,
    subject: item.subject,
    date: item.date,
    seen: item.seen,
    answered: item.answered,
    flagged: item.flagged,
    preview: item.preview,
    thread_id: item.thread_id,
  });

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') {
      return ok({
        email: 'admin@example.com',
        role: 'admin',
        recovery_email: 'recovery@example.net',
        needs_recovery_email: false,
        auth_stage: 'authenticated',
        mail_secret_required: false,
      });
    }
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/drafts') return ok({ items: [], page: 1, page_size: 100, total: 0 });
    if (path === '/api/v1/compose/identities') return ok({ items: [] });
    if (path === '/api/v1/mailboxes') {
      return ok([
        { name: 'INBOX', role: 'inbox', unread: 0, messages: messages.size },
      ]);
    }
    if (path === '/api/v1/messages') {
      return ok({
        items: Array.from(messages.values())
          .sort((a, b) => new Date(b.date).getTime() - new Date(a.date).getTime())
          .map(summarize),
      });
    }
    const threadMatch = path.match(/^\/api\/v1\/threads\/([^/]+)\/messages$/);
    if (threadMatch) {
      const threadID = decodeURIComponent(threadMatch[1]);
      return ok({
        thread_id: threadID,
        mailbox: 'INBOX',
        scope: 'conversation',
        truncated: false,
        items: Array.from(messages.values()).filter((item) => item.thread_id === threadID).map(summarize),
      });
    }
    if (/^\/api\/v1\/messages\/[^/]+\/flags$/.test(path)) return ok({ ok: true });
    const messageMatch = path.match(/^\/api\/v1\/messages\/([^/]+)$/);
    if (messageMatch) {
      const message = messages.get(decodeURIComponent(messageMatch[1]));
      if (message) return ok(message);
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: path }),
    });
  });
}

async function mockMailHealthQuotaScenario(page, options = {}) {
  const quotaSupported = options.quotaSupported !== false;
  const quotaAvailable = quotaSupported && options.quotaAvailable === true;
  const quotaRefreshedAt = quotaSupported ? String(options.quotaRefreshedAt || '') : '';
  const quotaLastError = quotaSupported
    ? String(options.quotaLastError || '')
    : 'Quota unavailable on this server.';
  const account = {
    id: 'acct-health-1',
    user_id: 'user-admin',
    display_name: 'webmaster',
    login: 'webmaster',
    is_default: true,
    status: 'active',
    last_sync_at: '2026-03-12T15:00:00.000Z',
    last_error: '',
  };
  const healthItem = {
    account_id: account.id,
    account_label: 'webmaster',
    is_default: true,
    status: 'ok',
    last_sync_at: '2026-03-12T15:00:00.000Z',
    last_error: '',
    quota_available: quotaAvailable,
    quota_supported: quotaSupported,
    used_bytes: quotaAvailable ? 1048576 : 0,
    total_bytes: quotaAvailable ? 4194304 : 0,
    used_messages: quotaAvailable ? 12 : 0,
    total_messages: quotaAvailable ? 120 : 0,
    quota_refreshed_at: quotaRefreshedAt,
    quota_last_error: quotaLastError,
    action_state: null,
  };
  const sender = {
    id: 'sender-primary',
    kind: 'primary',
    name: 'webmaster',
    from_email: 'webmaster@2h4s2d.ru',
    reply_to: '',
    signature_text: '',
    signature_html: '',
    account_id: account.id,
    account_label: 'webmaster',
    is_default: true,
    is_primary: true,
    can_delete: false,
    can_schedule: true,
    status: 'ok',
  };

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') {
      return ok({
        email: 'admin@example.com',
        role: 'admin',
        recovery_email: 'recovery@example.net',
        needs_recovery_email: false,
        auth_stage: 'authenticated',
        mail_secret_required: false,
      });
    }
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/accounts') return ok({ items: [account] });
    if (path === '/api/v2/accounts/health') {
      return ok({
        summary: {
          total_accounts: 1,
          healthy_accounts: 1,
          attention_accounts: 0,
          error_accounts: 0,
        },
        items: [healthItem],
      });
    }
    if (path === '/api/v2/mail/senders') return ok({ items: [sender] });
    if (path === '/api/v2/saved-searches') return ok({ items: [] });
    if (path === `/api/v2/accounts/${account.id}/mailboxes`) {
      return ok([{ name: 'INBOX', role: 'inbox', unread: 0, messages: 0 }]);
    }
    if (path === '/api/v2/messages' || path === '/api/v2/search') return ok({ items: [], total: 0 });
    if (path === '/api/v2/drafts') return ok({ items: [], page: 1, page_size: 100, total: 0 });
    if (path === `/api/v2/accounts/${account.id}/health/sync`) return ok({ status: 'queued' }, { status: 202 });
    if (path === `/api/v2/accounts/${account.id}/health/quota-refresh`) return ok({ status: 'queued' }, { status: 202 });
    if (path === `/api/v2/accounts/${account.id}/health/reindex`) return ok({ status: 'queued' }, { status: 202 });
    return ok({});
  });
}

function buildUpdaterStatus(overrides = {}) {
  return {
    enabled: true,
    configured: true,
    current: {
      version: 'v1.1.0-alpha.1',
      commit: '8fhb424234b2sbf',
      build_time: '2026-03-12T12:00:00.000Z',
      source_repo: 'https://github.com/2high4schooltoday/despatch',
    },
    latest: {
      tag_name: 'v1.1.0-alpha.1_01',
      published_at: '2026-03-12T14:00:00.000Z',
      html_url: 'https://github.com/2high4schooltoday/despatch/releases/tag/v1.1.0-alpha.1_01',
    },
    last_checked_at: '2026-03-12T15:05:00.000Z',
    last_check_error: '',
    update_available: true,
    apply: {
      state: 'idle',
      request_id: '',
      target_version: '',
      to_version: '',
      error: '',
      finished_at: '',
    },
    auto_update: {
      enabled: true,
      state: 'idle',
      target_version: '',
      downloaded_at: '',
      scheduled_for: '',
      error: '',
    },
    ...overrides,
  };
}

async function mockUpdaterNotificationScenario(page, options = {}) {
  const account = {
    id: 'acct-updater-1',
    user_id: 'user-admin',
    display_name: 'webmaster',
    login: 'webmaster',
    is_default: true,
    status: 'active',
    last_sync_at: '2026-03-12T15:00:00.000Z',
    last_error: '',
  };
  const sender = {
    id: 'sender-primary',
    kind: 'primary',
    name: 'webmaster',
    from_email: 'webmaster@2h4s2d.ru',
    reply_to: '',
    signature_text: '',
    signature_html: '',
    account_id: account.id,
    account_label: 'webmaster',
    is_default: true,
    is_primary: true,
    can_delete: false,
    can_schedule: true,
    status: 'ok',
  };
  const healthItem = {
    account_id: account.id,
    account_label: 'webmaster',
    is_default: true,
    status: 'ok',
    last_sync_at: '2026-03-12T15:00:00.000Z',
    last_error: '',
    quota_available: false,
    quota_supported: false,
    used_bytes: 0,
    total_bytes: 0,
    used_messages: 0,
    total_messages: 0,
    quota_refreshed_at: '',
    quota_last_error: 'Quota unavailable on this server.',
    action_state: null,
  };
  const runtime = {
    statusCalls: 0,
  };
  const statusSequence = Array.isArray(options.statusSequence) && options.statusSequence.length
    ? options.statusSequence
    : [buildUpdaterStatus()];
  const checkStatus = options.checkStatus || statusSequence[statusSequence.length - 1];

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const ok = async (body, extra = {}) => {
      await route.fulfill({
        status: extra.status || 200,
        contentType: extra.contentType || 'application/json',
        body: extra.rawBody ?? JSON.stringify(body),
      });
    };

    if (path === '/api/v1/public/captcha/config') return ok({ enabled: false });
    if (path === '/api/v1/public/password-reset/capabilities') return ok({ enabled: false, reason: 'disabled' });
    if (path === '/api/v1/public/auth/capabilities') {
      return ok({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'disabled',
      });
    }
    if (path === '/api/v1/setup/status') {
      return ok({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      });
    }
    if (path === '/api/v1/me') {
      return ok({
        email: 'admin@example.com',
        role: 'admin',
        recovery_email: 'recovery@example.net',
        needs_recovery_email: false,
        auth_stage: 'authenticated',
        mail_secret_required: false,
      });
    }
    if (path === '/api/v2/security/mfa/webauthn') return ok({ items: [] });
    if (path === '/api/v2/security/mfa/trusted-devices') return ok({ items: [] });
    if (path === '/api/v2/security/sessions') return ok({ items: [] });
    if (path === '/api/v2/accounts') return ok({ items: [account] });
    if (path === '/api/v2/accounts/health') {
      return ok({
        summary: {
          total_accounts: 1,
          healthy_accounts: 1,
          attention_accounts: 0,
          error_accounts: 0,
        },
        items: [healthItem],
      });
    }
    if (path === '/api/v2/mail/senders') return ok({ items: [sender] });
    if (path === '/api/v2/saved-searches') return ok({ items: [] });
    if (path === `/api/v2/accounts/${account.id}/mailboxes`) {
      return ok([{ name: 'INBOX', role: 'inbox', unread: 0, messages: 0 }]);
    }
    if (path === '/api/v2/messages' || path === '/api/v2/search') return ok({ items: [], total: 0 });
    if (path === '/api/v2/drafts') return ok({ items: [], page: 1, page_size: 100, total: 0 });
    if (path === '/api/v1/admin/system/update/status') {
      const index = Math.min(runtime.statusCalls, statusSequence.length - 1);
      runtime.statusCalls += 1;
      return ok(statusSequence[index]);
    }
    if (path === '/api/v1/admin/system/update/check') {
      if (options.checkError) {
        return ok({
          code: options.checkError.code || 'update_check_failed',
          message: options.checkError.message || 'update check failed',
        }, { status: options.checkError.status || 500 });
      }
      return ok(checkStatus);
    }
    if (path === '/api/v1/admin/system/update/apply') return ok({ ok: true });
    if (path === '/api/v1/admin/system/update/automatic') return ok({ ok: true });
    if (path === '/api/v1/admin/system/update/cancel-scheduled') return ok({ ok: true });
    return ok({});
  });

  return runtime;
}

test('auth hides passkey sign-in when unavailable', async ({ page }) => {
  await page.route('**/api/v1/public/auth/capabilities', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        passkey_mfa_available: false,
        passkey_passwordless_available: false,
        passkey_usernameless_enabled: true,
        reason: 'passwordless_disabled',
      }),
    });
  });
  await page.route('**/api/v1/setup/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        required: false,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        passkey_primary_sign_in_enabled: true,
      }),
    });
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });
  await expect(page.locator('#auth-pane-login')).not.toHaveClass(/hidden/);
  await expect(page.locator('#passkey-email')).toHaveCount(0);
  await expect(page.locator('#auth-passkey-card')).toHaveClass(/hidden/);
});

test('oobe uses the new focused setup flow', async ({ page }) => {
  await page.route('**/api/v1/setup/status', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        required: true,
        base_domain: 'example.com',
        default_admin_email: 'webmaster@example.com',
        auth_mode: 'sql',
        password_min_length: 12,
        password_max_length: 128,
        password_class_min: 3,
        automatic_updates_enabled: true,
        passkey_primary_sign_in_enabled: true,
      }),
    });
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });
  await expect(page.locator('#view-setup')).toBeVisible();
  await expect(page.locator('#setup-progress-title')).toHaveText(/Welcome/i);
  await expect(page.locator('#setup-step-0')).not.toHaveClass(/hidden/);

  await page.click('#setup-next');
  await expect(page.locator('#setup-progress-title')).toHaveText(/Where Despatch will be set up/i);
  await page.click('#setup-next');
  await expect(page.locator('#setup-progress-title')).toHaveText(/Choose your look/i);
  await page.click('#setup-theme-paper');
  await expect(page.locator('#setup-theme-paper')).toHaveClass(/is-selected/);
  await page.click('#setup-next');
  await expect(page.locator('#setup-progress-title')).toHaveText(/Software updates/i);
  await expect(page.locator('#setup-updates-auto')).toHaveClass(/is-selected/);
  await page.click('#setup-updates-manual');
  await expect(page.locator('#setup-updates-manual')).toHaveClass(/is-selected/);
  await page.click('#setup-next');
  await expect(page.locator('#setup-progress-title')).toHaveText(/Admin account/i);
  await page.fill('#setup-domain', 'example.com');
  await page.fill('#setup-admin-email', 'webmaster@example.com');
  await page.fill('#setup-admin-recovery-email', 'recovery@example.net');
  await page.click('#setup-next');

  await expect(page.locator('#setup-passkey-primary-enabled')).toBeVisible();
  await expect(page.locator('#setup-passkey-primary-enabled')).toBeChecked();
  await page.fill('#setup-password', 'SecretPass123!');
  await page.fill('#setup-password-confirm', 'SecretPass123!');
  await page.uncheck('#setup-passkey-primary-enabled');
  await page.click('#setup-next');
  await expect(page.locator('#setup-progress-title')).toHaveText(/Ready to initialize/i);
  await expect(page.locator('#setup-summary-theme')).toHaveText(/Paper/i);
  await expect(page.locator('#setup-summary-updates')).toHaveText(/Manual updates only/i);
  await expect(page.locator('#setup-summary-recovery-email')).toHaveText(/recovery@example.net/i);
  await expect(page.locator('#setup-summary-passkey')).toHaveText(/disabled/i);
});

test('reader conversation rail supports direct thread navigation on desktop', async ({ page }) => {
  await mockThreadedMailScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await page.locator('.message-row-btn').first().click();
  await page.waitForTimeout(200);

  await expect(page.locator('#thread-strip')).not.toHaveClass(/hidden/);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 4 messages/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 2 of 4/i);
  await expect(page.locator('#thread-truncated')).toBeVisible();
  await expect(page.locator('#thread-list .thread-row')).toHaveCount(4);
  await expect(page.locator('#thread-list .thread-row-mailbox')).toHaveCount(2);
  await expect(page.locator('#thread-list .thread-row.active')).toHaveCount(1);
  const railMetrics = await page.locator('#thread-list .thread-row-btn').first().evaluate((node) => {
    const rowRect = node.getBoundingClientRect();
    const from = node.querySelector('.thread-row-from');
    const subject = node.querySelector('.thread-row-subject');
    const preview = node.querySelector('.thread-row-preview');
    return {
      rowHeight: Math.round(rowRect.height),
      fromTop: Math.round((from?.getBoundingClientRect().top || 0) - rowRect.top),
      subjectTop: Math.round((subject?.getBoundingClientRect().top || 0) - rowRect.top),
      previewTop: Math.round((preview?.getBoundingClientRect().top || 0) - rowRect.top),
    };
  });
  expect(railMetrics.rowHeight).toBeGreaterThanOrEqual(74);
  expect(railMetrics.subjectTop).toBeGreaterThan(railMetrics.fromTop + 10);
  expect(railMetrics.previewTop).toBeGreaterThan(railMetrics.subjectTop + 6);

  await page.click('#btn-thread-next');
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Re: Updates to OpenAI Privacy Policy/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 3 of 4/i);

  await page.locator('#mail-pane-reader').click();
  await page.keyboard.press('k');
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Fwd: Updates to OpenAI Privacy Policy/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 2 of 4/i);

  await page.keyboard.press('End');
  await expect(page.locator('#message-subject-anchor')).toHaveText(/^Updates to OpenAI Privacy Policy$/);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 4 of 4/i);

  await page.click('#btn-thread-collapse');
  await expect(page.locator('#btn-thread-collapse')).toHaveAttribute('aria-expanded', 'false');
  await expect(page.locator('#thread-list-wrap')).toHaveClass(/hidden/);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 4 messages/i);

  await page.click('#btn-thread-collapse');
  await expect(page.locator('#btn-thread-collapse')).toHaveAttribute('aria-expanded', 'true');
  await expect(page.locator('#thread-list-wrap')).not.toHaveClass(/hidden/);

  await page.locator('#thread-list .thread-row-btn').first().click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/^Updates to OpenAI Privacy Policy$/);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 1 of 4/i);
});

test('reader conversation rail stays visible for singleton threads', async ({ page }) => {
  await mockSingletonThreadScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.locator('.message-row-btn').first().click();
  await page.waitForTimeout(200);

  await expect(page.locator('#thread-strip')).not.toHaveClass(/hidden/);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 1 message/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing current message/i);
  await expect(page.locator('#btn-thread-prev')).toBeDisabled();
  await expect(page.locator('#btn-thread-next')).toBeDisabled();
  await expect(page.locator('#btn-thread-collapse')).toBeHidden();
});

test('reader conversation rail recovers when list summary lacks thread id but detail has it', async ({ page }) => {
  await mockThreadedSummaryFallbackScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.locator('.message-row-btn').first().click();
  await page.waitForTimeout(200);

  await expect(page.locator('#thread-strip')).not.toHaveClass(/hidden/);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 4 messages/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 2 of 4/i);
  await expect(page.locator('#thread-list .thread-row')).toHaveCount(4);
  await expectNoHorizontalOverflow(page, '.reader-view-controls');

  const readerActionLayout = await readReaderActionLayout(page);
  expect(readerActionLayout.borderLeftWidth).toBe('0px');
  expect(readerActionLayout.marginLeft).toBe('0px');
  expect(readerActionLayout.scrollWidth).toBeLessThanOrEqual(readerActionLayout.clientWidth + 1);

  await page.setViewportSize({ width: 1180, height: 900 });
  await page.waitForTimeout(200);
  await expectNoHorizontalOverflow(page, '.reader-view-controls');
});

test('reader conversation rail stays inline and tappable on mobile', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  await mockThreadedMailScenario(page);
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await page.locator('.message-row-btn').first().click();
  await page.waitForTimeout(200);

  await expect(page.locator('#view-mail')).toHaveAttribute('data-mobile-pane', 'reader');
  await expect(page.locator('#thread-strip')).not.toHaveClass(/hidden/);
  await expect(page.locator('#thread-list-wrap')).not.toHaveClass(/hidden/);
  await expect(page.locator('#thread-list .thread-row')).toHaveCount(4);
  await expect(page.locator('#btn-thread-collapse')).toBeHidden();
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await expectNoHorizontalOverflow(page, '.reader-view-controls');

  const wrapBox = await page.locator('#thread-list-wrap').boundingBox();
  expect(wrapBox).not.toBeNull();
  expect(wrapBox.height).toBeLessThan(190);

  await page.locator('#thread-list .thread-row-btn').nth(2).click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Re: Updates to OpenAI Privacy Policy/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 3 of 4/i);

  await context.close();
});

test('reader HTML preview resizes to content without clipping', async ({ page }) => {
  await mockReaderHTMLSizingScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const frameMetrics = async () => page.locator('#message-body-html').evaluate((node) => {
    if (!(node instanceof HTMLIFrameElement)) return null;
    const host = node.parentElement?.parentElement;
    const doc = node.contentDocument;
    const root = doc?.documentElement;
    const body = doc?.body;
    const docHeight = Math.max(
      Math.ceil(root?.scrollHeight || 0),
      Math.ceil(root?.offsetHeight || 0),
      Math.ceil(root?.getBoundingClientRect?.().height || 0),
      Math.ceil(body?.scrollHeight || 0),
      Math.ceil(body?.offsetHeight || 0),
      Math.ceil(body?.getBoundingClientRect?.().height || 0),
    );
    return {
      frameHeight: Math.ceil(node.getBoundingClientRect().height),
      hostHeight: Math.ceil(host?.clientHeight || 0),
      hostScrollHeight: Math.ceil(host?.scrollHeight || 0),
      hostScrollTop: Math.ceil(host?.scrollTop || 0),
      docHeight,
    };
  });

  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#btn-reader-view-html')).toHaveClass(/is-active/);
  await expect(page.locator('#message-body-html-wrap')).not.toHaveClass(/hidden/);
  await page.waitForTimeout(900);

  const longMetrics = await frameMetrics();
  expect(longMetrics).not.toBeNull();
  expect(longMetrics.docHeight).toBeGreaterThan(longMetrics.hostHeight);
  expect(longMetrics.frameHeight).toBeGreaterThanOrEqual(longMetrics.docHeight - 2);
  expect(longMetrics.hostScrollHeight).toBeGreaterThan(longMetrics.hostHeight);
  const longFontFamily = await page.locator('#message-body-html').evaluate((node) => {
    if (!(node instanceof HTMLIFrameElement)) return '';
    const doc = node.contentDocument;
    const win = node.contentWindow;
    const target = doc?.body?.querySelector('div');
    return target && win ? win.getComputedStyle(target).fontFamily : '';
  });
  expect(longFontFamily.toLowerCase()).toContain('georgia');

  await page.locator('.reader-body-host').evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  const scrolledMetrics = await frameMetrics();
  expect(scrolledMetrics.hostScrollTop).toBeGreaterThan(0);

  await page.click('#btn-reader-view-plain');
  await expect(page.locator('#message-body-html-wrap')).toHaveClass(/hidden/);
  await page.click('#btn-reader-view-html');
  await expect(page.locator('#message-body-html-wrap')).not.toHaveClass(/hidden/);
  await page.waitForTimeout(500);

  const reopenedMetrics = await frameMetrics();
  expect(reopenedMetrics.frameHeight).toBeGreaterThanOrEqual(reopenedMetrics.docHeight - 2);

  await page.locator('.message-row-btn').nth(1).click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Short HTML preview check/i);
  await page.waitForTimeout(400);

  const shortMetrics = await frameMetrics();
  expect(shortMetrics).not.toBeNull();
  expect(shortMetrics.docHeight).toBeLessThanOrEqual(shortMetrics.hostHeight + 2);
  expect(shortMetrics.frameHeight).toBeGreaterThanOrEqual(shortMetrics.hostHeight);
  const shortFontFamily = await page.locator('#message-body-html').evaluate((node) => {
    if (!(node instanceof HTMLIFrameElement)) return '';
    const doc = node.contentDocument;
    const win = node.contentWindow;
    const target = doc?.body?.querySelector('div');
    return target && win ? win.getComputedStyle(target).fontFamily : '';
  });
  expect(shortFontFamily.toLowerCase()).toContain('georgia');

  await page.setViewportSize({ width: 980, height: 760 });
  await page.waitForTimeout(900);

  const resizedMetrics = await frameMetrics();
  expect(resizedMetrics).not.toBeNull();
  expect(resizedMetrics.frameHeight).toBeGreaterThanOrEqual(resizedMetrics.docHeight - 2);
});

test('mail health hides quota controls when quota is unsupported', async ({ page }) => {
  await mockMailHealthQuotaScenario(page, { quotaSupported: false });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#btn-mail-health-toggle')).toBeVisible();
  await page.click('#btn-mail-health-toggle');
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await expectNoHorizontalOverflow(page, '#mail-toolbar-context-cluster');
  await expectNoHorizontalOverflow(page, '#mail-health-body');

  const toggleMetrics = await readHealthToggleMetrics(page);
  expect(toggleMetrics.scrollWidth).toBeLessThanOrEqual(toggleMetrics.clientWidth + 1);
  expect(toggleMetrics.copyRight).toBeLessThanOrEqual(toggleMetrics.caretLeft + 1);

  const toggleBox = await page.locator('#btn-mail-health-toggle').boundingBox();
  const moreBox = await page.locator('#mail-view-menu > summary').boundingBox();
  expect(toggleBox).not.toBeNull();
  expect(moreBox).not.toBeNull();
  expect(Math.abs(toggleBox.y - moreBox.y)).toBeLessThanOrEqual(2);

  const row = page.locator('.mail-health-row').first();
  await expect(row).toContainText('webmaster');
  await expect(row).toContainText('Retry Sync Now');
  await expect(row).toContainText('Rebuild Index');
  await expect(row).not.toContainText('Refresh Quota');
  await expect(row).not.toContainText('Quota unavailable on this server.');
  await expect(row.locator('.mail-health-meta-label')).toHaveText(['Sync']);
});

test('mail health keeps quota controls when quota is supported but not refreshed yet', async ({ page }) => {
  await mockMailHealthQuotaScenario(page, { quotaSupported: true, quotaAvailable: false });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#btn-mail-health-toggle')).toBeVisible();
  await page.click('#btn-mail-health-toggle');
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await expectNoHorizontalOverflow(page, '#mail-toolbar-context-cluster');
  await expectNoHorizontalOverflow(page, '#mail-health-body');

  const toggleMetrics = await readHealthToggleMetrics(page);
  expect(toggleMetrics.scrollWidth).toBeLessThanOrEqual(toggleMetrics.clientWidth + 1);
  expect(toggleMetrics.copyRight).toBeLessThanOrEqual(toggleMetrics.caretLeft + 1);

  const row = page.locator('.mail-health-row').first();
  await expect(row).toContainText('Refresh Quota');
  await expect(row).toContainText('Quota not refreshed yet.');
  await expect(row).toContainText('Not refreshed');
  await expect(row.locator('.mail-health-meta-label')).toHaveText(['Sync', 'Quota', 'Quota Refresh']);
});

test('indexed mail filters stay bounded on desktop and in paper theme', async ({ page }) => {
  await mockMailHealthQuotaScenario(page, { quotaSupported: true, quotaAvailable: false });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const filtersOpened = await openMailFilters(page);
  expect(filtersOpened).toBe(true);
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await expectNoHorizontalOverflow(page, '#mail-filter-advanced');
  await expectNoHorizontalOverflow(page, '.mail-filter-toggle-row');

  await page.click('#btn-theme');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'paper-light');
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await expectNoHorizontalOverflow(page, '#mail-filter-advanced');
});

test('indexed mail filters stay bounded on mobile', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  await mockMailHealthQuotaScenario(page, { quotaSupported: true, quotaAvailable: false });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const filtersOpened = await openMailFilters(page);
  expect(filtersOpened).toBe(true);
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await expectNoHorizontalOverflow(page, '#mail-filter-advanced');
  await expectNoHorizontalOverflow(page, '.mail-filter-toggle-row');

  await context.close();
});

test('notification surfaces follow paper theme and update available keeps toast plus centre', async ({ page }) => {
  const initialStatus = buildUpdaterStatus({ update_available: false });
  await mockUpdaterNotificationScenario(page, {
    statusSequence: [initialStatus],
    checkStatus: buildUpdaterStatus(),
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.click('#btn-theme');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'paper-light');
  await page.click('#tab-admin');
  await page.click('#btn-update-check');

  const updateToast = page.locator('.status-toast--update').last();
  await expect(updateToast).toContainText('Update Available');
  await expect(page.locator('#notification-unread-badge')).not.toHaveClass(/hidden/);
  const toastTone = await readLocatorBackgroundLuminance(updateToast);
  await page.click('#btn-notification-center');
  await expect(page.locator('.notification-card-title', { hasText: 'Update Available' })).toBeVisible();

  const panelTone = await readBackgroundLuminance(page, '#notification-center-panel');
  expect(panelTone.luminance).toBeGreaterThan(150);
  expect(toastTone.luminance).toBeGreaterThan(150);
});

test('routine mail load and search stay out of the notification centre', async ({ page }) => {
  await mockMailActionScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('#notification-unread-badge')).toHaveClass(/hidden/);
  await page.click('#btn-notification-center');
  await expect(page.locator('#notification-center-list')).toContainText('All clear');
  await page.click('#btn-notification-center');

  await page.fill('#search-input', 'alice');
  await page.click('#btn-search');
  await expect(page.locator('#messages .message-row-btn')).toHaveCount(1);
  await expect(page.locator('#notification-unread-badge')).toHaveClass(/hidden/);

  await page.reload({ waitUntil: 'networkidle' });
  await expect(page.locator('#notification-unread-badge')).toHaveClass(/hidden/);
  await page.click('#btn-notification-center');
  await expect(page.locator('#notification-center-list')).toContainText('All clear');
});

test('conversation triage snooze hides threads from Inbox, shows them in Snoozed, and restores after time advances', async ({ page }) => {
  const control = await mockMailTriageScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await page.locator('.message-row[data-message-id="tri-1-msg-1"] .message-row-btn').click();
  await page.locator('#reader-action-controls [data-triage-menu="reader-snooze"] > summary').click();
  await page.locator('#reader-action-controls [data-triage-command="snooze-preset"][data-triage-preset="tomorrow"]').click();
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn')).not.toContainText('Budget review');

  const snoozedButton = page.locator('#mailboxes .mailbox-row button', { hasText: /^Snoozed/i });
  const inboxButton = page.locator('#mailboxes .mailbox-row button', { hasText: /^Inbox/i });
  await snoozedButton.click();
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn').first()).toContainText('Budget review');

  await inboxButton.click();
  const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
  await page.locator('.message-row[data-message-id="tri-2-msg-1"] .message-row-btn').click({ modifiers: [modifier] });
  await expect(page.locator('#mail-selection-count')).toHaveText('1 selected');
  await page.locator('#mail-selection-bar [data-triage-menu="snooze"] > summary').click();
  await page.locator('#mail-selection-bar [data-triage-command="snooze-preset"][data-triage-preset="tomorrow"]').click();
  await expect(page.locator('.message-row-btn')).toHaveCount(0);

  await snoozedButton.click();
  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await expect(page.locator('#messages')).toContainText('Budget review');
  await expect(page.locator('#messages')).toContainText('Vendor invoice');

  control.advanceTime(48 * 60 * 60 * 1000);
  await page.reload({ waitUntil: 'networkidle' });

  await inboxButton.click();
  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await snoozedButton.click();
  await expect(page.locator('.message-empty')).toContainText(/No messages to display/i);
});

test('conversation triage follow-up reminders stay quiet until due and notification click-through reopens the conversation', async ({ page }) => {
  const control = await mockMailTriageScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.locator('.message-row[data-message-id="tri-1-msg-1"] .message-row-btn').click();
  await page.locator('#reader-action-controls [data-triage-menu="reader-remind"] > summary').click();
  await page.locator('#reader-action-controls [data-triage-command="remind-preset"][data-triage-preset="later_today"]').click();
  await expect(page.locator('#reader-triage-chips')).toContainText('Follow Up');
  await expect(page.locator('#notification-unread-badge')).toHaveClass(/hidden/);

  const followUpButton = page.locator('#mailboxes .mailbox-row button', { hasText: /^Follow Up/i });
  await followUpButton.click();
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn').first()).toContainText('Budget review');

  control.advanceTime(24 * 60 * 60 * 1000);
  await page.reload({ waitUntil: 'networkidle' });
  await page.click('#tab-mail');

  await expect(page.locator('#notification-unread-badge')).not.toHaveClass(/hidden/);
  await page.click('#btn-notification-center');
  await expect(page.locator('.notification-card', { hasText: 'Follow Up Due' })).toHaveCount(1);
  await expect(page.locator('#notification-center-list')).toContainText('Budget review');
  await page.click('#btn-notification-center');

  await page.reload({ waitUntil: 'networkidle' });
  await page.click('#btn-notification-center');
  const dueCard = page.locator('.notification-card', { hasText: 'Follow Up Due' }).first();
  await expect(page.locator('.notification-card', { hasText: 'Follow Up Due' })).toHaveCount(1);
  await dueCard.click();

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('#mailboxes .mailbox-row button.active', { hasText: /^Follow Up/i })).toBeVisible();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Budget review/i);
});

test('conversation triage labels create on type, filter cleanly, and stay editable from Mail settings', async ({ page }) => {
  await mockMailTriageScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.locator('.message-row[data-message-id="tri-1-msg-1"] .message-row-btn').click();
  await page.locator('#reader-action-controls [data-triage-menu="reader-labels"] > summary').click();
  await page.locator('#reader-action-controls [data-triage-command="set-category"]').click();
  await expect(page.locator('#ui-modal-title')).toHaveText(/Set Category/i);
  await page.fill('#ui-modal-input', 'Finance');
  await page.click('#ui-modal-confirm');

  await page.locator('#reader-action-controls [data-triage-menu="reader-labels"] > summary').click();
  await page.locator('#reader-action-controls [data-triage-command="add-tags"]').click();
  await expect(page.locator('#ui-modal-title')).toHaveText(/Add Tags/i);
  await page.fill('#ui-modal-input', 'urgent, vendor');
  await page.click('#ui-modal-confirm');
  await expect(page.locator('#reader-triage-chips')).toContainText('Finance');
  await expect(page.locator('#reader-triage-chips')).toContainText('urgent');
  await expect(page.locator('#reader-triage-chips')).toContainText('vendor');

  await page.click('#btn-theme');
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'paper-light');

  const filtersOpened = await openMailFilters(page);
  expect(filtersOpened).toBe(true);
  await expectNoHorizontalOverflow(page, '#mail-filter-advanced');
  await page.selectOption('#mail-filter-category', { label: 'Finance' });
  await page.fill('#mail-filter-tags', 'urgent');
  await page.click('#btn-search');
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn').first()).toContainText('Budget review');

  await page.click('#tab-settings');
  await page.click('#settings-nav-mail');
  await expect(page.locator('#settings-section-mail')).not.toHaveClass(/hidden/);
  await expect(page.locator('#settings-mail-triage-category-list')).toContainText('Finance');
  await expect(page.locator('#settings-mail-triage-tag-list')).toContainText('urgent');
  await expect(page.locator('#settings-mail-triage-tag-list')).toContainText('vendor');

  const financeRow = page.locator('#settings-mail-triage-category-list .setting-list-item', { hasText: 'Finance' }).first();
  await financeRow.click();
  await financeRow.locator('.setting-list-action').click();
  await expect(page.locator('#ui-modal-title')).toHaveText(/Rename Category/i);
  await page.fill('#ui-modal-input', 'Finance Ops');
  await page.click('#ui-modal-confirm');
  await expect(page.locator('#settings-mail-triage-category-list')).toContainText('Finance Ops');

  const vendorRow = page.locator('#settings-mail-triage-tag-list .setting-list-item', { hasText: 'vendor' }).first();
  await vendorRow.click();
  await vendorRow.locator('.setting-list-action').click();
  await expect(page.locator('#ui-modal-title')).toHaveText(/Rename Tag/i);
  await page.fill('#ui-modal-input', 'payables');
  await page.click('#ui-modal-confirm');
  await expect(page.locator('#settings-mail-triage-tag-list')).toContainText('payables');

  const urgentRow = page.locator('#settings-mail-triage-tag-list .setting-list-item', { hasText: 'urgent' }).first();
  await urgentRow.click();
  await expect(page.locator('#btn-settings-mail-triage-tag-delete')).toBeEnabled();
  await page.click('#btn-settings-mail-triage-tag-delete');
  await expect(page.locator('#ui-modal-title')).toHaveText(/Delete tag/i);
  await page.click('#ui-modal-confirm');
  await expect(page.locator('#settings-mail-triage-tag-list')).not.toContainText('urgent');

  await page.click('#tab-mail');
  await expect(page.locator('#view-mail')).toBeVisible();
  const filtersOpenAlready = await page.locator('#mail-filter-advanced').evaluate((node) => !node.classList.contains('hidden'));
  if (!filtersOpenAlready) {
    const filtersOpenedAgain = await openMailFilters(page);
    expect(filtersOpenedAgain).toBe(true);
  }
  await expect(page.locator('#mail-filter-category')).toContainText('Finance Ops');
  await page.selectOption('#mail-filter-category', { label: 'Finance Ops' });
  await page.fill('#mail-filter-tags', 'payables');
  await page.click('#btn-search');
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#reader-triage-chips')).toContainText('Finance Ops');
  await expect(page.locator('#reader-triage-chips')).toContainText('payables');
  await expect(page.locator('#reader-triage-chips')).not.toContainText('urgent');
});

test('error notifications show both toast and persistent centre entry', async ({ page }) => {
  await mockUpdaterNotificationScenario(page, {
    checkError: {
      status: 500,
      code: 'update_check_failed',
      message: 'Update check failed because the release feed is unavailable.',
    },
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.click('#tab-admin');
  await page.click('#btn-update-check');

  const errorToast = page.locator('.status-toast--error').last();
  await expect(errorToast).toContainText('Update check failed because the release feed is unavailable.');
  await expect(page.locator('#notification-unread-badge')).not.toHaveClass(/hidden/);
  await page.click('#btn-notification-center');
  await expect(page.locator('.notification-card-title', { hasText: 'Needs Attention' })).toBeVisible();
  await expect(page.locator('#notification-center-list')).toContainText('Update check failed because the release feed is unavailable.');
});

test('legacy notifications migrate into v2 and prune old or excess entries', async ({ page }) => {
  const now = Date.now();
  const legacyItems = Array.from({ length: 26 }, (_, index) => ({
    id: `legacy-error-${index}`,
    kind: 'error',
    title: `Legacy Error ${index}`,
    body: `Problem ${index}`,
    created_at: new Date(now - (index * 60 * 1000)).toISOString(),
    read: false,
  }));
  legacyItems.push({
    id: 'legacy-success',
    kind: 'success',
    title: 'Legacy Success',
    body: 'This should be dropped during migration.',
    created_at: new Date(now - (5 * 60 * 1000)).toISOString(),
    read: false,
  });
  legacyItems.push({
    id: 'legacy-expired',
    kind: 'error',
    title: 'Expired Legacy Error',
    body: 'Too old to keep.',
    created_at: new Date(now - (16 * 24 * 60 * 60 * 1000)).toISOString(),
    read: false,
  });

  await page.addInitScript((items) => {
    localStorage.setItem('ui.notifications.v1', JSON.stringify(items));
  }, legacyItems);
  await mockUpdaterNotificationScenario(page, {
    statusSequence: [buildUpdaterStatus({ update_available: false })],
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#notification-unread-badge')).toHaveText('24');
  await page.click('#btn-notification-center');
  await expect(page.locator('.notification-card')).toHaveCount(24);
  await expect(page.locator('#notification-center-list')).not.toContainText('Legacy Success');
  await expect(page.locator('#notification-center-list')).not.toContainText('Expired Legacy Error');

  const storage = await page.evaluate(() => ({
    current: JSON.parse(localStorage.getItem('ui.notifications.v2') || '[]').length,
  }));
  expect(storage.current).toBe(24);
});

test('updater status stays truthful for scheduled releases and notifications use human versions', async ({ page }) => {
  const historicalCompleted = buildUpdaterStatus({
    current: {
      version: 'v1.1.0-alpha.1',
      commit: '8fhb424234b2sbf',
      build_time: '2026-03-12T12:00:00.000Z',
      source_repo: 'https://github.com/2high4schooltoday/despatch',
    },
    apply: {
      state: 'completed',
      request_id: 'apply-old',
      target_version: 'v1.1.0-alpha.1',
      to_version: 'v1.1.0-alpha.1',
      error: '',
      finished_at: '2026-03-12T12:15:00.000Z',
    },
  });
  const scheduledNext = buildUpdaterStatus({
    current: historicalCompleted.current,
    apply: historicalCompleted.apply,
    auto_update: {
      enabled: true,
      state: 'scheduled',
      target_version: 'v1.1.0-alpha.1_01',
      downloaded_at: '2026-03-12T15:10:00.000Z',
      scheduled_for: '2026-03-13T02:00:00.000Z',
      error: '',
    },
  });

  await mockUpdaterNotificationScenario(page, {
    statusSequence: [historicalCompleted],
    checkStatus: scheduledNext,
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.click('#tab-admin');
  await expect(page.locator('#update-current-version')).toHaveText('Alpha 1.1.0');
  await expect(page.locator('#update-latest-version')).toHaveText('Alpha 1.1.0_01');
  await expect(page.locator('#update-current-commit')).toHaveClass(/hidden/);

  await page.click('#btn-update-check');
  await expect(page.locator('#update-hero-headline')).toHaveText(/Update scheduled/i);
  await expect(page.locator('#update-hero-subline')).toContainText('Alpha 1.1.0_01');
  await expect(page.locator('#update-note')).toContainText(/scheduled/i);
  await expect(page.locator('.status-toast')).toHaveCount(0);

  await expect(page.locator('#notification-unread-badge')).not.toHaveClass(/hidden/);
  await page.click('#btn-notification-center');
  await expect(page.locator('#notification-center')).toBeVisible();
  await expect(page.locator('.notification-card-title', { hasText: 'Update Scheduled' })).toBeVisible();
  await expect(page.locator('#notification-center-list')).toContainText('Alpha 1.1.0_01');
  await expect(page.locator('#notification-center-list')).not.toContainText('Update Installed');
  await expect(page.locator('#notification-center-list')).not.toContainText('v1.1.0-alpha.1_01');

  await page.reload({ waitUntil: 'networkidle' });
  await page.click('#tab-admin');
  await page.click('#btn-notification-center');
  await expect(page.locator('#notification-center-list')).toContainText('Update Scheduled');
  await expect(page.locator('#notification-center-list')).toContainText('Alpha 1.1.0_01');
});

test('updater completion notification appears once and routes back to admin system', async ({ page }) => {
  const applyingStatus = buildUpdaterStatus({
    current: {
      version: 'v1.1.0-alpha.1',
      commit: '8fhb424234b2sbf',
      build_time: '2026-03-12T12:00:00.000Z',
      source_repo: 'https://github.com/2high4schooltoday/despatch',
    },
    update_available: false,
    apply: {
      state: 'in_progress',
      request_id: 'apply-new',
      target_version: 'v1.1.0-alpha.1_01',
      to_version: '',
      error: '',
      finished_at: '',
    },
    auto_update: {
      enabled: true,
      state: 'applying',
      target_version: 'v1.1.0-alpha.1_01',
      downloaded_at: '2026-03-12T15:10:00.000Z',
      scheduled_for: '',
      error: '',
    },
  });
  const completedStatus = buildUpdaterStatus({
    current: {
      version: 'v1.1.0-alpha.1_01',
      commit: '9abcedf234bcde12',
      build_time: '2026-03-12T15:18:00.000Z',
      source_repo: 'https://github.com/2high4schooltoday/despatch',
    },
    update_available: false,
    apply: {
      state: 'completed',
      request_id: 'apply-new',
      target_version: 'v1.1.0-alpha.1_01',
      to_version: 'v1.1.0-alpha.1_01',
      error: '',
      finished_at: '2026-03-12T15:18:30.000Z',
    },
    auto_update: {
      enabled: true,
      state: 'idle',
      target_version: '',
      downloaded_at: '',
      scheduled_for: '',
      error: '',
    },
  });

  await mockUpdaterNotificationScenario(page, {
    statusSequence: [applyingStatus, completedStatus, completedStatus],
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.click('#tab-admin');
  await expect(page.locator('#update-hero-headline')).toHaveText(/Installing update/i);
  await page.waitForTimeout(2800);
  await expect(page.locator('#update-hero-headline')).toHaveText(/up to date/i);

  await page.click('#tab-mail');
  await expect(page.locator('#view-mail')).toBeVisible();

  await expect(page.locator('#notification-unread-badge')).not.toHaveClass(/hidden/);
  await page.click('#btn-notification-center');
  const installedCard = page.locator('.notification-card', { hasText: 'Update Installed' }).first();
  await expect(installedCard).toContainText('Alpha 1.1.0_01');
  await expect(installedCard).not.toContainText('v1.1.0-alpha.1_01');
  await installedCard.click();

  await expect(page.locator('#view-admin')).toBeVisible();
  await expect(page.locator('#admin-section-system')).not.toHaveClass(/hidden/);
  await expect(page.locator('#update-current-version')).toHaveText('Alpha 1.1.0_01');
  await page.click('#btn-notification-center');
  await expect(page.locator('.notification-card', { hasText: 'Update Installed' })).toHaveCount(1);
});

test('compose drafts keep media and retry send after temporary failure', async ({ page }) => {
  await mockComposeReliabilityScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await page.locator('.message-row-btn').first().click();
  await page.waitForTimeout(200);

  await page.click('#btn-reply');
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-title')).toHaveText(/Reply/i);
  await expect(page.locator('#compose-subject-input')).toHaveValue(/Re: Draft reliability check/i);

  await page.setInputFiles('#compose-attachments-input', {
    name: 'numbers.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('q1,q2,q3'),
  });
  await expect(page.locator('#compose-assets')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(1);
  await expect(page.locator('#compose-assets-list')).toContainText('numbers.txt');
  await expect(page.locator('#compose-assets-list')).toContainText('Ready');
  await expect(page.locator('#compose-editor [data-compose-attachment-id]')).toHaveCount(0);

  await page.setInputFiles('#compose-attachments-input', {
    name: 'chart.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/w8AAgMBgJ4M3hQAAAAASUVORK5CYII=', 'base64'),
  });
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(2);
  const inlinePreview = page.locator('#compose-editor img[data-compose-inline-image-id]').first();
  await expect(inlinePreview).toBeVisible();
  await expect(inlinePreview).toHaveAttribute('src', /\/api\/v2\/drafts\/draft-\d+\/attachments\/att-\d+/);
  await expect(page.locator('#btn-compose-send')).toBeEnabled();

  await page.keyboard.press('Escape');
  await page.waitForTimeout(1200);

  const draftsMailbox = page.locator('#mailboxes .mailbox-row button[role="option"]', { hasText: /^Drafts/ });
  await draftsMailbox.click();
  await expect(draftsMailbox).toHaveClass(/active/);
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn .message-context-badge')).toHaveText(/Reply/i);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-title')).toHaveText(/Reply/i);
  await expect(page.locator('#compose-subject-input')).toHaveValue('Re: Draft reliability check');
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(2);
  await expect(page.locator('#compose-assets-list')).toContainText('numbers.txt');
  await expect(page.locator('#compose-assets-list')).toContainText('chart.png');
  await expect(page.locator('#compose-editor')).toContainText('Please reply with the latest numbers.');
  const reopenedInline = page.locator('#compose-editor img[data-compose-inline-image-id]').first();
  await expect(reopenedInline).toBeVisible();
  await expect(reopenedInline).toHaveAttribute('src', /\/api\/v2\/drafts\/draft-\d+\/attachments\/att-\d+/);

  await page.click('#btn-compose-send');
  await expect(page.locator('#compose-draft-note')).toContainText(/Send failed/i);
  await expect(page.locator('#btn-compose-send')).toHaveText(/Retry send/i);
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(2);
  await expect(page.locator('#compose-subject-input')).toHaveValue('Re: Draft reliability check');

  await page.keyboard.press('Escape');
  await page.waitForTimeout(300);
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#compose-draft-note')).toContainText(/Send failed/i);
  await expect(page.locator('#btn-compose-send')).toHaveText(/Retry send/i);

  await page.click('#btn-compose-send');
  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('.message-row-btn')).toHaveCount(0);
});

test('compose delivery stays on Schedule across stale autosave responses', async ({ page }) => {
  const runtime = await mockComposeReliabilityScenario(page, { delayFirstPatchMs: 1400 });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await page.click('#btn-compose-open');
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-send-mode')).toHaveValue('send_now');

  await page.fill('#compose-to-input', 'alice@example.com');
  await page.fill('#compose-subject-input', 'Scheduled compose check');
  await page.locator('#compose-editor').click();
  await page.keyboard.type('First saved body.');
  await page.waitForTimeout(1100);

  await page.locator('#compose-editor').click();
  await page.keyboard.type(' More text to trigger a delayed save.');
  await page.waitForTimeout(950);

  await page.locator('#compose-send-mode').selectOption('scheduled');
  await expect(page.locator('#compose-send-mode')).toHaveValue('scheduled');
  await expect(page.locator('#compose-scheduled-for')).toBeVisible();
  await expect(page.locator('#btn-compose-send')).toHaveText(/Schedule/i);

  await page.waitForTimeout(1600);
  await expect(page.locator('#compose-send-mode')).toHaveValue('scheduled');
  await expect(page.locator('#compose-scheduled-for')).toBeVisible();
  await expect(page.locator('#btn-compose-send')).toHaveText(/Schedule/i);
  expect((await page.locator('#compose-scheduled-for').inputValue()).trim()).not.toBe('');

  await page.click('#btn-compose-send');
  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('#status-line')).toContainText(/scheduled/i);
  expect(runtime.lastSendDraft).not.toBeNull();
  expect(String(runtime.lastSendDraft?.send_mode || '')).toBe('scheduled');
  expect(String(runtime.lastSendDraft?.scheduled_for || '')).not.toBe('');
});

test('compose HTML mode previews source live and restores draft mode', async ({ page }) => {
  await mockComposeReliabilityScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await page.click('#btn-compose-open');
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-editor')).toBeVisible();

  await page.click('#btn-compose-mode-html');
  await expect(page.locator('#compose-html-workspace')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-editor')).toHaveClass(/hidden/);
  await expect(page.locator('#compose-toggle-formatting')).toHaveClass(/hidden/);
  await expect(page.locator('#btn-compose-mode-html')).toHaveAttribute('aria-pressed', 'true');

  await page.fill('#compose-to-input', 'alice@example.com');
  await page.fill('#compose-subject-input', 'HTML compose draft');
  await page.locator('#compose-html-input').fill([
    '<style>',
    'h1 { color: rgb(255, 0, 0); }',
    '</style>',
    '<h1>Hello HTML</h1>',
    '<p>Preview body</p>',
  ].join('\n'));
  await expect(page.locator('#compose-html-gutter-lines')).toContainText('5');
  await expect(page.frameLocator('#compose-html-preview').locator('h1')).toHaveText('Hello HTML');
  await expect(page.frameLocator('#compose-html-preview').locator('p')).toHaveText('Preview body');

  await page.setInputFiles('#compose-attachments-input', {
    name: 'template.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/w8AAgMBgJ4M3hQAAAAASUVORK5CYII=', 'base64'),
  });
  await expect(page.locator('#compose-editor img[data-compose-inline-image-id]')).toHaveCount(0);
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(1);

  await page.waitForTimeout(1200);
  await page.keyboard.press('Escape');
  await page.waitForTimeout(250);

  const draftsMailbox = page.locator('#mailboxes .mailbox-row button[role="option"]', { hasText: /^Drafts/ });
  await draftsMailbox.click();
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-html-workspace')).not.toHaveClass(/hidden/);
  await expect(page.locator('#btn-compose-mode-html')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#compose-html-input')).toHaveValue(/Hello HTML/);
  await expect(page.frameLocator('#compose-html-preview').locator('h1')).toHaveText('Hello HTML');
});

test('compose HTML mode only switches back to rich text for simple fragments', async ({ page }) => {
  await mockComposeReliabilityScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await page.click('#btn-compose-open');
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);

  await page.click('#btn-compose-mode-html');
  await page.locator('#compose-html-input').fill('<p>Simple <strong>fragment</strong></p>');
  await page.click('#btn-compose-mode-rich');
  await expect(page.locator('#compose-html-workspace')).toHaveClass(/hidden/);
  await expect(page.locator('#compose-editor')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-editor')).toContainText('Simple');

  await page.click('#btn-compose-mode-html');
  await page.locator('#compose-html-input').fill('<!doctype html><html><head><style>body { color: red; }</style></head><body><p>Advanced document</p></body></html>');
  await page.click('#btn-compose-mode-rich');
  await expect(page.locator('#compose-html-workspace')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-html-note')).toContainText(/Keep editing this message in HTML mode/i);
  await expect(page.locator('#btn-compose-mode-html')).toHaveAttribute('aria-pressed', 'true');
});

test('indexed account mail opens and sends without unlocking session mail', async ({ page }) => {
  const counters = await mockIndexedAccountLockedScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('#mail-index-status')).toContainText(/Indexed/i);
  await expect(page.locator('#ui-modal-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('.message-row-btn')).toHaveCount(1);

  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#message-subject-anchor')).toContainText('Indexed reply target');

  await page.click('#btn-reply');
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-from-select')).toHaveValue('ident-indexed');
  await expect(page.locator('#compose-subject-input')).toHaveValue('Re: Indexed reply target');

  await page.click('#btn-compose-send');
  await expect(page.locator('#ui-modal-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#ui-modal-title')).toHaveText(/Choose Sent Mailbox/i);
  await page.fill('#ui-modal-input', 'Sent');
  await page.click('#ui-modal-confirm');

  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('#status-line')).toContainText(/Saved to Sent/i);
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Sent/i })).toBeVisible();
  expect(counters.unlockCalls).toBe(0);
  expect(counters.legacyMailboxCalls).toBe(0);
  expect(counters.legacyMessageCalls).toBe(0);
  expect(counters.accountSpecialCalls).toBe(1);
});

test('indexed Sent view keeps optimistic rows visible while the mailbox index catches up', async ({ page }) => {
  await mockIndexedSentRefreshLagScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Sent/i }).click();
  await expect(page.locator('.message-row-btn')).toHaveCount(0);

  await page.click('#btn-compose-open');
  await page.fill('#compose-to-input', 'alice@example.com');
  await page.keyboard.press('Enter');
  await page.fill('#compose-subject-input', 'Laggy sent row');
  await page.locator('#compose-editor').click();
  await page.keyboard.type('This sent copy should stay visible while refresh is stale.');
  await expect(page.locator('#btn-compose-send')).toBeEnabled();
  await page.click('#btn-compose-send');

  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn').first()).toContainText(/Laggy sent row/i);
  await expect(page.locator('.message-row-btn').first()).toContainText(/Syncing/i);

  await page.waitForTimeout(500);
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await expect(page.locator('.message-row-btn').first()).toContainText(/Laggy sent row/i);
});

test('mail identities and signatures stay editable, replace untouched inserts, and reopen without duplication', async ({ page }) => {
  await mockMailIdentityScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await page.click('#tab-settings');
  await page.click('#settings-nav-mail');
  await expect(page.locator('#settings-section-mail')).not.toHaveClass(/hidden/);
  await expect(page.locator('#settings-mail-session-display-name')).toHaveValue('Admin Session');
  await expect(page.locator('#settings-mail-account-list')).toContainText('Support Account');
  await expect(page.locator('#settings-mail-identity-list')).toContainText('Support Team');

  await page.fill('#settings-mail-session-display-name', 'Webmaster Admin');
  await page.locator('#settings-mail-session-signature').evaluate((node) => {
    node.innerHTML = '<p>Warm regards</p>';
  });
  await page.click('#btn-settings-mail-session-save');
  await expect(page.locator('#settings-mail-note')).toContainText(/Session sender saved/i);

  await page.click('#tab-mail');
  await page.click('#btn-compose-open');
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-from-row')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-from-select')).toContainText('Webmaster Admin <admin@example.com>');
  await expect(page.locator('#compose-from-select')).toContainText('Support Team <support@example.com>');
  await expect(page.locator('.compose-signature-block')).toHaveCount(1);
  await expect(page.locator('.compose-signature-body')).toContainText('Warm regards');

  await page.selectOption('#compose-from-select', 'ident-support');
  await expect(page.locator('.compose-signature-body')).toContainText('Support Signature');

  await page.locator('.compose-signature-body').evaluate((node) => {
    node.innerHTML = '<p>Custom Signature</p>';
  });
  await page.selectOption('#compose-from-select', 'session-admin');
  await expect(page.locator('.compose-signature-body')).toContainText('Custom Signature');

  await page.click('#btn-compose-discard');
  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);

  await page.locator('.message-row-btn').first().click();
  await page.click('#btn-reply');
  await expect(page.locator('#compose-title')).toHaveText(/Reply/i);
  await expect(page.locator('.compose-signature-block')).toHaveCount(1);
  await expect(page.locator('.compose-quoted-block[data-compose-quoted="reply"]')).toHaveCount(1);
  const replyOrder = await page.locator('#compose-editor').evaluate((node) => {
    const signature = node.querySelector('.compose-signature-block');
    const quoted = node.querySelector('.compose-quoted-block[data-compose-quoted="reply"]');
    if (!(signature instanceof HTMLElement) || !(quoted instanceof HTMLElement)) return null;
    return {
      signatureTop: signature.offsetTop,
      quotedTop: quoted.offsetTop,
    };
  });
  expect(replyOrder).not.toBeNull();
  expect(replyOrder.signatureTop).toBeLessThan(replyOrder.quotedTop);

  await page.keyboard.press('Escape');
  await page.waitForTimeout(1200);
  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Drafts/i }).click();
  await expect(page.locator('.message-row-btn')).toHaveCount(1);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#compose-title')).toHaveText(/Reply/i);
  await expect(page.locator('.compose-signature-block')).toHaveCount(1);
  await expect(page.locator('.compose-quoted-block[data-compose-quoted="reply"]')).toHaveCount(1);
  await expect(page.locator('.compose-signature-body')).toContainText('Warm regards');
  await page.click('#btn-compose-discard');
  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Inbox/i }).click();
  await page.locator('.message-row-btn').first().click();
  await page.click('#btn-forward');
  await expect(page.locator('#compose-title')).toHaveText(/Forward/i);
  await expect(page.locator('.compose-signature-block')).toHaveCount(1);
  await expect(page.locator('.compose-quoted-block[data-compose-quoted="forward"]')).toHaveCount(1);
  const forwardOrder = await page.locator('#compose-editor').evaluate((node) => {
    const signature = node.querySelector('.compose-signature-block');
    const quoted = node.querySelector('.compose-quoted-block[data-compose-quoted="forward"]');
    if (!(signature instanceof HTMLElement) || !(quoted instanceof HTMLElement)) return null;
    return {
      signatureTop: signature.offsetTop,
      quotedTop: quoted.offsetTop,
    };
  });
  expect(forwardOrder).not.toBeNull();
  expect(forwardOrder.signatureTop).toBeLessThan(forwardOrder.quotedTop);
  await page.click('#btn-compose-discard');
});

test('mailbox state resolves Sent on first send, updates thread context, and inserts sent rows immediately', async ({ page }) => {
  await mockReliableMailboxStateScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const inboxButton = page.locator('#mailboxes .mailbox-row button', { hasText: /^Inbox/i });
  await expect(inboxButton).toContainText('(1)');

  await page.locator('.message-row-btn').first().click();
  await expect(inboxButton).not.toContainText('(1)');
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 1 message/i);

  await page.click('#btn-reply');
  await page.click('#btn-compose-send');

  await expect(page.locator('#ui-modal-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#ui-modal-title')).toHaveText(/Choose Sent Mailbox/i);
  await expect(page.locator('#ui-modal-input')).toHaveValue('Sent');
  await page.click('#ui-modal-confirm');

  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('#status-line')).toContainText(/Saved to Sent/i);
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Sent/i })).toBeVisible();
  await expect(page.locator('#message-meta')).toContainText(/Replied/i);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 2 messages/i);

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Sent/i }).click();
  await expect(page.locator('.message-row-btn')).toHaveCount(1);

  await page.click('#btn-compose-open');
  await page.fill('#compose-to-input', 'alice@example.com');
  await page.keyboard.press('Enter');
  await page.fill('#compose-subject-input', 'Immediate sent row');
  await page.fill('#compose-editor', 'Second send body.');
  await page.click('#btn-compose-send');

  await expect(page.locator('#compose-overlay')).toHaveClass(/hidden/);
  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await expect(page.locator('.message-row-btn').first()).toContainText(/Immediate sent row/i);
});

test('mailbox core actions use row selection, create archive or trash folders on demand, and move messages', async ({ page }) => {
  await mockMailActionScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('.message-row-btn')).toHaveCount(4);
  await expect(page.locator('.message-row-check')).toHaveCount(0);
  await expect(page.locator('#mail-selection-tools')).toHaveClass(/hidden/);
  const inboxButton = page.locator('#mailboxes .mailbox-row button', { hasText: /^Inbox/i });
  await expect(inboxButton).toContainText('(1)');

  const rowMetrics = await page.locator('.message-row-btn').first().evaluate((node) => {
    const rowRect = node.getBoundingClientRect();
    const from = node.querySelector('.message-from');
    const subject = node.querySelector('.message-subject');
    const preview = node.querySelector('.message-preview');
    return {
      rowHeight: Math.round(rowRect.height),
      fromTop: Math.round((from?.getBoundingClientRect().top || 0) - rowRect.top),
      subjectTop: Math.round((subject?.getBoundingClientRect().top || 0) - rowRect.top),
      previewTop: Math.round((preview?.getBoundingClientRect().top || 0) - rowRect.top),
    };
  });
  expect(rowMetrics.rowHeight).toBeGreaterThanOrEqual(74);
  expect(rowMetrics.subjectTop).toBeGreaterThan(rowMetrics.fromTop + 10);
  expect(rowMetrics.previewTop).toBeGreaterThan(rowMetrics.subjectTop + 6);
  const heights = await page.locator('.message-row-btn').evaluateAll((nodes) => nodes.slice(0, 3).map((node) => Math.round(node.getBoundingClientRect().height)));
  expect(Math.max(...heights) - Math.min(...heights)).toBeLessThanOrEqual(2);

  await page.locator('.message-row[data-message-id="m3"] .message-row-btn').click();
  await expect(inboxButton).not.toContainText('(1)');
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Unread');
  await page.click('#btn-mark-seen');
  await expect(inboxButton).toContainText('(1)');
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Read');
  await page.click('#btn-mark-seen');
  await expect(inboxButton).not.toContainText('(1)');

  const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
  await page.locator('.message-row[data-message-id="m1"] .message-row-btn').click({ modifiers: [modifier] });
  await expect(page.locator('#mail-selection-tools')).not.toHaveClass(/hidden/);
  await expect(page.locator('#mail-selection-count')).toHaveText('1 selected');
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Unread');
  await page.click('#btn-mark-seen');
  await expect(page.locator('.message-row[data-message-id="m1"]')).toHaveClass(/is-unread/);
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Read');

  await page.click('#btn-mark-seen');
  await expect(page.locator('.message-row[data-message-id="m1"]')).not.toHaveClass(/is-unread/);
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Unread');

  await page.click('#btn-mail-clear');
  await expect(page.locator('#mail-selection-tools')).toHaveClass(/hidden/);

  await page.locator('.message-row[data-message-id="m2"] .message-row-btn').click({ modifiers: [modifier] });
  await page.locator('.message-row[data-message-id="m4"] .message-row-btn').click({ modifiers: [modifier] });
  await expect(page.locator('#mail-selection-count')).toHaveText('2 selected');
  await expect(page.locator('#btn-flag')).toHaveText('Flag');
  await page.click('#btn-flag');
  await expect(page.locator('.message-row[data-message-id="m2"]')).toHaveClass(/is-flagged/);
  await expect(page.locator('.message-row[data-message-id="m4"]')).toHaveClass(/is-flagged/);
  await expect(page.locator('#btn-flag')).toHaveText('Unflag');

  await page.click('#btn-mail-clear');
  await expect(page.locator('#mail-selection-tools')).toHaveClass(/hidden/);

  await page.locator('.message-row[data-message-id="m1"] .message-row-btn').click({ modifiers: [modifier] });
  await page.locator('.message-row[data-message-id="m4"] .message-row-btn').click({ modifiers: ['Shift'] });
  await expect(page.locator('#mail-selection-count')).toHaveText('4 selected');
  await expect(page.locator('#btn-archive')).toBeEnabled();
  await page.click('#btn-archive');
  await expect(page.locator('#ui-modal-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#ui-modal-input')).toHaveValue('Archive');
  await page.click('#ui-modal-confirm');

  await expect(page.locator('#messages .message-row-btn')).toHaveCount(0);
  await expect(page.locator('#messages .message-empty')).toHaveText(/No messages to display/i);
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Archive/ })).toBeVisible();

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Archive/ }).click();
  await expect(page.locator('#messages .message-row-btn')).toHaveCount(4);

  await page.locator('.message-row[data-message-id="m1"] .message-row-btn').click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Action one/i);
  await page.selectOption('#mail-move-target', 'Projects');
  await expect(page.locator('#btn-move')).toBeEnabled();
  await page.click('#btn-move');

  await expect(page.locator('#messages .message-row-btn')).toHaveCount(3);

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Projects/ }).click();
  await expect(page.locator('#messages .message-row-btn')).toHaveCount(1);
  await expect(page.locator('#messages .message-row-btn .message-subject')).toHaveText(/Action one/i);

  await page.locator('.message-row[data-message-id="m1"] .message-row-btn').click();
  await page.click('#btn-trash');
  await expect(page.locator('#ui-modal-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#ui-modal-input')).toHaveValue('Trash');
  await page.click('#ui-modal-confirm');

  await expect(page.locator('#messages .message-row-btn')).toHaveCount(0);
  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Trash/ }).click();
  await expect(page.locator('#messages .message-row-btn')).toHaveCount(1);
});

test('mailbox rows switch to compact layout when the message pane narrows', async ({ page }) => {
  await mockMailActionScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  await page.evaluate(() => {
    const layout = document.querySelector('.mail-layout');
    if (layout instanceof HTMLElement) {
      layout.style.width = '948px';
    }
  });
  await expect(page.locator('.message-row-btn')).toHaveCount(4);
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
  const compactMetrics = await page.locator('.message-row-btn').first().evaluate((node) => {
    const rowRect = node.getBoundingClientRect();
    const from = node.querySelector('.message-from');
    const subject = node.querySelector('.message-subject');
    const preview = node.querySelector('.message-preview');
    const pane = node.closest('.mail-pane--messages');
    return {
      paneWidth: Math.round(pane?.getBoundingClientRect().width || 0),
      rowHeight: Math.round(rowRect.height),
      fromTop: Math.round((from?.getBoundingClientRect().top || 0) - rowRect.top),
      subjectTop: Math.round((subject?.getBoundingClientRect().top || 0) - rowRect.top),
      previewTop: Math.round((preview?.getBoundingClientRect().top || 0) - rowRect.top),
    };
  });
  expect(compactMetrics.paneWidth).toBeLessThan(360);
  expect(compactMetrics.subjectTop).toBeGreaterThan(compactMetrics.fromTop + 10);
  expect(compactMetrics.previewTop).toBeGreaterThan(compactMetrics.subjectTop + 8);
  expect(compactMetrics.previewTop).toBeLessThanOrEqual(compactMetrics.subjectTop + 52);

  await page.locator('.message-row-btn').first().click();
  await expectNoHorizontalOverflow(page, '.reader-view-controls');
});

test('mailbox dedupes duplicate message rows from live payloads', async ({ page }) => {
  await mockLiveRefreshScenario(page, { duplicateSummaryIDs: ['m2'] });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await expect(page.locator('#messages .message-row')).toHaveCount(2);
  await expect(page.locator('#messages .message-row-btn .message-subject')).toHaveText([
    'Re: Updates to OpenAI Privacy Policy',
    'Fwd: Updates to OpenAI Privacy Policy',
  ]);

  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 4 messages/i);
});

test('mail search field clears inline and restores the full mailbox list', async ({ page }) => {
  await mockMailActionScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('.message-row-btn')).toHaveCount(4);
  await page.fill('#search-input', 'alice');
  await expect(page.locator('#btn-search-clear')).toBeVisible();
  await page.click('#btn-search');
  await expect(page.locator('.message-row-btn')).toHaveCount(1);

  await page.click('#btn-search-clear');
  await expect(page.locator('#search-input')).toHaveValue('');
  await expect(page.locator('#btn-search-clear')).toHaveClass(/hidden/);
  await expect(page.locator('.message-row-btn')).toHaveCount(4);
  await expectNoHorizontalOverflow(page, '.mail-commandbar');
});

test('opening a message preserves the mailbox list scroll position', async ({ page }) => {
  await mockMailActionScenario(page, { fixture: denseMailActionFixture(30) });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const list = page.locator('#messages');
  const before = await list.evaluate((node) => {
    node.scrollTop = 420;
    return Math.round(node.scrollTop);
  });

  const rows = page.locator('.message-row-btn');
  const targetIndex = await rows.evaluateAll((nodes) => {
    const listNode = document.querySelector('#messages');
    if (!(listNode instanceof HTMLElement)) return -1;
    const listRect = listNode.getBoundingClientRect();
    return nodes.findIndex((node) => {
      if (!(node instanceof HTMLElement)) return false;
      const rect = node.getBoundingClientRect();
      return rect.top >= listRect.top + 12 && rect.bottom <= listRect.bottom - 12;
    });
  });
  expect(targetIndex).toBeGreaterThanOrEqual(0);
  const target = rows.nth(targetIndex);
  const expectedSubject = await target.locator('.message-subject').innerText();

  await target.click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(expectedSubject);
  const after = await list.evaluate((node) => Math.round(node.scrollTop));
  expect(Math.abs(after - before)).toBeLessThanOrEqual(24);
});

test('top-level and sidebar navigation expose the current destination', async ({ page }) => {
  await mockMailActionScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#tab-mail')).toHaveAttribute('aria-current', 'page');
  await expectNoHorizontalOverflow(page, '.topbar');

  await page.click('#tab-settings');
  await expect(page.locator('#tab-settings')).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('#settings-nav-signin')).toHaveAttribute('aria-current', 'page');

  await page.click('#settings-nav-devices');
  await expect(page.locator('#settings-nav-devices')).toHaveAttribute('aria-current', 'page');

  await page.click('#tab-admin');
  await expect(page.locator('#tab-admin')).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('#admin-nav-system')).toHaveAttribute('aria-current', 'page');
});

test('mailbox live refresh updates counts and rows without losing the open thread context', async ({ page }) => {
  await page.addInitScript(() => {
    const originalSetInterval = window.setInterval.bind(window);
    window.setInterval = (fn, ms, ...args) => originalSetInterval(fn, ms >= 20000 ? 80 : ms, ...args);
  });
  const control = await mockLiveRefreshScenario(page, { duplicateSummaryIDs: ['m2'] });
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('.message-row-btn')).toHaveCount(2);
  await page.locator('.message-row-btn').first().click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Re: Updates to OpenAI Privacy Policy/i);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 4 messages/i);

  control.addIncomingMessage();
  control.duplicateSummary('m4');
  await page.evaluate(() => {
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Inbox/i })).toContainText('(1)');
  await expect(page.locator('.message-row-btn')).toHaveCount(3);
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Re: Updates to OpenAI Privacy Policy/i);
  await expect(page.locator('#thread-position')).toHaveText(/Conversation · 4 messages/i);
});

test('mobile primary controls keep touch-friendly target sizes', async ({ browser }) => {
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true,
  });
  const page = await context.newPage();
  await mockMailActionScenario(page);
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const mailTab = await page.locator('#tab-mail').boundingBox();
  const searchInput = await page.locator('#search-input').boundingBox();
  expect(Math.round(mailTab?.height || 0)).toBeGreaterThanOrEqual(44);
  expect(Math.round(searchInput?.height || 0)).toBeGreaterThanOrEqual(44);

  await page.click('#tab-settings');
  const settingsNav = await page.locator('#settings-nav-signin').boundingBox();
  expect(Math.round(settingsNav?.height || 0)).toBeGreaterThanOrEqual(44);

  await page.fill('#settings-search-input', 'passkeys');
  const clearButton = await page.locator('#btn-settings-search-clear').boundingBox();
  expect(Math.round(clearButton?.height || 0)).toBeGreaterThanOrEqual(44);
  await context.close();
});

test('mailbox long press enters selection mode on mobile', async ({ browser }) => {
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true,
  });
  const page = await context.newPage();
  await mockMailActionScenario(page);
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  const firstRow = page.locator('.message-row[data-message-id="m1"] .message-row-btn');
  await firstRow.dispatchEvent('pointerdown', { pointerType: 'touch', isPrimary: true, button: 0 });
  await page.waitForTimeout(520);
  await firstRow.dispatchEvent('pointerup', { pointerType: 'touch', isPrimary: true, button: 0 });

  await expect(page.locator('#mail-selection-tools')).not.toHaveClass(/hidden/);
  await expect(page.locator('#mail-selection-count')).toHaveText('1 selected');
  await page.locator('.message-row[data-message-id="m2"] .message-row-btn').click();
  await expect(page.locator('#mail-selection-count')).toHaveText('2 selected');

  await page.click('#btn-mail-clear');
  await expect(page.locator('#mail-selection-tools')).toHaveClass(/hidden/);
  await context.close();
});

test('desktop ux pass', async ({ page }) => {
  const consoleErrors = [];
  const dialogs = [];
  page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
  page.on('pageerror', (err) => consoleErrors.push(`pageerror: ${err.message}`));
  page.on('dialog', async (dialog) => {
    dialogs.push(dialog.type());
    await dialog.dismiss();
  });

  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/#/reset?token=RESET-TOKEN-123', { waitUntil: 'networkidle' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'machine-dark');
  await expect(page.locator('#auth-pane-reset')).not.toHaveClass(/hidden/);
  await expect(page.locator('#reset-token-input')).toHaveValue('RESET-TOKEN-123');
  await expect(page.locator('#reset-capability-note')).toContainText(/confirmed|unavailable|enabled/i);
  await expect(page.evaluate(() => window.location.hash)).resolves.toBe('');
  await expect(page.locator('#passkey-email')).toHaveCount(0);
  await page.click('#auth-mode-login');
  await page.screenshot({ path: '/tmp/ux-desktop-auth.png', fullPage: true });

  await page.fill('#form-login input[name="email"]', 'admin@example.com');
  await page.fill('#form-login input[name="password"]', 'SecretPass123!');
  await page.click('#form-login button[type="submit"]');
  await page.waitForTimeout(1200);
  await dismissRecoveryPromptIfPresent(page);
  await skipIfMFANavigationIsBlocked(page);
  await page.click('#tab-mail');
  await page.waitForTimeout(600);

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('.mail-commandbar')).toBeVisible();
  await expect(page.locator('.mail-commandbar-group')).toHaveCount(6);
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Drafts/ })).toBeVisible();
  await expect(page.locator('#btn-reader-view-html')).toBeVisible();
  await expect(page.locator('#btn-reader-view-plain')).toBeVisible();
  await expect(page.locator('#btn-compose-open')).toBeVisible();
  await expect(page.locator('#btn-reply')).toBeVisible();
  await expect(page.locator('#btn-forward')).toBeVisible();
  await expect(page.locator('#btn-flag')).toBeVisible();
  await expect(page.locator('#btn-mark-seen')).toBeVisible();
  await expect(page.locator('#btn-archive')).toBeVisible();
  await expect(page.locator('#mail-move-target')).toBeVisible();
  await expect(page.locator('#btn-move')).toBeVisible();
  await expect(page.locator('#btn-trash')).toBeVisible();
  await expect(page.locator('#mail-pane-messages .searchbar')).toHaveCount(0);
  await expect(page.locator('#mail-pane-mailboxes .pane-head h3')).toHaveCount(0);
  await expect(page.locator('#mail-pane-messages .pane-head h3')).toHaveCount(0);
  await expect(page.locator('#mail-pane-reader .pane-head h3')).toHaveCount(0);
  await expect(page.locator('.reader-view-controls .reader-view-label')).toHaveText(/View:/i);
  await expect(page.locator('#mailboxes .mailbox-section-title').first()).toHaveText(/SYSTEM/i);
  await page.screenshot({ path: '/tmp/ux-desktop-mail.png', fullPage: true });

  await page.locator('#mail-pane-reader').click();
  await page.keyboard.press('/');
  await page.waitForTimeout(150);
  await page.keyboard.type('probe');
  await page.keyboard.press('Enter');
  await page.locator('#mail-pane-reader').click();
  await page.keyboard.press('c');
  await page.waitForTimeout(400);
  await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-toolbar-layer')).toBeVisible();
  await expect(page.locator('#compose-window-more-menu')).toHaveCount(0);
  await expect(page.locator('#compose-toolbar-layer .compose-window-actions--right > button')).toHaveCount(4);
  await expect(page.locator('#btn-compose-discard')).toBeVisible();
  await expect(page.locator('#compose-draft-note')).toHaveClass(/hidden/);
  await expect(page.locator('#compose-tool-attach')).toBeVisible();
  await expect(page.locator('#btn-compose-mode-rich')).toBeVisible();
  await expect(page.locator('#btn-compose-mode-html')).toBeVisible();
  await expect(page.locator('#compose-toggle-formatting')).toBeVisible();
  await expect(page.locator('#compose-editor-tools')).toHaveClass(/hidden/);
  await page.click('#compose-toggle-formatting');
  await expect(page.locator('#compose-editor-tools')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-tool-bold')).toBeVisible();
  await expect(page.locator('#compose-tool-link')).toBeVisible();
  await expect(page.locator('#compose-to-input')).toBeVisible();
  await expect(page.locator('#compose-cc-row')).toHaveClass(/hidden/);
  await expect(page.locator('#compose-bcc-row')).toHaveClass(/hidden/);
  await expect(page.locator('#compose-cc-input')).toBeHidden();
  await expect(page.locator('#compose-subject-input')).toBeVisible();
  const fromRowVisible = await page.locator('#compose-from-row').isVisible();
  if (fromRowVisible) {
    await expect(page.locator('#compose-from-manual-wrap')).toHaveClass(/hidden/);
  }
  await expect(page.locator('#compose-from-note')).toHaveClass(/hidden/);

  await page.click('#compose-toggle-cc');
  await expect(page.locator('#compose-cc-row')).not.toHaveClass(/hidden/);
  await page.click('#compose-toggle-cc');
  await expect(page.locator('#compose-cc-row')).toHaveClass(/hidden/);

  await page.click('#compose-toggle-bcc');
  await expect(page.locator('#compose-bcc-row')).not.toHaveClass(/hidden/);
  await page.click('#compose-toggle-bcc');
  await expect(page.locator('#compose-bcc-row')).toHaveClass(/hidden/);

  await expect(page.locator('#btn-compose-send')).toBeDisabled();
  await page.fill('#compose-to-input', 'alice@example.com');
  await page.setInputFiles('#compose-attachments-input', {
    name: 'notes.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('attachment body'),
  });
  await expect(page.locator('#compose-editor [data-compose-attachment-id]')).toHaveCount(0);
  await expect(page.locator('#compose-assets')).not.toHaveClass(/hidden/);
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(1);
  await page.setInputFiles('#compose-attachments-input', {
    name: 'inline.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/w8AAgMBgJ4M3hQAAAAASUVORK5CYII=', 'base64'),
  });
  const inlinePreview = page.locator('#compose-editor img[data-compose-inline-image-id]').first();
  await expect(inlinePreview).toBeVisible();
  await expect(page.locator('#compose-assets-list .compose-asset-row')).toHaveCount(2);
  await expect(inlinePreview).toHaveAttribute('src', /\/api\/v2\/drafts\/[^/]+\/attachments\/[^/]+/);
  await page.locator('#compose-editor').click();
  await page.keyboard.type('Hello from compose test.');
  await page.waitForTimeout(1100);
  await expect(page.locator('#btn-compose-send')).toBeEnabled();
  await page.fill('#compose-subject-input', 'UX compose check');
  await expect(page.locator('#compose-draft-state')).toContainText(/Saved|Saving|Unsaved/);
  await expect(page.locator('#btn-compose-send')).toBeEnabled();

  await page.keyboard.press('Escape');
  await page.waitForTimeout(250);

  const messageRows = page.locator('.message-row-btn');
  await expect(page.locator('.message-preview', { hasText: '(no preview)' })).toHaveCount(0);
  if (await messageRows.count()) {
    const fromLabel = (await page.locator('.message-row-btn .message-from').first().textContent() || '').trim();
    expect(fromLabel).not.toMatch(/<[^>]+>/);
    await messageRows.first().click();
    await page.waitForTimeout(300);
    await expect(page.locator('#thread-position')).toBeVisible();
    await expect(page.locator('#thread-position')).not.toContainText('No thread context');
    await expect(page.locator('#btn-thread-prev')).toBeVisible();
    await expect(page.locator('#btn-thread-next')).toBeVisible();
    await expect(page.locator('#btn-reader-view-html')).toBeVisible();
    await expect(page.locator('#btn-reader-view-plain')).toBeVisible();

    const htmlModeEnabled = await page.locator('#btn-reader-view-html').isEnabled();
    if (htmlModeEnabled) {
      await expect(page.locator('#btn-reader-view-html')).toHaveClass(/is-active/);
      await expect(page.locator('#message-body-html-wrap')).not.toHaveClass(/hidden/);
      const srcdoc = (await page.locator('#message-body-html').getAttribute('srcdoc')) || '';
      expect(srcdoc).toContain('Content-Security-Policy');
      expect(srcdoc).toContain('meta name="color-scheme" content="light"');
      expect(srcdoc).toContain(':root{color-scheme:light;}');
      expect(srcdoc).toContain('html,body{margin:0;padding:0;background:#ffffff;color:#000000;}');
      expect(srcdoc).not.toContain('ui-monospace');
      expect(srcdoc).not.toContain('a{color:');

      await page.click('#btn-reader-view-plain');
      await expect(page.locator('#btn-reader-view-plain')).toHaveClass(/is-active/);
      await expect(page.locator('#message-body-plain')).not.toHaveClass(/hidden/);
      await expect(page.locator('#message-body-html-wrap')).toHaveClass(/hidden/);

      await page.click('#btn-reader-view-html');
      await expect(page.locator('#message-body-html-wrap')).not.toHaveClass(/hidden/);
      const rewrittenSrcdoc = (await page.locator('#message-body-html').getAttribute('srcdoc')) || '';
      if (rewrittenSrcdoc.includes('/remote-image?url=')) {
        expect(rewrittenSrcdoc).toContain('/api/v1/messages/');
      }
      if (rewrittenSrcdoc.toLowerCase().includes('/api/v1/attachments/')) {
        expect(rewrittenSrcdoc.toLowerCase()).not.toContain('cid:');
      }
    }

    await expect(page.locator('#btn-reply')).toBeEnabled();
    await page.click('#btn-reply');
    await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
    await expect(page.locator('#compose-title')).toHaveText(/Reply/i);
    await expect(page.locator('#compose-subject-input')).toHaveValue(/Re:/i);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(150);

    await expect(page.locator('#btn-forward')).toBeEnabled();
    await page.click('#btn-forward');
    await expect(page.locator('#compose-overlay')).not.toHaveClass(/hidden/);
    await expect(page.locator('#compose-title')).toHaveText(/Forward/i);
    await expect(page.locator('#compose-subject-input')).toHaveValue(/Fwd:/i);
    await expect(page.locator('#compose-editor')).toContainText('----- Forwarded message -----');
    await page.keyboard.press('Escape');
    await page.waitForTimeout(150);
  }

  await page.click('#tab-settings');
  await page.waitForTimeout(700);
  await expect(page.locator('.settings-layout')).toHaveCount(1);
  await page.screenshot({ path: '/tmp/ux-desktop-settings-signin.png', fullPage: true });

  await page.click('#settings-nav-devices');
  await page.waitForTimeout(350);
  await page.screenshot({ path: '/tmp/ux-desktop-settings-devices.png', fullPage: true });

  await page.click('#settings-nav-sessions');
  await page.waitForTimeout(350);
  await page.screenshot({ path: '/tmp/ux-desktop-settings-sessions.png', fullPage: true });

  await page.fill('#settings-search-input', 'passkey');
  await page.waitForTimeout(250);
  if (await page.locator('#settings-search-results .settings-search-result').count()) {
    await page.locator('#settings-search-results .settings-search-result').first().click();
    await page.waitForTimeout(250);
  }

  await page.click('#tab-admin');
  await page.waitForTimeout(700);
  await expect(page.locator('.admin-layout')).toHaveCount(1);
  await expect(page.locator('#update-hero-card')).toBeVisible();
  await expect(page.locator('#update-hero-headline')).toBeVisible();
  await expect(page.locator('#btn-update-check')).toBeVisible();
  await expect(page.locator('#btn-update-auto')).toBeVisible();
  await expect(page.locator('.update-detail-card')).toBeVisible();
  await expect(page.locator('#update-source-link')).toBeVisible();
  await page.screenshot({ path: '/tmp/ux-desktop-admin-system.png', fullPage: true });

  await page.fill('#admin-search-input', 'feature flags');
  await page.waitForTimeout(250);
  if (await page.locator('#admin-search-results .settings-search-result').count()) {
    await page.locator('#admin-search-results .settings-search-result').first().click();
    await page.waitForTimeout(250);
  }

  await page.click('#admin-nav-registrations');
  await page.fill('#admin-reg-q', 'test');
  await page.click('#btn-admin-reg-apply');
  await page.waitForTimeout(500);
  await page.screenshot({ path: '/tmp/ux-desktop-admin-registrations.png', fullPage: true });

  await page.click('#admin-nav-users');
  await page.selectOption('#admin-user-status', 'active');
  await page.click('#btn-admin-user-apply');
  await page.waitForTimeout(500);
  await page.screenshot({ path: '/tmp/ux-desktop-admin-users.png', fullPage: true });
  await expect(dialogs.length).toBe(0);

  await page.click('#admin-nav-audit');
  await page.selectOption('#admin-audit-action', 'registration.approve');
  await page.click('#btn-admin-audit-apply');
  await page.waitForTimeout(500);
  await page.screenshot({ path: '/tmp/ux-desktop-admin-audit.png', fullPage: true });

  console.log('DESKTOP_CONSOLE_ERRORS=' + JSON.stringify(consoleErrors));
});

test('mobile ux pass', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  const page = await context.newPage();
  const consoleErrors = [];
  const dialogs = [];
  page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
  page.on('pageerror', (err) => consoleErrors.push(`pageerror: ${err.message}`));
  page.on('dialog', async (dialog) => {
    dialogs.push(dialog.type());
    await dialog.dismiss();
  });

  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'machine-dark');
  await page.fill('#form-login input[name="email"]', 'admin@example.com');
  await page.fill('#form-login input[name="password"]', 'SecretPass123!');
  await page.click('#form-login button[type="submit"]');
  await page.waitForTimeout(1200);
  await dismissRecoveryPromptIfPresent(page);
  await skipIfMFANavigationIsBlocked(page);
  await page.click('#tab-mail');
  await page.waitForTimeout(600);

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('.mail-commandbar')).toBeVisible();
  await page.screenshot({ path: '/tmp/ux-mobile-mail.png', fullPage: true });

  await page.click('#tab-settings');
  await page.waitForTimeout(500);
  await expect(page.locator('.settings-layout')).toHaveCount(1);
  await page.screenshot({ path: '/tmp/ux-mobile-settings.png', fullPage: true });

  await page.click('#settings-nav-devices');
  await page.waitForTimeout(250);

  if (await page.locator('.mailbox-list button').count()) {
    await page.click('#tab-mail');
    await page.waitForTimeout(250);
    await page.locator('.mailbox-list button').first().click();
    await page.waitForTimeout(300);
    await page.screenshot({ path: '/tmp/ux-mobile-messages.png', fullPage: true });
    await page.click('#mail-mobile-back');
    await page.waitForTimeout(250);
  }
  await expect(dialogs.length).toBe(0);

  console.log('MOBILE_CONSOLE_ERRORS=' + JSON.stringify(consoleErrors));
  await context.close();
});
