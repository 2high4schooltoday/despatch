const { test, expect } = require('@playwright/test');

async function dismissRecoveryPromptIfPresent(page) {
  const overlay = page.locator('#ui-modal-overlay');
  if (!(await overlay.isVisible())) return;
  const title = (await page.locator('#ui-modal-title').textContent() || '').trim();
  if (!/set recovery email/i.test(title)) return;
  await page.click('#ui-modal-cancel');
  await expect(overlay).toHaveClass(/hidden/);
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
  await expect(page.locator('.mail-commandbar-group')).toHaveCount(4);
  await expect(page.locator('#mailboxes .mailbox-row button', { hasText: /^Drafts/ })).toBeVisible();
  await expect(page.locator('#btn-reader-view-html')).toBeVisible();
  await expect(page.locator('#btn-reader-view-plain')).toBeVisible();
  await expect(page.locator('#btn-compose-open')).toBeVisible();
  await expect(page.locator('#btn-reply')).toBeVisible();
  await expect(page.locator('#btn-forward')).toBeVisible();
  await expect(page.locator('#btn-flag')).toBeVisible();
  await expect(page.locator('#btn-mark-seen')).toBeVisible();
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
  await expect(page.locator('#compose-editor [data-compose-attachment-id]')).toHaveCount(1);
  await expect(page.locator('#compose-assets')).toHaveCount(0);
  await page.setInputFiles('#compose-attachments-input', {
    name: 'inline.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/w8AAgMBgJ4M3hQAAAAASUVORK5CYII=', 'base64'),
  });
  const inlinePreview = page.locator('#compose-editor img[data-compose-inline-image-id]').first();
  await expect(inlinePreview).toBeVisible();
  await expect(inlinePreview).toHaveAttribute('src', /^blob:/);
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
