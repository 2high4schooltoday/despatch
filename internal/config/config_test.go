package config

import (
	"net/http/httptest"
	"testing"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SESSION_ENCRYPT_KEY", "this_is_a_valid_long_session_encrypt_key_123456")
	t.Setenv("UPDATE_SIGNING_PUBLIC_KEYS", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
}

func TestLoadRejectsDefaultSessionKey(t *testing.T) {
	t.Setenv("UPDATE_SIGNING_PUBLIC_KEYS", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("SESSION_ENCRYPT_KEY", "CHANGE_ME_PRODUCTION_SESSION_KEY")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected Load to fail with default session key")
	}
}

func TestLoadPasswordBounds(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PASSWORD_MIN_LENGTH", "16")
	t.Setenv("PASSWORD_MAX_LENGTH", "12")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected Load to fail for invalid password bounds")
	}
}

func TestLoadRejectsInvalidDovecotAuthMode(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DOVECOT_AUTH_MODE", "ldap")
	_, err := Load()
	if err == nil {
		t.Fatalf("expected Load to fail for invalid DOVECOT_AUTH_MODE")
	}
}

func TestLoadPasswordResetSenderDefaultsToSMTP(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PASSWORD_RESET_SENDER", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PasswordResetSender != "smtp" {
		t.Fatalf("expected PASSWORD_RESET_SENDER default smtp, got %q", cfg.PasswordResetSender)
	}
}

func TestLoadRejectsInvalidPasswordResetSender(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PASSWORD_RESET_SENDER", "console")
	if _, err := Load(); err == nil {
		t.Fatalf("expected Load to fail for invalid PASSWORD_RESET_SENDER")
	}
}

func TestLoadRejectsLogSenderWhenPublicResetEnabled(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PASSWORD_RESET_SENDER", "log")
	t.Setenv("PASSWORD_RESET_PUBLIC_ENABLED", "true")
	if _, err := Load(); err == nil {
		t.Fatalf("expected Load to fail when public reset is enabled with PASSWORD_RESET_SENDER=log")
	}
}

func TestLoadAllowsLogSenderWhenPublicResetDisabled(t *testing.T) {
	setValidEnv(t)
	t.Setenv("PASSWORD_RESET_SENDER", "log")
	t.Setenv("PASSWORD_RESET_PUBLIC_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PasswordResetSender != "log" {
		t.Fatalf("expected sender mode log, got %q", cfg.PasswordResetSender)
	}
}

func TestLoadPasswordResetExternalSenderReadyDefaultsFalse(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PasswordResetExternalSenderReady {
		t.Fatalf("expected PASSWORD_RESET_EXTERNAL_SENDER_READY to default false")
	}
}

func TestLoadPasskeyPasswordlessDefaultsTrue(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.PasskeyPasswordlessEnabled {
		t.Fatalf("expected PASSKEY_PASSWORDLESS_ENABLED to default true")
	}
}

func TestLoadSessionTimeoutDefaults(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SessionIdleMinutes != 480 {
		t.Fatalf("expected SESSION_IDLE_MINUTES default 480, got %d", cfg.SessionIdleMinutes)
	}
	if cfg.SessionAbsoluteHour != 168 {
		t.Fatalf("expected SESSION_ABSOLUTE_HOURS default 168, got %d", cfg.SessionAbsoluteHour)
	}
}

func TestLoadCookieSecureModeLegacyFallback(t *testing.T) {
	setValidEnv(t)
	t.Setenv("COOKIE_SECURE_MODE", "")
	t.Setenv("COOKIE_SECURE", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CookieSecureMode != "always" {
		t.Fatalf("expected legacy true to map to always, got %q", cfg.CookieSecureMode)
	}

	t.Setenv("COOKIE_SECURE", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CookieSecureMode != "never" {
		t.Fatalf("expected legacy false to map to never, got %q", cfg.CookieSecureMode)
	}
}

func TestResolveCookieSecureAuto(t *testing.T) {
	setValidEnv(t)
	t.Setenv("COOKIE_SECURE_MODE", "auto")
	t.Setenv("TRUST_PROXY", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest("GET", "http://example.test", nil)
	if got := cfg.ResolveCookieSecure(req); got {
		t.Fatalf("expected http request to resolve secure=false")
	}

	req.Header.Set("X-Forwarded-Proto", "https")
	if got := cfg.ResolveCookieSecure(req); !got {
		t.Fatalf("expected proxied https request to resolve secure=true")
	}

	tlsReq := httptest.NewRequest("GET", "https://example.test", nil)
	if got := cfg.ResolveCookieSecure(tlsReq); !got {
		t.Fatalf("expected tls request to resolve secure=true")
	}
}

func TestLoadUpdateDefaults(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.UpdateEnabled {
		t.Fatalf("expected update feature enabled by default")
	}
	if cfg.UpdateRepoOwner == "" || cfg.UpdateRepoName == "" {
		t.Fatalf("expected default update repo owner/name to be set")
	}
	if cfg.UpdateCheckIntervalMin <= 0 || cfg.UpdateHTTPTimeoutSec <= 0 {
		t.Fatalf("expected positive update intervals/timeouts")
	}
}

func TestLoadRejectsInvalidUpdateConfig(t *testing.T) {
	setValidEnv(t)
	t.Setenv("UPDATE_CHECK_INTERVAL_MIN", "0")
	if _, err := Load(); err == nil {
		t.Fatalf("expected load failure for UPDATE_CHECK_INTERVAL_MIN=0")
	}
}

func TestLoadRequiresSigningKeysWhenUpdateSignatureEnforced(t *testing.T) {
	t.Setenv("SESSION_ENCRYPT_KEY", "this_is_a_valid_long_session_encrypt_key_123456")
	t.Setenv("UPDATE_REQUIRE_SIGNATURE", "true")
	t.Setenv("UPDATE_SIGNING_PUBLIC_KEYS", "")
	if _, err := Load(); err == nil {
		t.Fatalf("expected load failure when update signature keys are missing")
	}
}

func TestLoadRejectsRemoteInsecureSkipVerifyWithoutOverride(t *testing.T) {
	setValidEnv(t)
	t.Setenv("IMAP_HOST", "mail.example.com")
	t.Setenv("IMAP_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK", "false")
	if _, err := Load(); err == nil {
		t.Fatalf("expected load failure for non-loopback insecure skip verify")
	}
}

func TestLoadCaptchaCapDerivesVerifyURLFromAbsoluteWidgetURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("CAPTCHA_ENABLED", "true")
	t.Setenv("CAPTCHA_PROVIDER", "cap")
	t.Setenv("CAPTCHA_SITE_KEY", "cap-site-key-123")
	t.Setenv("CAPTCHA_SECRET", "cap-secret-123")
	t.Setenv("CAPTCHA_WIDGET_API_URL", "https://cap.example.test/cap/cap-site-key-123/")
	t.Setenv("CAPTCHA_VERIFY_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CaptchaWidgetURL != "https://cap.example.test/cap/cap-site-key-123/" {
		t.Fatalf("unexpected captcha widget URL: %q", cfg.CaptchaWidgetURL)
	}
	if cfg.CaptchaVerifyURL != "https://cap.example.test/cap/cap-site-key-123/siteverify" {
		t.Fatalf("unexpected captcha verify URL: %q", cfg.CaptchaVerifyURL)
	}
}

func TestLoadCaptchaCapRequiresSiteKey(t *testing.T) {
	setValidEnv(t)
	t.Setenv("CAPTCHA_ENABLED", "true")
	t.Setenv("CAPTCHA_PROVIDER", "cap")
	t.Setenv("CAPTCHA_SITE_KEY", "")
	t.Setenv("CAPTCHA_SECRET", "cap-secret-123")

	if _, err := Load(); err == nil {
		t.Fatalf("expected error when CAPTCHA_SITE_KEY is empty for cap provider")
	}
}

func TestLoadCaptchaCapRequiresVerifyURLWhenWidgetURLIsRelative(t *testing.T) {
	setValidEnv(t)
	t.Setenv("CAPTCHA_ENABLED", "true")
	t.Setenv("CAPTCHA_PROVIDER", "cap")
	t.Setenv("CAPTCHA_SITE_KEY", "cap-site-key-123")
	t.Setenv("CAPTCHA_SECRET", "cap-secret-123")
	t.Setenv("CAPTCHA_WIDGET_API_URL", "/cap/cap-site-key-123/")
	t.Setenv("CAPTCHA_VERIFY_URL", "")

	if _, err := Load(); err == nil {
		t.Fatalf("expected error for relative widget URL without CAPTCHA_VERIFY_URL")
	}
}

func TestLoadCaptchaCapAcceptsExplicitVerifyURLForRelativeWidgetURL(t *testing.T) {
	setValidEnv(t)
	t.Setenv("CAPTCHA_ENABLED", "true")
	t.Setenv("CAPTCHA_PROVIDER", "cap")
	t.Setenv("CAPTCHA_SITE_KEY", "cap-site-key-123")
	t.Setenv("CAPTCHA_SECRET", "cap-secret-123")
	t.Setenv("CAPTCHA_WIDGET_API_URL", "/cap/cap-site-key-123/")
	t.Setenv("CAPTCHA_VERIFY_URL", "http://127.0.0.1:8077/cap-site-key-123/siteverify")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CaptchaVerifyURL != "http://127.0.0.1:8077/cap-site-key-123/siteverify" {
		t.Fatalf("unexpected captcha verify URL: %q", cfg.CaptchaVerifyURL)
	}
}
