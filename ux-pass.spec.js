const { test, expect } = require('@playwright/test');

async function dismissRecoveryPromptIfPresent(page) {
  const overlay = page.locator('#ui-modal-overlay');
  if (!(await overlay.isVisible())) return;
  const title = (await page.locator('#ui-modal-title').textContent() || '').trim();
  if (!/set recovery email/i.test(title)) return;
  await page.click('#ui-modal-cancel');
  await expect(overlay).toHaveClass(/hidden/);
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
      { name: 'Archive', role: 'archive' },
      { name: 'Projects', role: '' },
      { name: 'Deleted Messages', role: 'trash' },
    ],
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

async function mockComposeReliabilityScenario(page) {
  const fixture = composeReliabilityFixture();
  const drafts = new Map();
  const attachmentBodies = new Map();
  let draftSeq = 1;
  let attachmentSeq = 1;
  let sendAttempts = 0;

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
}

async function mockMailActionScenario(page) {
  const fixture = mailActionFixture();
  const messages = new Map(fixture.messages.map((item) => [item.id, { ...item }]));

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
    const role = mailbox.role || '';
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

  const wrapBox = await page.locator('#thread-list-wrap').boundingBox();
  expect(wrapBox).not.toBeNull();
  expect(wrapBox.height).toBeLessThan(190);

  await page.locator('#thread-list .thread-row-btn').nth(2).click();
  await expect(page.locator('#message-subject-anchor')).toHaveText(/Re: Updates to OpenAI Privacy Policy/i);
  await expect(page.locator('#thread-selection-status')).toHaveText(/Viewing 3 of 4/i);

  await context.close();
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

test('mailbox core actions support bulk selection, state toggles, archive, and move', async ({ page }) => {
  await mockMailActionScenario(page);
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('.message-row-btn')).toHaveCount(4);
  await expect(page.locator('#mail-bulk-bar')).not.toHaveClass(/hidden/);
  await expect(page.locator('#mail-selection-count')).toHaveText('0 selected');

  await page.check('[data-message-select="m1"]');
  await expect(page.locator('#mail-selection-count')).toHaveText('1 selected');
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Unread');
  await page.click('#btn-mark-seen');
  await expect(page.locator('.message-row[data-message-id="m1"]')).toHaveClass(/is-unread/);
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Read');

  await page.click('#btn-mark-seen');
  await expect(page.locator('.message-row[data-message-id="m1"]')).not.toHaveClass(/is-unread/);
  await expect(page.locator('#btn-mark-seen')).toHaveText('Mark Unread');

  await page.click('#btn-mail-clear');
  await expect(page.locator('#mail-selection-count')).toHaveText('0 selected');

  await page.check('[data-message-select="m2"]');
  await page.check('[data-message-select="m3"]');
  await expect(page.locator('#mail-selection-count')).toHaveText('2 selected');
  await expect(page.locator('#btn-flag')).toHaveText('Flag');
  await page.click('#btn-flag');
  await expect(page.locator('.message-row[data-message-id="m2"]')).toHaveClass(/is-flagged/);
  await expect(page.locator('.message-row[data-message-id="m3"]')).toHaveClass(/is-flagged/);
  await expect(page.locator('#btn-flag')).toHaveText('Unflag');

  await page.click('#btn-mail-clear');
  await expect(page.locator('#mail-selection-count')).toHaveText('0 selected');

  await page.check('#mail-check-all');
  await expect(page.locator('#mail-selection-count')).toHaveText('4 selected');
  await expect(page.locator('#btn-archive')).toBeEnabled();
  await page.click('#btn-archive');

  await expect(page.locator('#messages .message-row-btn')).toHaveCount(0);
  await expect(page.locator('#messages .message-empty')).toHaveText(/No messages to display/i);
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Archive/ })).toContainText('(1)');

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Archive/ }).click();
  await expect(page.locator('#messages .message-row-btn')).toHaveCount(4);

  await page.check('[data-message-select="m1"]');
  await page.selectOption('#mail-move-target', 'Projects');
  await expect(page.locator('#btn-move')).toBeEnabled();
  await page.click('#btn-move');

  await expect(page.locator('#messages .message-row-btn')).toHaveCount(3);

  await page.locator('#mailboxes .mailbox-row button', { hasText: /^Projects/ }).click();
  await expect(page.locator('#messages .message-row-btn')).toHaveCount(1);
  await expect(page.locator('#messages .message-row-btn .message-subject')).toHaveText(/Action one/i);
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
  await page.click('#tab-mail');
  await page.waitForTimeout(600);

  await expect(page.locator('#view-mail')).toBeVisible();
  await expect(page.locator('.mail-commandbar')).toBeVisible();
  await expect(page.locator('.mail-commandbar-group')).toHaveCount(5);
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
  await expect(page.locator('#compose-toolbar-layer .compose-window-actions--right > button')).toHaveCount(3);
  await expect(page.locator('#btn-compose-discard')).toBeVisible();
  await expect(page.locator('#compose-draft-note')).toHaveClass(/hidden/);
  await expect(page.locator('#compose-tool-attach')).toBeVisible();
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
  await page.fill('#compose-subject-input', 'UX compose check');
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
