# Despatch Architecture Proposal: Instance Modes and External Accounts

## Status

Draft proposal for review.

## Summary

Despatch should evolve from a local-mail-stack control plane into a capability-driven mail workspace with three supported instance modes:

1. `local_stack`
   Despatch manages a local Postfix + Dovecot + Maildir installation, which is the current product shape.
2. `external_accounts`
   Despatch runs as an admin and workflow layer only. There is no attached local mail server. Users sign in to Despatch, then connect third-party mail accounts such as Gmail, Libero, or generic IMAP/SMTP accounts.
3. `hybrid`
   Despatch supports both local mailboxes and external accounts in the same instance.

The key architectural shift is to separate:

- app identity: the Despatch user/admin account used to sign in and manage the instance
- mailbox identity: a send/receive-capable mail account
- sender identity: the visible From/Reply-To/signature profile used when composing mail

Today those concepts are partially coupled. External mode requires them to become first-class and independent.

## Goals

- Preserve the current local-stack experience with minimal regression.
- Allow first-run setup without requiring a sendable local mailbox.
- Make external account connection a normal first-class workflow.
- Keep one mail UI and one account model instead of shipping separate apps.
- Create a clean foundation for future funnel/reply-farm workflows across many external accounts.

## Non-Goals

- Replacing provider-specific APIs with a full hosted mail backend.
- Supporting every provider-specific feature in v1 of external mode.
- Introducing team-shared mail account ownership in the first phase.

## Current Constraints

The current codebase is explicitly local-stack oriented:

- product framing in `README.md`
- setup returns `auth_mode` based on Dovecot mode in `internal/service/service.go`
- setup completion assumes local-domain admin semantics in `internal/service/service.go`
- the current sender bootstrap expects an authenticated mail identity in `internal/service/mail_sender.go`
- mail account CRUD and health routes assume a uniform IMAP/SMTP account model in `internal/api/router.go`

Those seams should be changed first, before provider adapters or funnels.

## Product Model

### Instance capability model

Do not fork the product into separate frontends. Instead, add instance-level capabilities and let the UI hide or reveal flows.

Proposed settings:

- `instance.mode = local_stack | external_accounts | hybrid`
- `instance.cap.local_mail = 0|1`
- `instance.cap.external_mail = 0|1`
- `instance.cap.server_admin = 0|1`

Rules:

- `local_stack`: local mail and server admin enabled, external mail optional later
- `external_accounts`: external mail enabled, local mail and server admin disabled
- `hybrid`: all enabled

Why settings instead of hard config only:

- the choice belongs to OOBE and should persist in SQLite alongside other setup decisions
- the web UI already depends on stored setup state and feature flags
- later upgrades may allow changing `local_stack` to `hybrid` without redeploying

### Account model

All connected mailboxes should still appear under one `Accounts` surface in Settings and one unified mail workspace.

Each account should advertise capabilities instead of assuming all accounts support the same actions:

- `receive_mail`
- `send_mail`
- `server_rules`
- `remote_folders`
- `oauth`
- `password_auth`
- `managed_special_mailboxes`
- `provider_labels`
- `provider_threads`

This keeps the UI simple:

- Gmail account: labels, OAuth, maybe no Sieve
- generic IMAP/SMTP: folders, password auth, maybe server rules if ManageSieve is configured
- local account: all current features

## Data Model

### 1. Instance mode settings

Use the existing settings store for v1.

New settings keys:

- `instance.mode`
- `instance.cap.local_mail`
- `instance.cap.external_mail`
- `instance.cap.server_admin`
- `instance.base_domain_required`

`instance.base_domain_required` allows setup validation to be conditional:

- required for `local_stack` and `hybrid`
- optional for `external_accounts`

### 2. Extend `mail_accounts`

Current model in `internal/models/v2.go` is good for generic IMAP/SMTP, but it lacks provider and capability metadata.

Add columns to `mail_accounts`:

- `source_kind TEXT NOT NULL DEFAULT 'local'`
  Values: `local`, `external`
- `provider TEXT NOT NULL DEFAULT 'generic_imap'`
  Values initially: `local_dovecot`, `generic_imap`, `gmail`, `libero`
- `auth_kind TEXT NOT NULL DEFAULT 'password'`
  Values: `password`, `oauth2`, `app_password`, `provider_session`
- `connection_mode TEXT NOT NULL DEFAULT 'imap_smtp'`
  Values: `imap_smtp`, `imap_only`, `smtp_only`, `provider_api`
- `capabilities_json TEXT NOT NULL DEFAULT '{}'`
- `status_reason TEXT NOT NULL DEFAULT ''`
- `sync_priority INTEGER NOT NULL DEFAULT 0`
- `provider_account_ref TEXT NOT NULL DEFAULT ''`
- `owner_scope TEXT NOT NULL DEFAULT 'user'`

Notes:

- keep `user_id` in v1 so ownership stays per user
- keep existing IMAP/SMTP fields for generic and local accounts
- provider adapters can ignore unused host fields

### 3. Add `mail_account_credentials`

Do not overload `mail_accounts.secret_enc` with OAuth and multi-token state.

New table:

- `id`
- `account_id`
- `credential_kind`
- `secret_enc`
- `refresh_token_enc`
- `access_token_enc`
- `expires_at`
- `metadata_json`
- `created_at`
- `updated_at`

Why:

- password auth and OAuth need different lifecycle
- token refresh should be isolated from account profile changes
- provider adapters will need metadata such as scopes and token issuer

Phase 1 compatibility:

- keep `mail_accounts.secret_enc`
- mirror password credentials into `mail_account_credentials`
- migrate code paths gradually

### 4. Add `mail_providers`

This can be a table or a static API-backed catalog. For v1, an API-backed static catalog is enough.

Provider metadata should include:

- `id`
- `name`
- `auth_kinds`
- `supports_oauth`
- `supports_rules`
- `supports_special_mailboxes`
- `setup_fields`
- `status`

Recommendation:

- implement as code first
- add a table only if custom provider registry becomes necessary

### 5. Decouple session sender bootstrap

Current sender bootstrap assumes there is always a primary sendable identity in `internal/service/mail_sender.go`.

Change `session_mail_profiles` semantics:

- `display_name` remains app/user facing
- `from_email` becomes nullable in the storage layer
- a session profile without `from_email` is valid in external mode before any mailbox is connected

Result:

- app users can exist without a mailbox
- compose can show `Connect an account to send mail` instead of erroring

### 6. Future funnel tables

These are not required for external mode MVP, but the schema should leave room for them.

Planned tables:

- `reply_funnels`
- `reply_funnel_accounts`
- `reply_funnel_sender_overrides`
- `mail_thread_bindings`

`mail_thread_bindings` should track:

- `thread_id`
- `account_id`
- `binding_type`
- `reply_account_id`
- `reply_sender_profile_id`
- `funnel_id`

That avoids rebuilding reply routing logic from scattered heuristics later.

## OOBE Flow

### Current flow

The current setup flow is wired around the existing OOBE in `web/index.html` and `web/app.js`.

Today it assumes:

- a base domain
- a local-server-oriented admin mailbox
- optional PAM mailbox login override

### Proposed new flow

Expand OOBE by one step and insert an explicit mail-source decision early.

Proposed steps:

0. Welcome
1. Mail Source
2. Region
3. Theme
4. Software Updates
5. Admin Account
6. Mail Bootstrap
7. Security
8. Review
9. Finished

### Step details

#### Step 1: Mail Source

New question:

`How will this Despatch instance get mail?`

Choices:

- `Local server mail`
- `External mail accounts`
- `Both`

Selection writes:

- `instance.mode`
- capability settings listed above

#### Step 5: Admin Account

Always create a Despatch admin user. This is no longer assumed to be a working mailbox.

Fields:

- admin email
- recovery email
- admin password

Validation rules:

- in `local_stack` and `hybrid`, admin email should still default to `webmaster@<base_domain>`
- in `external_accounts`, admin email only needs to be syntactically valid
- do not require that admin email be usable as a mailbox

#### Step 6: Mail Bootstrap

This step is conditional.

For `local_stack`:

- current domain and mailbox-login behavior stays
- `admin_mailbox_login` remains visible when PAM requires it

For `external_accounts`:

- replace local bootstrap copy with `Connect your first mailbox later`
- optionally offer immediate connection of:
  - Gmail
  - generic IMAP/SMTP

For `hybrid`:

- show both:
  - local mail setup summary
  - optional first external account connection shortcut

### Setup API changes

Change the payloads on:

- `GET /api/v1/setup/status`
- `POST /api/v1/setup/complete`

`SetupStatus` in `internal/service/service.go` should add:

- `instance_mode`
- `available_instance_modes`
- `base_domain_required`
- `local_mail_available`
- `external_mail_available`

`SetupCompleteRequest` in `internal/service/service.go` should add:

- `instance_mode`
- `connect_first_external_account`

Behavior changes in `CompleteSetup`:

- only enforce `@base_domain` admin mail for `local_stack` and `hybrid`
- only verify PAM login when local mail capability is enabled and auth mode is PAM
- persist the chosen instance mode before final session bootstrap

## Backend Changes

### Phase 1: Foundation

Make app users independent from mailboxes.

Files to change first:

- `internal/service/service.go`
- `internal/service/mail_sender.go`
- `internal/api/router.go`
- `internal/api/router_setup_test.go`
- `web/index.html`
- `web/app.js`

Concrete changes:

- add mode selection to setup payload and UI
- allow setup completion without a local sendable mailbox
- return an empty sender state instead of hard failure when no mail account exists
- make compose and sender-settings surfaces render a `needs_account` onboarding state

### Phase 2: Account capability model

Files:

- `internal/models/v2.go`
- `internal/store/v2_store.go`
- new migrations `031_instance_mode.sql`, `032_mail_account_capabilities.sql`, `033_mail_account_credentials.sql`

Concrete changes:

- extend `MailAccount`
- add credential storage
- add read/write support in store methods
- update tests that create or decode accounts

### Phase 3: Mail adapter layer

Today most mail operations ultimately assume one IMAP/SMTP client path.

Introduce an account-scoped adapter interface:

```go
type AccountAdapter interface {
    Capabilities(ctx context.Context, account models.MailAccount) (models.MailCapabilities, error)
    ListMailboxes(ctx context.Context, account models.MailAccount) ([]mail.Mailbox, error)
    Send(ctx context.Context, account models.MailAccount, req mail.SendRequest) error
    Sync(ctx context.Context, account models.MailAccount) error
    ValidateRuleSupport(ctx context.Context, account models.MailAccount) error
}
```

Implementations:

- `localIMAPSMTPAdapter`
- `genericIMAPSMTPAdapter`
- `gmailAdapter`
- `liberoAdapter` when feasible

Why this is early:

- workers, send, health, and mailbox APIs should dispatch by account, not by global install assumptions

### Phase 4: Provider connection flows

New API surface:

- `GET /api/v2/providers`
- `POST /api/v2/accounts/connect`
- `POST /api/v2/accounts/{id}/reauthorize`
- `POST /api/v2/accounts/{id}/disconnect`

For Gmail:

- add OAuth start/finish routes
- store tokens in `mail_account_credentials`

For generic IMAP/SMTP:

- reuse the current account create/update routes
- classify them as `source_kind=external`

## Existing Routes to Change First

### 1. Setup routes

Highest priority because the instance-mode choice must exist before the rest of the app can behave correctly.

- `GET /api/v1/setup/status`
- `POST /api/v1/setup/complete`

### 2. Account CRUD routes

Second priority because external accounts become first-class entities.

- `GET /api/v2/accounts`
- `POST /api/v2/accounts`
- `PATCH /api/v2/accounts/{id}`
- `DELETE /api/v2/accounts/{id}`

Required changes:

- include `source_kind`, `provider`, `auth_kind`, capability metadata
- make validation conditional by provider
- stop assuming every account has editable raw IMAP/SMTP settings

### 3. Account health and sync routes

Third priority because account behavior is no longer uniform.

- `GET /api/v2/accounts/health`
- `POST /api/v2/accounts/{id}/health/sync`
- `POST /api/v2/accounts/{id}/health/quota-refresh`
- `POST /api/v2/accounts/{id}/health/reindex`

Change:

- dispatch through account adapter instead of global config/client assumptions

### 4. Sender and compose routes

Fourth priority because external mode cannot require a local sender bootstrap.

- `GET /api/v1/compose/identities`
- draft send/reply endpoints in v1 and v2
- sender profile routes under `/api/v2/accounts/{id}/identities` and `/api/v2/accounts/{id}/senders`

Change:

- if no accounts exist, return empty sender catalog plus capability hints
- choose the sending account by selected sender or thread binding, never by app-admin identity

### 5. Rules routes

Fifth priority because they become capability-gated.

- `/api/v2/accounts/{id}/rules*`

Change:

- local and generic accounts with ManageSieve: current flow remains
- providers without server-rule support: return `unsupported_for_provider`
- later add provider-native rule adapters if justified

## Worker Changes

Current sync scheduling in `internal/workers/mail_workers.go` is account oriented and already a decent foundation.

Required changes:

- schedule by account capability and provider adapter
- move account mail config resolution behind an account adapter factory
- store provider-specific status and auth-expiry errors in `status_reason`
- add `sync_priority` so later funnel accounts can be polled more aggressively

## UI Changes

### Settings

Keep one `Mail` settings area, but split it into capability-aware cards:

- Accounts
- Senders
- Rules
- Provider Connections

If `instance.cap.local_mail=0`, hide or soften:

- local-stack-specific copy
- mailbox-provisioning assumptions
- server admin diagnostics

### Admin

In `external_accounts` mode, the `System` area should not foreground local mail health, reverse proxy, or Dovecot assumptions. Those become hidden or clearly marked unavailable.

### Compose

When there are zero connected mail accounts:

- `New Message` remains available if useful for draft onboarding, but send is disabled
- the compose surface should show `Connect a mail account to send mail`

## Migration Plan

### Step 1

Ship instance mode in setup and settings only, with no provider-specific connectors yet.

Result:

- external mode can be created
- admin users can sign in without a local mailbox

### Step 2

Ship external generic IMAP/SMTP accounts using the current account editor with new metadata.

Result:

- external mode becomes practically useful immediately

### Step 3

Ship Gmail OAuth connector and capability-aware account adapters.

### Step 4

Ship funnel and bulk-account workflows on top of the external-account base.

## First Implementation Slice

If this proposal is implemented incrementally, the first PR should do only this:

1. add `instance.mode` to setup status and setup completion
2. allow `external_accounts` setup without local mailbox assumptions
3. make sender bootstrap tolerate `no connected mail accounts`
4. update OOBE UI for the new mode-selection step
5. add tests covering all three modes

Why this slice first:

- it unlocks the product direction without requiring provider adapters yet
- it reduces coupling in the exact places that currently block the design
- it keeps the first migration and review surface small

## Open Questions

- Should external-mode admin email default to a neutral internal address, or just an empty field?
- Should user-owned accounts remain the only ownership model in v1, or should admin-connected shared accounts be allowed immediately?
- Do we want provider-specific rules at all, or is `rules` simply disabled unless the account supports ManageSieve?
- Should `hybrid` be shown in OOBE immediately, or enabled after `local_stack` and `external_accounts` prove stable?

## Recommendation

Ship all three modes in the architecture, but phase them in like this:

1. `local_stack` and `external_accounts` fully supported in setup
2. `hybrid` available in the schema and setup, but feature-gated if needed
3. external generic IMAP/SMTP before Gmail OAuth
4. funnels only after the external-account foundation is stable

That keeps the product coherent, matches the current codebase shape, and creates the right base for the high-volume external-account workflows users are asking for.
