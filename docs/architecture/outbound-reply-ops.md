# Despatch Architecture Proposal: Outbound Campaigns and Reply Ops

## Status

Draft proposal for implementation.

## Summary

Despatch should evolve from "a mail UI that can also send" into a reply-native outbound operations workspace.

The product goal is not to compete with CRM suites or generic newsletter tools. The goal is to make outbound email campaigns, follow-ups, and reply handling feel like one continuous operational surface:

- send many personalized emails safely
- stop or branch the right people automatically
- surface the important replies immediately
- preserve thread context and sender context
- protect domain reputation while operators work fast

Despatch already has several of the right primitives:

- external-account and hybrid account support in [instance-modes-and-external-accounts.md](/Users/quentinlaurier/Documents/despatch/docs/architecture/instance-modes-and-external-accounts.md)
- reply funnels in [migrations/031_reply_funnels.sql](/Users/quentinlaurier/Documents/despatch/migrations/031_reply_funnels.sql)
- contacts and groups in [migrations/028_contacts.sql](/Users/quentinlaurier/Documents/despatch/migrations/028_contacts.sql)
- account-aware sender selection in [internal/service/mail_sender.go](/Users/quentinlaurier/Documents/despatch/internal/service/mail_sender.go)
- conversation triage in [migrations/030_mail_triage.sql](/Users/quentinlaurier/Documents/despatch/migrations/030_mail_triage.sql)
- saved searches, snippets, favorites, scheduled send, and reply-thread-safe mail flows across the existing mail workspace

What is missing is the campaign-state layer above those primitives.

This document defines that layer.

## Product Thesis

Despatch should treat outbound as an operational system of record for conversations, not as a compose shortcut.

That means:

- the first-class object is not just a sent message
- the first-class object is "recipient progress through a thread-aware campaign"
- replies are not passive inbox events
- replies are state transitions with explicit outcomes and actions

In practical terms, Despatch should become:

- the place where campaigns are launched
- the place where recipient state is governed
- the place where replies are triaged
- the place where human takeover happens
- the place where sending safety is enforced

## Goals

- Make bulk personalized sending feel native, not bolted onto compose.
- Unify outbound sending and inbound reply handling into one model.
- Support thread-safe follow-ups, sender rotation, and multi-account workflows.
- Prevent duplicate contact pressure across campaigns, senders, and domains.
- Give operators a dedicated reply-operations queue instead of burying replies in a generic inbox.
- Add deliverability and compliance controls before high-volume sending grows dangerous.
- Reuse existing Despatch abstractions wherever possible instead of forking a second product inside the app.

## Non-Goals

- Building a full CRM with deals, forecasts, forms, or sales pipeline boards.
- Building a marketing automation suite with landing pages and audience acquisition.
- Replacing provider-native deliverability infrastructure with a hosted ESP.
- Shipping full autonomous AI reply behavior in phase 1.
- Requiring open tracking as the primary engagement signal.

## Product Principles

### 1. Reply-first, not send-first

Sending is only half the workflow. The product should optimize for what happens after the send.

### 2. One recipient, one effective state

If the same person appears in multiple campaigns, Despatch must be able to decide whether their status is local to one campaign or global across the workspace.

### 3. Thread context is sacred

Follow-ups should stay attached to the right thread unless an operator explicitly chooses to break thread continuity.

### 4. Automation must remain inspectable

Every pause, stop, branch, or suppression action should be explainable from event history.

### 5. Deliverability is part of product behavior

Domain safety, unsubscribe behavior, sender pacing, and bounce handling must be first-class system concerns.

### 6. Human takeover must be fast

When automation stops, the operator should be one click away from the full thread, sender context, history, and next action.

## Target Workflows

### Workflow A: Standard multi-step outbound

An operator selects a contact group or saved search, writes step 1 and step 2, sets stop conditions, reviews safety checks, and launches.

### Workflow B: Multi-account reply funnel outreach

An operator sends from many source accounts into a collector or unified reply flow, while preserving which sender and account own each live thread.

### Workflow C: Reply review and handoff

A recipient replies. Despatch classifies the outcome, pauses the right automation, puts the conversation in the right queue, and shows the operator the exact next action.

### Workflow D: Out-of-office handling

A recipient sends an out-of-office reply. Despatch pauses that recipient, records the return date if known, and resumes on the correct date if configured.

### Workflow E: Company/domain suppression

One person at a target domain replies positively, negatively, or asks not to be contacted. Despatch applies the chosen domain-level rule immediately.

## Current Building Blocks to Reuse

### Contacts and groups

The existing contacts model already supports:

- multiple emails per contact
- groups
- preferred account and sender hints

This is enough to support campaign audience sources without inventing a parallel lead table in phase 1.

Relevant files:

- [migrations/028_contacts.sql](/Users/quentinlaurier/Documents/despatch/migrations/028_contacts.sql)
- [internal/api/router_contacts.go](/Users/quentinlaurier/Documents/despatch/internal/api/router_contacts.go)
- [internal/store/v2_store.go](/Users/quentinlaurier/Documents/despatch/internal/store/v2_store.go:2401)

### Reply funnels

Reply funnels already establish:

- collector account
- source accounts
- routing mode
- reply mode

They should become optional campaign infrastructure rather than a separate product island.

Relevant files:

- [migrations/031_reply_funnels.sql](/Users/quentinlaurier/Documents/despatch/migrations/031_reply_funnels.sql)
- [internal/api/router_mail_funnels.go](/Users/quentinlaurier/Documents/despatch/internal/api/router_mail_funnels.go)

### Mail triage

Mail triage already proves that Despatch can store per-thread workflow state with:

- snooze
- follow-up reminder
- category
- tags

Reply Ops should reuse the thread-oriented workflow style, but not overload general mail triage with campaign semantics.

Relevant files:

- [migrations/030_mail_triage.sql](/Users/quentinlaurier/Documents/despatch/migrations/030_mail_triage.sql)
- [internal/api/router_mail_triage.go](/Users/quentinlaurier/Documents/despatch/internal/api/router_mail_triage.go)

### Sender profiles and accounts

The sender resolution model is already account-aware and can schedule only from viable accounts.

That should become the basis for campaign sender policy.

Relevant files:

- [internal/service/mail_sender.go](/Users/quentinlaurier/Documents/despatch/internal/service/mail_sender.go)
- [internal/api/router.go](/Users/quentinlaurier/Documents/despatch/internal/api/router.go:288)

## Core Product Model

Despatch needs six new conceptual objects:

1. `outbound_campaign`
2. `outbound_campaign_step`
3. `outbound_enrollment`
4. `outbound_event`
5. `recipient_state`
6. `mail_thread_binding`

### 1. `outbound_campaign`

Represents one operational outbound motion.

Fields:

- `id`
- `user_id`
- `name`
- `status`
  Values: `draft`, `running`, `paused`, `completed`, `archived`
- `audience_source_kind`
  Values: `contact_group`, `saved_search`, `csv_import`, `manual`
- `audience_source_ref`
- `sender_policy_kind`
  Values: `single_sender`, `preferred_sender`, `campaign_pool`, `reply_funnel`
- `sender_policy_ref`
- `reply_policy_json`
- `suppression_policy_json`
- `schedule_policy_json`
- `compliance_policy_json`
- `created_at`
- `updated_at`
- `launched_at`
- `completed_at`

### 2. `outbound_campaign_step`

Represents one timed step in a campaign.

Fields:

- `id`
- `campaign_id`
- `position`
- `kind`
  Values: `email`
- `thread_mode`
  Values: `same_thread`, `new_thread`
- `subject_template`
- `body_template`
- `wait_interval_minutes`
- `send_window_json`
- `stop_if_replied`
- `stop_if_clicked`
- `stop_if_booked`
- `stop_if_unsubscribed`
- `created_at`
- `updated_at`

Note:

- phase 1 should support only email steps
- task/call/linkedin steps can come later if ever needed

### 3. `outbound_enrollment`

Represents one recipient's progress inside one campaign.

This is the main operational object.

Fields:

- `id`
- `campaign_id`
- `contact_id`
- `recipient_email`
- `recipient_domain`
- `sender_account_id`
- `sender_profile_id`
- `reply_funnel_id`
- `thread_binding_id`
- `status`
  Values:
  - `pending`
  - `scheduled`
  - `sending`
  - `waiting_reply`
  - `paused`
  - `stopped`
  - `completed`
  - `bounced`
  - `unsubscribed`
  - `manual_only`
- `current_step_position`
- `last_sent_message_id`
- `last_sent_at`
- `next_action_at`
- `pause_reason`
- `stop_reason`
- `reply_outcome`
- `reply_confidence`
- `manual_owner_user_id`
- `created_at`
- `updated_at`

### 4. `outbound_event`

Append-only log for every meaningful state transition.

Fields:

- `id`
- `campaign_id`
- `enrollment_id`
- `event_kind`
- `event_payload_json`
- `actor_kind`
  Values: `system`, `user`, `worker`, `classifier`
- `actor_ref`
- `created_at`

Examples:

- `enrolled`
- `preflight_blocked`
- `step_scheduled`
- `step_sent`
- `reply_detected`
- `reply_classified`
- `paused_ooo`
- `stopped_positive_reply`
- `manual_takeover`
- `resume_requested`
- `unsubscribe_applied`
- `domain_suppressed`

### 5. `recipient_state`

Global or workspace-wide state for a recipient, independent of one campaign.

This is how Despatch avoids becoming campaign-fragmented.

Fields:

- `recipient_email` primary key
- `primary_contact_id`
- `recipient_domain`
- `status`
  Values:
  - `active`
  - `replied`
  - `interested`
  - `not_interested`
  - `wrong_person`
  - `meeting_booked`
  - `unsubscribed`
  - `hard_bounce`
  - `suppressed`
- `scope`
  Values: `workspace`, `campaign_only`
- `last_reply_at`
- `last_reply_outcome`
- `suppressed_until`
- `suppression_reason`
- `notes`
- `updated_at`

This table should be small, queryable, and fast to check during enrollment preflight.

### 6. `mail_thread_binding`

This should implement the planned binding concept from [instance-modes-and-external-accounts.md](/Users/quentinlaurier/Documents/despatch/docs/architecture/instance-modes-and-external-accounts.md:219).

Fields:

- `id`
- `account_id`
- `thread_id`
- `binding_type`
  Values: `campaign`, `reply_funnel`, `manual`
- `campaign_id`
- `enrollment_id`
- `funnel_id`
- `reply_account_id`
- `reply_sender_profile_id`
- `collector_account_id`
- `owner_user_id`
- `created_at`
- `updated_at`

This is the key to safe reply continuation and sender continuity.

## Reply Outcome Taxonomy

Despatch should normalize raw replies into explicit outcomes:

- `positive_interest`
- `meeting_intent`
- `question`
- `objection`
- `referral`
- `wrong_person`
- `not_interested`
- `unsubscribe_request`
- `out_of_office`
- `bounce`
- `auto_reply_other`
- `hostile`
- `manual_review_required`

Each outcome must map to a default action policy.

Example default mapping:

- `positive_interest` -> stop enrollment, route to Reply Ops `Interested`
- `meeting_intent` -> stop enrollment, route to `Meeting intent`
- `question` -> pause enrollment, assign human
- `objection` -> pause enrollment, assign human
- `referral` -> stop current recipient, suggest new contact creation
- `wrong_person` -> stop current recipient, optionally branch to referred contact
- `not_interested` -> stop recipient, optionally suppress domain peers
- `unsubscribe_request` -> unsubscribe recipient and suppress future enrollment
- `out_of_office` -> pause until return date if known, else route to `OOO review`
- `bounce` -> mark bounced and remove from future sends
- `hostile` -> stop and suppress
- `manual_review_required` -> queue for human

## Enrollment State Machine

High-level state flow:

1. `pending`
2. `scheduled`
3. `sending`
4. `waiting_reply`
5. one of:
   - `scheduled` for next step
   - `paused`
   - `stopped`
   - `completed`
   - `manual_only`
   - `bounced`
   - `unsubscribed`

Rules:

- every enrollment can have at most one next executable step
- replies always win over future scheduled steps
- manual takeover blocks automation until explicitly resumed
- unsubscribe and hard bounce are terminal
- `completed` means no further steps remain and no reply-based terminal action happened

## Company and Domain Logic

Despatch should support three scopes of suppression and stop logic:

1. recipient-level
2. domain-level
3. campaign-level

Domain logic is critical because the market already treats this as a high-value safety behavior.

Policy examples:

- if any person at domain replies positively, stop all pending enrollments at that domain in this campaign
- if any person at domain asks not to be contacted, suppress domain for this campaign or workspace
- if one domain is already active in another high-touch campaign, block new enrollment unless overridden

This can start with domain-only grouping and add explicit company identity later.

## Deliverability and Compliance Model

Despatch should add a first-class preflight before campaign launch.

### Preflight checks

- recipient duplicates in this campaign
- recipient already active in another campaign
- recipient or domain suppressed
- missing variables for templates
- sender account inactive
- sender/account daily cap would be exceeded
- campaign lacks unsubscribe mode when policy requires it
- sender domain auth is degraded
- high recent bounce rate on chosen sender/account/domain

### Sender policy

Each campaign should have explicit sender rules:

- fixed sender
- preferred sender from contact
- rotating sender pool
- reply-funnel sender mapping

Each sender pool should support:

- max sends per day
- max sends per hour
- max sends per domain per day
- cooldown after bounce spike
- cooldown after complaint or hostile cluster

### Compliance policy

Each campaign should specify:

- unsubscribe required: yes/no
- unsubscribe scope: recipient/campaign/workspace
- tracking mode: none/click/open
- promotional classification: yes/no
- body footer mode

This matters because major providers already treat one-click unsubscribe and bulk sender behavior as hard operational requirements.

## UI Proposal

Despatch should add one new top-level workspace and one new operational inbox.

### New top-level areas

- `Outbound`
- `Reply Ops`

`Mail` remains the general inbox workspace.

`Reply Ops` is not a duplicate inbox. It is an action queue for campaign-related replies.

### Outbound Home

Sections:

- Draft campaigns
- Running campaigns
- Paused campaigns
- Needs attention
- Sender health snapshot

Key per-campaign metrics:

- enrolled
- sent
- replied
- positive
- negative
- paused
- bounced
- unsubscribed
- waiting on human

### New Campaign Flow

Step 1: Choose audience

- contact group
- saved search
- CSV upload
- manual paste

Step 2: Choose sending policy

- single sender
- sender pool
- reply funnel

Step 3: Build sequence

- step editor
- wait times
- same-thread vs new-thread
- business window

Step 4: Configure reply behavior

- stop on reply
- pause on reply
- stop on same-domain reply
- OOO behavior
- unsubscribe behavior

Step 5: Review and launch

- preflight results
- recipient conflicts
- deliverability warnings
- policy summary

### Campaign Detail

Tabs:

- Overview
- Sequence
- Recipients
- Replies
- Events
- Settings

The `Recipients` tab should be the operational heart.

Each row should show:

- recipient
- domain
- sender used
- current step
- last send
- next action
- state
- reply outcome if any
- one-click actions: pause, resume, stop, assign, open thread

### Reply Ops

Queues:

- Needs review
- Interested
- Questions
- Objections
- Wrong person
- Out of office
- Bounces
- Unsubscribed
- Hostile

Every queue row should open a side sheet with:

- message thread
- campaign name
- step that triggered reply
- sender/account used
- classification and confidence
- recommended action
- quick actions

Quick actions:

- reply manually
- stop recipient
- pause until date
- resume later
- suppress recipient
- suppress domain
- convert referral into new contact

### Contact and Domain Sheet Enhancements

Existing Contacts should gain outbound-specific facts:

- last campaign touched
- last reply outcome
- active enrollments count
- preferred sender success rate
- suppression status

Domain sheet should show:

- active recipients
- paused recipients
- recent outcomes
- suppression policy

## API Plan

Add a new route family under `/api/v2/outbound`.

### Campaign management

- `GET /api/v2/outbound/campaigns`
- `POST /api/v2/outbound/campaigns`
- `GET /api/v2/outbound/campaigns/{id}`
- `PATCH /api/v2/outbound/campaigns/{id}`
- `POST /api/v2/outbound/campaigns/{id}/launch`
- `POST /api/v2/outbound/campaigns/{id}/pause`
- `POST /api/v2/outbound/campaigns/{id}/resume`
- `POST /api/v2/outbound/campaigns/{id}/archive`

### Campaign steps

- `GET /api/v2/outbound/campaigns/{id}/steps`
- `POST /api/v2/outbound/campaigns/{id}/steps`
- `PATCH /api/v2/outbound/steps/{step_id}`
- `DELETE /api/v2/outbound/steps/{step_id}`
- `POST /api/v2/outbound/campaigns/{id}/steps/reorder`

### Enrollment and audience

- `POST /api/v2/outbound/campaigns/{id}/audience/preview`
- `POST /api/v2/outbound/campaigns/{id}/enrollments/import`
- `GET /api/v2/outbound/campaigns/{id}/enrollments`
- `GET /api/v2/outbound/enrollments/{id}`
- `PATCH /api/v2/outbound/enrollments/{id}`
- `POST /api/v2/outbound/enrollments/{id}/pause`
- `POST /api/v2/outbound/enrollments/{id}/resume`
- `POST /api/v2/outbound/enrollments/{id}/stop`
- `POST /api/v2/outbound/enrollments/{id}/assign`

### Reply Ops

- `GET /api/v2/reply-ops/queue`
- `GET /api/v2/reply-ops/queue/{bucket}`
- `GET /api/v2/reply-ops/items/{id}`
- `POST /api/v2/reply-ops/items/{id}/classify`
- `POST /api/v2/reply-ops/items/{id}/takeover`
- `POST /api/v2/reply-ops/items/{id}/apply-action`

### Recipient and suppression state

- `GET /api/v2/outbound/recipients`
- `GET /api/v2/outbound/recipients/{email}`
- `PATCH /api/v2/outbound/recipients/{email}`
- `GET /api/v2/outbound/suppressions`
- `POST /api/v2/outbound/suppressions`
- `DELETE /api/v2/outbound/suppressions/{id}`

### Preflight and diagnostics

- `POST /api/v2/outbound/campaigns/{id}/preflight`
- `GET /api/v2/outbound/diagnostics/senders`
- `GET /api/v2/outbound/diagnostics/domains`

## Worker Plan

Despatch will need background workers for:

- due-step dispatch
- reply detection and classification
- OOO auto-resume
- suppression propagation
- campaign metrics rollups

### Send dispatcher worker

Responsibilities:

- find due enrollments
- re-check sender availability
- re-check suppression status
- send the correct step
- create outbound event
- update thread binding
- schedule next action

### Reply classifier worker

Responsibilities:

- watch new messages in bound threads
- map message to `mail_thread_binding`
- emit `reply_detected`
- classify outcome
- apply default policy
- enqueue human review when needed

### Resume worker

Responsibilities:

- wake paused OOO enrollments on due date
- resume only if no newer suppressing event exists

## Schema Plan

### New migrations

Recommended migration order:

1. `035_outbound_campaigns.sql`
2. `036_outbound_campaign_steps.sql`
3. `037_outbound_enrollments.sql`
4. `038_outbound_events.sql`
5. `039_recipient_state.sql`
6. `040_mail_thread_bindings.sql`
7. `041_outbound_suppressions.sql`

### 035_outbound_campaigns.sql

Create `outbound_campaigns`.

### 036_outbound_campaign_steps.sql

Create `outbound_campaign_steps`.

### 037_outbound_enrollments.sql

Create `outbound_enrollments` with indexes on:

- `(campaign_id, status, next_action_at)`
- `(recipient_email, status)`
- `(recipient_domain, status)`
- `(manual_owner_user_id, status)`

### 038_outbound_events.sql

Create append-only `outbound_events` with indexes on:

- `(campaign_id, created_at DESC)`
- `(enrollment_id, created_at DESC)`

### 039_recipient_state.sql

Create global `recipient_state`.

### 040_mail_thread_bindings.sql

Create `mail_thread_bindings` according to the planned shape in the external-account architecture doc.

### 041_outbound_suppressions.sql

Create `outbound_suppressions`.

Fields:

- `id`
- `scope_kind`
  Values: `recipient`, `domain`
- `scope_value`
- `campaign_id`
- `reason`
- `source_kind`
  Values: `manual`, `unsubscribe`, `bounce`, `reply_policy`, `compliance`
- `expires_at`
- `created_at`
- `updated_at`

## Migration Strategy

Do not migrate existing mail triage into campaign state.

Instead:

- keep mail triage as generic inbox workflow
- create separate campaign and reply-ops state
- allow a reply-ops action to optionally stamp ordinary mail triage tags or reminders for operator convenience

For reply funnels:

- keep existing funnel records valid
- allow campaigns to reference a funnel
- gradually move live outbound reply routing to `mail_thread_bindings`

## Implementation Phases

### Phase 1: Campaign state and Reply Ops foundation

Ship:

- new tables
- campaign CRUD
- step CRUD
- enrollment preview
- preflight checks
- send dispatcher
- thread bindings
- Reply Ops queue

Do not ship yet:

- AI classification
- open/click tracking
- automated branch campaigns

### Phase 2: Global recipient and domain governance

Ship:

- recipient state
- suppressions
- same-domain stop logic
- duplicate prevention across campaigns
- OOO pause/resume

### Phase 3: Deliverability and diagnostics

Ship:

- sender caps
- sender/domain diagnostics
- unsubscribe compliance policy
- bounce cluster controls
- per-campaign safety panel

### Phase 4: Assistive intelligence

Ship:

- reply classification assistance
- suggested actions
- suggested manual replies

Only after phases 1 through 3 are stable should Despatch consider auto-reply behavior.

## Why This Direction Fits Despatch

The market already has tools that can:

- send mail merge
- stop on reply
- pause on OOO
- track opens and clicks

That is no longer enough to differentiate.

Despatch's differentiator should be:

- stronger thread integrity
- better multi-account sender/reply continuity
- clearer operational reply handling
- more explicit domain and recipient governance
- a safer and more inspectable outbound state model

In short:

Despatch should not try to become "another sequence tool".

It should become the best place to run outbound threads without losing control after the first send.

## Open Questions

- Should `recipient_state` default to workspace-global or campaign-local for self-hosted single-user installs?
- Should reply outcome classification be deterministic-only in phase 1, or allow optional AI assist behind a feature flag?
- Should one-click unsubscribe be mandatory for all campaign types, or only for campaigns explicitly marked promotional?
- Should campaigns allow mixing `single_sender` and `reply_funnel` at the step level, or only at the campaign level?
- Do we want a first-class `company` entity later, or is domain grouping enough for the first two phases?

## Recommendation

The next implementation sequence should be:

1. add `mail_thread_bindings`
2. add `outbound_campaigns`, `steps`, `enrollments`, and `events`
3. build campaign preflight and launch
4. build Reply Ops queue
5. add recipient/domain suppression state
6. add OOO and domain-level automation
7. add diagnostics and policy rails
8. add assistive AI only after the workflow is operationally sound
