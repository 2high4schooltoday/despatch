const { test, expect } = require('@playwright/test');

async function dismissRecoveryPromptIfPresent(page) {
  const overlay = page.locator('#ui-modal-overlay');
  if (!(await overlay.isVisible())) return;
  const title = (await page.locator('#ui-modal-title').textContent() || '').trim();
  if (!/set recovery email/i.test(title)) return;
  await page.click('#ui-modal-cancel');
  await expect(overlay).toHaveClass(/hidden/);
}

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
  await page.goto('http://127.0.0.1:18081/', { waitUntil: 'networkidle' });
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'machine-dark');
  await page.screenshot({ path: '/tmp/ux-desktop-auth.png', fullPage: true });

  await page.fill('#form-login input[name="email"]', 'admin@example.com');
  await page.fill('#form-login input[name="password"]', 'SecretPass123!');
  await page.click('#form-login button[type="submit"]');
  await page.waitForTimeout(1200);
  await dismissRecoveryPromptIfPresent(page);
  await page.click('#tab-mail');
  await page.waitForTimeout(600);

  await expect(page.locator('#view-mail')).toBeVisible();
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
  await expect(page.locator('#compose-tool-undo')).toBeVisible();
  await expect(page.locator('#compose-tool-bold')).toBeVisible();
  await expect(page.locator('#compose-tool-link')).toBeVisible();
  await expect(page.locator('#compose-to-input')).toBeVisible();
  await expect(page.locator('#compose-cc-input')).toBeVisible();
  await expect(page.locator('#compose-subject-input')).toBeVisible();
  await expect(page.locator('#compose-from-manual-wrap')).not.toHaveClass(/hidden/);

  await page.click('#compose-toggle-bcc');
  await expect(page.locator('#compose-bcc-row')).not.toHaveClass(/hidden/);
  await page.click('#compose-toggle-bcc');
  await expect(page.locator('#compose-bcc-row')).toHaveClass(/hidden/);

  await expect(page.locator('#btn-compose-send')).toBeDisabled();
  await page.fill('#compose-to-input', 'alice@example.com');
  await page.fill('#compose-subject-input', 'UX compose check');
  await page.locator('#compose-editor').click();
  await page.keyboard.type('Hello from compose test.');
  await page.fill('#compose-from-manual', 'admin@example.com');
  await expect(page.locator('#btn-compose-send')).toBeEnabled();

  await page.keyboard.press('Escape');
  await page.waitForTimeout(250);

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
