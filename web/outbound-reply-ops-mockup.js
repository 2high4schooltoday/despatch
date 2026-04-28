const clone = (value) => JSON.parse(JSON.stringify(value));

const MOCK_DATA = {
  accounts: [
    { id: "acct_gmail_founder", display_name: "Founder / Gmail", login: "founder@northstar.example", provider_type: "gmail", status: "active", daily_cap: 180, reply_mailbox: "fun_eu_collector" },
    { id: "acct_libero_sales", display_name: "Sales / Libero", login: "sales@northstar.it", provider_type: "libero", status: "active", daily_cap: 140, reply_mailbox: "fun_eu_collector" },
    { id: "acct_gmail_ops", display_name: "Ops / Gmail", login: "ops@northstar.example", provider_type: "gmail", status: "active", daily_cap: 120, reply_mailbox: "acct_gmail_ops" },
    { id: "acct_collector", display_name: "Reply Collector", login: "replies@despatch.run", provider_type: "generic", status: "active", daily_cap: 0, reply_mailbox: "acct_collector" },
  ],
  senders: [
    { id: "snd_founder", name: "Elena Petrova <founder@northstar.example>", account_id: "acct_gmail_founder" },
    { id: "snd_sales", name: "Marco Bianchi <sales@northstar.it>", account_id: "acct_libero_sales" },
    { id: "snd_ops", name: "Ops Desk <ops@northstar.example>", account_id: "acct_gmail_ops" },
  ],
  funnels: [
    { id: "fun_eu_collector", name: "EU Collector Funnel", reply_mode: "collector", collector_account_id: "acct_collector", routed_sender_ids: ["snd_founder", "snd_sales"] },
  ],
  playbooks: [
    {
      key: "revive_existing_threads",
      name: "Revive Existing Threads",
      goal_kind: "revive_thread",
      campaign_mode: "existing_threads",
      audience_source_kind: "saved_search",
      sender_policy_kind: "thread_owner",
      reply_policy: { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: false },
      compliance_policy: { unsubscribe_required: true, promotional: false },
      suppression_policy: { same_domain_unsubscribe_suppress: true },
      governance_policy: {
        recipient_collision_mode: "warn",
        domain_collision_mode: "block",
        max_active_per_domain: 1,
        positive_domain_action: "pause_domain",
        negative_domain_action: "none",
        unsubscribe_domain_action: "suppress_workspace",
      },
    },
    {
      key: "find_owner_external",
      name: "Find Inbox Owner",
      goal_kind: "find_owner",
      campaign_mode: "new_threads",
      audience_source_kind: "saved_search",
      sender_policy_kind: "reply_funnel",
      reply_policy: { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: false },
      compliance_policy: { unsubscribe_required: true, promotional: false },
      suppression_policy: { same_domain_unsubscribe_suppress: true },
      governance_policy: {
        recipient_collision_mode: "warn",
        domain_collision_mode: "warn",
        max_active_per_domain: 2,
        positive_domain_action: "none",
        negative_domain_action: "pause_domain",
        unsubscribe_domain_action: "suppress_workspace",
      },
    },
    {
      key: "founder_intro_direct",
      name: "Founder Intro",
      goal_kind: "book_meeting",
      campaign_mode: "new_threads",
      audience_source_kind: "contact_group",
      sender_policy_kind: "single_sender",
      reply_policy: { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: true },
      compliance_policy: { unsubscribe_required: true, promotional: false },
      suppression_policy: { same_domain_unsubscribe_suppress: true },
      governance_policy: {
        recipient_collision_mode: "warn",
        domain_collision_mode: "warn",
        max_active_per_domain: 1,
        positive_domain_action: "pause_domain",
        negative_domain_action: "none",
        unsubscribe_domain_action: "suppress_workspace",
      },
    },
  ],
  campaigns: [
    {
      id: "camp_thread_revive",
      name: "Dormant External Threads",
      status: "running",
      playbook_key: "revive_existing_threads",
      goal_kind: "revive_thread",
      campaign_mode: "existing_threads",
      audience_source_kind: "saved_search",
      audience_source_ref: "dormant-external-threads",
      sender_policy_kind: "thread_owner",
      sender_policy_ref: "",
      reply_policy: { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: false },
      compliance_policy: { unsubscribe_required: true, promotional: false },
      suppression_policy: { same_domain_unsubscribe_suppress: true },
      governance_policy: {
        recipient_collision_mode: "warn",
        domain_collision_mode: "block",
        max_active_per_domain: 1,
        positive_domain_action: "pause_domain",
        negative_domain_action: "none",
        unsubscribe_domain_action: "suppress_workspace",
      },
      enrollment_count: 4,
      sent_count: 4,
      replied_count: 2,
      waiting_human_count: 1,
    },
    {
      id: "camp_collector_find_owner",
      name: "Libero Migration Holdouts",
      status: "running",
      playbook_key: "find_owner_external",
      goal_kind: "find_owner",
      campaign_mode: "new_threads",
      audience_source_kind: "saved_search",
      audience_source_ref: "warm-eu-prospects-q2",
      sender_policy_kind: "reply_funnel",
      sender_policy_ref: "fun_eu_collector",
      reply_policy: { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: false },
      compliance_policy: { unsubscribe_required: true, promotional: false },
      suppression_policy: { same_domain_unsubscribe_suppress: true },
      governance_policy: {
        recipient_collision_mode: "warn",
        domain_collision_mode: "warn",
        max_active_per_domain: 2,
        positive_domain_action: "none",
        negative_domain_action: "pause_domain",
        unsubscribe_domain_action: "suppress_workspace",
      },
      enrollment_count: 3,
      sent_count: 3,
      replied_count: 1,
      waiting_human_count: 0,
    },
    {
      id: "camp_founder_intro",
      name: "Founder Intro - Gmail Accounts",
      status: "draft",
      playbook_key: "founder_intro_direct",
      goal_kind: "book_meeting",
      campaign_mode: "new_threads",
      audience_source_kind: "contact_group",
      audience_source_ref: "founders-watchlist",
      sender_policy_kind: "single_sender",
      sender_policy_ref: "snd_founder",
      reply_policy: { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: true },
      compliance_policy: { unsubscribe_required: true, promotional: false },
      suppression_policy: { same_domain_unsubscribe_suppress: true },
      governance_policy: {
        recipient_collision_mode: "warn",
        domain_collision_mode: "warn",
        max_active_per_domain: 1,
        positive_domain_action: "pause_domain",
        negative_domain_action: "none",
        unsubscribe_domain_action: "suppress_workspace",
      },
      enrollment_count: 2,
      sent_count: 0,
      replied_count: 0,
      waiting_human_count: 0,
    },
  ],
  stepsByCampaign: {
    camp_thread_revive: [
      {
        id: "step_tr_1",
        position: 1,
        kind: "email",
        thread_mode: "same_thread",
        wait_interval_minutes: 2880,
        subject_template: "Re: Picking this back up for your Gmail + Libero queues",
        body_template: "Hi {{first_name}},\n\nReopening the same thread because the interesting part here is not a new sequence. It is using the original mailbox history, from the original mailbox owner, while Despatch tracks the operational state around it.\n\nIf this is still relevant, I can send a concise breakdown.",
        task_policy: {},
        branch_policy: {
          question: "manual_task:2",
          objection: "manual_task:2",
          wrong_person: "manual_task:2",
          out_of_office: "manual_task:2",
          manual_review_required: "manual_task:2",
        },
      },
      {
        id: "step_tr_2",
        position: 2,
        kind: "manual_task",
        thread_mode: "same_thread",
        wait_interval_minutes: 0,
        subject_template: "",
        body_template: "",
        task_policy: {
          title: "Review thread-owner reply",
          instructions: "Open the seeded mailbox thread, confirm the prior context, and answer from the original mailbox owner before you let the sequence continue.",
          action_label: "Open original mailbox thread",
        },
        branch_policy: {},
      },
      {
        id: "step_tr_3",
        position: 3,
        kind: "email",
        thread_mode: "same_thread",
        wait_interval_minutes: 4320,
        subject_template: "Re: Keeping the same mailbox context intact",
        body_template: "Following up in-thread because the mailbox context itself is the product surface here. If you want, I can show how the original thread owner, collector mailbox, and reply queue stay aligned.",
        task_policy: {},
        branch_policy: {},
      },
    ],
    camp_collector_find_owner: [
      {
        id: "step_co_1",
        position: 1,
        kind: "email",
        thread_mode: "new_thread",
        wait_interval_minutes: 1440,
        subject_template: "Quick question about EU support inbox ownership",
        body_template: "Hi {{first_name}},\n\nI noticed your team spans Gmail and Libero inboxes. Despatch keeps replies operator-visible without flattening everything into a CRM.\n\nWould you be the right owner for this?",
        task_policy: {},
        branch_policy: {
          question: "manual_task:3",
          wrong_person: "manual_task:3",
        },
      },
      {
        id: "step_co_2",
        position: 2,
        kind: "email",
        thread_mode: "same_thread",
        wait_interval_minutes: 2880,
        subject_template: "Re: EU support inbox ownership",
        body_template: "Following up in the same thread. The main value is not bulk send volume; it is keeping each reply attributable to the real mailbox while reply ops stays centralized.",
        task_policy: {},
        branch_policy: {},
      },
      {
        id: "step_co_3",
        position: 3,
        kind: "manual_task",
        thread_mode: "same_thread",
        wait_interval_minutes: 0,
        subject_template: "",
        body_template: "",
        task_policy: {
          title: "Send pricing and routing explanation",
          instructions: "Reply from the originating mailbox, include seat-based pricing, and explain how collector routing preserves original thread ownership.",
          action_label: "Prepare pricing reply",
        },
        branch_policy: {},
      },
    ],
    camp_founder_intro: [
      {
        id: "step_fi_1",
        position: 1,
        kind: "email",
        thread_mode: "new_thread",
        wait_interval_minutes: 1440,
        subject_template: "Founder outreach from your actual Gmail inbox",
        body_template: "Short version: Despatch lets the real sender inbox remain the real sender inbox, even when outreach gets operationally complex.",
        task_policy: {},
        branch_policy: {
          manual_review_required: "manual_task:2",
        },
      },
      {
        id: "step_fi_2",
        position: 2,
        kind: "manual_task",
        thread_mode: "same_thread",
        wait_interval_minutes: 0,
        subject_template: "",
        body_template: "",
        task_policy: {
          title: "Founder review before follow-up",
          instructions: "Read the thread, decide if this deserves a bespoke response, and only then continue.",
          action_label: "Review as founder",
        },
        branch_policy: {},
      },
    ],
  },
  enrollmentsByCampaign: {
    camp_thread_revive: [
      {
        id: "enr_ingrid",
        campaign_id: "camp_thread_revive",
        recipient_name: "Ingrid Olsen",
        recipient_email: "ingrid@fjord.dev",
        recipient_domain: "fjord.dev",
        status: "waiting_reply",
        sender_account_id: "acct_gmail_founder",
        sender_account_label: "Founder / Gmail",
        sender_id: "snd_founder",
        reply_funnel_id: "",
        thread_id: "thr_fjord_2025",
        last_sent_message_id: "msg_send_fjord_revive",
        last_reply_message_id: "",
        next_action_at: "2026-04-30T09:15:00Z",
        seed_account_id: "acct_gmail_founder",
        seed_thread_id: "thr_fjord_2025",
        seed_message_id: "msg_fjord_last",
        seed_thread_subject: "Re: support queue cleanup for Nordic team",
        seed_mailbox: "Founder / Gmail",
        existing_thread: true,
      },
      {
        id: "enr_daniel",
        campaign_id: "camp_thread_revive",
        recipient_name: "Daniel Weber",
        recipient_email: "daniel@berlinlabs.de",
        recipient_domain: "berlinlabs.de",
        status: "manual_only",
        sender_account_id: "acct_gmail_founder",
        sender_account_label: "Founder / Gmail",
        sender_id: "snd_founder",
        reply_funnel_id: "",
        thread_id: "thr_berlinlabs",
        last_sent_message_id: "msg_send_berlinlabs",
        last_reply_message_id: "msg_reply_berlinlabs",
        next_action_at: "",
        seed_account_id: "acct_gmail_founder",
        seed_thread_id: "thr_berlinlabs",
        seed_message_id: "msg_berlinlabs_last",
        seed_thread_subject: "Re: mailbox ownership follow-up",
        seed_mailbox: "Founder / Gmail",
        existing_thread: true,
      },
      {
        id: "enr_marta",
        campaign_id: "camp_thread_revive",
        recipient_name: "Marta Silva",
        recipient_email: "marta@portoapps.pt",
        recipient_domain: "portoapps.pt",
        status: "paused",
        sender_account_id: "acct_gmail_ops",
        sender_account_label: "Ops / Gmail",
        sender_id: "snd_ops",
        reply_funnel_id: "",
        thread_id: "thr_portoapps",
        last_sent_message_id: "msg_send_portoapps",
        last_reply_message_id: "msg_reply_portoapps",
        next_action_at: "2026-05-06T08:00:00Z",
        seed_account_id: "acct_gmail_ops",
        seed_thread_id: "thr_portoapps",
        seed_message_id: "msg_portoapps_last",
        seed_thread_subject: "Re: shared inbox cleanup before summer",
        seed_mailbox: "Ops / Gmail",
        existing_thread: true,
      },
      {
        id: "enr_lena",
        campaign_id: "camp_thread_revive",
        recipient_name: "Lena Kohl",
        recipient_email: "lena@northgrid.de",
        recipient_domain: "northgrid.de",
        status: "completed",
        sender_account_id: "acct_gmail_founder",
        sender_account_label: "Founder / Gmail",
        sender_id: "snd_founder",
        reply_funnel_id: "",
        thread_id: "thr_northgrid_reopen",
        last_sent_message_id: "msg_send_northgrid_reopen",
        last_reply_message_id: "msg_reply_northgrid_reopen",
        next_action_at: "",
        seed_account_id: "acct_gmail_founder",
        seed_thread_id: "thr_northgrid_reopen",
        seed_message_id: "msg_northgrid_last",
        seed_thread_subject: "Re: continuing this when budgets reopen",
        seed_mailbox: "Founder / Gmail",
        existing_thread: true,
      },
    ],
    camp_collector_find_owner: [
      {
        id: "enr_sofia",
        campaign_id: "camp_collector_find_owner",
        recipient_name: "Sofia Leone",
        recipient_email: "sofia@espresso.it",
        recipient_domain: "espresso.it",
        status: "completed",
        sender_account_id: "acct_libero_sales",
        sender_account_label: "Sales / Libero",
        sender_id: "snd_sales",
        reply_funnel_id: "fun_eu_collector",
        thread_id: "thr_espresso",
        last_sent_message_id: "msg_send_espresso",
        last_reply_message_id: "msg_reply_espresso",
        next_action_at: "",
        existing_thread: false,
      },
      {
        id: "enr_ana",
        campaign_id: "camp_collector_find_owner",
        recipient_name: "Ana Martins",
        recipient_email: "ana@portoapps.pt",
        recipient_domain: "portoapps.pt",
        status: "waiting_reply",
        sender_account_id: "acct_libero_sales",
        sender_account_label: "Sales / Libero",
        sender_id: "snd_sales",
        reply_funnel_id: "fun_eu_collector",
        thread_id: "thr_ana_new",
        last_sent_message_id: "msg_send_ana",
        last_reply_message_id: "",
        next_action_at: "2026-04-30T08:45:00Z",
        existing_thread: false,
      },
      {
        id: "enr_tomas",
        campaign_id: "camp_collector_find_owner",
        recipient_name: "Tomas Radu",
        recipient_email: "tomas@relay.ro",
        recipient_domain: "relay.ro",
        status: "scheduled",
        sender_account_id: "acct_gmail_founder",
        sender_account_label: "Founder / Gmail",
        sender_id: "snd_founder",
        reply_funnel_id: "fun_eu_collector",
        thread_id: "thr_relay_ro",
        last_sent_message_id: "",
        last_reply_message_id: "",
        next_action_at: "2026-04-29T11:00:00Z",
        existing_thread: false,
      },
    ],
    camp_founder_intro: [
      {
        id: "enr_zoe",
        campaign_id: "camp_founder_intro",
        recipient_name: "Zoe Park",
        recipient_email: "zoe@foundry.io",
        recipient_domain: "foundry.io",
        status: "scheduled",
        sender_account_id: "acct_gmail_founder",
        sender_account_label: "Founder / Gmail",
        sender_id: "snd_founder",
        reply_funnel_id: "",
        thread_id: "thr_foundry",
        last_sent_message_id: "",
        last_reply_message_id: "",
        next_action_at: "2026-04-29T08:15:00Z",
        existing_thread: false,
      },
      {
        id: "enr_mika",
        campaign_id: "camp_founder_intro",
        recipient_name: "Mika Svensson",
        recipient_email: "mika@fjord.dev",
        recipient_domain: "fjord.dev",
        status: "scheduled",
        sender_account_id: "acct_gmail_founder",
        sender_account_label: "Founder / Gmail",
        sender_id: "snd_founder",
        reply_funnel_id: "",
        thread_id: "thr_fjord_intro",
        last_sent_message_id: "",
        last_reply_message_id: "",
        next_action_at: "2026-04-29T09:15:00Z",
        existing_thread: false,
      },
    ],
  },
  replyOpsItems: [
    {
      id: "reply_thread_question",
      bucket: "questions",
      reply_outcome: "question",
      campaign_id: "camp_thread_revive",
      campaign_name: "Dormant External Threads",
      enrollment_id: "enr_daniel",
      recipient_name: "Daniel Weber",
      recipient_email: "daniel@berlinlabs.de",
      sender_account_id: "acct_gmail_founder",
      reply_account_id: "acct_gmail_founder",
      reply_funnel_id: "",
      thread_id: "thr_berlinlabs",
      message_id: "msg_reply_berlinlabs",
      subject: "Re: mailbox ownership follow-up",
      preview: "How do you keep the original Gmail thread intact if Despatch is tracking the state around it?",
      body: "Elena,\n\nI understand the appeal of keeping the original mailbox thread, but I need to know where the state actually lives. Does Despatch reopen my historical Gmail thread exactly as-is, or is it synthesizing something around it?\n\nDaniel",
      received_at: "2026-04-28T10:04:00Z",
      action_note: "Branched into a manual task for a human thread-owner response.",
    },
    {
      id: "reply_thread_ooo",
      bucket: "out_of_office",
      reply_outcome: "out_of_office",
      campaign_id: "camp_thread_revive",
      campaign_name: "Dormant External Threads",
      enrollment_id: "enr_marta",
      recipient_name: "Marta Silva",
      recipient_email: "marta@portoapps.pt",
      sender_account_id: "acct_gmail_ops",
      reply_account_id: "acct_gmail_ops",
      reply_funnel_id: "",
      thread_id: "thr_portoapps",
      message_id: "msg_reply_portoapps",
      subject: "Automatic reply: out until May 6",
      preview: "I am away until May 6. If urgent, route inbox ownership questions to joao@portoapps.pt.",
      body: "Automatic reply\n\nI am out of office until May 6.\n\nIf this is urgent, please route inbox ownership questions to joao@portoapps.pt.\n\nMarta",
      received_at: "2026-04-28T06:50:00Z",
      action_note: "Paused until return date; referral target detected.",
    },
    {
      id: "reply_collector_positive",
      bucket: "interested",
      reply_outcome: "positive_interest",
      campaign_id: "camp_collector_find_owner",
      campaign_name: "Libero Migration Holdouts",
      enrollment_id: "enr_sofia",
      recipient_name: "Sofia Leone",
      recipient_email: "sofia@espresso.it",
      sender_account_id: "acct_libero_sales",
      reply_account_id: "acct_collector",
      reply_funnel_id: "fun_eu_collector",
      thread_id: "thr_espresso",
      message_id: "msg_reply_espresso",
      subject: "Re: EU support inbox ownership",
      preview: "Could you send pricing for 40 seats and confirm whether Gmail and Libero stay connected directly?",
      body: "Hi Marco,\n\nThis is interesting. We want to keep our Libero mailbox as-is for the commercial team while still making replies visible in one operator queue.\n\nCould you send pricing for roughly 40 seats?\n\nSofia",
      received_at: "2026-04-28T09:18:00Z",
      action_note: "Collector captured the reply, then mapped the operator back into the original Libero sender thread.",
    },
    {
      id: "reply_founder_review",
      bucket: "needs_review",
      reply_outcome: "manual_review_required",
      campaign_id: "camp_founder_intro",
      campaign_name: "Founder Intro - Gmail Accounts",
      enrollment_id: "enr_mika",
      recipient_name: "Mika Svensson",
      recipient_email: "mika@fjord.dev",
      sender_account_id: "acct_gmail_founder",
      reply_account_id: "acct_gmail_founder",
      reply_funnel_id: "",
      thread_id: "thr_fjord_intro",
      message_id: "msg_reply_fjord_intro",
      subject: "Re: Founder outreach from your actual Gmail inbox",
      preview: "My EA handles vendor intake. If this really preserves thread history, send me the concise explanation yourself.",
      body: "Hi,\n\nMy EA handles vendor intake. If this really does keep the true mailbox thread intact, send me the concise explanation yourself and I will decide whether it is worth forwarding.\n\nMika",
      received_at: "2026-04-28T11:12:00Z",
      action_note: "Manual founder review requested before any automated next step.",
    },
    {
      id: "reply_unsub",
      bucket: "unsubscribed",
      reply_outcome: "unsubscribe_request",
      campaign_id: "camp_collector_find_owner",
      campaign_name: "Libero Migration Holdouts",
      enrollment_id: "enr_ana",
      recipient_name: "Ana Martins",
      recipient_email: "ana@portoapps.pt",
      sender_account_id: "acct_libero_sales",
      reply_account_id: "acct_collector",
      reply_funnel_id: "fun_eu_collector",
      thread_id: "thr_ana_new",
      message_id: "msg_reply_ana_unsub",
      subject: "Please remove me",
      preview: "Please remove me from future follow-up on this topic.",
      body: "Please remove me from future follow-up on this topic.\n\nAna",
      received_at: "2026-04-27T15:42:00Z",
      action_note: "Workspace-level suppression applied under campaign governance.",
    },
  ],
  senderDiagnostics: [
    {
      id: "diag_sender_gmail_founder",
      sender_id: "snd_founder",
      provider_type: "gmail",
      status: "active",
      daily_cap: 180,
      sent_today: 46,
      recommended_daily_cap: 120,
      recommended_hourly_cap: 15,
      recommended_gap_seconds: 210,
      collector_account_id: "acct_collector",
      collector_account_label: "Reply Collector",
      reply_topology: "Thread-owner direct for existing-thread campaigns, collector-routed for EU owner discovery",
    },
    {
      id: "diag_sender_libero_sales",
      sender_id: "snd_sales",
      provider_type: "libero",
      status: "active",
      daily_cap: 140,
      sent_today: 37,
      recommended_daily_cap: 90,
      recommended_hourly_cap: 10,
      recommended_gap_seconds: 260,
      collector_account_id: "acct_collector",
      collector_account_label: "Reply Collector",
      reply_topology: "Primary external mailbox for collector-routed EU campaigns",
    },
    {
      id: "diag_sender_ops",
      sender_id: "snd_ops",
      provider_type: "gmail",
      status: "active",
      daily_cap: 120,
      sent_today: 11,
      recommended_daily_cap: 80,
      recommended_hourly_cap: 8,
      recommended_gap_seconds: 300,
      collector_account_id: "",
      collector_account_label: "",
      reply_topology: "Direct mailbox path for ops-owned existing threads",
    },
  ],
  domainDiagnostics: [
    { id: "diag_domain_fjord", domain: "fjord.dev", active_enrollments: 2, suppressed: false, domain_cap: 1, last_event: "Recipient active in founder intro and dormant-thread revive" },
    { id: "diag_domain_northgrid", domain: "northgrid.de", active_enrollments: 1, suppressed: false, domain_cap: 1, last_event: "Positive reply paused further same-domain outreach" },
    { id: "diag_domain_portoapps", domain: "portoapps.pt", active_enrollments: 1, suppressed: false, domain_cap: 2, last_event: "Out-of-office reply delayed next step until May 6" },
    { id: "diag_domain_atlas", domain: "atlas.support", active_enrollments: 0, suppressed: true, domain_cap: 1, last_event: "Workspace suppression carried from prior unsubscribe" },
  ],
  eventsByCampaign: {
    camp_thread_revive: [
      { id: "evt_tr_1", type: "manual_task_due", summary: "Daniel Weber's reply branched into manual task step 2 for thread-owner review.", created_at: "2026-04-28T10:05:00Z" },
      { id: "evt_tr_2", type: "out_of_office", summary: "Marta Silva paused until May 6 from the original Ops / Gmail thread.", created_at: "2026-04-28T06:50:00Z" },
      { id: "evt_tr_3", type: "step_sent", summary: "Same-thread revive sent to Ingrid Olsen from Founder / Gmail using the seeded historical thread.", created_at: "2026-04-28T08:04:00Z" },
    ],
    camp_collector_find_owner: [
      { id: "evt_co_1", type: "reply_detected", summary: "Collector inbox captured Sofia Leone's reply from the Libero-originated thread.", created_at: "2026-04-28T09:18:30Z" },
      { id: "evt_co_2", type: "provider_pacing_delayed", summary: "Libero pacing rail deferred Tomas Radu to stay under the recommended hourly cap.", created_at: "2026-04-28T09:40:00Z" },
    ],
    camp_founder_intro: [
      { id: "evt_fi_1", type: "enrolled", summary: "Zoe Park and Mika Svensson enrolled from founders-watchlist.", created_at: "2026-04-28T07:22:00Z" },
    ],
  },
  audienceLibrary: {
    "saved_search:dormant-external-threads": [
      {
        recipient_name: "Paolo Conti",
        recipient_email: "paolo@contatto.it",
        preferred_sender_id: "snd_sales",
        seed_account_id: "acct_libero_sales",
        seed_thread_id: "thr_contatto_archived",
        seed_message_id: "msg_contatto_last",
        seed_thread_subject: "Re: support migration once Libero backlog clears",
        seed_mailbox: "Sales / Libero",
        existing_thread: true,
      },
      {
        recipient_name: "Lea Novak",
        recipient_email: "lea@northgrid.de",
        preferred_sender_id: "snd_founder",
        seed_account_id: "acct_gmail_founder",
        seed_thread_id: "thr_northgrid_archived",
        seed_message_id: "msg_northgrid_archived",
        seed_thread_subject: "Re: keeping thread ownership while scaling outreach",
        seed_mailbox: "Founder / Gmail",
        existing_thread: true,
        active_elsewhere: true,
      },
      {
        recipient_name: "Jonas Berg",
        recipient_email: "jonas@fjord.dev",
        preferred_sender_id: "snd_ops",
        seed_account_id: "acct_gmail_ops",
        seed_thread_id: "thr_fjord_ops",
        seed_message_id: "msg_fjord_ops",
        seed_thread_subject: "Re: queue handoff from ops mailbox",
        seed_mailbox: "Ops / Gmail",
        existing_thread: true,
      },
    ],
    "saved_search:warm-eu-prospects-q2": [
      { recipient_name: "Nina Guerra", recipient_email: "nina@contatto.it", preferred_sender_id: "snd_sales" },
      { recipient_name: "Paul Richter", recipient_email: "paul@northgrid.de", preferred_sender_id: "snd_founder", active_elsewhere: true },
      { recipient_name: "Helene Roche", recipient_email: "helene@marche.fr", preferred_sender_id: "snd_sales" },
    ],
    "contact_group:founders-watchlist": [
      { recipient_name: "Arman Shah", recipient_email: "arman@relay.qa", preferred_sender_id: "snd_founder", active_elsewhere: true },
      { recipient_name: "Elliot Kane", recipient_email: "elliot@harbor.dev", preferred_sender_id: "snd_founder" },
      { recipient_name: "Noa Lind", recipient_email: "noa@fjord.dev", preferred_sender_id: "snd_founder" },
    ],
  },
};

const state = {
  theme: "paper-light",
  view: "outbound",
  selectedCampaignID: "camp_thread_revive",
  selectedStepID: "step_tr_1",
  selectedReplyID: "reply_thread_question",
  selectedBucket: "questions",
  playbooks: clone(MOCK_DATA.playbooks),
  campaigns: clone(MOCK_DATA.campaigns),
  stepsByCampaign: clone(MOCK_DATA.stepsByCampaign),
  enrollmentsByCampaign: clone(MOCK_DATA.enrollmentsByCampaign),
  replyOpsItems: clone(MOCK_DATA.replyOpsItems),
  senderDiagnostics: clone(MOCK_DATA.senderDiagnostics),
  domainDiagnostics: clone(MOCK_DATA.domainDiagnostics),
  eventsByCampaign: clone(MOCK_DATA.eventsByCampaign),
  audiencePreview: [],
  activeOutboundSection: "strategy",
  modalOpen: false,
};

const el = {
  html: document.documentElement,
  status: document.getElementById("mk-status"),
  themeBtn: document.getElementById("mk-theme"),
  tabOutbound: document.getElementById("mk-tab-outbound"),
  tabReplyOps: document.getElementById("mk-tab-reply-ops"),
  viewOutbound: document.getElementById("mk-view-outbound"),
  viewReplyOps: document.getElementById("mk-view-reply-ops"),
  topologyGrid: document.getElementById("mk-topology-grid"),
  quickOpenCampaign: document.getElementById("mk-quick-open-campaign"),
  quickOpenQueue: document.getElementById("mk-quick-open-queue"),
  quickOpenPositive: document.getElementById("mk-quick-open-positive"),
  outboundNote: document.getElementById("outbound-note"),
  outboundCampaignList: document.getElementById("outbound-campaign-list"),
  outboundSummary: document.getElementById("outbound-summary"),
  outboundSectionStrategy: document.getElementById("outbound-section-strategy"),
  outboundSectionAudience: document.getElementById("outbound-section-audience"),
  outboundSectionHealth: document.getElementById("outbound-section-health"),
  outboundCampaignName: document.getElementById("outbound-campaign-name"),
  outboundCampaignPlaybook: document.getElementById("outbound-campaign-playbook"),
  outboundCampaignGoal: document.getElementById("outbound-campaign-goal"),
  outboundCampaignMode: document.getElementById("outbound-campaign-mode"),
  outboundCampaignAudienceKind: document.getElementById("outbound-campaign-audience-kind"),
  outboundCampaignAudienceRef: document.getElementById("outbound-campaign-audience-ref"),
  outboundCampaignSenderKind: document.getElementById("outbound-campaign-sender-kind"),
  outboundCampaignSenderRef: document.getElementById("outbound-campaign-sender-ref"),
  outboundReplyStop: document.getElementById("outbound-reply-stop"),
  outboundReplyQuestion: document.getElementById("outbound-reply-question"),
  outboundReplyDomain: document.getElementById("outbound-reply-domain"),
  outboundComplianceUnsubscribe: document.getElementById("outbound-compliance-unsubscribe"),
  outboundCompliancePromotional: document.getElementById("outbound-compliance-promotional"),
  outboundSuppressionDomain: document.getElementById("outbound-suppression-domain"),
  outboundSenderControl: document.getElementById("outbound-sender-control"),
  outboundGovernanceRecipientCollision: document.getElementById("outbound-governance-recipient-collision"),
  outboundGovernanceDomainCollision: document.getElementById("outbound-governance-domain-collision"),
  outboundGovernanceDomainCap: document.getElementById("outbound-governance-domain-cap"),
  outboundGovernancePositiveAction: document.getElementById("outbound-governance-positive-action"),
  outboundGovernanceNegativeAction: document.getElementById("outbound-governance-negative-action"),
  outboundGovernanceUnsubAction: document.getElementById("outbound-governance-unsub-action"),
  outboundStepPosition: document.getElementById("outbound-step-position"),
  outboundStepKind: document.getElementById("outbound-step-kind"),
  outboundStepThreadMode: document.getElementById("outbound-step-thread-mode"),
  outboundStepWait: document.getElementById("outbound-step-wait"),
  outboundStepSubject: document.getElementById("outbound-step-subject"),
  outboundStepBody: document.getElementById("outbound-step-body"),
  outboundStepTaskTitleWrap: document.getElementById("outbound-step-task-title-wrap"),
  outboundStepTaskInstructionsWrap: document.getElementById("outbound-step-task-instructions-wrap"),
  outboundStepTaskActionLabelWrap: document.getElementById("outbound-step-task-action-label-wrap"),
  outboundStepTaskTitle: document.getElementById("outbound-step-task-title"),
  outboundStepTaskInstructions: document.getElementById("outbound-step-task-instructions"),
  outboundStepTaskActionLabel: document.getElementById("outbound-step-task-action-label"),
  outboundStepBranchQuestion: document.getElementById("outbound-step-branch-question"),
  outboundStepBranchObjection: document.getElementById("outbound-step-branch-objection"),
  outboundStepBranchReferral: document.getElementById("outbound-step-branch-referral"),
  outboundStepBranchOOO: document.getElementById("outbound-step-branch-ooo"),
  outboundStepBranchReview: document.getElementById("outbound-step-branch-review"),
  outboundStepList: document.getElementById("outbound-step-list"),
  outboundAudienceKind: document.getElementById("outbound-audience-kind"),
  outboundAudienceRef: document.getElementById("outbound-audience-ref"),
  outboundAudienceText: document.getElementById("outbound-audience-text"),
  outboundAudiencePreviewList: document.getElementById("outbound-audience-preview-list"),
  outboundEnrollmentList: document.getElementById("outbound-enrollment-list"),
  outboundSenderDiagnostics: document.getElementById("outbound-sender-diagnostics"),
  outboundDomainDiagnostics: document.getElementById("outbound-domain-diagnostics"),
  outboundEventList: document.getElementById("outbound-event-list"),
  replyOpsNote: document.getElementById("reply-ops-note"),
  replyOpsBuckets: document.getElementById("reply-ops-buckets"),
  replyOpsList: document.getElementById("reply-ops-list"),
  replyOpsDetail: document.getElementById("reply-ops-detail"),
  btnOutboundRefresh: document.getElementById("btn-outbound-refresh"),
  btnOutboundNew: document.getElementById("btn-outbound-new"),
  btnOutboundSave: document.getElementById("btn-outbound-save"),
  btnOutboundPreflight: document.getElementById("btn-outbound-preflight"),
  btnOutboundLaunch: document.getElementById("btn-outbound-launch"),
  btnOutboundPause: document.getElementById("btn-outbound-pause"),
  btnOutboundResume: document.getElementById("btn-outbound-resume"),
  btnOutboundArchive: document.getElementById("btn-outbound-archive"),
  btnOutboundStepSave: document.getElementById("btn-outbound-step-save"),
  btnOutboundStepNew: document.getElementById("btn-outbound-step-new"),
  btnOutboundAudiencePreview: document.getElementById("btn-outbound-audience-preview"),
  btnOutboundAudienceImport: document.getElementById("btn-outbound-audience-import"),
  btnReplyOpsRefresh: document.getElementById("btn-reply-ops-refresh"),
  btnReplyOpsOpenThread: document.getElementById("btn-reply-ops-open-thread"),
  btnReplyOpsTakeover: document.getElementById("btn-reply-ops-takeover"),
  btnReplyOpsStop: document.getElementById("btn-reply-ops-stop"),
  replyOpsClassifyPositive: document.getElementById("reply-ops-classify-positive"),
  replyOpsClassifyQuestion: document.getElementById("reply-ops-classify-question"),
  replyOpsClassifyObjection: document.getElementById("reply-ops-classify-objection"),
  replyOpsClassifyWrong: document.getElementById("reply-ops-classify-wrong"),
  replyOpsClassifyNegative: document.getElementById("reply-ops-classify-negative"),
  replyOpsClassifyUnsub: document.getElementById("reply-ops-classify-unsub"),
  replyOpsClassifyOOO: document.getElementById("reply-ops-classify-ooo"),
  replyOpsClassifyBounce: document.getElementById("reply-ops-classify-bounce"),
  replyOpsClassifyHostile: document.getElementById("reply-ops-classify-hostile"),
  replyOpsActionSuppressRecipient: document.getElementById("reply-ops-action-suppress-recipient"),
  replyOpsActionSuppressDomain: document.getElementById("reply-ops-action-suppress-domain"),
  replyOpsActionPause: document.getElementById("reply-ops-action-pause"),
  replyOpsActionResume: document.getElementById("reply-ops-action-resume"),
  modal: document.getElementById("mk-modal"),
  modalKicker: document.getElementById("mk-modal-kicker"),
  modalTitle: document.getElementById("mk-modal-title"),
  modalBody: document.getElementById("mk-modal-body"),
  modalDoc: document.getElementById("mk-modal-doc"),
  modalClose: document.getElementById("mk-modal-close"),
};

function setStatus(text, tone = "info") {
  el.status.textContent = String(text || "").trim() || "Ready.";
  if (tone === "error") el.status.style.color = "var(--sig-err)";
  else if (tone === "ok") el.status.style.color = "var(--sig-ok)";
  else if (tone === "warn") el.status.style.color = "var(--sig-warn)";
  else el.status.style.color = "var(--fg-muted)";
}

function currentCampaign() {
  return state.campaigns.find((item) => item.id === state.selectedCampaignID) || null;
}

function currentSteps() {
  return state.stepsByCampaign[state.selectedCampaignID] || [];
}

function currentEnrollments() {
  return state.enrollmentsByCampaign[state.selectedCampaignID] || [];
}

function currentEvents() {
  return state.eventsByCampaign[state.selectedCampaignID] || [];
}

function currentReplyItems() {
  return state.replyOpsItems.filter((item) => !state.selectedBucket || item.bucket === state.selectedBucket);
}

function currentReplyItem() {
  return state.replyOpsItems.find((item) => item.id === state.selectedReplyID) || null;
}

function outboundActiveSection() {
  const value = String(state.activeOutboundSection || "").trim().toLowerCase();
  return ["strategy", "audience", "health"].includes(value) ? value : "strategy";
}

function setActiveOutboundSection(section) {
  const next = ["strategy", "audience", "health"].includes(String(section || "").trim()) ? String(section).trim() : "strategy";
  state.activeOutboundSection = next;
  el.outboundSectionStrategy?.classList.toggle("is-active", next === "strategy");
  el.outboundSectionAudience?.classList.toggle("is-active", next === "audience");
  el.outboundSectionHealth?.classList.toggle("is-active", next === "health");
  document.querySelectorAll("[data-outbound-section]").forEach((panel) => {
    panel.classList.toggle("is-hidden", String(panel.getAttribute("data-outbound-section") || "").trim() !== next);
  });
}

function accountByID(accountID) {
  return MOCK_DATA.accounts.find((item) => item.id === String(accountID || "").trim()) || null;
}

function senderByID(senderID) {
  return MOCK_DATA.senders.find((item) => item.id === String(senderID || "").trim()) || null;
}

function funnelByID(funnelID) {
  return MOCK_DATA.funnels.find((item) => item.id === String(funnelID || "").trim()) || null;
}

function playbookByKey(playbookKey) {
  return state.playbooks.find((item) => item.key === String(playbookKey || "").trim()) || null;
}

function formatDateTime(value) {
  if (!value) return "None";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return String(value);
  return parsed.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

function humanizeStatus(value) {
  const raw = String(value || "").trim().toLowerCase();
  const table = {
    active: "Active",
    archived: "Archived",
    bounced: "Bounced",
    completed: "Completed",
    draft: "Draft",
    manual_only: "Needs Human",
    paused: "Paused",
    running: "Running",
    scheduled: "Scheduled",
    stopped: "Stopped",
    unsubscribed: "Unsubscribed",
    waiting_reply: "Waiting Reply",
  };
  return table[raw] || raw.replaceAll("_", " ").replace(/\b\w/g, (m) => m.toUpperCase());
}

function humanizeOutcome(value) {
  const raw = String(value || "").trim().toLowerCase();
  const table = {
    bounce: "Bounce",
    hostile: "Hostile",
    manual_review_required: "Needs Review",
    not_interested: "Not Interested",
    objection: "Objection",
    out_of_office: "Out Of Office",
    positive_interest: "Interested",
    question: "Question",
    unsubscribe_request: "Unsubscribe",
    wrong_person: "Wrong Person",
  };
  return table[raw] || "Unclassified";
}

function humanizeBucket(value) {
  const raw = String(value || "").trim().toLowerCase();
  const table = {
    bounces: "Bounces",
    hostile: "Hostile",
    interested: "Interested",
    needs_review: "Needs Review",
    objections: "Objections",
    out_of_office: "Out Of Office",
    questions: "Questions",
    unsubscribed: "Unsubscribed",
    wrong_person: "Wrong Person",
  };
  return table[raw] || "Replies";
}

function toneClass(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (["active", "completed", "interested", "running", "scheduled"].includes(raw)) return "outbound-chip outbound-chip--ok";
  if (["manual_only", "needs_review", "objection", "out_of_office", "paused", "question"].includes(raw)) return "outbound-chip outbound-chip--warn";
  if (["archived", "bounce", "bounced", "hostile", "not_interested", "stopped", "unsubscribe_request", "unsubscribed", "wrong_person"].includes(raw)) return "outbound-chip outbound-chip--err";
  return "outbound-chip";
}

function providerLabel(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (raw === "gmail") return "Gmail";
  if (raw === "libero") return "Libero";
  if (raw === "generic") return "Collector";
  return raw || "Mailbox";
}

function humanizeGoal(value) {
  const raw = String(value || "").trim().toLowerCase();
  const table = {
    revive_thread: "Revive thread",
    find_owner: "Find owner",
    book_meeting: "Book meeting",
    price_quote: "Price quote",
    qualify_interest: "Qualify interest",
  };
  return table[raw] || raw.replaceAll("_", " ").replace(/\b\w/g, (m) => m.toUpperCase());
}

function humanizeCampaignMode(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (raw === "existing_threads") return "Existing threads";
  if (raw === "new_threads") return "New threads";
  return raw.replaceAll("_", " ").replace(/\b\w/g, (m) => m.toUpperCase()) || "Campaign mode";
}

function humanizeSenderPolicy(value) {
  const raw = String(value || "").trim().toLowerCase();
  const table = {
    thread_owner: "Thread owner mailbox",
    preferred_sender: "Preferred sender",
    single_sender: "Single sender",
    campaign_pool: "Sender pool",
    reply_funnel: "Reply funnel",
  };
  return table[raw] || raw.replaceAll("_", " ").replace(/\b\w/g, (m) => m.toUpperCase());
}

function humanizeStepKind(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (raw === "manual_task") return "Manual task";
  if (raw === "email") return "Email";
  return raw.replaceAll("_", " ").replace(/\b\w/g, (m) => m.toUpperCase()) || "Step";
}

function humanizeCollisionMode(value) {
  const raw = String(value || "").trim().toLowerCase();
  if (raw === "warn") return "Warn only";
  if (raw === "block") return "Block launch";
  if (raw === "allow") return "Allow";
  return raw || "Default";
}

function humanizeGovernanceAction(value) {
  const raw = String(value || "").trim().toLowerCase();
  const table = {
    none: "No automatic action",
    pause_domain: "Pause same domain",
    stop_domain: "Stop same domain",
    suppress_workspace: "Suppress across workspace",
    suppress_campaign: "Suppress only in this campaign",
  };
  return table[raw] || raw.replaceAll("_", " ").replace(/\b\w/g, (m) => m.toUpperCase());
}

function defaultGovernancePolicy() {
  return {
    recipient_collision_mode: "warn",
    domain_collision_mode: "warn",
    max_active_per_domain: 0,
    positive_domain_action: "none",
    negative_domain_action: "none",
    unsubscribe_domain_action: "suppress_workspace",
  };
}

function summarizeCampaignSender(campaign) {
  if (!campaign) return "";
  if (campaign.sender_policy_kind === "thread_owner") {
    return "Thread owner mailbox -> seeded historical mailbox decides who sends";
  }
  if (campaign.sender_policy_kind === "reply_funnel") {
    const funnel = funnelByID(campaign.sender_policy_ref);
    const collector = accountByID(funnel?.collector_account_id);
    return `Reply funnel -> ${funnel?.name || campaign.sender_policy_ref} -> ${collector?.login || "collector inbox"}`;
  }
  if (campaign.sender_policy_kind === "single_sender") {
    const sender = senderByID(campaign.sender_policy_ref);
    return `Single sender -> ${sender?.name || campaign.sender_policy_ref}`;
  }
  if (campaign.sender_policy_kind === "campaign_pool") {
    return `Sender pool -> ${campaign.sender_policy_ref}`;
  }
  return "Preferred sender -> contact and account hints";
}

function summarizeCampaignSenderCompact(campaign) {
  if (!campaign) return "";
  if (campaign.sender_policy_kind === "thread_owner") {
    return "Original thread owner mailbox";
  }
  if (campaign.sender_policy_kind === "reply_funnel") {
    const funnel = funnelByID(campaign.sender_policy_ref);
    const collector = accountByID(funnel?.collector_account_id);
    return collector
      ? `${funnel?.name || "Reply funnel"} via ${collector.login}`
      : (funnel?.name || "Reply funnel");
  }
  if (campaign.sender_policy_kind === "single_sender") {
    const sender = senderByID(campaign.sender_policy_ref);
    return sender?.name || "Single sender";
  }
  if (campaign.sender_policy_kind === "campaign_pool") {
    return "Sender pool";
  }
  return "Preferred sender";
}

function summarizeCampaignMetrics(campaign) {
  return `${Number(campaign?.enrollment_count || 0)} enrolled • ${Number(campaign?.sent_count || 0)} sent • ${Number(campaign?.replied_count || 0)} replied • ${Number(campaign?.waiting_human_count || 0)} needs human`;
}

function outboundSummaryNote(campaign) {
  if (!campaign) return "";
  if (campaign.campaign_mode === "existing_threads") {
    return "Reopens seeded threads in their original mailbox context, so the operator never loses who owns the conversation.";
  }
  if (campaign.sender_policy_kind === "reply_funnel") {
    return "Replies can land in a collector mailbox without breaking the original sender context that the operator responds from.";
  }
  if (campaign.sender_policy_kind === "single_sender") {
    return "One mailbox owns every conversation from first send through manual follow-up.";
  }
  return "Use the playbook, sender policy, and safety rails to keep the motion easy to read and easy to operate.";
}

function outboundFocusLabel() {
  if (outboundActiveSection() === "audience") return "Reviewing audience and live recipients";
  if (outboundActiveSection() === "health") return "Checking routing health and recent events";
  return "Editing setup and sequence";
}

function renderOutboundSummary() {
  if (!el.outboundSummary) return;
  el.outboundSummary.replaceChildren();
  const savedCampaign = currentCampaign();
  if (!savedCampaign) {
    const empty = document.createElement("p");
    empty.className = "outbound-summary-empty";
    empty.textContent = "Choose a campaign to see a short operational summary before diving into detailed controls.";
    el.outboundSummary.appendChild(empty);
    return;
  }
  const senderPolicyKind = String(el.outboundCampaignSenderKind?.value || savedCampaign.sender_policy_kind || "preferred_sender").trim() || "preferred_sender";
  const campaign = {
    ...savedCampaign,
    name: String(el.outboundCampaignName?.value || savedCampaign.name || "").trim() || savedCampaign.name,
    playbook_key: String(el.outboundCampaignPlaybook?.value || savedCampaign.playbook_key || "").trim(),
    campaign_mode: String(el.outboundCampaignMode?.value || savedCampaign.campaign_mode || "new_threads").trim() || "new_threads",
    sender_policy_kind: senderPolicyKind,
    sender_policy_ref: senderPolicyKind === "thread_owner"
      ? ""
      : String(el.outboundCampaignSenderRef?.value || savedCampaign.sender_policy_ref || "").trim(),
  };
  const playbook = playbookByKey(campaign.playbook_key);
  const head = document.createElement("div");
  head.className = "outbound-summary-head";
  const copy = document.createElement("div");
  copy.className = "outbound-summary-copy";
  const kicker = document.createElement("p");
  kicker.className = "outbound-summary-kicker";
  kicker.textContent = "Overview";
  const title = document.createElement("h3");
  title.className = "outbound-summary-title";
  title.textContent = campaign.name || "Campaign";
  const meta = document.createElement("div");
  meta.className = "outbound-summary-meta";
  meta.textContent = summarizeCampaignMetrics(campaign);
  copy.append(kicker, title, meta);
  const chips = document.createElement("div");
  chips.className = "reply-ops-summary-chips";
  const statusChip = document.createElement("span");
  statusChip.className = toneClass(campaign.status);
  statusChip.textContent = humanizeStatus(campaign.status);
  chips.appendChild(statusChip);
  if (playbook?.name) {
    const playbookChip = document.createElement("span");
    playbookChip.className = "outbound-chip";
    playbookChip.textContent = playbook.name;
    chips.appendChild(playbookChip);
  }
  head.append(copy, chips);
  const grid = document.createElement("div");
  grid.className = "outbound-summary-grid";
  [
    ["Delivery mode", humanizeCampaignMode(campaign.campaign_mode)],
    ["Sender plan", summarizeCampaignSenderCompact(campaign)],
    ["Current focus", outboundFocusLabel()],
  ].forEach(([label, value]) => {
    const card = document.createElement("div");
    card.className = "outbound-summary-item";
    const labelNode = document.createElement("div");
    labelNode.className = "outbound-summary-label";
    labelNode.textContent = label;
    const valueNode = document.createElement("div");
    valueNode.className = "outbound-summary-value";
    valueNode.textContent = value;
    card.append(labelNode, valueNode);
    grid.appendChild(card);
  });
  const note = document.createElement("div");
  note.className = "outbound-summary-note";
  note.textContent = outboundSummaryNote(campaign);
  el.outboundSummary.append(head, grid, note);
}

function summarizeGovernance(campaign) {
  const policy = campaign?.governance_policy || defaultGovernancePolicy();
  return [
    `Recipient collisions: ${humanizeCollisionMode(policy.recipient_collision_mode)}`,
    `Domain collisions: ${humanizeCollisionMode(policy.domain_collision_mode)}`,
    `Max active per domain: ${Number(policy.max_active_per_domain || 0) || "No cap"}`,
  ];
}

function branchTargetLabel(value) {
  const raw = String(value || "").trim();
  if (!raw) return "No branch";
  if (raw.startsWith("manual_task:")) {
    const position = Number(raw.split(":")[1] || "0") || 0;
    const task = (state.stepsByCampaign[state.selectedCampaignID] || []).find((item) => Number(item.position || 0) === position && String(item.kind || "").trim().toLowerCase() === "manual_task");
    return task?.task_policy?.title || `Manual task ${position || "?"}`;
  }
  return raw;
}

function replyBucketOrder() {
  return ["needs_review", "interested", "questions", "objections", "wrong_person", "out_of_office", "bounces", "unsubscribed", "hostile"];
}

function outcomeToBucket(outcome) {
  const table = {
    bounce: "bounces",
    hostile: "hostile",
    manual_review_required: "needs_review",
    not_interested: "objections",
    objection: "objections",
    out_of_office: "out_of_office",
    positive_interest: "interested",
    question: "questions",
    unsubscribe_request: "unsubscribed",
    wrong_person: "wrong_person",
  };
  return table[String(outcome || "").trim().toLowerCase()] || "needs_review";
}

function recomputeCampaignMetrics(campaignID) {
  const campaign = state.campaigns.find((item) => item.id === campaignID);
  if (!campaign) return;
  const enrollments = state.enrollmentsByCampaign[campaignID] || [];
  campaign.enrollment_count = enrollments.length;
  campaign.sent_count = enrollments.filter((item) => item.last_sent_message_id).length;
  campaign.replied_count = enrollments.filter((item) => item.last_reply_message_id).length;
  campaign.waiting_human_count = enrollments.filter((item) => item.status === "manual_only").length;
}

function recomputeAllCampaignMetrics() {
  state.campaigns.forEach((item) => recomputeCampaignMetrics(item.id));
}

function manualTaskOptions(campaignID = state.selectedCampaignID) {
  return (state.stepsByCampaign[campaignID] || [])
    .filter((item) => String(item.kind || "email").trim().toLowerCase() === "manual_task")
    .map((item) => ({
      value: `manual_task:${Number(item.position || 0)}`,
      label: String(item?.task_policy?.title || "").trim() || `Manual task ${Number(item.position || 0)}`,
    }));
}

function resolveBranchTarget(campaignID, outcome) {
  const key = String(outcome || "").trim().toLowerCase();
  const emailSteps = (state.stepsByCampaign[campaignID] || []).filter((item) => String(item.kind || "email").trim().toLowerCase() === "email");
  for (const step of emailSteps) {
    const branchPolicy = step.branch_policy || {};
    if (branchPolicy[key]) return branchPolicy[key];
  }
  return "";
}

function describeReplyRoute(item) {
  if (!item) return "";
  if (item.reply_funnel_id) {
    const funnel = funnelByID(item.reply_funnel_id);
    const collector = accountByID(funnel?.collector_account_id);
    return `External sender -> ${collector?.login || "collector"} -> original sender thread`;
  }
  return "Original mailbox thread stays the operational thread";
}

function showModal({ kicker = "Review", title = "", body = "", doc = "" }) {
  el.modalKicker.textContent = kicker;
  el.modalTitle.textContent = title;
  el.modalBody.textContent = body;
  el.modalDoc.textContent = doc;
  el.modal.classList.remove("hidden");
  el.modal.setAttribute("aria-hidden", "false");
  state.modalOpen = true;
}

function hideModal() {
  el.modal.classList.add("hidden");
  el.modal.setAttribute("aria-hidden", "true");
  state.modalOpen = false;
}

function setView(name) {
  state.view = name === "reply-ops" ? "reply-ops" : "outbound";
  el.tabOutbound.classList.toggle("active", state.view === "outbound");
  el.tabReplyOps.classList.toggle("active", state.view === "reply-ops");
  el.viewOutbound.classList.toggle("hidden", state.view !== "outbound");
  el.viewReplyOps.classList.toggle("hidden", state.view !== "reply-ops");
}

function setTheme(theme) {
  state.theme = theme === "machine-dark" ? "machine-dark" : "paper-light";
  el.html.setAttribute("data-theme", state.theme);
  localStorage.setItem("mock.outbound.theme", state.theme);
  el.themeBtn.textContent = state.theme === "machine-dark" ? "Theme: Machine" : "Theme: Paper";
}

function initTheme() {
  const query = new URLSearchParams(window.location.search).get("theme");
  if (query === "machine-dark" || query === "paper-light") {
    setTheme(query);
    return;
  }
  setTheme(localStorage.getItem("mock.outbound.theme") || "paper-light");
}

function renderTopology() {
  const externalAccounts = MOCK_DATA.accounts.filter((item) => item.provider_type !== "generic");
  const collectorCampaigns = state.campaigns.filter((item) => item.sender_policy_kind === "reply_funnel").length;
  const existingThreadCampaigns = state.campaigns.filter((item) => item.campaign_mode === "existing_threads").length;
  const manualBranchReplies = state.replyOpsItems.filter((item) => !!resolveBranchTarget(item.campaign_id, item.reply_outcome)).length;
  el.topologyGrid.replaceChildren();

  const cards = [
    {
      title: "External sender accounts",
      body: "Real outreach originates from connected third-party mailboxes. Gmail and Libero are not side integrations here; they define the routing model.",
      lines: externalAccounts.map((item) => `${providerLabel(item.provider_type)} -> ${item.display_name} -> ${item.login}`),
    },
    {
      title: "Two campaign modes",
      body: "The mock now exposes both apex paths: native existing-thread continuation and collector-routed new-thread campaigns.",
      lines: [
        `${existingThreadCampaigns} existing-thread campaigns with thread-owner sending`,
        `${collectorCampaigns} collector-routed campaigns for external mailbox coverage`,
        "Reply Ops reopens the same operational thread regardless of reply path.",
      ],
    },
    {
      title: "Reply-driven branching",
      body: "Replies do not just stop a sequence. They can branch into manual tasks, domain governance, and human takeover while preserving sender provenance.",
      lines: [
        `${state.replyOpsItems.length} replies currently in Reply Ops`,
        `${manualBranchReplies} replies already map into manual-task branches`,
        "Preflight, pacing, and suppression are visible in the same mock workspace.",
      ],
    },
  ];

  cards.forEach((item) => {
    const card = document.createElement("article");
    card.className = "panel mock-topology-card";
    card.innerHTML = `
      <h3>${item.title}</h3>
      <p>${item.body}</p>
      <div class="mock-topology-list"></div>
    `;
    const list = card.querySelector(".mock-topology-list");
    item.lines.forEach((line) => {
      const row = document.createElement("div");
      row.textContent = line;
      list.appendChild(row);
    });
    el.topologyGrid.appendChild(card);
  });
}

function renderOutboundNote(message = "") {
  el.outboundNote.textContent = message || "This mock now mirrors the richer product model: playbook-first setup, existing-thread revival, thread-owner sending, collector routing, domain governance, and reply branching into manual tasks.";
}

function renderReplyOpsNote(message = "") {
  el.replyOpsNote.textContent = message || "Reply Ops shows not only who answered, but what branch fires next, which mailbox owns the live thread, and what governance action applies at recipient or domain scope.";
}

function renderPlaybookOptions(selectedKey = "") {
  if (!el.outboundCampaignPlaybook) return;
  el.outboundCampaignPlaybook.replaceChildren();
  state.playbooks.forEach((item) => {
    const option = document.createElement("option");
    option.value = item.key;
    option.textContent = item.name;
    if (item.key === selectedKey) option.selected = true;
    el.outboundCampaignPlaybook.appendChild(option);
  });
}

function syncBranchOptions(selected = {}) {
  const options = manualTaskOptions();
  const selects = [
    [el.outboundStepBranchQuestion, selected.question || ""],
    [el.outboundStepBranchObjection, selected.objection || ""],
    [el.outboundStepBranchReferral, selected.wrong_person || selected.referral || ""],
    [el.outboundStepBranchOOO, selected.out_of_office || ""],
    [el.outboundStepBranchReview, selected.manual_review_required || ""],
  ];
  selects.forEach(([node, value]) => {
    if (!node) return;
    node.replaceChildren();
    const blank = document.createElement("option");
    blank.value = "";
    blank.textContent = "No branch";
    node.appendChild(blank);
    options.forEach((item) => {
      const option = document.createElement("option");
      option.value = item.value;
      option.textContent = item.label;
      if (item.value === value) option.selected = true;
      node.appendChild(option);
    });
  });
}

function updateStepFormVisibility() {
  const kind = String(el.outboundStepKind?.value || "email").trim().toLowerCase();
  el.outboundStepTaskTitleWrap?.classList.toggle("hidden", kind !== "manual_task");
  el.outboundStepTaskInstructionsWrap?.classList.toggle("hidden", kind !== "manual_task");
  el.outboundStepTaskActionLabelWrap?.classList.toggle("hidden", kind !== "manual_task");
}

function renderCampaignOverview() {
  const campaign = currentCampaign();
  if (!campaign || !el.outboundCampaignOverview) return;
  const playbook = playbookByKey(campaign.playbook_key);
  const policy = campaign.governance_policy || defaultGovernancePolicy();
  const enrollments = currentEnrollments();
  const routeAudience = campaign.campaign_mode === "existing_threads" ? "Seeded mailbox threads" : "New-thread audience";
  const routeSender = summarizeCampaignSender(campaign);
  const routeReply = campaign.sender_policy_kind === "reply_funnel"
    ? describeReplyRoute({ reply_funnel_id: campaign.sender_policy_ref })
    : "Replies stay in the original mailbox thread until Reply Ops or the operator intervenes.";
  const routeHuman = `${Number(campaign.waiting_human_count || 0)} recipients currently need manual action`;
  el.outboundCampaignOverview.innerHTML = `
    <div class="mock-overview-grid">
      <article class="mock-summary-card">
        <p class="mock-summary-eyebrow">Playbook</p>
        <h3>${playbook?.name || "Custom campaign"}</h3>
        <div class="mock-kv-list">
          <div class="mock-kv-row"><strong>Goal</strong><span>${humanizeGoal(campaign.goal_kind)}</span></div>
          <div class="mock-kv-row"><strong>Mode</strong><span>${humanizeCampaignMode(campaign.campaign_mode)}</span></div>
          <div class="mock-kv-row"><strong>Sender policy</strong><span>${humanizeSenderPolicy(campaign.sender_policy_kind)}</span></div>
        </div>
      </article>
      <article class="mock-summary-card">
        <p class="mock-summary-eyebrow">Launch posture</p>
        <h3>${summarizeCampaignMetrics(campaign)}</h3>
        <div class="mock-step-meta">
          <span class="${toneClass(campaign.status)}">${humanizeStatus(campaign.status)}</span>
          <span class="outbound-chip">${Number(enrollments.filter((item) => item.existing_thread).length)} seeded threads</span>
          <span class="outbound-chip">${Number(currentSteps().filter((item) => item.kind === "manual_task").length)} manual tasks</span>
        </div>
      </article>
      <article class="mock-summary-card">
        <p class="mock-summary-eyebrow">Governance</p>
        <h3>Recipient and domain safety</h3>
        <div class="mock-kv-list">
          <div class="mock-kv-row"><strong>Recipient collision</strong><span>${humanizeCollisionMode(policy.recipient_collision_mode)}</span></div>
          <div class="mock-kv-row"><strong>Domain collision</strong><span>${humanizeCollisionMode(policy.domain_collision_mode)}</span></div>
          <div class="mock-kv-row"><strong>Positive domain action</strong><span>${humanizeGovernanceAction(policy.positive_domain_action)}</span></div>
        </div>
      </article>
    </div>
    <div class="mock-flow">
      <div class="mock-flow-step"><strong>Audience</strong><span>${routeAudience}</span></div>
      <div class="mock-flow-step"><strong>Thread seed</strong><span>${campaign.campaign_mode === "existing_threads" ? "Prior mailbox history is preserved and reopened" : "New thread starts clean, but still records sender provenance"}</span></div>
      <div class="mock-flow-step"><strong>Sender</strong><span>${routeSender}</span></div>
      <div class="mock-flow-step"><strong>Reply path</strong><span>${routeReply}</span></div>
      <div class="mock-flow-step"><strong>Human handoff</strong><span>${routeHuman}</span></div>
    </div>
  `;
}

function renderSenderControl() {
  const campaign = currentCampaign();
  if (!campaign || !el.outboundSenderControl) return;
  el.outboundSenderControl.replaceChildren();
  const activeSenderIDs = new Set(currentEnrollments().map((item) => String(item.sender_id || "").trim()).filter(Boolean));
  const diagnostics = state.senderDiagnostics.filter((item) => {
    const sender = senderByID(item.sender_id);
    if (campaign.sender_policy_kind === "thread_owner") {
      return activeSenderIDs.size ? activeSenderIDs.has(sender?.id || "") : true;
    }
    if (campaign.sender_policy_kind === "single_sender") return sender?.id === campaign.sender_policy_ref;
    if (campaign.sender_policy_kind === "campaign_pool") {
      return String(campaign.sender_policy_ref || "").split(",").map((entry) => entry.trim()).filter(Boolean).includes(sender?.id || "");
    }
    if (campaign.sender_policy_kind === "reply_funnel") {
      const funnel = funnelByID(campaign.sender_policy_ref);
      return Array.isArray(funnel?.routed_sender_ids) ? funnel.routed_sender_ids.includes(sender?.id || "") : false;
    }
    return activeSenderIDs.size ? activeSenderIDs.has(sender?.id || "") : true;
  });
  diagnostics.forEach((item) => {
    const sender = senderByID(item.sender_id);
    const card = document.createElement("div");
    card.className = "diagnostic-card";
    const head = document.createElement("div");
    head.className = "diagnostic-card-head";
    const title = document.createElement("div");
    title.className = "diagnostic-card-title";
    title.textContent = sender?.name || item.sender_id;
    const chip = document.createElement("span");
    chip.className = "outbound-chip";
    chip.textContent = providerLabel(item.provider_type);
    head.append(title, chip);
    const stack = document.createElement("div");
    stack.className = "diagnostic-card-stack";
    [
      item.reply_topology === "collector"
        ? `Collector mailbox: ${accountByID(item.collector_account_id)?.login || "collector mailbox"}`
        : item.reply_topology === "smart"
          ? "Smart collector reply path"
          : "Replies stay in this mailbox",
      `${Number(item.sent_today || 0)} sent today • recommended cap ${Number(item.recommended_daily_cap || item.daily_cap || 0)}`,
      `${Number(item.recommended_hourly_cap || 0)} per hour • ${Number(item.recommended_gap_seconds || 0)}s gap`,
    ].forEach((line) => {
      const row = document.createElement("div");
      row.className = "diagnostic-card-meta";
      row.textContent = line;
      stack.appendChild(row);
    });
    card.append(head, stack);
    el.outboundSenderControl.appendChild(card);
  });
}

function fillCampaignForm() {
  const campaign = currentCampaign();
  if (!campaign) return;
  const policy = campaign.governance_policy || defaultGovernancePolicy();
  renderPlaybookOptions(campaign.playbook_key || "");
  el.outboundCampaignName.value = campaign.name || "";
  el.outboundCampaignGoal.value = campaign.goal_kind || "revive_thread";
  el.outboundCampaignMode.value = campaign.campaign_mode || "new_threads";
  el.outboundCampaignAudienceKind.value = campaign.audience_source_kind || "manual";
  el.outboundCampaignAudienceRef.value = campaign.audience_source_ref || "";
  el.outboundCampaignSenderKind.value = campaign.sender_policy_kind || "preferred_sender";
  el.outboundCampaignSenderRef.value = campaign.sender_policy_ref || "";
  el.outboundReplyStop.checked = campaign.reply_policy?.stop_on_reply !== false;
  el.outboundReplyQuestion.checked = campaign.reply_policy?.pause_on_question !== false;
  el.outboundReplyDomain.checked = !!campaign.reply_policy?.stop_same_domain_on_reply;
  el.outboundComplianceUnsubscribe.checked = campaign.compliance_policy?.unsubscribe_required !== false;
  el.outboundCompliancePromotional.checked = !!campaign.compliance_policy?.promotional;
  el.outboundSuppressionDomain.checked = campaign.suppression_policy?.same_domain_unsubscribe_suppress !== false;
  el.outboundGovernanceRecipientCollision.value = policy.recipient_collision_mode || "warn";
  el.outboundGovernanceDomainCollision.value = policy.domain_collision_mode || "warn";
  el.outboundGovernanceDomainCap.value = String(Number(policy.max_active_per_domain || 0));
  el.outboundGovernancePositiveAction.value = policy.positive_domain_action || "none";
  el.outboundGovernanceNegativeAction.value = policy.negative_domain_action || "none";
  el.outboundGovernanceUnsubAction.value = policy.unsubscribe_domain_action || "suppress_workspace";
  if (el.outboundCampaignSenderRef) {
    el.outboundCampaignSenderRef.disabled = campaign.sender_policy_kind === "thread_owner";
    el.outboundCampaignSenderRef.placeholder = campaign.sender_policy_kind === "thread_owner"
      ? "Seeded mailbox decides the sender"
      : "Sender ID, account ID, comma-separated pool, or funnel ID";
  }
}

function applyPlaybookToForm(playbookKey) {
  const playbook = playbookByKey(playbookKey);
  if (!playbook) return;
  el.outboundCampaignGoal.value = playbook.goal_kind || "revive_thread";
  el.outboundCampaignMode.value = playbook.campaign_mode || "new_threads";
  el.outboundCampaignAudienceKind.value = playbook.audience_source_kind || "manual";
  el.outboundAudienceKind.value = playbook.audience_source_kind || "manual";
  el.outboundCampaignSenderKind.value = playbook.sender_policy_kind || "preferred_sender";
  el.outboundReplyStop.checked = playbook.reply_policy?.stop_on_reply !== false;
  el.outboundReplyQuestion.checked = playbook.reply_policy?.pause_on_question !== false;
  el.outboundReplyDomain.checked = !!playbook.reply_policy?.stop_same_domain_on_reply;
  el.outboundComplianceUnsubscribe.checked = playbook.compliance_policy?.unsubscribe_required !== false;
  el.outboundCompliancePromotional.checked = !!playbook.compliance_policy?.promotional;
  el.outboundSuppressionDomain.checked = playbook.suppression_policy?.same_domain_unsubscribe_suppress !== false;
  el.outboundGovernanceRecipientCollision.value = playbook.governance_policy?.recipient_collision_mode || "warn";
  el.outboundGovernanceDomainCollision.value = playbook.governance_policy?.domain_collision_mode || "warn";
  el.outboundGovernanceDomainCap.value = String(Number(playbook.governance_policy?.max_active_per_domain || 0));
  el.outboundGovernancePositiveAction.value = playbook.governance_policy?.positive_domain_action || "none";
  el.outboundGovernanceNegativeAction.value = playbook.governance_policy?.negative_domain_action || "none";
  el.outboundGovernanceUnsubAction.value = playbook.governance_policy?.unsubscribe_domain_action || "suppress_workspace";
  if (playbook.sender_policy_kind === "thread_owner") {
    el.outboundCampaignSenderRef.value = "";
  } else if (playbook.sender_policy_kind === "reply_funnel" && !String(el.outboundCampaignSenderRef.value || "").trim()) {
    el.outboundCampaignSenderRef.value = "fun_eu_collector";
  } else if (playbook.sender_policy_kind === "single_sender" && !String(el.outboundCampaignSenderRef.value || "").trim()) {
    el.outboundCampaignSenderRef.value = "snd_founder";
  }
  el.outboundCampaignSenderRef.disabled = playbook.sender_policy_kind === "thread_owner";
}

function fillAudienceForm() {
  const campaign = currentCampaign();
  if (!campaign) return;
  el.outboundAudienceKind.value = campaign.audience_source_kind || "manual";
  el.outboundAudienceRef.value = campaign.audience_source_ref || "";
}

function fillStepForm(step = null) {
  const record = step || currentSteps().find((item) => item.id === state.selectedStepID) || {
    position: currentSteps().length + 1,
    kind: "email",
    thread_mode: "same_thread",
    wait_interval_minutes: 1440,
    subject_template: "",
    body_template: "",
    task_policy: {},
    branch_policy: {},
  };
  el.outboundStepPosition.value = String(record.position || 1);
  el.outboundStepKind.value = record.kind || "email";
  el.outboundStepThreadMode.value = record.thread_mode || "same_thread";
  el.outboundStepWait.value = String(record.wait_interval_minutes || 1440);
  el.outboundStepSubject.value = record.subject_template || "";
  el.outboundStepBody.value = record.body_template || "";
  el.outboundStepTaskTitle.value = record.task_policy?.title || "";
  el.outboundStepTaskInstructions.value = record.task_policy?.instructions || "";
  el.outboundStepTaskActionLabel.value = record.task_policy?.action_label || "";
  syncBranchOptions(record.branch_policy || {});
  updateStepFormVisibility();
}

function renderCampaignList() {
  el.outboundCampaignList.replaceChildren();
  state.campaigns.forEach((item) => {
    const playbook = playbookByKey(item.playbook_key);
    const card = document.createElement("button");
    card.type = "button";
    card.className = "outbound-card";
    if (item.id === state.selectedCampaignID) card.classList.add("is-active");
    card.innerHTML = `
      <div class="outbound-card-head">
        <strong class="outbound-card-title">${item.name}</strong>
        <span class="${toneClass(item.status)}">${humanizeStatus(item.status)}</span>
      </div>
      <div class="mock-step-meta">
        <span class="outbound-chip">${playbook?.name || "Custom"}</span>
        <span class="outbound-chip">${humanizeCampaignMode(item.campaign_mode)}</span>
        <span class="outbound-chip">${humanizeGoal(item.goal_kind)}</span>
      </div>
      <div class="outbound-card-meta">${summarizeCampaignMetrics(item)}</div>
      <div class="outbound-card-meta">${summarizeCampaignSender(item)}</div>
    `;
    card.addEventListener("click", () => {
      state.selectedCampaignID = item.id;
      state.selectedStepID = (state.stepsByCampaign[item.id] || [])[0]?.id || "";
      state.audiencePreview = [];
      fillCampaignForm();
      fillAudienceForm();
      fillStepForm();
      renderAll();
      setStatus(`Focused campaign: ${item.name}`, "info");
    });
    el.outboundCampaignList.appendChild(card);
  });
}

function renderStepList() {
  el.outboundStepList.replaceChildren();
  const steps = currentSteps();
  if (!steps.length) {
    const empty = document.createElement("div");
    empty.className = "mock-empty";
    empty.textContent = "No steps yet. Add one and keep replies in-thread when that makes sense.";
    el.outboundStepList.appendChild(empty);
    return;
  }
  steps
    .slice()
    .sort((a, b) => Number(a.position || 0) - Number(b.position || 0))
    .forEach((item, index) => {
      const branchPolicy = item.branch_policy || {};
      const branchSummary = [
        branchPolicy.question ? `question -> ${branchTargetLabel(branchPolicy.question)}` : "",
        branchPolicy.objection ? `objection -> ${branchTargetLabel(branchPolicy.objection)}` : "",
        branchPolicy.wrong_person ? `wrong person -> ${branchTargetLabel(branchPolicy.wrong_person)}` : "",
        branchPolicy.out_of_office ? `OOO -> ${branchTargetLabel(branchPolicy.out_of_office)}` : "",
        branchPolicy.manual_review_required ? `review -> ${branchTargetLabel(branchPolicy.manual_review_required)}` : "",
      ].filter(Boolean).join(" • ");
      const card = document.createElement("div");
      card.className = "outbound-card";
      if (item.id === state.selectedStepID) card.classList.add("is-active");
      const primary = item.kind === "manual_task"
        ? item.task_policy?.title || "Manual task"
        : item.subject_template || "Body-only follow-up";
      const secondary = item.kind === "manual_task"
        ? item.task_policy?.instructions || "Human action required before the campaign continues."
        : String(item.body_template || "").slice(0, 160);
      card.innerHTML = `
        <div class="outbound-card-head">
          <strong class="outbound-card-title">Step ${item.position}</strong>
          <span class="outbound-chip">${humanizeStepKind(item.kind)}</span>
        </div>
        <div class="outbound-card-meta">${primary}</div>
        <div class="outbound-card-meta">${secondary}</div>
        <div class="outbound-card-meta">${branchSummary || "Uses default reply handling"}</div>
        <div class="outbound-card-meta">${item.thread_mode === "same_thread" ? "Same thread" : "New thread"} • wait ${Number(item.wait_interval_minutes || 0)} minutes</div>
        <div class="outbound-card-actions"></div>
      `;
      card.addEventListener("click", () => {
        state.selectedStepID = item.id;
        fillStepForm(item);
        renderStepList();
      });
      const actions = card.querySelector(".outbound-card-actions");
      [
        { label: "Up", disabled: index === 0, onClick: () => moveStep(item.id, -1) },
        { label: "Down", disabled: index === steps.length - 1, onClick: () => moveStep(item.id, 1) },
        { label: "Delete", disabled: false, onClick: () => deleteStep(item.id) },
      ].forEach((action) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "cmd-btn cmd-btn--dense cmd-btn--ghost";
        btn.textContent = action.label;
        btn.disabled = action.disabled;
        btn.addEventListener("click", (event) => {
          event.stopPropagation();
          action.onClick();
        });
        actions.appendChild(btn);
      });
      el.outboundStepList.appendChild(card);
    });
}

function renderAudiencePreview() {
  el.outboundAudiencePreviewList.replaceChildren();
  if (!state.audiencePreview.length) {
    const empty = document.createElement("div");
    empty.className = "mock-empty";
    empty.textContent = "Preview recipients here before importing them into the campaign.";
    el.outboundAudiencePreviewList.appendChild(empty);
    return;
  }
  state.audiencePreview.forEach((item) => {
    const sender = senderByID(item.preferred_sender_id);
    const card = document.createElement("div");
    card.className = "outbound-card";
    const status = item.suppressed ? "stopped" : item.active_elsewhere ? "paused" : "running";
    card.innerHTML = `
      <div class="outbound-card-head">
        <strong class="outbound-card-title">${item.recipient_name || item.recipient_email}</strong>
        <span class="${toneClass(status)}">${item.suppressed ? "Suppressed" : item.active_elsewhere ? "Active Elsewhere" : "Ready"}</span>
      </div>
      <div class="outbound-card-meta">${item.recipient_email}</div>
      <div class="outbound-card-meta">${sender ? `Preferred sender: ${sender.name}` : "No sender preference stored."}</div>
      <div class="outbound-card-meta">${item.existing_thread ? `Thread: ${item.seed_thread_subject || item.seed_thread_id || "Historical mailbox thread"}` : "New-thread enrollment preview."}</div>
      <div class="outbound-card-meta">${item.existing_thread ? `Mailbox owner: ${item.seed_mailbox || accountByID(item.seed_account_id)?.display_name || item.seed_account_id}` : "No seeded mailbox context required."}</div>
    `;
    el.outboundAudiencePreviewList.appendChild(card);
  });
}

function renderEnrollments() {
  el.outboundEnrollmentList.replaceChildren();
  const items = currentEnrollments();
  if (!items.length) {
    const empty = document.createElement("div");
    empty.className = "mock-empty";
    empty.textContent = "Import recipients into this campaign to start scheduling outreach.";
    el.outboundEnrollmentList.appendChild(empty);
    return;
  }
  items.forEach((item) => {
    const card = document.createElement("div");
    card.className = "outbound-card";
    card.innerHTML = `
      <div class="outbound-card-head">
        <strong class="outbound-card-title">${item.recipient_name || item.recipient_email}</strong>
        <span class="${toneClass(item.status)}">${humanizeStatus(item.status)}</span>
      </div>
      <div class="outbound-card-meta">${item.recipient_email}</div>
      <div class="outbound-card-meta">${item.sender_account_label || accountByID(item.sender_account_id)?.display_name || item.sender_account_id}</div>
      <div class="outbound-card-meta">${item.existing_thread ? `Thread: ${item.seed_thread_subject || item.thread_id}` : `Thread: ${item.thread_id}`}</div>
      <div class="outbound-card-meta">${item.next_action_at ? `Next action: ${formatDateTime(item.next_action_at)}` : "No next action scheduled."}</div>
      <div class="outbound-card-actions"></div>
    `;
    const actions = card.querySelector(".outbound-card-actions");
    [
      { label: "Open Thread", onClick: () => openEnrollmentThread(item) },
      { label: item.status === "paused" ? "Resume" : "Pause", onClick: () => updateEnrollmentStatus(item.id, item.status === "paused" ? "waiting_reply" : "paused") },
      { label: "Stop", onClick: () => updateEnrollmentStatus(item.id, "stopped") },
      { label: "Take Over", onClick: () => updateEnrollmentStatus(item.id, "manual_only") },
    ].forEach((action) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "cmd-btn cmd-btn--dense cmd-btn--ghost";
      btn.textContent = action.label;
      btn.addEventListener("click", (event) => {
        event.stopPropagation();
        action.onClick();
      });
      actions.appendChild(btn);
    });
    el.outboundEnrollmentList.appendChild(card);
  });
}

function renderSenderDiagnostics() {
  el.outboundSenderDiagnostics.replaceChildren();
  state.senderDiagnostics.forEach((item) => {
    const sender = senderByID(item.sender_id);
    const card = document.createElement("div");
    card.className = "diagnostic-card";
    const head = document.createElement("div");
    head.className = "diagnostic-card-head";
    const title = document.createElement("strong");
    title.className = "diagnostic-card-title";
    title.textContent = sender?.name || item.sender_id;
    const chip = document.createElement("span");
    chip.className = toneClass(item.status);
    chip.textContent = providerLabel(item.provider_type);
    head.append(title, chip);
    const stack = document.createElement("div");
    stack.className = "diagnostic-card-stack";
    [
      `${Number(item.sent_today || 0)} sent today • cap ${Number(item.daily_cap || 0)}`,
      `${Number(item.recommended_daily_cap || 0)} daily • ${Number(item.recommended_hourly_cap || 0)} hourly • ${Number(item.recommended_gap_seconds || 0)}s gap`,
      `Reply topology: ${item.reply_topology}`,
    ].forEach((line) => {
      const row = document.createElement("div");
      row.className = "diagnostic-card-meta";
      row.textContent = line;
      stack.appendChild(row);
    });
    card.append(head, stack);
    el.outboundSenderDiagnostics.appendChild(card);
  });
}

function renderDomainDiagnostics() {
  el.outboundDomainDiagnostics.replaceChildren();
  state.domainDiagnostics.forEach((item) => {
    const card = document.createElement("div");
    card.className = "diagnostic-card";
    const head = document.createElement("div");
    head.className = "diagnostic-card-head";
    const title = document.createElement("strong");
    title.className = "diagnostic-card-title";
    title.textContent = item.domain;
    const chip = document.createElement("span");
    chip.className = toneClass(item.suppressed ? "stopped" : "running");
    chip.textContent = item.suppressed ? "Suppressed" : "Live";
    head.append(title, chip);
    const stack = document.createElement("div");
    stack.className = "diagnostic-card-stack";
    [
      `${Number(item.active_enrollments || 0)} active enrollments • cap ${Number(item.domain_cap || 0) || "none"}`,
      item.last_event,
    ].forEach((line) => {
      const row = document.createElement("div");
      row.className = "diagnostic-card-meta";
      row.textContent = line;
      stack.appendChild(row);
    });
    card.append(head, stack);
    el.outboundDomainDiagnostics.appendChild(card);
  });
}

function renderEvents() {
  el.outboundEventList.replaceChildren();
  const items = currentEvents();
  if (!items.length) {
    const empty = document.createElement("div");
    empty.className = "mock-empty";
    empty.textContent = "Campaign events will appear here as enrollments, replies, and safety actions occur.";
    el.outboundEventList.appendChild(empty);
    return;
  }
  items.forEach((item) => {
    const card = document.createElement("div");
    card.className = "outbound-card";
    card.innerHTML = `
      <div class="outbound-card-head">
        <strong class="outbound-card-title">${humanizeStatus(item.type)}</strong>
        <span class="outbound-chip">${formatDateTime(item.created_at)}</span>
      </div>
      <div class="outbound-card-meta">${item.summary}</div>
    `;
    el.outboundEventList.appendChild(card);
  });
}

function renderReplyBuckets() {
  el.replyOpsBuckets.replaceChildren();
  replyBucketOrder().forEach((bucket) => {
    const count = state.replyOpsItems.filter((item) => item.bucket === bucket).length;
    if (!count) return;
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "cmd-btn cmd-btn--dense cmd-btn--ghost reply-ops-bucket-btn";
    if (bucket === state.selectedBucket) btn.classList.add("is-active");
    btn.textContent = `${humanizeBucket(bucket)} (${count})`;
    btn.addEventListener("click", () => {
      state.selectedBucket = bucket;
      const next = currentReplyItems()[0];
      state.selectedReplyID = next?.id || "";
      renderReplyBuckets();
      renderReplyList();
      renderReplyDetail();
      setStatus(`Focused Reply Ops bucket: ${humanizeBucket(bucket)}`, "info");
    });
    el.replyOpsBuckets.appendChild(btn);
  });
}

function renderReplyList() {
  el.replyOpsList.replaceChildren();
  const items = currentReplyItems();
  if (!items.length) {
    const empty = document.createElement("div");
    empty.className = "mock-empty";
    empty.textContent = "No replies currently sit in this bucket.";
    el.replyOpsList.appendChild(empty);
    return;
  }
  items.forEach((item) => {
    const branchTarget = resolveBranchTarget(item.campaign_id, item.reply_outcome);
    const card = document.createElement("button");
    card.type = "button";
    card.className = "reply-ops-card";
    if (item.id === state.selectedReplyID) card.classList.add("is-active");
    card.innerHTML = `
      <div class="reply-ops-card-head">
        <strong class="reply-ops-card-title">${item.recipient_name || item.recipient_email}</strong>
        <span class="${toneClass(item.reply_outcome || item.bucket)}">${humanizeOutcome(item.reply_outcome)}</span>
      </div>
      <div class="reply-ops-card-meta">${[item.campaign_name, item.subject, formatDateTime(item.received_at)].filter(Boolean).join(" • ")}</div>
      <div class="reply-ops-card-meta">${item.preview}</div>
      <div class="reply-ops-card-meta">${branchTarget ? `Next move: ${branchTargetLabel(branchTarget)}` : "Next move: manual review"}</div>
    `;
    card.addEventListener("click", () => {
      state.selectedReplyID = item.id;
      renderReplyList();
      renderReplyDetail();
    });
    el.replyOpsList.appendChild(card);
  });
}

function renderReplyDetail() {
  el.replyOpsDetail.replaceChildren();
  const item = currentReplyItem();
  if (!item) {
    const empty = document.createElement("div");
    empty.className = "mock-empty";
    empty.textContent = "Choose a reply to inspect its thread, mailbox route, and action state.";
    el.replyOpsDetail.appendChild(empty);
    return;
  }
  const senderAccount = accountByID(item.sender_account_id);
  const replyAccount = accountByID(item.reply_account_id);
  const funnel = funnelByID(item.reply_funnel_id);
  const branchTarget = resolveBranchTarget(item.campaign_id, item.reply_outcome);
  const branchDetail = branchTarget ? branchTargetLabel(branchTarget) : "Manual review";
  const enrollment = Object.values(state.enrollmentsByCampaign).flat().find((entry) => entry.id === item.enrollment_id);
  const campaign = state.campaigns.find((entry) => entry.id === item.campaign_id);
  const policy = campaign?.governance_policy || defaultGovernancePolicy();
  const routeSummary = replyAccount && senderAccount && replyAccount.id !== senderAccount.id
    ? `Reply arrived in ${replyAccount.login}. Continue from ${senderAccount.login}.`
    : `Continue in ${senderAccount?.login || item.sender_account_id}.`;
  const primaryAction = (() => {
    const outcome = String(item.reply_outcome || "").trim().toLowerCase();
    const status = String(enrollment?.status || item.status || "").trim().toLowerCase();
    if (outcome === "unsubscribe_request" || status === "unsubscribed") {
      return "This recipient is suppressed. No further follow-up should be sent.";
    }
    if (outcome === "positive_interest") {
      return "Interest is confirmed. Continue the conversation manually from the working sender mailbox.";
    }
    if (outcome === "out_of_office" || status === "paused") {
      return "This conversation is paused for timing. Resume only when it makes sense.";
    }
    if (outcome === "not_interested" || outcome === "hostile" || outcome === "bounce" || status === "stopped") {
      return "Further outreach should stop unless an operator deliberately overrides it.";
    }
    return "Review the thread by hand before the sequence continues.";
  })();
  const summary = document.createElement("div");
  summary.className = "reply-ops-summary";
  const head = document.createElement("div");
  head.className = "reply-ops-summary-head";
  const copy = document.createElement("div");
  copy.className = "reply-ops-summary-copy";
  const title = document.createElement("h4");
  title.className = "reply-ops-summary-title";
  title.textContent = item.recipient_name || item.recipient_email;
  const subtitle = document.createElement("div");
  subtitle.className = "reply-ops-summary-subtitle";
  subtitle.textContent = [item.campaign_name, item.subject, formatDateTime(item.received_at)].filter(Boolean).join(" • ");
  copy.append(title, subtitle);
  const chips = document.createElement("div");
  chips.className = "reply-ops-summary-chips";
  const bucketChip = document.createElement("span");
  bucketChip.className = toneClass(item.bucket);
  bucketChip.textContent = humanizeBucket(item.bucket);
  chips.appendChild(bucketChip);
  const stateChip = document.createElement("span");
  stateChip.className = "outbound-chip";
  stateChip.textContent = `Next: ${humanizeStatus(enrollment?.status || "manual_only")}`;
  chips.appendChild(stateChip);
  head.append(copy, chips);
  summary.appendChild(head);
  const callout = document.createElement("div");
  callout.className = "reply-ops-callout";
  callout.textContent = primaryAction;
  summary.appendChild(callout);
  const grid = document.createElement("div");
  grid.className = "reply-ops-grid";
  const routeCard = document.createElement("div");
  routeCard.className = "reply-ops-detail-card";
  routeCard.innerHTML = `<div class="reply-ops-detail-card-title">Where to respond</div><div class="reply-ops-detail-card-value">${routeSummary}</div>`;
  const nextCard = document.createElement("div");
  nextCard.className = "reply-ops-detail-card";
  nextCard.innerHTML = `<div class="reply-ops-detail-card-title">What happens next</div><div class="reply-ops-detail-card-value">${branchDetail}</div>`;
  grid.append(routeCard, nextCard);
  summary.appendChild(grid);
  const advanced = document.createElement("details");
  advanced.className = "reply-ops-advanced";
  const advancedSummary = document.createElement("summary");
  advancedSummary.textContent = "More details";
  advanced.appendChild(advancedSummary);
  const advancedGrid = document.createElement("div");
  advancedGrid.className = "reply-ops-advanced-grid";
  [
    ["Sender mailbox", senderAccount?.login || item.sender_account_id],
    ["Reply mailbox", replyAccount?.login || item.reply_account_id],
    ["Reply funnel", funnel?.name || "Direct reply path"],
    ["Outcome", humanizeOutcome(item.reply_outcome)],
    ["Policy on positive reply", humanizeGovernanceAction(policy.positive_domain_action)],
    ["Policy on unsubscribe", humanizeGovernanceAction(policy.unsubscribe_domain_action)],
    ["Thread binding", `${item.thread_id} -> ${item.message_id}`],
    ["Seeded thread", enrollment?.seed_thread_subject || "Not an existing-thread enrollment"],
    ["Last action", item.action_note],
  ].forEach(([label, value]) => {
    const row = document.createElement("div");
    row.className = "reply-ops-detail-line";
    row.innerHTML = `<strong>${label}:</strong> ${value}`;
    advancedGrid.appendChild(row);
  });
  advanced.appendChild(advancedGrid);
  const body = document.createElement("div");
  body.className = "mock-reply-body";
  body.textContent = item.body;
  el.replyOpsDetail.appendChild(summary);
  el.replyOpsDetail.appendChild(advanced);
  el.replyOpsDetail.appendChild(body);
}

function renderAll() {
  recomputeAllCampaignMetrics();
  renderTopology();
  renderOutboundNote();
  renderReplyOpsNote();
  renderOutboundSummary();
  renderCampaignList();
  setActiveOutboundSection(state.activeOutboundSection);
  renderSenderControl();
  renderStepList();
  renderAudiencePreview();
  renderEnrollments();
  renderSenderDiagnostics();
  renderDomainDiagnostics();
  renderEvents();
  renderReplyBuckets();
  renderReplyList();
  renderReplyDetail();
}

function collectCampaignForm() {
  return {
    name: String(el.outboundCampaignName.value || "").trim(),
    playbook_key: String(el.outboundCampaignPlaybook.value || "").trim(),
    goal_kind: String(el.outboundCampaignGoal.value || "revive_thread").trim(),
    campaign_mode: String(el.outboundCampaignMode.value || "new_threads").trim(),
    audience_source_kind: String(el.outboundCampaignAudienceKind.value || "manual").trim(),
    audience_source_ref: String(el.outboundCampaignAudienceRef.value || "").trim(),
    sender_policy_kind: String(el.outboundCampaignSenderKind.value || "preferred_sender").trim(),
    sender_policy_ref: String(el.outboundCampaignSenderKind.value || "").trim() === "thread_owner" ? "" : String(el.outboundCampaignSenderRef.value || "").trim(),
    reply_policy: {
      stop_on_reply: !!el.outboundReplyStop.checked,
      pause_on_question: !!el.outboundReplyQuestion.checked,
      stop_same_domain_on_reply: !!el.outboundReplyDomain.checked,
    },
    compliance_policy: {
      unsubscribe_required: !!el.outboundComplianceUnsubscribe.checked,
      promotional: !!el.outboundCompliancePromotional.checked,
    },
    suppression_policy: {
      same_domain_unsubscribe_suppress: !!el.outboundSuppressionDomain.checked,
    },
    governance_policy: {
      recipient_collision_mode: String(el.outboundGovernanceRecipientCollision.value || "warn").trim(),
      domain_collision_mode: String(el.outboundGovernanceDomainCollision.value || "warn").trim(),
      max_active_per_domain: Math.max(0, Number(el.outboundGovernanceDomainCap.value || "0") || 0),
      positive_domain_action: String(el.outboundGovernancePositiveAction.value || "none").trim(),
      negative_domain_action: String(el.outboundGovernanceNegativeAction.value || "none").trim(),
      unsubscribe_domain_action: String(el.outboundGovernanceUnsubAction.value || "suppress_workspace").trim(),
    },
  };
}

function collectStepForm() {
  return {
    position: Math.max(1, Number(el.outboundStepPosition.value || "1") || 1),
    kind: String(el.outboundStepKind.value || "email").trim(),
    thread_mode: String(el.outboundStepThreadMode.value || "same_thread").trim(),
    wait_interval_minutes: Math.max(0, Number(el.outboundStepWait.value || "0") || 0),
    subject_template: String(el.outboundStepSubject.value || "").trim(),
    body_template: String(el.outboundStepBody.value || ""),
    task_policy: {
      title: String(el.outboundStepTaskTitle.value || "").trim(),
      instructions: String(el.outboundStepTaskInstructions.value || "").trim(),
      action_label: String(el.outboundStepTaskActionLabel.value || "").trim(),
    },
    branch_policy: {
      question: String(el.outboundStepBranchQuestion.value || "").trim(),
      objection: String(el.outboundStepBranchObjection.value || "").trim(),
      wrong_person: String(el.outboundStepBranchReferral.value || "").trim(),
      out_of_office: String(el.outboundStepBranchOOO.value || "").trim(),
      manual_review_required: String(el.outboundStepBranchReview.value || "").trim(),
    },
  };
}

function audienceKey(kind, ref) {
  return `${String(kind || "manual").trim()}:${String(ref || "").trim()}`;
}

function parseAudienceText(text) {
  const lines = String(text || "")
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const rows = [];
  lines.forEach((line) => {
    if (/^email\s*,/i.test(line)) return;
    const parts = line.split(",").map((part) => part.trim()).filter(Boolean);
    if (!parts.length) return;
    const email = parts[0];
    const name = parts[1] || "";
    if (!email.includes("@")) return;
    rows.push({ recipient_name: name, recipient_email: email });
  });
  return rows;
}

function activeInAnotherCampaign(email, currentID) {
  return state.campaigns.some((campaign) => {
    if (campaign.id === currentID) return false;
    return (state.enrollmentsByCampaign[campaign.id] || []).some((item) => item.recipient_email === email && !["stopped", "completed", "unsubscribed", "bounced"].includes(item.status));
  });
}

function previewAudience() {
  const campaign = currentCampaign();
  const kind = String(el.outboundAudienceKind.value || "manual").trim();
  const ref = String(el.outboundAudienceRef.value || "").trim();
  const manual = parseAudienceText(el.outboundAudienceText.value);
  const base = manual.length ? manual : clone(MOCK_DATA.audienceLibrary[audienceKey(kind, ref)] || []);
  state.audiencePreview = base.map((item, index) => {
    const email = String(item.recipient_email || "").trim().toLowerCase();
    const domain = email.split("@")[1] || "";
    return {
      id: `preview_${Date.now()}_${index}`,
      recipient_name: item.recipient_name || "",
      recipient_email: email,
      recipient_domain: domain,
      preferred_sender_id: item.preferred_sender_id || "",
      active_elsewhere: item.active_elsewhere === true || activeInAnotherCampaign(email, campaign?.id),
      suppressed: item.suppressed === true || state.domainDiagnostics.some((entry) => entry.domain === domain && entry.suppressed),
      seed_account_id: item.seed_account_id || "",
      seed_thread_id: item.seed_thread_id || "",
      seed_message_id: item.seed_message_id || "",
      seed_thread_subject: item.seed_thread_subject || "",
      seed_mailbox: item.seed_mailbox || "",
      existing_thread: item.existing_thread === true || campaign?.campaign_mode === "existing_threads",
    };
  });
  renderAudiencePreview();
  setStatus(`Previewed ${state.audiencePreview.length} audience candidates.`, state.audiencePreview.length ? "ok" : "warn");
}

function campaignSenderAccountID(campaign, member, index) {
  if (!campaign) return "";
  if (campaign.sender_policy_kind === "thread_owner") {
    return member.seed_account_id || senderByID(member.preferred_sender_id)?.account_id || "acct_gmail_founder";
  }
  if (campaign.sender_policy_kind === "single_sender") {
    return senderByID(campaign.sender_policy_ref)?.account_id || campaign.sender_policy_ref;
  }
  if (campaign.sender_policy_kind === "campaign_pool") {
    const ids = String(campaign.sender_policy_ref || "")
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
    const target = ids[index % Math.max(ids.length, 1)] || "";
    return senderByID(target)?.account_id || target;
  }
  if (campaign.sender_policy_kind === "reply_funnel") {
    const funnel = funnelByID(campaign.sender_policy_ref);
    const routed = Array.isArray(funnel?.routed_sender_ids) ? funnel.routed_sender_ids : [];
    const preferred = senderByID(member.preferred_sender_id)?.account_id || "";
    if (preferred) return preferred;
    const routedSender = routed[index % Math.max(routed.length, 1)] || "";
    return senderByID(routedSender)?.account_id || "";
  }
  return senderByID(member.preferred_sender_id)?.account_id || "acct_gmail_founder";
}

function importAudience() {
  const campaign = currentCampaign();
  if (!campaign) return;
  if (!state.audiencePreview.length) {
    previewAudience();
  }
  const target = state.enrollmentsByCampaign[campaign.id] || [];
  let created = 0;
  state.audiencePreview.forEach((member, index) => {
    if (target.some((item) => item.recipient_email === member.recipient_email)) return;
    created += 1;
    const senderAccountID = campaignSenderAccountID(campaign, member, index);
    const senderAccount = accountByID(senderAccountID);
    target.push({
      id: `enr_${campaign.id}_${Date.now()}_${index}`,
      campaign_id: campaign.id,
      recipient_name: member.recipient_name,
      recipient_email: member.recipient_email,
      recipient_domain: member.recipient_domain,
      status: member.suppressed ? "stopped" : member.active_elsewhere ? "paused" : campaign.status === "running" ? "scheduled" : "scheduled",
      sender_account_id: senderAccountID,
      sender_account_label: senderAccount?.display_name || senderAccountID,
      sender_id: member.preferred_sender_id || "",
      reply_funnel_id: campaign.sender_policy_kind === "reply_funnel" ? campaign.sender_policy_ref : "",
      thread_id: member.seed_thread_id || `thr_${member.recipient_domain.replace(/[^a-z0-9]/g, "_")}_${index}`,
      last_sent_message_id: campaign.status === "running" ? `msg_send_${member.recipient_domain}_${index}` : "",
      last_reply_message_id: "",
      next_action_at: campaign.status === "running" ? new Date(Date.now() + 86400000).toISOString() : new Date(Date.now() + 3600000).toISOString(),
      seed_account_id: member.seed_account_id || "",
      seed_thread_id: member.seed_thread_id || "",
      seed_message_id: member.seed_message_id || "",
      seed_thread_subject: member.seed_thread_subject || "",
      seed_mailbox: member.seed_mailbox || "",
      existing_thread: member.existing_thread === true,
    });
  });
  state.enrollmentsByCampaign[campaign.id] = target;
  appendEvent(campaign.id, "enrolled", `Imported ${created} recipients from ${campaign.audience_source_kind}.`);
  recomputeCampaignMetrics(campaign.id);
  renderAll();
  setStatus(`Audience imported: ${created} created.`, created ? "ok" : "warn");
}

function appendEvent(campaignID, type, summary) {
  if (!state.eventsByCampaign[campaignID]) state.eventsByCampaign[campaignID] = [];
  state.eventsByCampaign[campaignID].unshift({
    id: `evt_${Date.now()}_${Math.random().toString(16).slice(2)}`,
    type,
    summary,
    created_at: new Date().toISOString(),
  });
}

function outboundPreflightIssues(campaign) {
  const issues = [];
  if (!campaign?.name) issues.push({ severity: "blocking", message: "Campaign needs a name." });
  if (!String(campaign?.sender_policy_ref || "").trim() && !["preferred_sender", "thread_owner"].includes(campaign?.sender_policy_kind)) {
    issues.push({ severity: "blocking", message: "Chosen sender policy needs a sender or funnel reference." });
  }
  if (!(state.stepsByCampaign[campaign?.id] || []).length) {
    issues.push({ severity: "blocking", message: "Campaign must contain at least one step." });
  }
  if (!(state.enrollmentsByCampaign[campaign?.id] || []).length) {
    issues.push({ severity: "blocking", message: "Campaign must contain at least one enrolled recipient." });
  }
  if (campaign?.compliance_policy?.promotional && campaign?.compliance_policy?.unsubscribe_required === false) {
    issues.push({ severity: "blocking", message: "Promotional campaigns must keep unsubscribe handling enabled." });
  }
  if ((state.stepsByCampaign[campaign?.id] || []).some((item) => item.kind === "manual_task" && !String(item.task_policy?.title || "").trim())) {
    issues.push({ severity: "blocking", message: "Manual-task steps need a visible task title." });
  }
  if (campaign?.campaign_mode === "existing_threads" && currentEnrollments().some((item) => !item.seed_thread_id)) {
    issues.push({ severity: "blocking", message: "Existing-thread campaigns need seeded mailbox threads for every enrollment." });
  }
  if ((state.enrollmentsByCampaign[campaign?.id] || []).some((item) => item.status === "paused")) {
    issues.push({ severity: "warning", message: "One or more recipients are already paused before launch." });
  }
  if (campaign?.sender_policy_kind === "reply_funnel") {
    issues.push({ severity: "info", message: "Replies will enter the collector inbox first, then reopen in the original sender context from Reply Ops." });
  }
  if (campaign?.sender_policy_kind === "thread_owner") {
    issues.push({ severity: "info", message: "This campaign will send from the seeded historical mailbox owner for each recipient." });
  }
  if (Number(campaign?.governance_policy?.max_active_per_domain || 0) > 0) {
    issues.push({ severity: "info", message: `Domain rail is capped at ${Number(campaign.governance_policy.max_active_per_domain)} active recipients.` });
  }
  return issues;
}

function runPreflight() {
  const campaign = currentCampaign();
  const issues = outboundPreflightIssues(campaign);
  const doc = issues.length
    ? issues.map((item, index) => `${index + 1}. [${item.severity.toUpperCase()}] ${item.message}`).join("\n")
    : "No issues found.";
  showModal({
    kicker: "Outbound Preflight",
    title: campaign?.name || "Selected campaign",
    body: "This mirrors the safety review surface before launch.",
    doc,
  });
  setStatus(issues.some((item) => item.severity === "blocking") ? "Preflight found blocking issues." : "Preflight completed.", issues.some((item) => item.severity === "blocking") ? "warn" : "ok");
}

function saveCampaign() {
  const campaign = currentCampaign();
  if (!campaign) return;
  Object.assign(campaign, collectCampaignForm());
  appendEvent(campaign.id, "updated", `Campaign settings saved for ${campaign.name}.`);
  renderAll();
  setStatus(`Campaign saved: ${campaign.name}`, "ok");
}

function launchCampaign() {
  const campaign = currentCampaign();
  if (!campaign) return;
  const issues = outboundPreflightIssues(campaign);
  if (issues.some((item) => item.severity === "blocking")) {
    runPreflight();
    return;
  }
  campaign.status = "running";
  currentEnrollments().forEach((item) => {
    if (item.status === "scheduled") {
      item.status = "waiting_reply";
      item.last_sent_message_id = item.last_sent_message_id || `msg_send_${item.id}`;
    }
  });
  appendEvent(campaign.id, "launched", `Campaign launched with ${currentEnrollments().length} enrolled recipients.`);
  recomputeCampaignMetrics(campaign.id);
  renderAll();
  setStatus(`Campaign launched: ${campaign.name}`, "ok");
}

function updateCampaignStatus(status) {
  const campaign = currentCampaign();
  if (!campaign) return;
  campaign.status = status;
  appendEvent(campaign.id, status, `Campaign status changed to ${humanizeStatus(status)}.`);
  renderAll();
  setStatus(`Campaign is now ${humanizeStatus(status)}.`, status === "archived" ? "warn" : "ok");
}

function newCampaign() {
  const playbook = playbookByKey("revive_existing_threads");
  const next = {
    id: `camp_${Date.now()}`,
    name: "New External-Mailbox Campaign",
    status: "draft",
    playbook_key: playbook?.key || "",
    goal_kind: playbook?.goal_kind || "revive_thread",
    campaign_mode: playbook?.campaign_mode || "existing_threads",
    audience_source_kind: "manual",
    audience_source_ref: "",
    sender_policy_kind: playbook?.sender_policy_kind || "thread_owner",
    sender_policy_ref: "",
    reply_policy: clone(playbook?.reply_policy || { stop_on_reply: true, pause_on_question: true, stop_same_domain_on_reply: false }),
    compliance_policy: clone(playbook?.compliance_policy || { unsubscribe_required: true, promotional: false }),
    suppression_policy: clone(playbook?.suppression_policy || { same_domain_unsubscribe_suppress: true }),
    governance_policy: clone(playbook?.governance_policy || defaultGovernancePolicy()),
    enrollment_count: 0,
    sent_count: 0,
    replied_count: 0,
    waiting_human_count: 0,
  };
  state.campaigns.unshift(next);
  state.stepsByCampaign[next.id] = [];
  state.enrollmentsByCampaign[next.id] = [];
  state.eventsByCampaign[next.id] = [];
  state.selectedCampaignID = next.id;
  state.selectedStepID = "";
  state.audiencePreview = [];
  state.activeOutboundSection = "strategy";
  fillCampaignForm();
  fillAudienceForm();
  fillStepForm(null);
  renderAll();
  setStatus("Created a new draft campaign.", "ok");
}

function saveStep() {
  const campaign = currentCampaign();
  if (!campaign) return;
  const payload = collectStepForm();
  const steps = state.stepsByCampaign[campaign.id] || [];
  if (state.selectedStepID) {
    const step = steps.find((item) => item.id === state.selectedStepID);
    if (step) Object.assign(step, payload);
  } else {
    const step = { id: `step_${Date.now()}`, ...payload };
    steps.push(step);
    state.selectedStepID = step.id;
  }
  steps.sort((a, b) => Number(a.position || 0) - Number(b.position || 0));
  state.stepsByCampaign[campaign.id] = steps;
  fillStepForm(steps.find((item) => item.id === state.selectedStepID) || null);
  appendEvent(campaign.id, "step_saved", `Sequence now contains ${steps.length} steps.`);
  syncBranchOptions(payload.branch_policy || {});
  renderAll();
  setStatus("Step saved.", "ok");
}

function newStep() {
  state.selectedStepID = "";
  state.activeOutboundSection = "strategy";
  fillStepForm(null);
  renderAll();
  setStatus("Editing a new step.", "info");
}

function moveStep(stepID, delta) {
  const steps = currentSteps().slice().sort((a, b) => Number(a.position || 0) - Number(b.position || 0));
  const index = steps.findIndex((item) => item.id === stepID);
  const target = index + delta;
  if (index < 0 || target < 0 || target >= steps.length) return;
  [steps[index], steps[target]] = [steps[target], steps[index]];
  steps.forEach((item, position) => { item.position = position + 1; });
  state.stepsByCampaign[state.selectedCampaignID] = steps;
  fillStepForm(steps.find((item) => item.id === state.selectedStepID) || null);
  appendEvent(state.selectedCampaignID, "step_reordered", "Sequence order updated.");
  renderAll();
  setStatus("Step order updated.", "ok");
}

function deleteStep(stepID) {
  const steps = currentSteps().filter((item) => item.id !== stepID);
  steps.forEach((item, index) => { item.position = index + 1; });
  state.stepsByCampaign[state.selectedCampaignID] = steps;
  state.selectedStepID = steps[0]?.id || "";
  fillStepForm();
  appendEvent(state.selectedCampaignID, "step_deleted", "A step was removed from the sequence.");
  renderAll();
  setStatus("Step deleted.", "warn");
}

function updateEnrollmentStatus(enrollmentID, status) {
  const items = currentEnrollments();
  const target = items.find((item) => item.id === enrollmentID);
  if (!target) return;
  target.status = status;
  if (status === "manual_only") target.next_action_at = "";
  if (status === "paused" && !target.next_action_at) target.next_action_at = new Date(Date.now() + 172800000).toISOString();
  appendEvent(target.campaign_id, "enrollment_updated", `${target.recipient_email} -> ${humanizeStatus(status)}.`);
  recomputeCampaignMetrics(target.campaign_id);
  renderAll();
  setStatus(`Recipient updated: ${target.recipient_email} -> ${humanizeStatus(status)}`, "ok");
}

function openEnrollmentThread(item) {
  const replyPath = item.reply_funnel_id ? `collector ${accountByID("acct_collector")?.login}` : `${item.sender_account_label}`;
  const seed = item.existing_thread ? ` using seeded thread "${item.seed_thread_subject || item.thread_id}"` : "";
  setStatus(`Thread open would hydrate ${item.thread_id}${seed} and jump from ${replyPath} back into the original sender-thread context.`, "ok");
}

function classifyReply(outcome) {
  const item = currentReplyItem();
  if (!item) return;
  item.reply_outcome = outcome;
  item.bucket = outcomeToBucket(outcome);
  const branchTarget = resolveBranchTarget(item.campaign_id, outcome);
  item.action_note = branchTarget
    ? `Classified as ${humanizeOutcome(outcome)} and routed into ${branchTargetLabel(branchTarget)}.`
    : `Classified as ${humanizeOutcome(outcome)}.`;
  const enrollment = Object.values(state.enrollmentsByCampaign).flat().find((entry) => entry.id === item.enrollment_id);
  if (enrollment) {
    const statusMap = {
      bounce: "bounced",
      hostile: "stopped",
      manual_review_required: "manual_only",
      not_interested: "stopped",
      objection: "manual_only",
      out_of_office: "paused",
      positive_interest: "completed",
      question: "manual_only",
      unsubscribe_request: "unsubscribed",
      wrong_person: "stopped",
    };
    enrollment.status = statusMap[outcome] || enrollment.status;
    enrollment.last_reply_message_id = enrollment.last_reply_message_id || item.message_id;
    if (branchTarget && branchTarget.startsWith("manual_task:")) {
      enrollment.status = "manual_only";
      enrollment.next_action_at = "";
    }
    recomputeCampaignMetrics(enrollment.campaign_id);
    appendEvent(enrollment.campaign_id, "reply_classified", `${item.recipient_email} classified as ${humanizeOutcome(outcome)}.`);
  }
  const nextBucketItems = currentReplyItems();
  if (!nextBucketItems.some((entry) => entry.id === state.selectedReplyID)) {
    state.selectedReplyID = nextBucketItems[0]?.id || state.replyOpsItems[0]?.id || "";
  }
  renderAll();
  setStatus(`Reply classified as ${humanizeOutcome(outcome)}.`, "ok");
}

function suppressScope(scope) {
  const item = currentReplyItem();
  if (!item) return;
  if (scope === "recipient") {
    const enrollment = Object.values(state.enrollmentsByCampaign).flat().find((entry) => entry.id === item.enrollment_id);
    if (enrollment) enrollment.status = "unsubscribed";
    item.bucket = "unsubscribed";
    item.reply_outcome = "unsubscribe_request";
    appendEvent(item.campaign_id, "suppressed", `${item.recipient_email} suppressed at recipient scope.`);
    recomputeCampaignMetrics(item.campaign_id);
    renderAll();
    setStatus(`Recipient suppression applied for ${item.recipient_email}.`, "warn");
    return;
  }
  const domain = String(item.recipient_email || "").split("@")[1] || "";
  const record = state.domainDiagnostics.find((entry) => entry.domain === domain);
  if (record) {
    record.suppressed = true;
    record.last_event = "Domain suppression applied from Reply Ops";
  } else {
    state.domainDiagnostics.unshift({ id: `diag_${Date.now()}`, domain, active_enrollments: 0, suppressed: true, last_event: "Domain suppression applied from Reply Ops" });
  }
  appendEvent(item.campaign_id, "suppressed", `${domain} suppressed at domain scope.`);
  renderAll();
  setStatus(`Domain suppression applied for ${domain}.`, "warn");
}

function pauseReplyOpsItem() {
  const item = currentReplyItem();
  if (!item) return;
  const enrollment = Object.values(state.enrollmentsByCampaign).flat().find((entry) => entry.id === item.enrollment_id);
  if (enrollment) {
    enrollment.status = "paused";
    enrollment.next_action_at = new Date(Date.now() + 5 * 86400000).toISOString();
    appendEvent(enrollment.campaign_id, "paused", `${item.recipient_email} paused from Reply Ops until a later date.`);
    recomputeCampaignMetrics(enrollment.campaign_id);
  }
  item.bucket = "needs_review";
  item.action_note = "Paused manually from Reply Ops.";
  renderAll();
  setStatus(`Reply paused for ${item.recipient_email}.`, "ok");
}

function resumeReplyOpsItem() {
  const item = currentReplyItem();
  if (!item) return;
  const enrollment = Object.values(state.enrollmentsByCampaign).flat().find((entry) => entry.id === item.enrollment_id);
  if (enrollment) {
    enrollment.status = "waiting_reply";
    enrollment.next_action_at = new Date(Date.now() + 86400000).toISOString();
    appendEvent(enrollment.campaign_id, "resumed", `${item.recipient_email} resumed from Reply Ops.`);
    recomputeCampaignMetrics(enrollment.campaign_id);
  }
  item.bucket = "needs_review";
  item.action_note = "Returned to waiting state after manual review.";
  renderAll();
  setStatus(`Reply resumed for ${item.recipient_email}.`, "ok");
}

function openSelectedReplyThread() {
  const item = currentReplyItem();
  if (!item) return;
  const senderMailbox = accountByID(item.sender_account_id)?.login || item.sender_account_id;
  const replyMailbox = accountByID(item.reply_account_id)?.login || item.reply_account_id;
  setStatus(`Thread open would read from ${replyMailbox} and then anchor the operator back into ${senderMailbox} for thread ${item.thread_id}.`, "ok");
}

function initEventHandlers() {
  el.themeBtn.addEventListener("click", () => setTheme(state.theme === "machine-dark" ? "paper-light" : "machine-dark"));
  el.tabOutbound.addEventListener("click", () => setView("outbound"));
  el.tabReplyOps.addEventListener("click", () => setView("reply-ops"));
  el.quickOpenCampaign.addEventListener("click", () => {
    state.selectedCampaignID = "camp_thread_revive";
    state.selectedStepID = "step_tr_1";
    state.audiencePreview = [];
    state.activeOutboundSection = "strategy";
    setView("outbound");
    fillCampaignForm();
    fillAudienceForm();
    fillStepForm();
    renderAll();
    setStatus("Focused the existing-thread revival campaign.", "ok");
  });
  el.quickOpenQueue.addEventListener("click", () => {
    state.selectedCampaignID = "camp_collector_find_owner";
    state.selectedStepID = "step_co_1";
    state.audiencePreview = [];
    state.activeOutboundSection = "strategy";
    setView("outbound");
    fillCampaignForm();
    fillAudienceForm();
    fillStepForm();
    renderAll();
    setStatus("Focused the collector-routed external mailbox campaign.", "ok");
  });
  el.quickOpenPositive.addEventListener("click", () => {
    state.selectedBucket = "questions";
    state.selectedReplyID = "reply_thread_question";
    setView("reply-ops");
    renderAll();
    setStatus("Focused a reply that branches into a manual task.", "ok");
  });
  el.outboundCampaignPlaybook?.addEventListener("change", () => {
    applyPlaybookToForm(String(el.outboundCampaignPlaybook.value || "").trim());
    renderOutboundSummary();
  });
  el.outboundCampaignName?.addEventListener("input", renderOutboundSummary);
  el.outboundCampaignMode?.addEventListener("change", renderOutboundSummary);
  el.outboundCampaignSenderKind?.addEventListener("change", () => {
    const isThreadOwner = String(el.outboundCampaignSenderKind.value || "").trim() === "thread_owner";
    el.outboundCampaignSenderRef.disabled = isThreadOwner;
    if (isThreadOwner) el.outboundCampaignSenderRef.value = "";
    renderOutboundSummary();
  });
  el.outboundCampaignSenderRef?.addEventListener("input", renderOutboundSummary);
  el.outboundSectionStrategy?.addEventListener("click", () => {
    setActiveOutboundSection("strategy");
    renderOutboundSummary();
  });
  el.outboundSectionAudience?.addEventListener("click", () => {
    setActiveOutboundSection("audience");
    renderOutboundSummary();
  });
  el.outboundSectionHealth?.addEventListener("click", () => {
    setActiveOutboundSection("health");
    renderOutboundSummary();
  });
  el.outboundStepKind?.addEventListener("change", updateStepFormVisibility);
  el.btnOutboundRefresh.addEventListener("click", () => {
    renderAll();
    setStatus("Outbound workspace refreshed from mock state.", "ok");
  });
  el.btnOutboundNew.addEventListener("click", newCampaign);
  el.btnOutboundSave.addEventListener("click", saveCampaign);
  el.btnOutboundPreflight.addEventListener("click", runPreflight);
  el.btnOutboundLaunch.addEventListener("click", launchCampaign);
  el.btnOutboundPause.addEventListener("click", () => updateCampaignStatus("paused"));
  el.btnOutboundResume.addEventListener("click", () => updateCampaignStatus("running"));
  el.btnOutboundArchive.addEventListener("click", () => updateCampaignStatus("archived"));
  el.btnOutboundStepSave.addEventListener("click", saveStep);
  el.btnOutboundStepNew.addEventListener("click", newStep);
  el.btnOutboundAudiencePreview.addEventListener("click", previewAudience);
  el.btnOutboundAudienceImport.addEventListener("click", importAudience);
  el.btnReplyOpsRefresh.addEventListener("click", () => {
    renderAll();
    setStatus("Reply Ops queue refreshed from mock state.", "ok");
  });
  el.btnReplyOpsOpenThread.addEventListener("click", openSelectedReplyThread);
  el.btnReplyOpsTakeover.addEventListener("click", () => classifyReply("manual_review_required"));
  el.btnReplyOpsStop.addEventListener("click", () => suppressScope("recipient"));
  el.replyOpsClassifyPositive.addEventListener("click", () => classifyReply("positive_interest"));
  el.replyOpsClassifyQuestion.addEventListener("click", () => classifyReply("question"));
  el.replyOpsClassifyObjection.addEventListener("click", () => classifyReply("objection"));
  el.replyOpsClassifyWrong.addEventListener("click", () => classifyReply("wrong_person"));
  el.replyOpsClassifyNegative.addEventListener("click", () => classifyReply("not_interested"));
  el.replyOpsClassifyUnsub.addEventListener("click", () => classifyReply("unsubscribe_request"));
  el.replyOpsClassifyOOO.addEventListener("click", () => classifyReply("out_of_office"));
  el.replyOpsClassifyBounce.addEventListener("click", () => classifyReply("bounce"));
  el.replyOpsClassifyHostile.addEventListener("click", () => classifyReply("hostile"));
  el.replyOpsActionSuppressRecipient.addEventListener("click", () => suppressScope("recipient"));
  el.replyOpsActionSuppressDomain.addEventListener("click", () => suppressScope("domain"));
  el.replyOpsActionPause.addEventListener("click", pauseReplyOpsItem);
  el.replyOpsActionResume.addEventListener("click", resumeReplyOpsItem);
  el.modalClose.addEventListener("click", hideModal);
  el.modal.addEventListener("click", (event) => {
    if (event.target === el.modal) hideModal();
  });
}

function initFromQuery() {
  const view = new URLSearchParams(window.location.search).get("view");
  if (view === "reply-ops") {
    setView("reply-ops");
  }
}

function init() {
  initTheme();
  initFromQuery();
  state.selectedBucket = currentReplyItem() ? currentReplyItem().bucket : "needs_review";
  fillCampaignForm();
  fillAudienceForm();
  fillStepForm();
  initEventHandlers();
  renderAll();
  setStatus("Mockup ready. This is a live local rendering of the outbound and reply-ops wiring.", "ok");
}

init();
