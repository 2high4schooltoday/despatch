package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"despatch/internal/api"
	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/notify"
	"despatch/internal/pamreset"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/update"
	"despatch/internal/workers"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update-worker" {
		if err := update.RunWorkerFromEnv(context.Background()); err != nil {
			log.Fatalf("update worker: %v", err)
		}
		return
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "pam-reset-helper" {
		helperCfg := pamreset.HelperConfig{
			SocketPath:      cfg.PAMResetHelperSocket,
			SocketGroupID:   cfg.PAMResetAllowedGID,
			AllowedUID:      cfg.PAMResetAllowedUID,
			AllowedGID:      cfg.PAMResetAllowedGID,
			IOTimeout:       time.Duration(cfg.PAMResetHelperTimeoutSec) * time.Second,
			CommandTimeout:  time.Duration(cfg.PAMResetHelperTimeoutSec) * time.Second,
			SocketFilePerms: 0660,
		}
		log.Printf("starting pam reset helper on %s", cfg.PAMResetHelperSocket)
		if err := pamreset.RunServer(context.Background(), helperCfg); err != nil {
			log.Fatalf("pam reset helper: %v", err)
		}
		return
	}
	if cfg.CookiePolicyWarning != "" {
		log.Printf("config_warning: %s", cfg.CookiePolicyWarning)
	}
	if cfg.MailTLSWarning != "" {
		log.Printf("config_warning: %s", cfg.MailTLSWarning)
	}
	sqdb, err := db.OpenSQLite(cfg.DBPath, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer sqdb.Close()
	for _, migration := range []string{
		"migrations/001_init.sql",
		"migrations/002_users_mail_login.sql",
		"migrations/003_cleanup_rejected_users.sql",
		"migrations/004_cleanup_rejected_users_casefold.sql",
		"migrations/005_admin_query_indexes.sql",
		"migrations/006_users_recovery_email.sql",
		"migrations/007_mail_accounts.sql",
		"migrations/008_mail_index.sql",
		"migrations/009_preferences_and_search.sql",
		"migrations/010_drafts_schedule.sql",
		"migrations/011_rules_sieve.sql",
		"migrations/012_mfa_totp_webauthn.sql",
		"migrations/013_crypto_keys.sql",
		"migrations/014_session_management.sql",
		"migrations/015_sync_state.sql",
		"migrations/016_quota_and_health.sql",
		"migrations/017_mfa_onboarding_flags.sql",
		"migrations/018_mfa_usability_trusted_devices.sql",
		"migrations/019_users_mail_secret.sql",
		"migrations/020_mail_index_scoped_ids.sql",
		"migrations/021_password_reset_token_reservations.sql",
		"migrations/022_draft_compose_context.sql",
		"migrations/023_drafts_nullable_account.sql",
	} {
		if err := db.ApplyMigrationFile(sqdb, migration); err != nil {
			log.Fatalf("migration %s: %v", migration, err)
		}
	}

	st := store.New(sqdb)
	if err := st.EnsureScopedIndexedIDs(context.Background()); err != nil {
		log.Fatalf("scoped indexed id migration: %v", err)
	}
	if cfg.BootstrapAdminEmail != "" && cfg.BootstrapAdminPassword != "" {
		hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
		if err != nil {
			log.Fatalf("bootstrap admin hash: %v", err)
		}
		if err := st.EnsureAdmin(context.Background(), cfg.BootstrapAdminEmail, hash); err != nil {
			log.Fatalf("bootstrap admin create: %v", err)
		}
	}

	despatch := mail.NewIMAPSMTPClient(cfg)
	provisioner, err := mail.NewProvisioner(cfg)
	if err != nil {
		log.Fatalf("provisioner: %v", err)
	}
	sender := notify.NewSender(cfg)

	svc := service.New(cfg, st, despatch, provisioner, sender)
	if senderState, err := svc.EnsurePasswordResetSenderIdentity(context.Background()); err != nil {
		log.Printf("password_reset_sender_init_failed address=%s err=%v", senderState.Address, err)
	} else {
		log.Printf("password_reset_sender_status status=%s reason=%s address=%s", senderState.Status, senderState.Reason, senderState.Address)
	}
	workers.StartMailWorkers(context.Background(), cfg, st)
	workers.StartUpdateWorkers(context.Background(), cfg, st)
	r := api.NewRouter(cfg, svc)

	hsrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadTimeout:       time.Duration(cfg.HTTPReadTimeoutSec) * time.Second,
		ReadHeaderTimeout: time.Duration(cfg.HTTPReadHeaderTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTPWriteTimeoutSec) * time.Second,
		IdleTimeout:       time.Duration(cfg.HTTPIdleTimeoutSec) * time.Second,
	}

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := hsrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}
