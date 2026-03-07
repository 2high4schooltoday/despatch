package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr string
	BaseDomain string

	DBPath            string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	SessionCookieName    string
	MFATrustedCookieName string
	SessionIdleMinutes   int
	SessionAbsoluteHour  int
	SessionEncryptKey    string
	CSRFCookieName       string
	CookieSecure         bool
	CookieSecureMode     string
	CookiePolicyWarning  string
	TrustProxy           bool
	CORSAllowedOrigins   []string

	CaptchaEnabled   bool
	CaptchaProvider  string
	CaptchaSiteKey   string
	CaptchaWidgetURL string
	CaptchaVerifyURL string
	CaptchaSecret    string

	PasswordMinLength int
	PasswordMaxLength int

	IMAPHost               string
	IMAPPort               int
	IMAPTLS                bool
	IMAPStartTLS           bool
	IMAPInsecureSkipVerify bool

	SMTPHost               string
	SMTPPort               int
	SMTPTLS                bool
	SMTPStartTLS           bool
	SMTPInsecureSkipVerify bool

	DovecotAuthDBDriver  string
	DovecotAuthDBDSN     string
	DovecotAuthMode      string
	DovecotAuthTable     string
	DovecotEmailColumn   string
	DovecotPassColumn    string
	DovecotActiveColumn  string
	DovecotMaildirColumn string
	DovecotMaildirBase   string

	HTTPReadTimeoutSec       int
	HTTPReadHeaderTimeoutSec int
	HTTPWriteTimeoutSec      int
	HTTPIdleTimeoutSec       int

	BootstrapAdminEmail    string
	BootstrapAdminPassword string

	PasswordResetSender              string
	PasswordResetFrom                string
	PasswordResetBaseURL             string
	PasswordResetTokenTTLMinutes     int
	PasswordResetPublicEnabled       bool
	PasswordResetExternalSenderReady bool
	PasswordResetRequireMappedLogin  bool

	PAMResetHelperEnabled    bool
	PAMResetHelperSocket     string
	PAMResetHelperTimeoutSec int
	PAMResetAllowedUID       int
	PAMResetAllowedGID       int

	UpdateEnabled           bool
	UpdateRepoOwner         string
	UpdateRepoName          string
	UpdateCheckIntervalMin  int
	UpdateHTTPTimeoutSec    int
	UpdateGitHubToken       string
	UpdateBackupKeep        int
	UpdateBaseDir           string
	UpdateInstallDir        string
	UpdateServiceName       string
	UpdateSystemdUnitDir    string
	UpdateRequireSignature  bool
	UpdateSignatureAsset    string
	UpdateSigningPublicKeys []string

	MailSecEnabled   bool
	MailSecSocket    string
	MailSecTimeoutMS int

	MailAllowInsecureSkipVerifyNonLoopback bool
	MailTLSWarning                         string

	WebAuthnRPID               string
	WebAuthnAllowedOrigins     []string
	PasskeyPasswordlessEnabled bool
	PasskeyUsernamelessEnabled bool
}

func Load() (Config, error) {
	cookieSecureLegacy, cookieSecureLegacySet := envBoolWithSet("COOKIE_SECURE", false)
	cookieSecureMode := strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE_MODE")))
	if cookieSecureMode == "" {
		if cookieSecureLegacySet {
			if cookieSecureLegacy {
				cookieSecureMode = "always"
			} else {
				cookieSecureMode = "never"
			}
		} else {
			cookieSecureMode = "auto"
		}
	}
	cfg := Config{
		ListenAddr:                             env("LISTEN_ADDR", ":8080"),
		BaseDomain:                             env("BASE_DOMAIN", "example.com"),
		DBPath:                                 env("APP_DB_PATH", "./data/app.db"),
		DBMaxOpenConns:                         envInt("APP_DB_MAX_OPEN_CONNS", 4),
		DBMaxIdleConns:                         envInt("APP_DB_MAX_IDLE_CONNS", 2),
		DBConnMaxLifetime:                      time.Duration(envInt("APP_DB_CONN_MAX_LIFETIME_MIN", 30)) * time.Minute,
		SessionCookieName:                      env("SESSION_COOKIE_NAME", "despatch_session"),
		MFATrustedCookieName:                   env("MFA_TRUSTED_COOKIE_NAME", "despatch_mfa_trusted"),
		SessionIdleMinutes:                     envInt("SESSION_IDLE_MINUTES", 30),
		SessionAbsoluteHour:                    envInt("SESSION_ABSOLUTE_HOURS", 24),
		SessionEncryptKey:                      env("SESSION_ENCRYPT_KEY", "CHANGE_ME_PRODUCTION_SESSION_KEY"),
		CSRFCookieName:                         env("CSRF_COOKIE_NAME", "despatch_csrf"),
		CookieSecure:                           cookieSecureLegacy,
		CookieSecureMode:                       cookieSecureMode,
		TrustProxy:                             envBool("TRUST_PROXY", false),
		CORSAllowedOrigins:                     envCSV("CORS_ALLOWED_ORIGINS"),
		CaptchaEnabled:                         envBool("CAPTCHA_ENABLED", false),
		CaptchaProvider:                        strings.ToLower(env("CAPTCHA_PROVIDER", "turnstile")),
		CaptchaSiteKey:                         strings.TrimSpace(env("CAPTCHA_SITE_KEY", "")),
		CaptchaWidgetURL:                       strings.TrimSpace(env("CAPTCHA_WIDGET_API_URL", "")),
		CaptchaVerifyURL:                       strings.TrimSpace(env("CAPTCHA_VERIFY_URL", "")),
		CaptchaSecret:                          env("CAPTCHA_SECRET", ""),
		PasswordMinLength:                      envInt("PASSWORD_MIN_LENGTH", 12),
		PasswordMaxLength:                      envInt("PASSWORD_MAX_LENGTH", 128),
		IMAPHost:                               env("IMAP_HOST", "127.0.0.1"),
		IMAPPort:                               envInt("IMAP_PORT", 993),
		IMAPTLS:                                envBool("IMAP_TLS", true),
		IMAPStartTLS:                           envBool("IMAP_STARTTLS", false),
		IMAPInsecureSkipVerify:                 envBool("IMAP_INSECURE_SKIP_VERIFY", false),
		SMTPHost:                               env("SMTP_HOST", "127.0.0.1"),
		SMTPPort:                               envInt("SMTP_PORT", 587),
		SMTPTLS:                                envBool("SMTP_TLS", false),
		SMTPStartTLS:                           envBool("SMTP_STARTTLS", true),
		SMTPInsecureSkipVerify:                 envBool("SMTP_INSECURE_SKIP_VERIFY", false),
		DovecotAuthMode:                        strings.ToLower(env("DOVECOT_AUTH_MODE", "sql")),
		DovecotAuthDBDriver:                    env("DOVECOT_AUTH_DB_DRIVER", ""),
		DovecotAuthDBDSN:                       env("DOVECOT_AUTH_DB_DSN", ""),
		DovecotAuthTable:                       env("DOVECOT_AUTH_TABLE", "users"),
		DovecotEmailColumn:                     env("DOVECOT_AUTH_EMAIL_COL", "email"),
		DovecotPassColumn:                      env("DOVECOT_AUTH_PASS_COL", "password_hash"),
		DovecotActiveColumn:                    env("DOVECOT_AUTH_ACTIVE_COL", "active"),
		DovecotMaildirColumn:                   env("DOVECOT_AUTH_MAILDIR_COL", "maildir"),
		DovecotMaildirBase:                     env("DOVECOT_MAILDIR_BASE", "/var/mail/vhosts"),
		HTTPReadTimeoutSec:                     envInt("HTTP_READ_TIMEOUT_SEC", 10),
		HTTPReadHeaderTimeoutSec:               envInt("HTTP_READ_HEADER_TIMEOUT_SEC", 5),
		HTTPWriteTimeoutSec:                    envInt("HTTP_WRITE_TIMEOUT_SEC", 30),
		HTTPIdleTimeoutSec:                     envInt("HTTP_IDLE_TIMEOUT_SEC", 60),
		BootstrapAdminEmail:                    env("BOOTSTRAP_ADMIN_EMAIL", ""),
		BootstrapAdminPassword:                 env("BOOTSTRAP_ADMIN_PASSWORD", ""),
		PasswordResetSender:                    strings.ToLower(env("PASSWORD_RESET_SENDER", "smtp")),
		PasswordResetFrom:                      env("PASSWORD_RESET_FROM", "no-reply@example.com"),
		PasswordResetBaseURL:                   env("PASSWORD_RESET_BASE_URL", ""),
		PasswordResetTokenTTLMinutes:           envInt("PASSWORD_RESET_TOKEN_TTL_MINUTES", 30),
		PasswordResetPublicEnabled:             envBool("PASSWORD_RESET_PUBLIC_ENABLED", true),
		PasswordResetExternalSenderReady:       envBool("PASSWORD_RESET_EXTERNAL_SENDER_READY", false),
		PasswordResetRequireMappedLogin:        envBool("PASSWORD_RESET_REQUIRE_MAPPED_LOGIN", true),
		PAMResetHelperEnabled:                  envBool("PAM_RESET_HELPER_ENABLED", false),
		PAMResetHelperSocket:                   strings.TrimSpace(env("PAM_RESET_HELPER_SOCKET", "/run/despatch/pam-reset-helper.sock")),
		PAMResetHelperTimeoutSec:               envInt("PAM_RESET_HELPER_TIMEOUT_SEC", 5),
		PAMResetAllowedUID:                     envInt("PAM_RESET_ALLOWED_UID", -1),
		PAMResetAllowedGID:                     envInt("PAM_RESET_ALLOWED_GID", -1),
		UpdateEnabled:                          envBool("UPDATE_ENABLED", true),
		UpdateRepoOwner:                        env("UPDATE_REPO_OWNER", "2high4schooltoday"),
		UpdateRepoName:                         env("UPDATE_REPO_NAME", "despatch"),
		UpdateCheckIntervalMin:                 envInt("UPDATE_CHECK_INTERVAL_MIN", 60),
		UpdateHTTPTimeoutSec:                   envInt("UPDATE_HTTP_TIMEOUT_SEC", 10),
		UpdateGitHubToken:                      env("UPDATE_GITHUB_TOKEN", ""),
		UpdateBackupKeep:                       envInt("UPDATE_BACKUP_KEEP", 3),
		UpdateBaseDir:                          env("UPDATE_BASE_DIR", "/var/lib/despatch/update"),
		UpdateInstallDir:                       env("UPDATE_INSTALL_DIR", "/opt/despatch"),
		UpdateServiceName:                      env("UPDATE_SERVICE_NAME", "despatch"),
		UpdateSystemdUnitDir:                   env("UPDATE_SYSTEMD_UNIT_DIR", "/etc/systemd/system"),
		UpdateRequireSignature:                 envBool("UPDATE_REQUIRE_SIGNATURE", true),
		UpdateSignatureAsset:                   env("UPDATE_SIGNATURE_ASSET", "checksums.txt.sig"),
		UpdateSigningPublicKeys:                envCSV("UPDATE_SIGNING_PUBLIC_KEYS"),
		MailSecEnabled:                         envBool("MAILSEC_ENABLED", false),
		MailSecSocket:                          env("MAILSEC_SOCKET", "/run/despatch/mailsec.sock"),
		MailSecTimeoutMS:                       envInt("MAILSEC_TIMEOUT_MS", 5000),
		MailAllowInsecureSkipVerifyNonLoopback: envBool("MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK", false),
		WebAuthnRPID:                           strings.TrimSpace(env("WEBAUTHN_RP_ID", "")),
		WebAuthnAllowedOrigins:                 envCSV("WEBAUTHN_ALLOWED_ORIGINS"),
		PasskeyPasswordlessEnabled:             envBool("PASSKEY_PASSWORDLESS_ENABLED", true),
		PasskeyUsernamelessEnabled:             envBool("PASSKEY_USERNAMELESS_ENABLED", true),
	}

	if cfg.SessionIdleMinutes <= 0 || cfg.SessionAbsoluteHour <= 0 {
		return Config{}, fmt.Errorf("session timeouts must be positive")
	}
	if cfg.DBMaxOpenConns <= 0 || cfg.DBMaxIdleConns < 0 {
		return Config{}, fmt.Errorf("invalid DB pool config")
	}
	if cfg.IMAPPort <= 0 || cfg.SMTPPort <= 0 {
		return Config{}, fmt.Errorf("invalid mail host port")
	}
	if cfg.PasswordMinLength < 8 {
		return Config{}, fmt.Errorf("password min length must be >= 8")
	}
	if cfg.PasswordMaxLength < cfg.PasswordMinLength {
		return Config{}, fmt.Errorf("password max length must be >= min length")
	}
	if cfg.PasswordResetTokenTTLMinutes <= 0 {
		return Config{}, fmt.Errorf("PASSWORD_RESET_TOKEN_TTL_MINUTES must be positive")
	}
	if cfg.PAMResetHelperTimeoutSec <= 0 {
		return Config{}, fmt.Errorf("PAM_RESET_HELPER_TIMEOUT_SEC must be positive")
	}
	if cfg.PAMResetHelperEnabled && strings.TrimSpace(cfg.PAMResetHelperSocket) == "" {
		return Config{}, fmt.Errorf("PAM_RESET_HELPER_SOCKET is required when PAM_RESET_HELPER_ENABLED=true")
	}
	if cfg.PAMResetHelperEnabled && cfg.PAMResetAllowedUID < 0 {
		return Config{}, fmt.Errorf("PAM_RESET_ALLOWED_UID must be set when PAM_RESET_HELPER_ENABLED=true")
	}
	if cfg.PAMResetHelperEnabled && cfg.PAMResetAllowedGID < 0 {
		return Config{}, fmt.Errorf("PAM_RESET_ALLOWED_GID must be set when PAM_RESET_HELPER_ENABLED=true")
	}
	switch cfg.DovecotAuthMode {
	case "", "sql", "pam":
		if cfg.DovecotAuthMode == "" {
			cfg.DovecotAuthMode = "sql"
		}
	default:
		return Config{}, fmt.Errorf("DOVECOT_AUTH_MODE must be one of: sql, pam")
	}
	switch cfg.PasswordResetSender {
	case "", "smtp":
		cfg.PasswordResetSender = "smtp"
	case "log":
		if cfg.PasswordResetPublicEnabled {
			return Config{}, fmt.Errorf("PASSWORD_RESET_SENDER=log requires PASSWORD_RESET_PUBLIC_ENABLED=false")
		}
	default:
		return Config{}, fmt.Errorf("PASSWORD_RESET_SENDER must be one of: smtp, log")
	}
	if strings.TrimSpace(cfg.SessionEncryptKey) == "" ||
		cfg.SessionEncryptKey == "CHANGE_ME_PRODUCTION_SESSION_KEY" ||
		len(cfg.SessionEncryptKey) < 24 {
		return Config{}, fmt.Errorf("SESSION_ENCRYPT_KEY must be set to a strong non-default value (>=24 chars)")
	}
	switch cfg.CookieSecureMode {
	case "auto", "always", "never":
	default:
		return Config{}, fmt.Errorf("COOKIE_SECURE_MODE must be one of: auto, always, never")
	}
	cfg.CookieSecure = cfg.CookieSecureMode == "always"
	if cfg.CookieSecureMode == "never" && isPotentiallyPublicListen(cfg.ListenAddr) {
		cfg.CookiePolicyWarning = "COOKIE_SECURE_MODE=never with a potentially public LISTEN_ADDR may expose cookies on plaintext HTTP"
	}
	if cfg.CaptchaEnabled {
		if strings.TrimSpace(cfg.CaptchaSecret) == "" {
			return Config{}, fmt.Errorf("CAPTCHA_SECRET is required when CAPTCHA_ENABLED=true")
		}
		switch cfg.CaptchaProvider {
		case "turnstile", "":
			cfg.CaptchaProvider = "turnstile"
			if strings.TrimSpace(cfg.CaptchaVerifyURL) == "" {
				cfg.CaptchaVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
			}
		case "hcaptcha":
			if strings.TrimSpace(cfg.CaptchaVerifyURL) == "" {
				cfg.CaptchaVerifyURL = "https://hcaptcha.com/siteverify"
			}
		case "cap":
			if cfg.CaptchaSiteKey == "" {
				return Config{}, fmt.Errorf("CAPTCHA_SITE_KEY is required when CAPTCHA_PROVIDER=cap")
			}
			if cfg.CaptchaWidgetURL == "" {
				cfg.CaptchaWidgetURL = "/cap/" + path.Clean(cfg.CaptchaSiteKey) + "/"
			}
			cfg.CaptchaWidgetURL = ensureTrailingSlash(cfg.CaptchaWidgetURL)
			if strings.TrimSpace(cfg.CaptchaVerifyURL) == "" {
				if isAbsoluteHTTPURL(cfg.CaptchaWidgetURL) {
					cfg.CaptchaVerifyURL = cfg.CaptchaWidgetURL + "siteverify"
				} else {
					return Config{}, fmt.Errorf("CAPTCHA_VERIFY_URL is required when CAPTCHA_PROVIDER=cap and CAPTCHA_WIDGET_API_URL is relative")
				}
			}
			if !isAbsoluteHTTPURL(cfg.CaptchaVerifyURL) {
				return Config{}, fmt.Errorf("CAPTCHA_VERIFY_URL must be an absolute http(s) URL when CAPTCHA_PROVIDER=cap")
			}
		default:
			return Config{}, fmt.Errorf("unsupported CAPTCHA_PROVIDER: %s", cfg.CaptchaProvider)
		}
	}
	if cfg.UpdateCheckIntervalMin <= 0 {
		return Config{}, fmt.Errorf("UPDATE_CHECK_INTERVAL_MIN must be positive")
	}
	if cfg.UpdateHTTPTimeoutSec <= 0 {
		return Config{}, fmt.Errorf("UPDATE_HTTP_TIMEOUT_SEC must be positive")
	}
	if cfg.UpdateBackupKeep < 1 {
		return Config{}, fmt.Errorf("UPDATE_BACKUP_KEEP must be >= 1")
	}
	if strings.TrimSpace(cfg.UpdateRepoOwner) == "" || strings.TrimSpace(cfg.UpdateRepoName) == "" {
		return Config{}, fmt.Errorf("UPDATE_REPO_OWNER and UPDATE_REPO_NAME are required")
	}
	if strings.TrimSpace(cfg.UpdateBaseDir) == "" {
		return Config{}, fmt.Errorf("UPDATE_BASE_DIR is required")
	}
	if strings.TrimSpace(cfg.UpdateInstallDir) == "" {
		return Config{}, fmt.Errorf("UPDATE_INSTALL_DIR is required")
	}
	if strings.TrimSpace(cfg.UpdateServiceName) == "" {
		return Config{}, fmt.Errorf("UPDATE_SERVICE_NAME is required")
	}
	if strings.TrimSpace(cfg.UpdateSystemdUnitDir) == "" {
		return Config{}, fmt.Errorf("UPDATE_SYSTEMD_UNIT_DIR is required")
	}
	cfg.UpdateSignatureAsset = strings.TrimSpace(cfg.UpdateSignatureAsset)
	if cfg.UpdateSignatureAsset == "" {
		return Config{}, fmt.Errorf("UPDATE_SIGNATURE_ASSET is required")
	}
	parsedKeys, err := parseEd25519PublicKeys(cfg.UpdateSigningPublicKeys)
	if err != nil {
		return Config{}, err
	}
	cfg.UpdateSigningPublicKeys = parsedKeys
	if cfg.UpdateEnabled && cfg.UpdateRequireSignature && len(cfg.UpdateSigningPublicKeys) == 0 {
		return Config{}, fmt.Errorf("UPDATE_SIGNING_PUBLIC_KEYS is required when UPDATE_ENABLED=true and UPDATE_REQUIRE_SIGNATURE=true")
	}
	if cfg.MailSecEnabled && strings.TrimSpace(cfg.MailSecSocket) == "" {
		return Config{}, fmt.Errorf("MAILSEC_SOCKET is required when MAILSEC_ENABLED=true")
	}
	if cfg.MailSecTimeoutMS <= 0 {
		return Config{}, fmt.Errorf("MAILSEC_TIMEOUT_MS must be positive")
	}
	insecureWarnings := make([]string, 0, 2)
	if cfg.IMAPInsecureSkipVerify {
		insecureWarnings = append(insecureWarnings, fmt.Sprintf("IMAP_INSECURE_SKIP_VERIFY=true host=%s", strings.TrimSpace(cfg.IMAPHost)))
		if !isLoopbackHost(cfg.IMAPHost) && !cfg.MailAllowInsecureSkipVerifyNonLoopback {
			return Config{}, fmt.Errorf("IMAP_INSECURE_SKIP_VERIFY=true is only allowed for loopback hosts unless MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK=true")
		}
	}
	if cfg.SMTPInsecureSkipVerify {
		insecureWarnings = append(insecureWarnings, fmt.Sprintf("SMTP_INSECURE_SKIP_VERIFY=true host=%s", strings.TrimSpace(cfg.SMTPHost)))
		if !isLoopbackHost(cfg.SMTPHost) && !cfg.MailAllowInsecureSkipVerifyNonLoopback {
			return Config{}, fmt.Errorf("SMTP_INSECURE_SKIP_VERIFY=true is only allowed for loopback hosts unless MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK=true")
		}
	}
	if len(insecureWarnings) > 0 {
		cfg.MailTLSWarning = strings.Join(insecureWarnings, "; ")
	}
	return cfg, nil
}

func (c Config) SessionIdleDuration() time.Duration {
	return time.Duration(c.SessionIdleMinutes) * time.Minute
}

func (c Config) SessionAbsoluteDuration() time.Duration {
	return time.Duration(c.SessionAbsoluteHour) * time.Hour
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func envInt(k string, d int) int {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}

func envBool(k string, d bool) bool {
	b, set := envBoolWithSet(k, d)
	if !set {
		return d
	}
	return b
}

func envBoolWithSet(k string, d bool) (bool, bool) {
	v := os.Getenv(k)
	if v == "" {
		return d, false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return d, true
	}
	return b, true
}

func ensureTrailingSlash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if strings.HasSuffix(v, "/") {
		return v
	}
	return v + "/"
}

func isAbsoluteHTTPURL(v string) bool {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || u == nil {
		return false
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return true
}

func envCSV(k string) []string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isLoopbackListen(addr string) bool {
	a := strings.ToLower(strings.TrimSpace(addr))
	return strings.Contains(a, "127.0.0.1") || strings.Contains(a, "localhost") || strings.Contains(a, "[::1]")
}

func isPotentiallyPublicListen(addr string) bool {
	a := strings.ToLower(strings.TrimSpace(addr))
	if a == "" {
		return false
	}
	if strings.HasPrefix(a, ":") || strings.Contains(a, "0.0.0.0") {
		return true
	}
	if strings.Contains(a, "[::]") || a == "::" {
		return true
	}
	return !isLoopbackListen(a)
}

func isLoopbackHost(rawHost string) bool {
	host := strings.TrimSpace(strings.Trim(rawHost, "[]"))
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func parseEd25519PublicKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, raw := range keys {
		k := strings.TrimSpace(raw)
		if k == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(k)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(k)
			if err != nil {
				return nil, fmt.Errorf("invalid UPDATE_SIGNING_PUBLIC_KEYS entry: %q", k)
			}
		}
		if len(decoded) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid UPDATE_SIGNING_PUBLIC_KEYS entry length: %q", k)
		}
		normalized := base64.StdEncoding.EncodeToString(decoded)
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func (c Config) ResolveCookieSecure(r *http.Request) bool {
	switch c.CookieSecureMode {
	case "always":
		return true
	case "never":
		return false
	case "auto":
		if r == nil {
			return false
		}
		if r.TLS != nil {
			return true
		}
		if c.TrustProxy {
			proto := forwardedProto(r.Header.Get("X-Forwarded-Proto"))
			return proto == "https"
		}
		return false
	default:
		return c.CookieSecure
	}
}

func forwardedProto(v string) string {
	p := strings.TrimSpace(strings.ToLower(v))
	if p == "" {
		return ""
	}
	if strings.Contains(p, ",") {
		p = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	if strings.Contains(p, ";") {
		p = strings.TrimSpace(strings.Split(p, ";")[0])
	}
	return p
}
