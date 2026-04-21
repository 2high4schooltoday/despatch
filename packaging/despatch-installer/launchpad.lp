app is "Despatch Installer".

app metadata runtime.command_policy is "native-only".
app metadata package.summary is "Launchpad-managed installer for Despatch".
app metadata package.homepage is "https://github.com/2high4schooltoday/despatch".

let home be env.HOME or else "/tmp".

type DeployMode is one of:
  "proxy" as "Reverse proxy".
  "direct" as "Direct HTTP".

type ProxyServer is one of:
  "nginx" as "Nginx".
  "apache2" as "Apache HTTP Server".

type DovecotAuthMode is one of:
  "sql" as "SQL database".
  "pam" as "PAM / system accounts".

field release is a text:
  label is "Release".
  help is "Use `latest` for the newest GitHub release or pin a specific tag such as `v1.2.3`.".
  default is "latest".

field baseDomain is a text:
  label is "Primary domain".
  help is "Used for Despatch setup defaults, reset links, and reverse-proxy hostnames.".
  default is host.name.

field listenAddr is a text:
  label is "Listen address".
  help is "The HTTP listen address Despatch should bind to before any reverse proxy.".
  default is ":8080".
  it is advanced.

field installService is a boolean:
  label is "Install system services".
  help is "Enable and start systemd units for Despatch, updater, mailsec, and the PAM helper socket.".
  default is true.

field deployMode is a DeployMode:
  label is "Deployment mode".
  help is "Choose direct HTTP exposure or place Despatch behind Nginx/Apache.".
  default is "proxy".

field proxyServer is a ProxyServer:
  label is "Proxy server".
  help is "Which reverse proxy Launchpad should configure when proxy mode is selected.".
  default is "nginx".
  it is advanced.

field proxyServerName is a text:
  label is "Proxy hostname".
  help is "Public hostname served by the reverse proxy.".
  default is baseDomain.

field proxyTls is a boolean:
  label is "Enable proxy TLS".
  help is "Use the selected certificate and key in the generated proxy configuration.".
  default is false.
  it is advanced.

field proxyCert is a text:
  label is "TLS certificate path".
  help is "Defaults to the LetsEncrypt fullchain path for the selected proxy hostname.".
  default is letsencrypt certificate path for proxyServerName.
  it is advanced.

field proxyKey is a text:
  label is "TLS private key path".
  help is "Defaults to the LetsEncrypt key path for the selected proxy hostname.".
  default is letsencrypt key path for proxyServerName.
  it is advanced.

fact dovecotConfigPath:
  when "/etc/dovecot/dovecot.conf" exists: "/etc/dovecot/dovecot.conf"
  when "/usr/local/etc/dovecot/dovecot.conf" exists: "/usr/local/etc/dovecot/dovecot.conf"
  otherwise: "".

config dovecotConfig:
  path is dovecotConfigPath.
  format is "dovecot".

fact detectedDovecotAuthSettings:
  when trimmed dovecotConfigPath != "": extract "dovecot.auth" from dovecotConfig
  otherwise: {}.

fact detectedDovecotSqlSettings:
  when trimmed dovecotConfigPath != "": extract "dovecot.sql" from dovecotConfig
  otherwise: {}.

fact detectedDovecotAuthMode is when value for key "auth_mode" in detectedDovecotAuthSettings is "pam": "pam" otherwise: "sql".

fact detectedDovecotSqlDsn is value for key "sql_dsn" in detectedDovecotAuthSettings.
fact detectedDovecotSqlDriver is value for key "sql_driver" in detectedDovecotSqlSettings.

field dovecotAuthMode is a DovecotAuthMode:
  label is "Dovecot auth mode".
  help is "Use `sql` for database-backed accounts or `pam` for system-account verification.".
  default is "sql".

field dovecotAuthDbDriver is a text:
  label is "Dovecot SQL driver".
  help is "Only used when SQL auth is selected and auto-detection does not find a working driver.".
  default is "mysql".
  it is advanced.

field dovecotAuthDbDsn is a text:
  label is "Dovecot SQL DSN".
  help is "Leave blank so Launchpad can derive the DSN from the Dovecot SQL config.".
  default is "".
  it is advanced.

field autoInstallDeps is a boolean:
  label is "Install host dependencies".
  help is "Install missing proxy and utility packages on Debian/Ubuntu hosts when needed.".
  default is true.
  it is advanced.

field ufwEnable is a boolean:
  label is "Enable UFW automation".
  help is "Apply local firewall rules through ufw when it is available.".
  default is false.
  it is advanced.

field ufwOpenProxyPorts is a boolean:
  label is "Open 80 and 443".
  help is "Relevant when reverse proxy mode is enabled.".
  default is true.
  it is advanced.

field ufwOpenDirectPort is a boolean:
  label is "Open direct listen port".
  help is "Relevant when direct mode is enabled.".
  default is true.
  it is advanced.

field enableCapCaptcha is a boolean:
  label is "Enable CAP captcha".
  help is "Configure Despatch for self-hosted CAP captcha endpoints routed through the reverse proxy.".
  default is false.
  it is advanced.

field capSiteKey is a text:
  label is "CAP site key".
  default is "cap-site-key".
  it is advanced.

field capSecret is a text:
  label is "CAP secret".
  help is "Leave blank to let the installer generate a new secret.".
  default is "".
  it is advanced.

field capUpstream is a text:
  label is "CAP upstream".
  help is "Host:port used by the reverse proxy for the `/cap/` route.".
  default is "127.0.0.1:8077".
  it is advanced.

field capAutoProvision is a boolean:
  label is "Auto-provision CAP".
  help is "Start the CAP container with Docker on the selected upstream port.".
  default is false.
  it is advanced.

field capImage is a text:
  label is "CAP image".
  default is "tiagozip/cap:latest".
  it is advanced.

field capAdminKey is a text:
  label is "CAP admin key".
  help is "Leave blank to let the installer generate a new admin key when CAP auto-provisioning is enabled.".
  default is "".
  it is advanced.

field capWidgetVersion is a text:
  label is "CAP widget version".
  default is "0.0.6".
  it is advanced.

field capWasmVersion is a text:
  label is "CAP WASM version".
  default is "0.0.6".
  it is advanced.

field runDiagnose is a boolean:
  label is "Run final diagnostics".
  help is "Run the packaged Despatch diagnostics after installation.".
  default is true.
  it is advanced.

field backupEnv is a boolean:
  label is "Backup /opt/despatch/.env".
  default is true.

field backupData is a boolean:
  label is "Backup /var/lib/despatch".
  default is true.

field removeAppFiles is a boolean:
  label is "Remove /opt/despatch".
  default is true.

field removeAppData is a boolean:
  label is "Remove /var/lib/despatch".
  default is true.

field removeSystemUser is a boolean:
  label is "Remove despatch system user".
  default is false.
  it is advanced.

field removeNginxSite is a boolean:
  label is "Remove Nginx site".
  default is true.
  it is advanced.

field removeApacheSite is a boolean:
  label is "Remove Apache site".
  default is true.
  it is advanced.

field removeCheckout is a boolean:
  label is "Remove /opt/despatch-installer".
  default is true.
  it is advanced.

fact archiveName:
  when host.architecture is "aarch64": "despatch-linux-arm64.tar.gz"
  otherwise: "despatch-linux-amd64.tar.gz".

fact releaseUrl:
  when release is "latest": "https://github.com/2high4schooltoday/despatch/releases/latest/download/{{archiveName}}"
  otherwise: "https://github.com/2high4schooltoday/despatch/releases/download/{{release}}/{{archiveName}}".

fact baseDomainMissing is trimmed baseDomain is "".
fact proxyServerNameMissing is trimmed proxyServerName is "".
fact proxyCertMissing is trimmed proxyCert is "".
fact proxyKeyMissing is trimmed proxyKey is "".
fact capSiteKeyMissing is trimmed capSiteKey is "".
fact capUpstreamMissing is trimmed capUpstream is "".
fact trimmedDovecotAuthDbDsn is trimmed dovecotAuthDbDsn.

path downloadDir is home / ".cache/despatch-installer".
path archivePath is downloadDir / archiveName.
path stagingRoot is downloadDir / "staging".
path extractDir is stagingRoot / "release".
path installRoot is "/opt/despatch".
path dataRoot is "/var/lib/despatch".
path envPath is installRoot / ".env".
path commandPath is installRoot / "despatch".
path installMetadataPath is dataRoot / "launchpad-install.json".
path checkoutRoot is "/opt/despatch-installer".
path backupRoot is "/var/backups/despatch".
path nginxSitePath is "/etc/nginx/sites-available/despatch.conf".
path nginxEnabledSitePath is "/etc/nginx/sites-enabled/despatch.conf".
path apacheSitePath is "/etc/apache2/sites-available/despatch.conf".

fact generatedSecretSeedA:
  when "/proc/sys/kernel/random/uuid" exists: replace "-" with "" in trimmed (read config from "/proc/sys/kernel/random/uuid" as plainText)
  otherwise: "0123456789abcdef0123456789abcdef".

fact generatedSecretSeedB:
  when "/proc/sys/kernel/random/uuid" exists: replace "-" with "" in trimmed (read config from "/proc/sys/kernel/random/uuid" as plainText)
  otherwise: "fedcba9876543210fedcba9876543210".

fact generatedSessionSecret is text joined from [generatedSecretSeedA, generatedSecretSeedB].

fact installServiceEnabled is installService is true.
fact proxyModeSelected is deployMode is "proxy".
fact directModeSelected is deployMode is "direct".
fact nginxProxyServerSelected is proxyServer is "nginx".
fact apacheProxyServerSelected is proxyServer is "apache2".
fact proxyTlsEnabled is proxyTls is true.
fact ufwEnabledSelected is ufwEnable is true.
fact ufwOpenProxyPortsSelected is ufwOpenProxyPorts is true.
fact ufwOpenDirectPortSelected is ufwOpenDirectPort is true.
fact enableCapSelected is enableCapCaptcha is true.
fact capAutoProvisionEnabled is capAutoProvision is true.

fact resolvedCapSecret:
  when trimmed capSecret != "": capSecret
  otherwise: generatedSecretSeedA.

fact resolvedCapAdminKey:
  when trimmed capAdminKey != "": capAdminKey
  otherwise: generatedSecretSeedB.

fact resolvedListenAddr:
  when proxyModeSelected and listenAddr is ":8080": "127.0.0.1:8080"
  otherwise: listenAddr.

fact resolvedProxyServer:
  when proxyModeSelected: proxyServer
  otherwise: "".

fact resolvedProxyServerName:
  when proxyModeSelected: proxyServerName
  otherwise: "".

fact resolvedProxyTls:
  when proxyModeSelected: proxyTls
  otherwise: false.

fact resolvedProxyCert:
  when proxyModeSelected and proxyTlsEnabled: proxyCert
  otherwise: "".

fact resolvedProxyKey:
  when proxyModeSelected and proxyTlsEnabled: proxyKey
  otherwise: "".

fact resolvedDovecotAuthMode:
  when dovecotAuthMode is "pam": "pam"
  otherwise: "sql".

fact resolvedDovecotDsn:
  when resolvedDovecotAuthMode is "sql" and trimmedDovecotAuthDbDsn != "": trimmedDovecotAuthDbDsn
  when resolvedDovecotAuthMode is "sql": detectedDovecotSqlDsn or else ""
  otherwise: "".

fact resolvedDovecotSqlDriver:
  when detectedDovecotSqlDriver is "pgsql": "pgx"
  when detectedDovecotSqlDriver is "mysql": "mysql"
  when detectedDovecotSqlDriver is "mariadb": "mysql"
  otherwise: dovecotAuthDbDriver.

fact resolvedDovecotDsnText is resolvedDovecotDsn or else "".
fact resolvedDovecotDsnMissing is trimmed resolvedDovecotDsnText is "".

fact cookieSecureMode:
  when proxyModeSelected and proxyTlsEnabled: "always"
  otherwise: "never".

fact cookieSecureLegacy:
  when proxyModeSelected and proxyTlsEnabled: true
  otherwise: false.

fact trustProxyEnabled:
  when proxyModeSelected: true
  otherwise: false.

fact imapPort is "993".
fact imapTls is true.
fact imapStarttls is false.
fact imapSkipVerify is true.

fact smtpPort is "25".
fact smtpTls is false.
fact smtpStarttls is false.
fact smtpSkipVerify is false.

fact passwordResetFromDomain:
  when baseDomain starts with "mail.": replace prefix "mail." with "" in baseDomain
  otherwise: baseDomain.

fact passwordResetFromAddr is text joined from ["no-reply@", passwordResetFromDomain].

fact passwordResetBaseUrl:
  when proxyModeSelected and proxyTlsEnabled: text joined from ["https://", proxyServerName]
  when proxyModeSelected: text joined from ["http://", proxyServerName]
  otherwise: text joined from ["http://", baseDomain, ":8080"].

fact passwordResetExternalSenderReady:
  when resolvedDovecotAuthMode is "pam" and installService and smtpPort is "25": true
  otherwise: false.

fact pamResetHelperEnabled is installServiceEnabled and pamModeSelected.

fact capEnabled:
  when enableCapSelected: true
  otherwise: false.

fact pamModeSelected is resolvedDovecotAuthMode is "pam".

fact capWidgetApiUrl:
  when enableCapSelected: text joined from ["/cap/", capSiteKey, "/"]
  otherwise: "".

fact capVerifyUrl:
  when enableCapSelected: text joined from ["http://", capUpstream, "/", capSiteKey, "/siteverify"]
  otherwise: "".

fact resolvedCaptchaProvider:
  when enableCapSelected: "cap"
  otherwise: "turnstile".

fact resolvedCaptchaSiteKey:
  when enableCapSelected: capSiteKey
  otherwise: "".

fact resolvedCaptchaSecret:
  when enableCapSelected: resolvedCapSecret
  otherwise: "".

fact resolvedWebauthnRpId:
  when proxyModeSelected: proxyServerName
  otherwise: baseDomain.

fact proxyUpstreamUrl is text joined from ["http://", resolvedListenAddr].
fact capContainerPortBinding is text joined from [capUpstream, ":80/tcp"].

fact capAutoProvisionSelected is capEnabled and capAutoProvisionEnabled.

fact capProxyRoutes:
  when capEnabled: [{ path: "/cap/", proxy_to: text joined from ["http://", capUpstream] }]
  otherwise: [].

fact apacheProxyModules:
  when proxyTls: ["proxy", "proxy_http", "headers", "rewrite", "ssl"]
  otherwise: ["proxy", "proxy_http", "headers", "rewrite"].

fact nginxExtraConfig:
  when proxyTls: """
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    """
  otherwise: """
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    """.

fact apacheExtraConfig:
  when proxyTls: """
    Header always set X-Frame-Options "DENY"
    Header always set X-Content-Type-Options "nosniff"
    Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
    """
  otherwise: """
    Header always set X-Frame-Options "DENY"
    Header always set X-Content-Type-Options "nosniff"
    """.

fact directListenAddrSegments is split resolvedListenAddr by ":".
fact directListenPort is last item in directListenAddrSegments.

fact stagedEnvValues is { "LISTEN_ADDR": resolvedListenAddr, "BASE_DOMAIN": baseDomain, "APP_DB_PATH": "/var/lib/despatch/app.db", "SESSION_COOKIE_NAME": "despatch_session", "MFA_TRUSTED_COOKIE_NAME": "despatch_mfa_trusted", "SESSION_IDLE_MINUTES": "30", "SESSION_ABSOLUTE_HOURS": "24", "SESSION_ENCRYPT_KEY": generatedSessionSecret, "CSRF_COOKIE_NAME": "despatch_csrf", "COOKIE_SECURE_MODE": cookieSecureMode, "COOKIE_SECURE": cookieSecureLegacy, "TRUST_PROXY": trustProxyEnabled, "CORS_ALLOWED_ORIGINS": "", "CAPTCHA_ENABLED": capEnabled, "CAPTCHA_PROVIDER": resolvedCaptchaProvider, "CAPTCHA_SITE_KEY": resolvedCaptchaSiteKey, "CAPTCHA_WIDGET_API_URL": capWidgetApiUrl, "CAPTCHA_VERIFY_URL": capVerifyUrl, "CAPTCHA_SECRET": resolvedCaptchaSecret, "PASSWORD_MIN_LENGTH": "12", "PASSWORD_MAX_LENGTH": "128", "IMAP_HOST": "127.0.0.1", "IMAP_PORT": imapPort, "IMAP_TLS": imapTls, "IMAP_STARTTLS": imapStarttls, "IMAP_INSECURE_SKIP_VERIFY": imapSkipVerify, "MAIL_ALLOW_INSECURE_SKIP_VERIFY_NON_LOOPBACK": false, "SMTP_HOST": "127.0.0.1", "SMTP_PORT": smtpPort, "SMTP_TLS": smtpTls, "SMTP_STARTTLS": smtpStarttls, "SMTP_INSECURE_SKIP_VERIFY": smtpSkipVerify, "DOVECOT_AUTH_DB_DRIVER": resolvedDovecotSqlDriver, "DOVECOT_AUTH_DB_DSN": resolvedDovecotDsnText, "DOVECOT_AUTH_MODE": resolvedDovecotAuthMode, "DOVECOT_AUTH_TABLE": "users", "DOVECOT_AUTH_EMAIL_COL": "email", "DOVECOT_AUTH_PASS_COL": "password_hash", "DOVECOT_AUTH_ACTIVE_COL": "active", "DOVECOT_AUTH_MAILDIR_COL": "maildir", "DOVECOT_MAILDIR_BASE": "/var/mail/vhosts", "HTTP_READ_TIMEOUT_SEC": "10", "HTTP_READ_HEADER_TIMEOUT_SEC": "5", "HTTP_WRITE_TIMEOUT_SEC": "30", "HTTP_IDLE_TIMEOUT_SEC": "60", "PASSWORD_RESET_SENDER": "smtp", "PASSWORD_RESET_FROM": passwordResetFromAddr, "PASSWORD_RESET_BASE_URL": passwordResetBaseUrl, "PASSWORD_RESET_SMTP_USER": "", "PASSWORD_RESET_SMTP_PASS": "", "PASSWORD_RESET_TOKEN_TTL_MINUTES": "30", "PASSWORD_RESET_PUBLIC_ENABLED": true, "PASSWORD_RESET_EXTERNAL_SENDER_READY": passwordResetExternalSenderReady, "PASSWORD_RESET_REQUIRE_MAPPED_LOGIN": true, "PAM_RESET_HELPER_ENABLED": pamResetHelperEnabled, "PAM_RESET_HELPER_SOCKET": "/run/despatch/pam-reset-helper.sock", "PAM_RESET_HELPER_TIMEOUT_SEC": "5", "PAM_RESET_ALLOWED_UID": "-1", "PAM_RESET_ALLOWED_GID": "-1", "UPDATE_ENABLED": true, "UPDATE_REPO_OWNER": "2high4schooltoday", "UPDATE_REPO_NAME": "despatch", "UPDATE_CHECK_INTERVAL_MIN": "60", "UPDATE_HTTP_TIMEOUT_SEC": "10", "UPDATE_GITHUB_TOKEN": "", "UPDATE_BACKUP_KEEP": "3", "UPDATE_BASE_DIR": dataRoot / "update", "UPDATE_INSTALL_DIR": installRoot, "UPDATE_SERVICE_NAME": "despatch", "UPDATE_SYSTEMD_UNIT_DIR": "/etc/systemd/system", "UPDATE_REQUIRE_SIGNATURE": true, "UPDATE_SIGNATURE_ASSET": "checksums.txt.sig", "UPDATE_SIGNING_PUBLIC_KEYS": "NgWFCGvluSgGiR4KpQhrNGqK4UVDRcEb76RNBEproBw=", "MAILSEC_ENABLED": true, "MAILSEC_SOCKET": "/run/despatch/mailsec.sock", "MAILSEC_TIMEOUT_MS": "5000", "WEBAUTHN_RP_ID": resolvedWebauthnRpId, "WEBAUTHN_ALLOWED_ORIGINS": passwordResetBaseUrl, "PASSKEY_PASSWORDLESS_ENABLED": true, "PROXY_SERVER": resolvedProxyServer, "PROXY_SERVER_NAME": resolvedProxyServerName, "PROXY_TLS": resolvedProxyTls, "BOOTSTRAP_ADMIN_EMAIL": "", "BOOTSTRAP_ADMIN_PASSWORD": "" }.

config runtimeEnv:
  path is envPath.
  format is "env".

package caCertificatesPackage:
  generic name is "ca-certificates".

package curlPackage:
  generic name is "curl".

package tarPackage:
  generic name is "tar".

package systemdPackage:
  generic name is "systemd".

package reverseProxyPackage:
  generic name is when proxyServer is "apache2": "apache2" otherwise: "nginx".

package ufwPackage:
  generic name is "ufw".

package dockerPackage:
  generic name is "docker.io".

service despatchService:
  unit name is "despatch.service".
  manager is systemd.
  file is installRoot / "deploy" / "despatch.service".
  present is true.
  enabled is installService.
  remove override directory is true.

service despatchUpdaterService:
  unit name is "despatch-updater.service".
  manager is systemd.
  file is installRoot / "deploy" / "despatch-updater.service".
  present is true.
  remove override directory is true.

service despatchUpdaterPath:
  unit name is "despatch-updater.path".
  manager is systemd.
  file is installRoot / "deploy" / "despatch-updater.path".
  present is true.
  enabled is installService.
  remove override directory is true.

service despatchPamResetHelperService:
  unit name is "despatch-pam-reset-helper.service".
  manager is systemd.
  file is installRoot / "deploy" / "despatch-pam-reset-helper.service".
  present is pamResetHelperEnabled.
  remove override directory is true.

service despatchPamResetHelperSocket:
  unit name is "despatch-pam-reset-helper.socket".
  manager is systemd.
  file is installRoot / "deploy" / "despatch-pam-reset-helper.socket".
  present is pamResetHelperEnabled.
  enabled is pamResetHelperEnabled.
  remove override directory is true.

service despatchMailsecService:
  unit name is "despatch-mailsec.service".
  manager is systemd.
  file is installRoot / "deploy" / "despatch-mailsec.service".
  present is true.
  enabled is installService.
  remove override directory is true.

service legacyMailclientService:
  unit name is "mailclient.service".
  manager is systemd.
  present is false.
  remove override directory is true.

service legacyMailclientUpdaterService:
  unit name is "mailclient-updater.service".
  manager is systemd.
  present is false.
  remove override directory is true.

service legacyMailclientUpdaterPath:
  unit name is "mailclient-updater.path".
  manager is systemd.
  present is false.
  remove override directory is true.

service legacyMailclientMailsecService:
  unit name is "mailclient-mailsec.service".
  manager is systemd.
  present is false.
  remove override directory is true.

service legacyMailclientPamResetHelperService:
  unit name is "mailclient-pam-reset-helper.service".
  manager is systemd.
  present is false.
  remove override directory is true.

service legacyMailclientPamResetHelperSocket:
  unit name is "mailclient-pam-reset-helper.socket".
  manager is systemd.
  present is false.
  remove override directory is true.

nginx site despatchNginx:
  site name is "despatch".
  available path is nginxSitePath.
  enabled path is nginxEnabledSitePath.
  server names are [proxyServerName].
  proxy to is proxyUpstreamUrl.
  proxy headers are { "Host": "$host", "X-Forwarded-For": "$proxy_add_x_forwarded_for", "X-Forwarded-Proto": "$scheme" }.
  request body limit is "30m".
  tls cert is resolvedProxyCert.
  tls key is resolvedProxyKey.
  routes are capProxyRoutes.
  extra config is nginxExtraConfig.
  validate is true.
  reload is true.
  enabled is proxyModeSelected and nginxProxyServerSelected.
  present is proxyModeSelected and nginxProxyServerSelected.

apache site despatchApache:
  site name is "despatch.conf".
  path is apacheSitePath.
  server names are [proxyServerName].
  proxy to is proxyUpstreamUrl.
  modules are apacheProxyModules.
  request body limit is "31457280".
  tls cert is resolvedProxyCert.
  tls key is resolvedProxyKey.
  routes are capProxyRoutes.
  extra config is apacheExtraConfig.
  validate is true.
  reload is true.
  enabled is proxyModeSelected and apacheProxyServerSelected.
  present is proxyModeSelected and apacheProxyServerSelected.

container service despatchCapService:
  unit name is "despatch-cap.service".
  engine is "docker".
  container name is "cap".
  image is capImage.
  pull policy is "missing".
  restart policy is "always".
  env is { "ADMIN_KEY": resolvedCapAdminKey, "ENABLE_ASSETS_SERVER": "true", "WIDGET_VERSION": capWidgetVersion, "WASM_VERSION": capWasmVersion }.
  ports are [capContainerPortBinding].
  volumes are [{ source: "cap-data", target: "/usr/src/app/.data", kind: "volume", create: true }].
  present is installServiceEnabled and capAutoProvisionSelected.
  enabled is true.
  state is "started".

firewall rule proxyHttpIngress:
  port is "80".
  protocol is "tcp".
  action is "allow".
  direction is "in".
  from is "any".
  to is "any".
  comment is "despatch proxy http".
  present is ufwEnabledSelected and proxyModeSelected and ufwOpenProxyPortsSelected.

firewall rule proxyHttpsIngress:
  port is "443".
  protocol is "tcp".
  action is "allow".
  direction is "in".
  from is "any".
  to is "any".
  comment is "despatch proxy https".
  present is ufwEnabledSelected and proxyModeSelected and ufwOpenProxyPortsSelected.

firewall rule directIngress:
  port is directListenPort.
  protocol is "tcp".
  action is "allow".
  direction is "in".
  from is "any".
  to is "any".
  comment is "despatch direct listen".
  present is ufwEnabledSelected and directModeSelected and ufwOpenDirectPortSelected.

fact detectedInstallState:
  when commandPath exists: "installed"
  otherwise: "not installed".

fact detectedVersion:
  when commandPath exists: version of executable at commandPath
  otherwise: "not installed".

fact detectedConfigState:
  when envPath exists: "present"
  otherwise: "missing".

fact installedMetadata is read config from installMetadataPath as json.

fact metadataListenAddr is value for key "listen_addr" in installedMetadata.
fact metadataDeployMode is value for key "deploy_mode" in installedMetadata.
fact metadataBaseDomain is value for key "base_domain" in installedMetadata.
fact metadataProxyServer is value for key "proxy_server" in installedMetadata.
fact metadataProxyServerName is value for key "proxy_server_name" in installedMetadata.
fact metadataProxyTls is value for key "proxy_tls" in installedMetadata.
fact metadataCapEnabled is value for key "cap_enabled" in installedMetadata.
fact metadataUfwEnable is value for key "ufw_enable" in installedMetadata.
fact metadataUfwOpenProxyPorts is value for key "ufw_open_proxy_ports" in installedMetadata.
fact metadataUfwOpenDirectPort is value for key "ufw_open_direct_port" in installedMetadata.

fact installedListenAddr is (metadataListenAddr or else ":8080") as text.
fact installedDeployMode is (metadataDeployMode or else "direct") as text.
fact installedBaseDomain is (metadataBaseDomain or else "") as text.
fact installedProxyServer is (metadataProxyServer or else "not configured") as text.
fact installedProxyServerName is (metadataProxyServerName or else "") as text.
fact installedProxyTls is metadataProxyTls or else false.
fact installedCapEnabled is metadataCapEnabled or else false.
fact installedUfwEnable is metadataUfwEnable or else false.
fact installedUfwOpenProxyPorts is metadataUfwOpenProxyPorts or else false.
fact installedUfwOpenDirectPort is metadataUfwOpenDirectPort or else false.
fact installedProxyTlsEnabled is installedProxyTls is true.
fact installedCapEnabledFlag is installedCapEnabled is true.
fact installedUfwEnabledFlag is installedUfwEnable is true.
fact installedUfwOpenProxyPortsFlag is installedUfwOpenProxyPorts is true.
fact installedUfwOpenDirectPortFlag is installedUfwOpenDirectPort is true.

fact installedHealthListenAddr:
  when installedListenAddr starts with ":": text joined from ["127.0.0.1", installedListenAddr]
  otherwise: installedListenAddr.

fact localHealthUrl is health url for listen address installedHealthListenAddr.
fact readinessReport:
  when commandPath exists: service readiness report for "despatch" at localHealthUrl within 45 seconds
  otherwise: { ready: false, state: "not installed" }.

fact detectedProxyState:
  when installedDeployMode is "direct": "direct"
  when installedProxyServer is "nginx": "nginx"
  when installedProxyServer is "apache2": "apache2"
  when nginxSitePath exists: "nginx"
  when apacheSitePath exists: "apache2"
  otherwise: "not configured".

fact detectedServiceState is state of system service "despatch.service".
fact detectedUpdaterState is state of system service "despatch-updater.path".
fact detectedMailsecState is state of system service "despatch-mailsec.service".
fact detectedPamSocketState is state of system service "despatch-pam-reset-helper.socket".
fact detectedCapServiceState is state of system service "despatch-cap.service".
fact detectedFirewallStatus is status of firewall.

fact installedListenAddrSegments is split installedListenAddr by ":".
fact installedListenPort is last item in installedListenAddrSegments.

fact localHttpDiagnostics:
  when commandPath exists: http diagnostics for { url: localHealthUrl, expected_status: 200, timeout: 5, retries: 2 }
  otherwise: { ok: false, findings: [] }.

fact installedProxyConnectTarget:
  when installedProxyTlsEnabled: "127.0.0.1:443"
  otherwise: "127.0.0.1:80".

fact installedProxyHealthUrl:
  when installedDeployMode is "proxy" and installedProxyTlsEnabled: text joined from ["https://", installedProxyServerName, "/health/live"]
  when installedDeployMode is "proxy": text joined from ["http://", installedProxyServerName, "/health/live"]
  otherwise: localHealthUrl.

fact proxyHttpDiagnostics:
  when commandPath exists and installedDeployMode is "proxy": http diagnostics for { url: installedProxyHealthUrl, expected_status: 200, timeout: 5, retries: 2, host_header: installedProxyServerName, connect_target: installedProxyConnectTarget }
  otherwise: { ok: true, findings: [] }.

fact proxyRoutingDiagnostics:
  when commandPath exists and installedDeployMode is "proxy": proxy routing diagnostics for { hostname: installedProxyServerName, path: "/health/live" }
  otherwise: { ok: true, findings: [] }.

fact tlsEndpointDiagnostics:
  when commandPath exists and installedDeployMode is "proxy" and installedProxyTlsEnabled: tls diagnostics for { hostname: installedProxyServerName, validate_redirect: true }
  otherwise: { ok: true, findings: [] }.

fact proxyConfigDiagnostics:
  when commandPath exists and installedDeployMode is "proxy" and installedProxyServer is "nginx": proxy configuration diagnostics for { kind: "nginx", hostname: installedProxyServerName }
  when commandPath exists and installedDeployMode is "proxy" and installedProxyServer is "apache2": proxy configuration diagnostics for { kind: "apache", hostname: installedProxyServerName }
  otherwise: { ok: true, findings: [] }.

fact appListenDiagnostics:
  when commandPath exists: listening port diagnostics for { port: installedListenPort, role: "app" }
  otherwise: { ok: false, findings: [] }.

fact proxyListenDiagnostics:
  when commandPath exists and installedDeployMode is "proxy" and installedProxyTlsEnabled: listening port diagnostics for { port: 443, role: "proxy" }
  when commandPath exists and installedDeployMode is "proxy": listening port diagnostics for { port: 80, role: "proxy" }
  otherwise: { ok: true, findings: [] }.

fact proxyHttpsFirewallDiagnostics:
  when commandPath exists and installedUfwEnabledFlag and installedDeployMode is "proxy" and installedUfwOpenProxyPortsFlag: firewall diagnostics for { port: "443", protocol: "tcp", action: "allow" }
  otherwise: { ok: true, findings: [] }.

fact proxyHttpFirewallDiagnostics:
  when commandPath exists and installedUfwEnabledFlag and installedDeployMode is "proxy" and installedUfwOpenProxyPortsFlag: firewall diagnostics for { port: "80", protocol: "tcp", action: "allow" }
  otherwise: { ok: true, findings: [] }.

fact directFirewallDiagnostics:
  when commandPath exists and installedUfwEnabledFlag and installedDeployMode is "direct" and installedUfwOpenDirectPortFlag: firewall diagnostics for { port: installedListenPort, protocol: "tcp", action: "allow" }
  otherwise: { ok: true, findings: [] }.

fact dnsHostName:
  when installedDeployMode is "proxy": installedProxyServerName
  otherwise: installedBaseDomain.

fact dnsDiagnostics:
  when commandPath exists and trimmed dnsHostName != "": dns diagnostics for { hostname: dnsHostName, compare_to_public_ip: true }
  otherwise: { ok: true, findings: [] }.

fact combinedHealthDiagnostics:
  when commandPath exists: combined diagnostics for [localHttpDiagnostics, proxyHttpDiagnostics, proxyRoutingDiagnostics, tlsEndpointDiagnostics, proxyConfigDiagnostics, appListenDiagnostics, proxyListenDiagnostics, proxyHttpFirewallDiagnostics, proxyHttpsFirewallDiagnostics, directFirewallDiagnostics, dnsDiagnostics]
  otherwise: { ok: false, findings: [] }.

management:
  kind is application.
  title is "Despatch".
  icon is "package".
  uninstall operation is uninstall.
  status operation is status.
  diagnose operation is diagnose.
  installation roots are:
    installRoot
  application data roots are:
    dataRoot

requirement group installRequirements:
  release, baseDomain, listenAddr, archiveName, releaseUrl,
  downloadDir, archivePath, installRoot, dataRoot, installService,
  deployMode, proxyServer, proxyServerName, proxyTls,
  dovecotAuthMode, dovecotAuthDbDriver, dovecotAuthDbDsn,
  autoInstallDeps, ufwEnable, ufwOpenProxyPorts, ufwOpenDirectPort,
  enableCapCaptcha, capSiteKey, capUpstream, capAutoProvision,
  capImage, capWidgetVersion, capWasmVersion, runDiagnose.

operation install:
  presents as:
    title is "Install Despatch".
    shortTitle is "Install".
    summary is "Download the selected Despatch release archive, install its runtime and native host integrations, and register it for Launchpad Applications Centre management.".
    accent is "Install".
    risk is "MEDIUM".

  validation baseDomainRequired:
    when baseDomainMissing != false.
    message is "Primary domain is required.".

  validation proxyHostRequired:
    when deployMode is "proxy" and proxyServerNameMissing != false.
    message is "Proxy hostname is required when reverse proxy mode is selected.".

  validation proxyCertRequired:
    when deployMode is "proxy" and proxyTls != false and proxyCertMissing != false.
    message is "TLS certificate path is required when proxy TLS is enabled.".

  validation proxyKeyRequired:
    when deployMode is "proxy" and proxyTls != false and proxyKeyMissing != false.
    message is "TLS private key path is required when proxy TLS is enabled.".

  validation capNeedsProxy:
    when enableCapCaptcha != false and deployMode != "proxy".
    message is "CAP captcha requires reverse proxy mode so `/cap/` can be routed correctly.".

  validation capSiteRequired:
    when enableCapCaptcha != false and capSiteKeyMissing != false.
    message is "CAP site key is required when CAP captcha is enabled.".

  validation capUpstreamRequired:
    when enableCapCaptcha != false and capUpstreamMissing != false.
    message is "CAP upstream is required when CAP captcha is enabled.".

  validation sqlDsnRequired:
    when resolvedDovecotAuthMode is "sql" and resolvedDovecotDsnMissing is true.
    message is "Launchpad could not derive a Dovecot SQL DSN. Set one explicitly or fix the Dovecot SQL config.".

  preflight noSudo:
    when system is not root and host does not have sudo.
    code is "E_NO_SUDO".
    message is "Despatch installation needs administrator access.".
    suggested fix is "Run the installer as root or install `sudo` first.".

  preflight sudoReady:
    when system is not root and host has sudo and sudo is not ready.
    code is "E_SUDO".
    message is "Launchpad needs a ready sudo session before installing Despatch.".
    suggested fix is "Run `sudo -v` in this shell and start the install again.".

  step home is an intro:
    title is "Launchpad Despatch Installer".
    summary is "This migrates Despatch to a fully Launchpad-native install so Applications Centre can own install, status, diagnose, and uninstall.".
    hero:
      icon is "package".
      title is "Install Despatch".
      summary is "Launchpad downloads the selected release archive, wires native system resources, and registers the app for later management.".

  step setup is a form:
    title is "Installation Options".
    summary is "Choose the release, domain, runtime mode, and optional host automation.".
    basic fields are:
      release
      baseDomain
      installService
      deployMode
      proxyServerName
      dovecotAuthMode
    advanced fields are:
      listenAddr
      proxyServer
      proxyTls
      proxyCert
      proxyKey
      dovecotAuthDbDriver
      dovecotAuthDbDsn
      autoInstallDeps
      ufwEnable
      ufwOpenProxyPorts
      ufwOpenDirectPort
      enableCapCaptcha
      capSiteKey
      capSecret
      capUpstream
      capAutoProvision
      capImage
      capAdminKey
      capWidgetVersion
      capWasmVersion
      runDiagnose
    tip "Most production installs should keep system services enabled and use reverse proxy mode.".
    warning "Direct mode exposes Despatch on the selected listen address without a Launchpad-managed reverse proxy." when deployMode is "direct".
    warning "CAP captcha auto-provisioning installs and manages a Docker-backed container service." when enableCapCaptcha and capAutoProvision.

  step review is a review:
    title is "Ready To Install".
    summary is "Confirm the resolved release, deployment surface, and lifecycle automation before Launchpad changes the host.".
    groups are:
      group "Release":
        shows "Release" as release
        shows "Archive" as archiveName
        shows "Download URL" as releaseUrl
      group "Deployment":
        shows "Domain" as baseDomain
        shows "Listen address" as resolvedListenAddr
        shows "Mode" as deployMode
        shows "Proxy server" as proxyServer
        shows "Proxy hostname" as proxyServerName
        shows "Proxy TLS" as proxyTls
      group "Auth And Ops":
        shows "Dovecot auth mode" as resolvedDovecotAuthMode
        shows "Derived Dovecot SQL DSN" as resolvedDovecotDsnText
        shows "Install system services" as installService
        shows "Install dependencies" as autoInstallDeps
        shows "Run final diagnostics" as runDiagnose

  stage "Prepare":
    require installRequirements are set.
    ensure directory downloadDir.
    when stagingRoot exists:
      delete stagingRoot.
    ensure directory stagingRoot.
    ensure directory extractDir.

  stage "Download" with timeout 300 seconds:
    retry 3 times:
      download releaseUrl to archivePath within 240 seconds.

  stage "Extract":
    extract archivePath to extractDir.
    check that extractDir / "despatch" exists.
    check that extractDir / "despatch-pam-reset-helper" exists.
    check that extractDir / "despatch-update-worker" exists.
    check that extractDir / "despatch-mailsec-service" exists.
    check that extractDir / "web" exists.
    check that extractDir / "migrations" exists.
    check that extractDir / "deploy" exists.

  stage "Dependencies" when autoInstallDeps:
    refresh package metadata.
    ensure package caCertificatesPackage is installed.
    ensure package curlPackage is installed.
    ensure package tarPackage is installed.
    ensure package systemdPackage is installed.
    when deployMode is "proxy":
      ensure package reverseProxyPackage is installed.
    when ufwEnable:
      ensure package ufwPackage is installed.
    when capEnabled and capAutoProvision:
      ensure package dockerPackage is installed.

  stage "Install Runtime":
    ensure service account "despatch" exists.
    ensure directory installRoot:
      owner is "root".
      group is "root".
      mode is "755".
    ensure directory dataRoot:
      owner is "despatch".
      group is "despatch".
      mode is "750".
    ensure directory dataRoot / "update":
      owner is "root".
      group is "despatch".
      mode is "750".
    ensure directory dataRoot / "update" / "request":
      owner is "root".
      group is "despatch".
      mode is "770".
    ensure directory dataRoot / "update" / "status":
      owner is "root".
      group is "despatch".
      mode is "770".
    ensure directory dataRoot / "update" / "lock":
      owner is "root".
      group is "root".
      mode is "750".
    ensure directory dataRoot / "update" / "backups":
      owner is "root".
      group is "root".
      mode is "750".
    ensure directory dataRoot / "update" / "work":
      owner is "root".
      group is "root".
      mode is "750".
    install tree extractDir to installRoot:
      owner is "root".
      group is "root".
      file mode is "644".
      directory mode is "755".
      overwrite is true.
      prune is true.
      required paths are ["despatch", "despatch-pam-reset-helper", "despatch-update-worker", "despatch-mailsec-service", "web", "migrations", "deploy"].
      non empty is true.
    set permissions of installRoot / "despatch" to "755".
    set permissions of installRoot / "despatch-pam-reset-helper" to "755".
    set permissions of installRoot / "despatch-update-worker" to "755".
    set permissions of installRoot / "despatch-mailsec-service" to "755".
    write env file stagedEnvValues to envPath.
    when pamResetHelperEnabled:
      set runtime value pamResetAllowedUid to (value for key "uid" in user lookup for "despatch") as text.
      set runtime value pamResetAllowedGid to (value for key "gid" in user lookup for "despatch") as text.
      set env key "PAM_RESET_ALLOWED_UID" in runtimeEnv to pamResetAllowedUid.
      set env key "PAM_RESET_ALLOWED_GID" in runtimeEnv to pamResetAllowedGid.
    set owner envPath to "root".
    set group envPath to "despatch".
    set permissions of envPath to "640".
    write config { release: release, base_domain: baseDomain, listen_addr: resolvedListenAddr, deploy_mode: deployMode, proxy_server: proxyServer, proxy_server_name: proxyServerName, proxy_tls: proxyTls, dovecot_auth_mode: resolvedDovecotAuthMode, cap_enabled: capEnabled, cap_auto_provision: capAutoProvision, ufw_enable: ufwEnable, ufw_open_proxy_ports: ufwOpenProxyPorts, ufw_open_direct_port: ufwOpenDirectPort } to installMetadataPath.
    set owner installMetadataPath to "root".
    set group installMetadataPath to "root".
    set permissions of installMetadataPath to "644".
    record artifact "despatch-root" from installRoot.
    record artifact "despatch-binary" from commandPath.
    record artifact "despatch-env" from envPath.
    record artifact "despatch-metadata" from installMetadataPath.

  stage "Wire Services":
    install service unit despatchService.
    install service unit despatchUpdaterService.
    install service unit despatchUpdaterPath.
    when pamResetHelperEnabled:
      install service unit despatchPamResetHelperService.
      install service unit despatchPamResetHelperSocket.
    otherwise:
      remove service unit despatchPamResetHelperSocket.
      remove service unit despatchPamResetHelperService.
    install service unit despatchMailsecService.
    when capEnabled and capAutoProvision:
      install container service despatchCapService.
    otherwise:
      remove container service despatchCapService.
    reload service manager.
    reset failed service despatchService.
    reset failed service despatchUpdaterPath.
    reset failed service despatchMailsecService.
    when pamResetHelperEnabled:
      reset failed service despatchPamResetHelperSocket.
    remove service unit legacyMailclientService.
    remove service unit legacyMailclientUpdaterService.
    remove service unit legacyMailclientUpdaterPath.
    remove service unit legacyMailclientMailsecService.
    remove service unit legacyMailclientPamResetHelperService.
    remove service unit legacyMailclientPamResetHelperSocket.
    when installService:
      enable service despatchUpdaterPath.
      start service despatchUpdaterPath.
      enable service despatchService.
      start service despatchService.
      enable service despatchMailsecService.
      start service despatchMailsecService.
    when installServiceEnabled and pamResetHelperEnabled:
      enable service despatchPamResetHelperSocket.
      start service despatchPamResetHelperSocket.
    when installServiceEnabled is false:
      disable service despatchUpdaterPath.
      ensure service despatchUpdaterPath is stopped.
      disable service despatchService.
      ensure service despatchService is stopped.
      disable service despatchMailsecService.
      ensure service despatchMailsecService is stopped.
    when installServiceEnabled is false or pamResetHelperEnabled is false:
      disable service despatchPamResetHelperSocket.
      ensure service despatchPamResetHelperSocket is stopped.

  stage "Wire Proxy":
    when deployMode is "proxy" and proxyServer is "nginx":
      install nginx site despatchNginx.
      remove apache site despatchApache.
    when deployMode is "proxy" and proxyServer is "apache2":
      install apache site despatchApache.
      remove nginx site despatchNginx.
    when deployMode is "direct":
      remove nginx site despatchNginx.
      remove apache site despatchApache.

  stage "Wire Firewall" when ufwEnable:
    ensure firewall is enabled.
    install firewall rule proxyHttpIngress.
    install firewall rule proxyHttpsIngress.
    install firewall rule directIngress.

  stage "Verify":
    verify artifacts.
    when installService:
      check that detectedServiceState is "active".
      check that detectedUpdaterState is "active".
      check that detectedMailsecState is "active".
      check that value for key "ready" in readinessReport is true.
    when installService and resolvedDovecotAuthMode is "pam":
      check that detectedPamSocketState is "active".
    when installService and capEnabled and capAutoProvision:
      check that detectedCapServiceState is "active".
    when runDiagnose:
      check that value for key "ok" in combinedHealthDiagnostics is true.
    add next action "launchpad apps status despatch".

  stage "Finish":
    show panel "Applications Centre" with "Despatch is now registered for Launchpad management. Use `launchpad apps`, `launchpad apps status despatch`, or `launchpad apps diagnose despatch` for the day-two lifecycle.".

operation status:
  presents as:
    title is "Despatch Status".
    summary is "Read the current Despatch installation state without mutating the host.".

  step overview is a status:
    title is "Current State".
    summary is "Launchpad reports the installed files, units, proxy footprint, and firewall surface it can detect right now.".

  status shows:
    shows "Install state" as detectedInstallState
    shows "Version" as detectedVersion
    shows "Config file" as detectedConfigState
    shows "Proxy config" as detectedProxyState
    shows "Listen address" as installedListenAddr
    shows "Deploy mode" as installedDeployMode
    shows "Firewall" as detectedFirewallStatus
    shows "despatch.service" as detectedServiceState
    shows "despatch-updater.path" as detectedUpdaterState
    shows "despatch-mailsec.service" as detectedMailsecState
    shows "despatch-pam-reset-helper.socket" as detectedPamSocketState
    shows "despatch-cap.service" as detectedCapServiceState

  stage "Inspect":
    show panel "Despatch" with "Install root: `{{installRoot}}`\nData root: `{{dataRoot}}`\nBinary: `{{commandPath}}`.\nBase domain: `{{installedBaseDomain}}`.".

operation diagnose:
  presents as:
    title is "Diagnose Despatch".
    summary is "Run Launchpad-native service, routing, TLS, firewall, and DNS checks against the current Despatch installation.".

  stage "Inspect":
    check that commandPath exists.
    show panel "Deployment" with "Mode: `{{installedDeployMode}}`\nListen: `{{installedListenAddr}}`\nDomain: `{{dnsHostName}}`.".
    check that detectedServiceState is "active".
    check that detectedUpdaterState is "active".
    check that detectedMailsecState is "active".
    when installedDeployMode is "proxy" and installedProxyServer is "nginx":
      check that state of system service "nginx" is "active".
    when installedDeployMode is "proxy" and installedProxyServer is "apache2":
      check that state of system service "apache2" is "active".
    when installedCapEnabledFlag:
      check that detectedCapServiceState is "active".
    check that value for key "ready" in readinessReport is true.
    check that value for key "ok" in localHttpDiagnostics is true.
    check that value for key "ok" in appListenDiagnostics is true.
    when installedDeployMode is "proxy":
      check that value for key "ok" in proxyHttpDiagnostics is true.
      check that value for key "ok" in proxyRoutingDiagnostics is true.
      check that value for key "ok" in proxyConfigDiagnostics is true.
      check that value for key "ok" in proxyListenDiagnostics is true.
    when installedDeployMode is "proxy" and installedProxyTlsEnabled:
      check that value for key "ok" in tlsEndpointDiagnostics is true.
    when installedUfwEnabledFlag and installedDeployMode is "proxy" and installedUfwOpenProxyPortsFlag:
      check that value for key "ok" in proxyHttpFirewallDiagnostics is true.
      check that value for key "ok" in proxyHttpsFirewallDiagnostics is true.
    when installedUfwEnabledFlag and installedDeployMode is "direct" and installedUfwOpenDirectPortFlag:
      check that value for key "ok" in directFirewallDiagnostics is true.
    when trimmed dnsHostName != "":
      check that value for key "ok" in dnsDiagnostics is true.
    show panel "Health" with "Local health endpoint: `{{localHealthUrl}}`\nReadiness state: `{{value for key \"state\" in readinessReport}}`.\nCombined diagnostics ok: `{{value for key \"ok\" in combinedHealthDiagnostics}}`.".
    show panel "Next Step" with "If diagnostics fail, check `{{installMetadataPath}}`, reverse-proxy configuration, and systemd service status for the affected unit.".

operation uninstall:
  presents as:
    title is "Uninstall Despatch".
    shortTitle is "Uninstall".
    summary is "Remove the Despatch install and its Launchpad-managed lifecycle while keeping backups when requested.".
    accent is "Remove".
    risk is "HIGH".

  preflight uninstallNoSudo:
    when system is not root and host does not have sudo.
    code is "E_NO_SUDO".
    message is "Despatch uninstall needs administrator access.".
    suggested fix is "Run the installer as root or install `sudo` first.".

  preflight uninstallSudoReady:
    when system is not root and host has sudo and sudo is not ready.
    code is "E_SUDO".
    message is "Launchpad needs a ready sudo session before uninstalling Despatch.".
    suggested fix is "Run `sudo -v` in this shell and try again.".

  step setup is a form:
    title is "Removal Options".
    summary is "Choose how much state to preserve while Despatch is removed.".
    basic fields are:
      backupEnv
      backupData
      removeAppFiles
      removeAppData
    advanced fields are:
      removeSystemUser
      removeNginxSite
      removeApacheSite
      removeCheckout
    warning "Removing app data deletes the working database and update state under `/var/lib/despatch`." when removeAppData.

  step review is a review:
    title is "Ready To Uninstall".
    summary is "Confirm the removal scope before Launchpad tears Despatch down.".
    groups are:
      group "Backups":
        shows "Backup .env" as backupEnv
        shows "Backup data" as backupData
      group "Removal":
        shows "Remove app files" as removeAppFiles
        shows "Remove app data" as removeAppData
        shows "Remove system user" as removeSystemUser
        shows "Remove Nginx site" as removeNginxSite
        shows "Remove Apache site" as removeApacheSite
        shows "Remove checkout" as removeCheckout

  stage "Backup":
    ensure directory backupRoot.
    when backupEnv:
      rotate backup of envPath to backupRoot / "despatch.env" keeping source when true overwriting existing backup when true.
    when backupData:
      rotate backup of dataRoot to backupRoot / "despatch-data" keeping source when true overwriting existing backup when true.

  stage "Remove":
    remove container service despatchCapService.
    remove service unit despatchPamResetHelperSocket.
    remove service unit despatchPamResetHelperService.
    remove service unit despatchMailsecService.
    remove service unit despatchUpdaterPath.
    remove service unit despatchUpdaterService.
    remove service unit despatchService.
    remove service unit legacyMailclientService.
    remove service unit legacyMailclientUpdaterService.
    remove service unit legacyMailclientUpdaterPath.
    remove service unit legacyMailclientMailsecService.
    remove service unit legacyMailclientPamResetHelperService.
    remove service unit legacyMailclientPamResetHelperSocket.
    when removeNginxSite:
      remove nginx site despatchNginx.
    when removeApacheSite:
      remove apache site despatchApache.
    when ufwEnable:
      remove firewall rule proxyHttpIngress.
      remove firewall rule proxyHttpsIngress.
      remove firewall rule directIngress.
    when removeAppFiles:
      delete installRoot.
    when removeAppData:
      delete dataRoot.
    when removeCheckout:
      delete checkoutRoot.
    when removeSystemUser:
      remove service account "despatch".

  stage "Finish":
    show panel "Applications Centre" with "Despatch uninstall has completed. Launchpad will update the registered application state after this run ends.".
