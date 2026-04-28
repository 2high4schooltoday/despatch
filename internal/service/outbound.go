package service

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	netmail "net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

type outboundReplyPolicy struct {
	StopOnReply                 bool `json:"stop_on_reply"`
	PauseOnQuestion             bool `json:"pause_on_question"`
	PauseOnObjection            bool `json:"pause_on_objection"`
	PauseOnManualReview         bool `json:"pause_on_manual_review"`
	PauseOnOutOfOffice          bool `json:"pause_on_out_of_office"`
	AutoResumeOutOfOffice       bool `json:"auto_resume_out_of_office"`
	StopSameDomainOnReply       bool `json:"stop_same_domain_on_reply"`
	StopSameDomainOnPositive    bool `json:"stop_same_domain_on_positive"`
	StopSameDomainOnNegative    bool `json:"stop_same_domain_on_negative"`
	SuppressSameDomainOnHostile bool `json:"suppress_same_domain_on_hostile"`
}

type outboundSuppressionPolicy struct {
	SameDomainPositiveStop        bool `json:"same_domain_positive_stop"`
	SameDomainNegativeStop        bool `json:"same_domain_negative_stop"`
	SameDomainUnsubscribeSuppress bool `json:"same_domain_unsubscribe_suppress"`
	SameDomainHostileSuppress     bool `json:"same_domain_hostile_suppress"`
	WorkspaceSuppressUnsubscribe  bool `json:"workspace_suppress_unsubscribe"`
	WorkspaceSuppressBounce       bool `json:"workspace_suppress_bounce"`
}

type outboundSchedulePolicy struct {
	Timezone                string `json:"timezone,omitempty"`
	MaxSendsPerDay          int    `json:"max_sends_per_day,omitempty"`
	MaxSendsPerHour         int    `json:"max_sends_per_hour,omitempty"`
	MaxSendsPerDomainPerDay int    `json:"max_sends_per_domain_per_day,omitempty"`
	RespectProviderCaps     bool   `json:"respect_provider_caps,omitempty"`
}

type outboundCompliancePolicy struct {
	UnsubscribeRequired bool   `json:"unsubscribe_required"`
	UnsubscribeScope    string `json:"unsubscribe_scope,omitempty"`
	TrackingMode        string `json:"tracking_mode,omitempty"`
	Promotional         bool   `json:"promotional"`
	FooterMode          string `json:"footer_mode,omitempty"`
}

type outboundGovernancePolicy struct {
	RecipientCollisionMode  string `json:"recipient_collision_mode,omitempty"`
	DomainCollisionMode     string `json:"domain_collision_mode,omitempty"`
	MaxActivePerDomain      int    `json:"max_active_per_domain,omitempty"`
	PositiveDomainAction    string `json:"positive_domain_action,omitempty"`
	NegativeDomainAction    string `json:"negative_domain_action,omitempty"`
	UnsubscribeDomainAction string `json:"unsubscribe_domain_action,omitempty"`
	HostileDomainAction     string `json:"hostile_domain_action,omitempty"`
}

type outboundTaskPolicy struct {
	Title        string `json:"title,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	ActionLabel  string `json:"action_label,omitempty"`
}

type outboundSeedContext struct {
	AccountID     string `json:"account_id,omitempty"`
	ThreadID      string `json:"thread_id,omitempty"`
	MessageID     string `json:"message_id,omitempty"`
	ThreadSubject string `json:"thread_subject,omitempty"`
	Mailbox       string `json:"mailbox,omitempty"`
}

type outboundProviderPacing struct {
	DailyCap   int
	HourlyCap  int
	GapSeconds int
}

type outboundSenderSelection struct {
	Account            models.MailAccount
	SenderProfileID    string
	HeaderFromName     string
	HeaderFromEmail    string
	EnvelopeFrom       string
	ReplyTo            string
	ReplyAccountID     string
	ReplyFunnelID      string
	CollectorAccountID string
}

type outboundRenderedStep struct {
	Step      models.OutboundCampaignStep
	Subject   string
	Body      string
	Missing   []string
	Selection outboundSenderSelection
}

var (
	outboundTemplateTokenRx = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_\.]+)\s*\}\}`)
	outboundBounceMarkers   = []string{
		"delivery status notification",
		"delivery failure",
		"mail delivery failed",
		"undeliverable",
		"returned mail",
		"failure notice",
	}
	outboundOOOMarkers = []string{
		"out of office",
		"out-of-office",
		"automatic reply",
		"auto reply",
		"auto-reply",
		"autoreply",
		"vacation responder",
		"away from the office",
	}
	outboundUnsubscribeMarkers = []string{
		"unsubscribe",
		"remove me",
		"don't contact me",
		"do not contact me",
		"stop emailing me",
		"stop contacting me",
		"take me off",
		"не пишите мне",
	}
	outboundNotInterestedMarkers = []string{
		"not interested",
		"no interest",
		"no thanks",
		"not a fit",
		"pass on this",
		"not for us",
		"не интересно",
	}
	outboundPositiveMarkers = []string{
		"interested",
		"sounds good",
		"let's talk",
		"lets talk",
		"happy to chat",
		"looks relevant",
		"давайте обсудим",
	}
	outboundMeetingMarkers = []string{
		"meeting",
		"calendar",
		"schedule",
		"book time",
		"book a call",
		"call next week",
		"demo",
		"встреч",
		"созвон",
	}
	outboundWrongPersonMarkers = []string{
		"wrong person",
		"not the right person",
		"not the best person",
		"left the company",
		"no longer work here",
		"i'm not the right",
		"не тот человек",
	}
	outboundReferralMarkers = []string{
		"reach out to",
		"contact ",
		"speak with",
		"talk to",
		"loop in",
		"connect with",
	}
	outboundObjectionMarkers = []string{
		"budget",
		"timing",
		"already use",
		"already working with",
		"contract",
		"security review",
	}
	outboundHostileMarkers = []string{
		"stop spamming",
		"spam me",
		"fuck off",
		"leave me alone",
		"иди нах",
	}
	outboundReturnDateRx = regexp.MustCompile(`(?i)(?:until|back on|return(?:ing)? on|available on)\s+([A-Z][a-z]{2,8}\s+\d{1,2}(?:,\s*\d{4})?|\d{4}-\d{2}-\d{2}|\d{1,2}/\d{1,2}/\d{2,4})`)
)

func parseOutboundReplyPolicy(raw string) outboundReplyPolicy {
	policy := outboundReplyPolicy{
		StopOnReply:                 true,
		PauseOnQuestion:             true,
		PauseOnObjection:            true,
		PauseOnManualReview:         true,
		PauseOnOutOfOffice:          true,
		AutoResumeOutOfOffice:       true,
		StopSameDomainOnReply:       false,
		StopSameDomainOnPositive:    false,
		StopSameDomainOnNegative:    false,
		SuppressSameDomainOnHostile: true,
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return policy
	}
	_ = json.Unmarshal([]byte(trimmed), &policy)
	return policy
}

func parseOutboundSuppressionPolicy(raw string) outboundSuppressionPolicy {
	policy := outboundSuppressionPolicy{
		SameDomainPositiveStop:        false,
		SameDomainNegativeStop:        false,
		SameDomainUnsubscribeSuppress: true,
		SameDomainHostileSuppress:     true,
		WorkspaceSuppressUnsubscribe:  true,
		WorkspaceSuppressBounce:       true,
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return policy
	}
	_ = json.Unmarshal([]byte(trimmed), &policy)
	return policy
}

func parseOutboundSchedulePolicy(raw string) outboundSchedulePolicy {
	policy := outboundSchedulePolicy{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return policy
	}
	_ = json.Unmarshal([]byte(trimmed), &policy)
	return policy
}

func parseOutboundCompliancePolicy(raw string) outboundCompliancePolicy {
	policy := outboundCompliancePolicy{
		UnsubscribeScope: "recipient",
		TrackingMode:     "none",
		FooterMode:       "compliance",
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return policy
	}
	_ = json.Unmarshal([]byte(trimmed), &policy)
	if strings.TrimSpace(policy.UnsubscribeScope) == "" {
		policy.UnsubscribeScope = "recipient"
	}
	if strings.TrimSpace(policy.TrackingMode) == "" {
		policy.TrackingMode = "none"
	}
	if strings.TrimSpace(policy.FooterMode) == "" {
		policy.FooterMode = "compliance"
	}
	return policy
}

func normalizeOutboundGovernanceCollisionMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "block":
		return "block"
	default:
		return "warn"
	}
}

func normalizeOutboundGovernanceAction(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "stop_campaign":
		return "stop_campaign"
	case "stop_workspace":
		return "stop_workspace"
	case "pause_workspace":
		return "pause_workspace"
	case "suppress_campaign":
		return "suppress_campaign"
	case "suppress_workspace":
		return "suppress_workspace"
	default:
		return "none"
	}
}

func parseOutboundGovernancePolicy(raw string) outboundGovernancePolicy {
	policy := outboundGovernancePolicy{
		RecipientCollisionMode:  "warn",
		DomainCollisionMode:     "warn",
		PositiveDomainAction:    "none",
		NegativeDomainAction:    "none",
		UnsubscribeDomainAction: "suppress_workspace",
		HostileDomainAction:     "suppress_workspace",
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && trimmed != "{}" {
		_ = json.Unmarshal([]byte(trimmed), &policy)
	}
	policy.RecipientCollisionMode = normalizeOutboundGovernanceCollisionMode(policy.RecipientCollisionMode)
	policy.DomainCollisionMode = normalizeOutboundGovernanceCollisionMode(policy.DomainCollisionMode)
	policy.PositiveDomainAction = normalizeOutboundGovernanceAction(policy.PositiveDomainAction)
	policy.NegativeDomainAction = normalizeOutboundGovernanceAction(policy.NegativeDomainAction)
	policy.UnsubscribeDomainAction = normalizeOutboundGovernanceAction(policy.UnsubscribeDomainAction)
	policy.HostileDomainAction = normalizeOutboundGovernanceAction(policy.HostileDomainAction)
	if policy.MaxActivePerDomain < 0 {
		policy.MaxActivePerDomain = 0
	}
	return policy
}

func parseOutboundTaskPolicy(raw string) outboundTaskPolicy {
	policy := outboundTaskPolicy{
		ActionLabel: "Mark handled and continue",
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && trimmed != "{}" {
		_ = json.Unmarshal([]byte(trimmed), &policy)
	}
	policy.Title = strings.TrimSpace(policy.Title)
	policy.Instructions = strings.TrimSpace(policy.Instructions)
	policy.ActionLabel = firstNonEmptyString(strings.TrimSpace(policy.ActionLabel), "Mark handled and continue")
	return policy
}

func parseOutboundSeedContext(raw string) outboundSeedContext {
	var seed outboundSeedContext
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return seed
	}
	_ = json.Unmarshal([]byte(trimmed), &seed)
	seed.AccountID = strings.TrimSpace(seed.AccountID)
	seed.ThreadID = strings.TrimSpace(seed.ThreadID)
	seed.MessageID = strings.TrimSpace(seed.MessageID)
	seed.ThreadSubject = strings.TrimSpace(seed.ThreadSubject)
	seed.Mailbox = strings.TrimSpace(seed.Mailbox)
	return seed
}

func parseOutboundBranchPolicy(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return nil
	}
	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil
	}
	out := make(map[string]string, len(parsed))
	for key, value := range parsed {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		normalizedValue := strings.ToLower(strings.TrimSpace(value))
		if normalizedKey == "" || normalizedValue == "" || normalizedValue == "default" {
			continue
		}
		out[normalizedKey] = normalizedValue
	}
	return out
}

func outboundProviderPacingForAccount(account models.MailAccount) outboundProviderPacing {
	switch NormalizeMailProviderType(account.ProviderType) {
	case MailProviderTypeGmail:
		return outboundProviderPacing{DailyCap: 150, HourlyCap: 20, GapSeconds: 120}
	case MailProviderTypeLibero:
		return outboundProviderPacing{DailyCap: 100, HourlyCap: 12, GapSeconds: 240}
	default:
		return outboundProviderPacing{DailyCap: 80, HourlyCap: 10, GapSeconds: 240}
	}
}

func outboundPlaybookCatalog() []models.OutboundPlaybook {
	return []models.OutboundPlaybook{
		{
			Key:                "thread_revival",
			Name:               "Thread Revival",
			Description:        "Continue dormant Gmail, Libero, and other connected mailbox threads from the original conversation owner.",
			GoalKind:           "thread_revival",
			CampaignMode:       "existing_threads",
			AudienceSourceKind: "saved_search",
			SenderPolicyKind:   "thread_owner",
			ReplyPolicy: map[string]any{
				"stop_on_reply":             true,
				"pause_on_question":         true,
				"pause_on_objection":        true,
				"pause_on_manual_review":    true,
				"pause_on_out_of_office":    true,
				"auto_resume_out_of_office": true,
			},
			SuppressionPolicy: map[string]any{
				"same_domain_unsubscribe_suppress": true,
				"same_domain_hostile_suppress":     true,
				"workspace_suppress_unsubscribe":   true,
				"workspace_suppress_bounce":        true,
			},
			SchedulePolicy: map[string]any{
				"respect_provider_caps": true,
			},
			CompliancePolicy: map[string]any{
				"unsubscribe_required": true,
				"unsubscribe_scope":    "recipient",
				"tracking_mode":        "none",
				"footer_mode":          "compliance",
			},
			GovernancePolicy: map[string]any{
				"recipient_collision_mode":  "block",
				"domain_collision_mode":     "block",
				"max_active_per_domain":     2,
				"positive_domain_action":    "pause_workspace",
				"negative_domain_action":    "stop_workspace",
				"unsubscribe_domain_action": "suppress_workspace",
				"hostile_domain_action":     "suppress_workspace",
			},
			Steps: []models.OutboundPlaybookStep{
				{
					Position:            1,
					Kind:                "email",
					ThreadMode:          "same_thread",
					WaitIntervalMinutes: 2880,
					SubjectTemplate:     "",
					BodyTemplate:        "Hi {{first_name}},\n\nBumping this thread in case it got buried.\n\nDoes this still sit with you, or is there a better owner for it?\n\nBest,\n{{sender_name}}",
					BranchPolicy: map[string]any{
						"question":               "manual_task:2",
						"objection":              "manual_task:2",
						"wrong_person":           "manual_task:2",
						"referral":               "manual_task:2",
						"out_of_office":          "manual_task:2",
						"auto_reply_other":       "manual_task:2",
						"manual_review_required": "manual_task:2",
					},
				},
				{
					Position:            2,
					Kind:                "manual_task",
					ThreadMode:          "same_thread",
					WaitIntervalMinutes: 0,
					TaskPolicy: map[string]any{
						"title":        "Review the live thread and respond from the same mailbox",
						"instructions": "Use the original Gmail, Libero, or connected mailbox owner for continuity. Resolve questions, confirm referrals, or decide whether the thread should continue.",
						"action_label": "Handled, continue sequence",
					},
				},
				{
					Position:            3,
					Kind:                "email",
					ThreadMode:          "same_thread",
					WaitIntervalMinutes: 0,
					BodyTemplate:        "Following up once more in case this is still relevant on your side.\n\nIf there is a better owner for this thread, I would really appreciate a redirect.\n\nBest,\n{{sender_name}}",
				},
			},
		},
		{
			Key:                "find_owner",
			Name:               "Find The Right Owner",
			Description:        "Start a concise outbound thread and branch wrong-person or referral replies into a human follow-up step.",
			GoalKind:           "find_owner",
			CampaignMode:       "new_threads",
			AudienceSourceKind: "contact_group",
			SenderPolicyKind:   "reply_funnel",
			ReplyPolicy: map[string]any{
				"stop_on_reply":             true,
				"pause_on_question":         true,
				"pause_on_objection":        true,
				"pause_on_manual_review":    true,
				"pause_on_out_of_office":    true,
				"auto_resume_out_of_office": true,
			},
			SuppressionPolicy: map[string]any{
				"same_domain_unsubscribe_suppress": true,
				"same_domain_hostile_suppress":     true,
				"workspace_suppress_unsubscribe":   true,
				"workspace_suppress_bounce":        true,
			},
			SchedulePolicy: map[string]any{
				"respect_provider_caps": true,
			},
			CompliancePolicy: map[string]any{
				"unsubscribe_required": true,
				"unsubscribe_scope":    "recipient",
				"tracking_mode":        "none",
				"footer_mode":          "compliance",
			},
			GovernancePolicy: map[string]any{
				"recipient_collision_mode":  "block",
				"domain_collision_mode":     "warn",
				"max_active_per_domain":     3,
				"positive_domain_action":    "pause_workspace",
				"negative_domain_action":    "stop_workspace",
				"unsubscribe_domain_action": "suppress_workspace",
				"hostile_domain_action":     "suppress_workspace",
			},
			Steps: []models.OutboundPlaybookStep{
				{
					Position:            1,
					Kind:                "email",
					ThreadMode:          "new_thread",
					WaitIntervalMinutes: 4320,
					SubjectTemplate:     "Quick question for {{domain}}",
					BodyTemplate:        "Hi {{first_name}},\n\nCould you point me to the best person for this topic on your side?\n\nIf that is you, I can send the short version here.\n\nBest,\n{{sender_name}}",
					BranchPolicy: map[string]any{
						"wrong_person":           "manual_task:2",
						"referral":               "manual_task:2",
						"question":               "manual_task:2",
						"objection":              "manual_task:2",
						"manual_review_required": "manual_task:2",
					},
				},
				{
					Position:   2,
					Kind:       "manual_task",
					ThreadMode: "same_thread",
					TaskPolicy: map[string]any{
						"title":        "Work the reply or referral by hand",
						"instructions": "If the recipient names another owner, update contacts and continue from the right mailbox. If they ask a question, answer directly before continuing.",
						"action_label": "Handled, continue",
					},
				},
				{
					Position:     3,
					Kind:         "email",
					ThreadMode:   "same_thread",
					BodyTemplate: "Wanted to circle back in case this should go to someone else on your team.\n\nHappy to redirect if you point me to the right owner.\n\nBest,\n{{sender_name}}",
				},
			},
		},
		{
			Key:                "reengage_dormant",
			Name:               "Re-Engage Dormant Accounts",
			Description:        "Restart cold but relevant accounts with strong domain governance and a manual branch for nuanced replies.",
			GoalKind:           "reengage_dormant",
			CampaignMode:       "new_threads",
			AudienceSourceKind: "saved_search",
			SenderPolicyKind:   "campaign_pool",
			ReplyPolicy: map[string]any{
				"stop_on_reply":             true,
				"pause_on_question":         true,
				"pause_on_objection":        true,
				"pause_on_manual_review":    true,
				"pause_on_out_of_office":    true,
				"auto_resume_out_of_office": true,
			},
			SuppressionPolicy: map[string]any{
				"same_domain_unsubscribe_suppress": true,
				"same_domain_hostile_suppress":     true,
				"workspace_suppress_unsubscribe":   true,
				"workspace_suppress_bounce":        true,
			},
			SchedulePolicy: map[string]any{
				"respect_provider_caps": true,
			},
			CompliancePolicy: map[string]any{
				"unsubscribe_required": true,
				"unsubscribe_scope":    "recipient",
				"tracking_mode":        "none",
				"footer_mode":          "compliance",
			},
			GovernancePolicy: map[string]any{
				"recipient_collision_mode":  "block",
				"domain_collision_mode":     "block",
				"max_active_per_domain":     2,
				"positive_domain_action":    "pause_workspace",
				"negative_domain_action":    "stop_workspace",
				"unsubscribe_domain_action": "suppress_workspace",
				"hostile_domain_action":     "suppress_workspace",
			},
			Steps: []models.OutboundPlaybookStep{
				{
					Position:            1,
					Kind:                "email",
					ThreadMode:          "new_thread",
					WaitIntervalMinutes: 4320,
					SubjectTemplate:     "Re-opening the conversation with {{domain}}",
					BodyTemplate:        "Hi {{first_name}},\n\nReaching out because this still looks relevant for {{domain}} and I did not want to assume the timing was just wrong.\n\nIf this is still worth a look, I can send a concise update.\n\nBest,\n{{sender_name}}",
					BranchPolicy: map[string]any{
						"question":               "manual_task:2",
						"objection":              "manual_task:2",
						"manual_review_required": "manual_task:2",
					},
				},
				{
					Position:   2,
					Kind:       "manual_task",
					ThreadMode: "same_thread",
					TaskPolicy: map[string]any{
						"title":        "Review nuanced replies before the next follow-up",
						"instructions": "Handle pricing, timing, and security objections directly from the mailbox that already owns the relationship.",
						"action_label": "Handled, continue",
					},
				},
				{
					Position:     3,
					Kind:         "email",
					ThreadMode:   "same_thread",
					BodyTemplate: "Checking once more before I close the loop on my side.\n\nIf there is no fit right now, a quick no is totally fine.\n\nBest,\n{{sender_name}}",
				},
			},
		},
	}
}

func outboundPlaybookByKey(key string) (models.OutboundPlaybook, bool) {
	for _, item := range outboundPlaybookCatalog() {
		if strings.EqualFold(strings.TrimSpace(item.Key), strings.TrimSpace(key)) {
			return item, true
		}
	}
	return models.OutboundPlaybook{}, false
}

func mustCompactOutboundJSON(raw map[string]any) string {
	if len(raw) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func (s *Service) OutboundPlaybooks() []models.OutboundPlaybook {
	items := outboundPlaybookCatalog()
	out := make([]models.OutboundPlaybook, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func OutboundProviderPacing(account models.MailAccount) (dailyCap, hourlyCap, gapSeconds int) {
	pacing := outboundProviderPacingForAccount(account)
	return pacing.DailyCap, pacing.HourlyCap, pacing.GapSeconds
}

func normalizeOutboundSenderPolicyKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "single_sender":
		return "single_sender"
	case "campaign_pool":
		return "campaign_pool"
	case "reply_funnel":
		return "reply_funnel"
	case "thread_owner":
		return "thread_owner"
	default:
		return "preferred_sender"
	}
}

func normalizeReplyOutcome(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "positive_interest":
		return "positive_interest"
	case "meeting_intent":
		return "meeting_intent"
	case "question":
		return "question"
	case "objection":
		return "objection"
	case "referral":
		return "referral"
	case "wrong_person":
		return "wrong_person"
	case "not_interested":
		return "not_interested"
	case "unsubscribe_request":
		return "unsubscribe_request"
	case "out_of_office":
		return "out_of_office"
	case "bounce":
		return "bounce"
	case "auto_reply_other":
		return "auto_reply_other"
	case "hostile":
		return "hostile"
	case "manual_review_required":
		return "manual_review_required"
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, item := range values {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func outboundDeterministicIndex(seed string, length int) int {
	if length <= 0 {
		return 0
	}
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(seed))))
	value := int(sum[0])<<8 | int(sum[1])
	if value < 0 {
		value = -value
	}
	return value % length
}

func outboundTemplateTokens(text string) []string {
	matches := outboundTemplateTokenRx.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		key := strings.ToLower(strings.TrimSpace(match[1]))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func renderOutboundTemplate(text string, vars map[string]string) (string, []string) {
	missing := map[string]struct{}{}
	rendered := outboundTemplateTokenRx.ReplaceAllStringFunc(text, func(token string) string {
		match := outboundTemplateTokenRx.FindStringSubmatch(token)
		if len(match) < 2 {
			return token
		}
		key := strings.ToLower(strings.TrimSpace(match[1]))
		value := strings.TrimSpace(vars[key])
		if value == "" {
			missing[key] = struct{}{}
			return ""
		}
		return value
	})
	outMissing := make([]string, 0, len(missing))
	for key := range missing {
		outMissing = append(outMissing, key)
	}
	sort.Strings(outMissing)
	return strings.TrimSpace(rendered), outMissing
}

func outboundFallbackName(email string) string {
	local := strings.TrimSpace(email)
	if at := strings.Index(local, "@"); at > 0 {
		local = local[:at]
	}
	local = strings.ReplaceAll(local, ".", " ")
	local = strings.ReplaceAll(local, "_", " ")
	local = strings.ReplaceAll(local, "-", " ")
	local = strings.TrimSpace(local)
	if local == "" {
		return "there"
	}
	parts := strings.Fields(local)
	if len(parts) == 0 {
		return local
	}
	for i := range parts {
		runes := []rune(parts[i])
		if len(runes) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(string(runes[0])) + strings.ToLower(string(runes[1:]))
	}
	return strings.Join(parts, " ")
}

func outboundFirstName(name, email string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = outboundFallbackName(email)
	}
	parts := strings.Fields(base)
	if len(parts) == 0 {
		return outboundFallbackName(email)
	}
	return parts[0]
}

func outboundSelectionName(selection outboundSenderSelection) string {
	name := strings.TrimSpace(selection.HeaderFromName)
	if name != "" {
		return name
	}
	return outboundFallbackName(selection.HeaderFromEmail)
}

func outboundTemplateVars(contact models.Contact, enrollment models.OutboundEnrollment, selection outboundSenderSelection) map[string]string {
	recipient := strings.ToLower(strings.TrimSpace(enrollment.RecipientEmail))
	domain := strings.TrimSpace(enrollment.RecipientDomain)
	if domain == "" {
		domain = strings.TrimSpace(strings.TrimPrefix(recipient[strings.LastIndex(recipient, "@"):], "@"))
	}
	name := strings.TrimSpace(contact.Name)
	if name == "" {
		name = strings.TrimSpace(enrollment.ContactName)
	}
	if name == "" {
		name = outboundFallbackName(recipient)
	}
	senderName := outboundSelectionName(selection)
	vars := map[string]string{
		"name":               name,
		"contact.name":       name,
		"first_name":         outboundFirstName(name, recipient),
		"contact.first_name": outboundFirstName(name, recipient),
		"email":              recipient,
		"contact.email":      recipient,
		"domain":             domain,
		"contact.domain":     domain,
		"sender_name":        senderName,
		"sender_email":       strings.TrimSpace(selection.HeaderFromEmail),
		"account_login":      strings.TrimSpace(selection.Account.Login),
	}
	return vars
}

func (s *Service) outboundMailClientForAccount(account models.MailAccount) mail.Client {
	cfg := s.cfg
	cfg.IMAPHost = account.IMAPHost
	cfg.IMAPPort = account.IMAPPort
	cfg.IMAPTLS = account.IMAPTLS
	cfg.IMAPStartTLS = account.IMAPStartTLS
	cfg.SMTPHost = account.SMTPHost
	cfg.SMTPPort = account.SMTPPort
	cfg.SMTPTLS = account.SMTPTLS
	cfg.SMTPStartTLS = account.SMTPStartTLS
	return mail.NewIMAPSMTPClient(cfg)
}

func (s *Service) decryptOutboundAccountSecret(account models.MailAccount) (string, error) {
	return util.DecryptString(s.encryptKey, account.SecretEnc)
}

func outboundActiveAccount(account models.MailAccount) bool {
	return strings.EqualFold(strings.TrimSpace(account.Status), "active") || strings.TrimSpace(account.Status) == ""
}

func (s *Service) outboundDefaultIdentityForAccount(ctx context.Context, account models.MailAccount) (models.MailIdentity, error) {
	identities, err := s.st.ListMailIdentities(ctx, account.ID)
	if err != nil {
		return models.MailIdentity{}, err
	}
	for _, item := range identities {
		if item.IsDefault && strings.TrimSpace(item.FromEmail) != "" {
			return item, nil
		}
	}
	for _, item := range identities {
		if strings.EqualFold(strings.TrimSpace(item.FromEmail), strings.TrimSpace(account.Login)) {
			return item, nil
		}
	}
	if len(identities) > 0 && strings.TrimSpace(identities[0].FromEmail) != "" {
		return identities[0], nil
	}
	return models.MailIdentity{}, store.ErrNotFound
}

func (s *Service) outboundSelectionFromAccount(ctx context.Context, u models.User, account models.MailAccount) (outboundSenderSelection, error) {
	selection := outboundSenderSelection{Account: account}
	if !outboundActiveAccount(account) {
		return outboundSenderSelection{}, fmt.Errorf("selected sender account is not available for sending")
	}
	identity, err := s.outboundDefaultIdentityForAccount(ctx, account)
	if err == nil {
		selection.SenderProfileID = identity.ID
		selection.HeaderFromName = strings.TrimSpace(identity.DisplayName)
		selection.HeaderFromEmail = strings.TrimSpace(identity.FromEmail)
		selection.EnvelopeFrom = strings.TrimSpace(identity.FromEmail)
		selection.ReplyTo = strings.TrimSpace(identity.ReplyTo)
		selection.ReplyAccountID = account.ID
		if selection.HeaderFromEmail != "" {
			return selection, nil
		}
	}
	login := strings.TrimSpace(account.Login)
	if login == "" {
		return outboundSenderSelection{}, fmt.Errorf("selected sender account is missing login")
	}
	selection.HeaderFromName = strings.TrimSpace(account.DisplayName)
	selection.HeaderFromEmail = login
	selection.EnvelopeFrom = login
	selection.ReplyAccountID = account.ID
	return selection, nil
}

func (s *Service) outboundThreadOwnerAccountID(ctx context.Context, enrollment models.OutboundEnrollment) string {
	if strings.TrimSpace(enrollment.ThreadBindingID) != "" {
		if binding, err := s.st.GetMailThreadBindingByID(ctx, enrollment.ThreadBindingID); err == nil && strings.TrimSpace(binding.AccountID) != "" {
			return strings.TrimSpace(binding.AccountID)
		}
	}
	if binding, err := s.st.GetMailThreadBindingByEnrollment(ctx, enrollment.ID); err == nil && strings.TrimSpace(binding.AccountID) != "" {
		return strings.TrimSpace(binding.AccountID)
	}
	seed := parseOutboundSeedContext(enrollment.SeedContextJSON)
	return strings.TrimSpace(seed.AccountID)
}

func (s *Service) outboundSelectionFromThreadOwner(ctx context.Context, u models.User, enrollment models.OutboundEnrollment) (outboundSenderSelection, error) {
	accountID := s.outboundThreadOwnerAccountID(ctx, enrollment)
	if accountID == "" {
		return outboundSenderSelection{}, fmt.Errorf("existing thread owner mailbox is not available for this recipient")
	}
	account, err := s.st.GetMailAccountByID(ctx, u.ID, accountID)
	if err != nil {
		return outboundSenderSelection{}, fmt.Errorf("existing thread owner mailbox is not available for this recipient")
	}
	return s.outboundSelectionFromAccount(ctx, u, account)
}

func (s *Service) outboundSelectionFromSenderProfile(ctx context.Context, u models.User, senderProfileID string) (outboundSenderSelection, error) {
	profile, err := s.GetSenderProfileByID(ctx, u, strings.TrimSpace(senderProfileID))
	if err != nil {
		return outboundSenderSelection{}, err
	}
	if strings.TrimSpace(profile.AccountID) == "" {
		return outboundSenderSelection{}, fmt.Errorf("selected sender requires a sending account")
	}
	account, err := s.st.GetMailAccountByID(ctx, u.ID, profile.AccountID)
	if err != nil {
		return outboundSenderSelection{}, err
	}
	if !outboundActiveAccount(account) {
		return outboundSenderSelection{}, fmt.Errorf("selected sender is not available for sending")
	}
	selection := outboundSenderSelection{
		Account:         account,
		SenderProfileID: profile.ID,
		HeaderFromName:  strings.TrimSpace(profile.Name),
		HeaderFromEmail: strings.TrimSpace(profile.FromEmail),
		EnvelopeFrom:    strings.TrimSpace(profile.FromEmail),
		ReplyTo:         strings.TrimSpace(profile.ReplyTo),
		ReplyAccountID:  account.ID,
	}
	if selection.HeaderFromEmail == "" {
		return outboundSenderSelection{}, fmt.Errorf("selected sender is missing from_email")
	}
	if selection.HeaderFromName == "" {
		selection.HeaderFromName = outboundFallbackName(selection.HeaderFromEmail)
	}
	return selection, nil
}

func outboundPolicyIDs(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var list []string
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &list); err == nil {
			out := make([]string, 0, len(list))
			for _, item := range list {
				if v := strings.TrimSpace(item); v != "" {
					out = append(out, v)
				}
			}
			return out
		}
	}
	if strings.HasPrefix(trimmed, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
			out := make([]string, 0, 8)
			for _, key := range []string{"sender_profile_ids", "sender_profiles", "account_ids", "accounts", "ids"} {
				rawItems, ok := obj[key]
				if !ok {
					continue
				}
				if items, ok := rawItems.([]any); ok {
					for _, item := range items {
						if v := strings.TrimSpace(fmt.Sprint(item)); v != "" {
							out = append(out, v)
						}
					}
				}
			}
			if len(out) > 0 {
				return out
			}
			for _, key := range []string{"sender_profile_id", "sender_profile", "account_id", "id"} {
				if v := strings.TrimSpace(fmt.Sprint(obj[key])); v != "" && v != "<nil>" {
					return []string{v}
				}
			}
		}
	}
	if strings.Contains(trimmed, ",") {
		parts := strings.Split(trimmed, ",")
		out := make([]string, 0, len(parts))
		for _, item := range parts {
			if v := strings.TrimSpace(item); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	return []string{trimmed}
}

func (s *Service) outboundSelectionFromPolicyPool(ctx context.Context, u models.User, policyRef, seed string) (outboundSenderSelection, error) {
	ids := outboundPolicyIDs(policyRef)
	if len(ids) == 0 {
		return outboundSenderSelection{}, fmt.Errorf("campaign sender pool is empty")
	}
	index := outboundDeterministicIndex(seed, len(ids))
	lastErr := error(nil)
	for offset := 0; offset < len(ids); offset++ {
		candidate := ids[(index+offset)%len(ids)]
		if selection, err := s.outboundSelectionFromSenderProfile(ctx, u, candidate); err == nil {
			return selection, nil
		} else {
			lastErr = err
		}
		if account, err := s.st.GetMailAccountByID(ctx, u.ID, candidate); err == nil {
			if selection, err := s.outboundSelectionFromAccount(ctx, u, account); err == nil {
				return selection, nil
			} else {
				lastErr = err
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("campaign sender pool has no usable sender")
	}
	return outboundSenderSelection{}, lastErr
}

func (s *Service) outboundSelectionFromReplyFunnelAccount(ctx context.Context, u models.User, funnelID, accountID string) (outboundSenderSelection, error) {
	funnel, err := s.st.GetReplyFunnelByID(ctx, u.ID, funnelID)
	if err != nil {
		return outboundSenderSelection{}, err
	}
	accounts, err := s.st.ListReplyFunnelAccounts(ctx, funnel.ID)
	if err != nil {
		return outboundSenderSelection{}, err
	}
	for _, row := range accounts {
		if !strings.EqualFold(strings.TrimSpace(row.Role), "source") || strings.TrimSpace(row.AccountID) != strings.TrimSpace(accountID) {
			continue
		}
		account, err := s.st.GetMailAccountByID(ctx, u.ID, row.AccountID)
		if err != nil {
			return outboundSenderSelection{}, err
		}
		var selection outboundSenderSelection
		if strings.TrimSpace(row.SenderIdentityID) != "" {
			if identity, err := s.st.GetMailIdentityByID(ctx, row.SenderIdentityID); err == nil && identity.AccountID == account.ID {
				selection = outboundSenderSelection{
					Account:            account,
					SenderProfileID:    identity.ID,
					HeaderFromName:     strings.TrimSpace(identity.DisplayName),
					HeaderFromEmail:    strings.TrimSpace(identity.FromEmail),
					EnvelopeFrom:       strings.TrimSpace(identity.FromEmail),
					ReplyTo:            strings.TrimSpace(identity.ReplyTo),
					ReplyFunnelID:      funnel.ID,
					CollectorAccountID: strings.TrimSpace(funnel.CollectorAccountID),
					ReplyAccountID:     account.ID,
				}
			}
		}
		if strings.TrimSpace(selection.HeaderFromEmail) == "" {
			base, err := s.outboundSelectionFromAccount(ctx, u, account)
			if err != nil {
				return outboundSenderSelection{}, err
			}
			selection = base
			selection.ReplyFunnelID = funnel.ID
			selection.CollectorAccountID = strings.TrimSpace(funnel.CollectorAccountID)
		}
		if strings.TrimSpace(funnel.CollectorAccountID) != "" {
			collectorAccount, err := s.st.GetMailAccountByID(ctx, u.ID, funnel.CollectorAccountID)
			if err == nil && outboundActiveAccount(collectorAccount) {
				collectorSelection, collectorErr := s.outboundSelectionFromAccount(ctx, u, collectorAccount)
				if collectorErr == nil {
					switch strings.ToLower(strings.TrimSpace(funnel.ReplyMode)) {
					case "collector":
						selection.ReplyTo = firstNonEmptyString(
							strings.TrimSpace(collectorSelection.ReplyTo),
							strings.TrimSpace(collectorSelection.HeaderFromEmail),
							strings.TrimSpace(collectorAccount.Login),
						)
						selection.ReplyAccountID = collectorAccount.ID
					case "smart":
						if strings.TrimSpace(selection.ReplyTo) == "" {
							selection.ReplyTo = firstNonEmptyString(
								strings.TrimSpace(collectorSelection.ReplyTo),
								strings.TrimSpace(collectorSelection.HeaderFromEmail),
								strings.TrimSpace(collectorAccount.Login),
							)
							selection.ReplyAccountID = collectorAccount.ID
						}
					}
				}
			}
		}
		return selection, nil
	}
	return outboundSenderSelection{}, fmt.Errorf("reply funnel does not include the seeded thread owner mailbox")
}

func (s *Service) outboundSelectionFromReplyFunnel(ctx context.Context, u models.User, funnelID, seed string) (outboundSenderSelection, error) {
	funnel, err := s.st.GetReplyFunnelByID(ctx, u.ID, funnelID)
	if err != nil {
		return outboundSenderSelection{}, err
	}
	accounts, err := s.st.ListReplyFunnelAccounts(ctx, funnel.ID)
	if err != nil {
		return outboundSenderSelection{}, err
	}
	sourceRows := make([]models.ReplyFunnelAccount, 0, len(accounts))
	for _, row := range accounts {
		if strings.EqualFold(strings.TrimSpace(row.Role), "source") {
			sourceRows = append(sourceRows, row)
		}
	}
	if len(sourceRows) == 0 {
		return outboundSenderSelection{}, fmt.Errorf("reply funnel has no source accounts")
	}
	sort.SliceStable(sourceRows, func(i, j int) bool {
		if sourceRows[i].Position != sourceRows[j].Position {
			return sourceRows[i].Position < sourceRows[j].Position
		}
		return sourceRows[i].AccountID < sourceRows[j].AccountID
	})
	row := sourceRows[outboundDeterministicIndex(seed, len(sourceRows))]
	account, err := s.st.GetMailAccountByID(ctx, u.ID, row.AccountID)
	if err != nil {
		return outboundSenderSelection{}, err
	}
	var selection outboundSenderSelection
	if strings.TrimSpace(row.SenderIdentityID) != "" {
		if identity, err := s.st.GetMailIdentityByID(ctx, row.SenderIdentityID); err == nil && identity.AccountID == account.ID {
			selection = outboundSenderSelection{
				Account:            account,
				SenderProfileID:    identity.ID,
				HeaderFromName:     strings.TrimSpace(identity.DisplayName),
				HeaderFromEmail:    strings.TrimSpace(identity.FromEmail),
				EnvelopeFrom:       strings.TrimSpace(identity.FromEmail),
				ReplyTo:            strings.TrimSpace(identity.ReplyTo),
				ReplyFunnelID:      funnel.ID,
				CollectorAccountID: strings.TrimSpace(funnel.CollectorAccountID),
			}
		}
	}
	if strings.TrimSpace(selection.HeaderFromEmail) == "" {
		base, err := s.outboundSelectionFromAccount(ctx, u, account)
		if err != nil {
			return outboundSenderSelection{}, err
		}
		selection = base
		selection.ReplyFunnelID = funnel.ID
		selection.CollectorAccountID = strings.TrimSpace(funnel.CollectorAccountID)
	}
	selection.ReplyAccountID = account.ID
	if strings.TrimSpace(funnel.CollectorAccountID) != "" {
		collectorAccount, err := s.st.GetMailAccountByID(ctx, u.ID, funnel.CollectorAccountID)
		if err == nil && outboundActiveAccount(collectorAccount) {
			collectorSelection, collectorErr := s.outboundSelectionFromAccount(ctx, u, collectorAccount)
			if collectorErr == nil {
				selection.CollectorAccountID = collectorAccount.ID
				switch strings.ToLower(strings.TrimSpace(funnel.ReplyMode)) {
				case "collector":
					selection.ReplyTo = firstNonEmptyString(
						strings.TrimSpace(collectorSelection.ReplyTo),
						strings.TrimSpace(collectorSelection.HeaderFromEmail),
						strings.TrimSpace(collectorAccount.Login),
					)
					selection.ReplyAccountID = collectorAccount.ID
				case "smart":
					if strings.TrimSpace(selection.ReplyTo) == "" {
						selection.ReplyTo = firstNonEmptyString(
							strings.TrimSpace(collectorSelection.ReplyTo),
							strings.TrimSpace(collectorSelection.HeaderFromEmail),
							strings.TrimSpace(collectorAccount.Login),
						)
						selection.ReplyAccountID = collectorAccount.ID
					}
				}
			}
		}
	}
	return selection, nil
}

func (s *Service) resolveOutboundSenderSelection(ctx context.Context, u models.User, campaign models.OutboundCampaign, contact models.Contact, enrollment models.OutboundEnrollment) (outboundSenderSelection, error) {
	if strings.EqualFold(strings.TrimSpace(campaign.CampaignMode), "existing_threads") {
		switch normalizeOutboundSenderPolicyKind(campaign.SenderPolicyKind) {
		case "thread_owner":
			return s.outboundSelectionFromThreadOwner(ctx, u, enrollment)
		case "reply_funnel":
			ids := outboundPolicyIDs(campaign.SenderPolicyRef)
			if len(ids) > 0 {
				if accountID := s.outboundThreadOwnerAccountID(ctx, enrollment); accountID != "" {
					if selection, err := s.outboundSelectionFromReplyFunnelAccount(ctx, u, ids[0], accountID); err == nil {
						return selection, nil
					}
				}
			}
		default:
			if selection, err := s.outboundSelectionFromThreadOwner(ctx, u, enrollment); err == nil {
				return selection, nil
			}
		}
	}
	switch normalizeOutboundSenderPolicyKind(campaign.SenderPolicyKind) {
	case "thread_owner":
		return s.outboundSelectionFromThreadOwner(ctx, u, enrollment)
	case "single_sender":
		ids := outboundPolicyIDs(campaign.SenderPolicyRef)
		if len(ids) == 0 {
			return outboundSenderSelection{}, fmt.Errorf("campaign sender is not configured")
		}
		if selection, err := s.outboundSelectionFromSenderProfile(ctx, u, ids[0]); err == nil {
			return selection, nil
		}
		account, err := s.st.GetMailAccountByID(ctx, u.ID, ids[0])
		if err != nil {
			return outboundSenderSelection{}, fmt.Errorf("campaign sender is not configured")
		}
		return s.outboundSelectionFromAccount(ctx, u, account)
	case "campaign_pool":
		return s.outboundSelectionFromPolicyPool(ctx, u, campaign.SenderPolicyRef, enrollment.RecipientEmail)
	case "reply_funnel":
		ids := outboundPolicyIDs(campaign.SenderPolicyRef)
		if len(ids) == 0 {
			return outboundSenderSelection{}, fmt.Errorf("reply funnel is not configured")
		}
		return s.outboundSelectionFromReplyFunnel(ctx, u, ids[0], enrollment.RecipientEmail)
	default:
		if strings.TrimSpace(contact.PreferredSenderID) != "" {
			if selection, err := s.outboundSelectionFromSenderProfile(ctx, u, contact.PreferredSenderID); err == nil {
				return selection, nil
			}
		}
		if strings.TrimSpace(contact.PreferredAccountID) != "" {
			if account, err := s.st.GetMailAccountByID(ctx, u.ID, contact.PreferredAccountID); err == nil {
				if selection, err := s.outboundSelectionFromAccount(ctx, u, account); err == nil {
					return selection, nil
				}
			}
		}
		accounts, err := s.st.ListMailAccounts(ctx, u.ID)
		if err != nil {
			return outboundSenderSelection{}, err
		}
		for _, account := range accounts {
			if !outboundActiveAccount(account) {
				continue
			}
			if selection, err := s.outboundSelectionFromAccount(ctx, u, account); err == nil {
				return selection, nil
			}
		}
		return outboundSenderSelection{}, fmt.Errorf("no usable sending account is available")
	}
}

func (s *Service) loadOutboundContact(ctx context.Context, u models.User, enrollment models.OutboundEnrollment) (models.Contact, error) {
	if strings.TrimSpace(enrollment.ContactID) != "" {
		return s.st.GetContactByID(ctx, u.ID, enrollment.ContactID)
	}
	return models.Contact{}, store.ErrNotFound
}

func (s *Service) renderOutboundStep(ctx context.Context, u models.User, campaign models.OutboundCampaign, step models.OutboundCampaignStep, enrollment models.OutboundEnrollment) (outboundRenderedStep, error) {
	contact, _ := s.loadOutboundContact(ctx, u, enrollment)
	selection, err := s.resolveOutboundSenderSelection(ctx, u, campaign, contact, enrollment)
	if err != nil {
		return outboundRenderedStep{}, err
	}
	vars := outboundTemplateVars(contact, enrollment, selection)
	subject, missingSubject := renderOutboundTemplate(step.SubjectTemplate, vars)
	body, missingBody := renderOutboundTemplate(step.BodyTemplate, vars)
	missing := append([]string{}, missingSubject...)
	for _, item := range missingBody {
		duplicate := false
		for _, seen := range missing {
			if seen == item {
				duplicate = true
				break
			}
		}
		if !duplicate {
			missing = append(missing, item)
		}
	}
	sort.Strings(missing)
	return outboundRenderedStep{
		Step:      step,
		Subject:   subject,
		Body:      body,
		Missing:   missing,
		Selection: selection,
	}, nil
}

func appendOutboundComplianceFooter(body string, compliance outboundCompliancePolicy) string {
	body = strings.TrimSpace(body)
	if !compliance.UnsubscribeRequired {
		return body
	}
	if strings.EqualFold(strings.TrimSpace(compliance.FooterMode), "none") {
		return body
	}
	footer := "To opt out of future outreach, reply with \"unsubscribe\"."
	if body == "" {
		return footer
	}
	return body + "\n\n--\n" + footer
}

func nextOutboundMessageID(fromEmail string) string {
	host := "despatch.local"
	if at := strings.LastIndex(strings.TrimSpace(fromEmail), "@"); at >= 0 && at < len(strings.TrimSpace(fromEmail))-1 {
		host = strings.TrimSpace(fromEmail[at+1:])
	}
	return fmt.Sprintf("<outbound-%s@%s>", uuid.NewString(), host)
}

func outboundLooksLikeSelfMessage(indexed models.IndexedMessage, account models.MailAccount, identities []models.MailIdentity) bool {
	candidates := map[string]struct{}{}
	if login, err := mail.NormalizeMailboxAddress(account.Login); err == nil {
		candidates[strings.ToLower(strings.TrimSpace(login))] = struct{}{}
	}
	for _, item := range identities {
		if addr, err := mail.NormalizeMailboxAddress(item.FromEmail); err == nil {
			candidates[strings.ToLower(strings.TrimSpace(addr))] = struct{}{}
		}
		if addr, err := mail.NormalizeMailboxAddress(item.ReplyTo); err == nil {
			candidates[strings.ToLower(strings.TrimSpace(addr))] = struct{}{}
		}
	}
	list, err := netmail.ParseAddressList(strings.TrimSpace(indexed.FromValue))
	if err != nil || len(list) == 0 {
		if addr, err := netmail.ParseAddress(strings.TrimSpace(indexed.FromValue)); err == nil && addr != nil {
			list = []*netmail.Address{addr}
		}
	}
	for _, item := range list {
		if _, ok := candidates[strings.ToLower(strings.TrimSpace(item.Address))]; ok {
			return true
		}
	}
	return false
}

func containsAnyFold(haystack string, needles []string) bool {
	value := strings.ToLower(strings.TrimSpace(haystack))
	for _, item := range needles {
		if item == "" {
			continue
		}
		if strings.Contains(value, strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func extractReplyBodyFingerprint(indexed models.IndexedMessage) string {
	body := strings.TrimSpace(indexed.BodyText)
	if body == "" {
		body = strings.TrimSpace(indexed.Snippet)
	}
	return strings.ToLower(body)
}

func parseOutboundOOOReturnDate(indexed models.IndexedMessage) time.Time {
	candidate := indexed.Subject + "\n" + indexed.BodyText
	match := outboundReturnDateRx.FindStringSubmatch(candidate)
	if len(match) < 2 {
		return time.Time{}
	}
	value := strings.TrimSpace(match[1])
	layouts := []string{
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
		"01/02/06",
		"Jan 2, 2006",
		"January 2, 2006",
		"Jan 2",
		"January 2",
	}
	now := time.Now()
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			if !strings.Contains(layout, "2006") {
				parsed = time.Date(now.Year(), parsed.Month(), parsed.Day(), 9, 0, 0, 0, time.UTC)
				if parsed.Before(now.UTC()) {
					parsed = parsed.AddDate(1, 0, 0)
				}
			}
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func classifyOutboundReply(indexed models.IndexedMessage) (string, float64, time.Time) {
	subject := strings.ToLower(strings.TrimSpace(indexed.Subject))
	body := extractReplyBodyFingerprint(indexed)
	from := strings.ToLower(strings.TrimSpace(indexed.FromValue))
	raw := strings.ToLower(strings.TrimSpace(indexed.RawSource))
	joined := strings.Join([]string{subject, body, from, raw}, "\n")
	if containsAnyFold(from, []string{"mailer-daemon", "postmaster"}) || containsAnyFold(joined, outboundBounceMarkers) {
		return "bounce", 0.98, time.Time{}
	}
	if strings.Contains(raw, "auto-submitted: auto-replied") || containsAnyFold(joined, outboundOOOMarkers) {
		return "out_of_office", 0.9, parseOutboundOOOReturnDate(indexed)
	}
	if strings.Contains(raw, "auto-submitted:") {
		return "auto_reply_other", 0.7, time.Time{}
	}
	if containsAnyFold(joined, outboundUnsubscribeMarkers) {
		return "unsubscribe_request", 0.96, time.Time{}
	}
	if containsAnyFold(joined, outboundHostileMarkers) {
		return "hostile", 0.9, time.Time{}
	}
	if containsAnyFold(joined, outboundWrongPersonMarkers) {
		if strings.Contains(body, "@") || containsAnyFold(joined, outboundReferralMarkers) {
			return "referral", 0.82, time.Time{}
		}
		return "wrong_person", 0.9, time.Time{}
	}
	if containsAnyFold(joined, outboundNotInterestedMarkers) {
		return "not_interested", 0.9, time.Time{}
	}
	if containsAnyFold(joined, outboundMeetingMarkers) {
		return "meeting_intent", 0.86, time.Time{}
	}
	if containsAnyFold(joined, outboundPositiveMarkers) {
		return "positive_interest", 0.82, time.Time{}
	}
	if containsAnyFold(joined, outboundObjectionMarkers) {
		return "objection", 0.68, time.Time{}
	}
	if strings.Contains(body, "?") {
		return "question", 0.62, time.Time{}
	}
	return "manual_review_required", 0.35, time.Time{}
}

func (s *Service) outboundSendCounts24h(ctx context.Context, campaignID, senderAccountID, domain string) (int, int, error) {
	events, err := s.st.ListOutboundCampaignEvents(ctx, campaignID, 5000)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now().UTC()
	senderCount := 0
	domainCount := 0
	for _, item := range events {
		if item.EventKind != "step_sent" || item.CreatedAt.Before(now.Add(-24*time.Hour)) {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(item.EventPayloadJSON), &payload); err != nil {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(payload["sender_account_id"])) == strings.TrimSpace(senderAccountID) {
			senderCount++
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(payload["recipient_domain"])), strings.TrimSpace(domain)) {
			domainCount++
		}
	}
	return senderCount, domainCount, nil
}

func (s *Service) outboundAccountSendsSince(ctx context.Context, userID, senderAccountID string, since time.Time) (int, error) {
	campaigns, err := s.st.ListOutboundCampaigns(ctx, userID)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, campaign := range campaigns {
		events, err := s.st.ListOutboundCampaignEvents(ctx, campaign.ID, 5000)
		if err != nil {
			return 0, err
		}
		for _, item := range events {
			if item.EventKind != "step_sent" || item.CreatedAt.Before(since) {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(item.EventPayloadJSON), &payload); err != nil {
				continue
			}
			if strings.TrimSpace(fmt.Sprint(payload["sender_account_id"])) == strings.TrimSpace(senderAccountID) {
				total++
			}
		}
	}
	return total, nil
}

func outboundStepByPosition(steps []models.OutboundCampaignStep, position int) (models.OutboundCampaignStep, bool) {
	for _, step := range steps {
		if step.Position == position {
			return step, true
		}
	}
	return models.OutboundCampaignStep{}, false
}

func outboundBranchActionForOutcome(step models.OutboundCampaignStep, outcome string) string {
	branches := parseOutboundBranchPolicy(step.BranchPolicyJSON)
	if len(branches) == 0 {
		return ""
	}
	key := normalizeReplyOutcome(outcome)
	if key == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(branches[key]))
}

func (s *Service) applyOutboundReplyBranch(campaign models.OutboundCampaign, enrollment *models.OutboundEnrollment, steps []models.OutboundCampaignStep, outcome string) bool {
	if enrollment == nil {
		return false
	}
	switch normalizeReplyOutcome(outcome) {
	case "unsubscribe_request", "bounce", "hostile":
		return false
	}
	currentStep, ok := outboundStepByPosition(steps, enrollment.CurrentStepPosition)
	if !ok {
		return false
	}
	action := outboundBranchActionForOutcome(currentStep, outcome)
	if action == "" {
		return false
	}
	now := time.Now().UTC()
	switch action {
	case "continue", "next_step":
		enrollment.Status = "scheduled"
		enrollment.PauseReason = ""
		enrollment.StopReason = ""
		enrollment.NextActionAt = now
		return true
	case "pause":
		enrollment.Status = "paused"
		enrollment.PauseReason = "branch_pause"
		enrollment.StopReason = ""
		enrollment.NextActionAt = time.Time{}
		return true
	case "manual_review":
		enrollment.Status = "manual_only"
		enrollment.ManualOwnerUserID = campaign.UserID
		enrollment.PauseReason = "branch_manual_review"
		enrollment.StopReason = ""
		enrollment.NextActionAt = time.Time{}
		return true
	case "stop":
		enrollment.Status = "stopped"
		enrollment.StopReason = firstNonEmptyString(outcome, "branch_stop")
		enrollment.PauseReason = ""
		enrollment.NextActionAt = time.Time{}
		return true
	default:
		if strings.HasPrefix(action, "manual_task:") {
			targetRaw := strings.TrimSpace(strings.TrimPrefix(action, "manual_task:"))
			targetPos := 0
			fmt.Sscanf(targetRaw, "%d", &targetPos)
			target, ok := outboundStepByPosition(steps, targetPos)
			if !ok || target.Kind != "manual_task" {
				return false
			}
			enrollment.Status = "scheduled"
			enrollment.CurrentStepPosition = target.Position - 1
			enrollment.PauseReason = ""
			enrollment.StopReason = ""
			enrollment.NextActionAt = now
			return true
		}
	}
	return false
}

func (s *Service) PreflightOutboundCampaign(ctx context.Context, u models.User, campaign models.OutboundCampaign) ([]models.OutboundPreflightIssue, error) {
	steps, err := s.st.ListOutboundCampaignSteps(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}
	issues := make([]models.OutboundPreflightIssue, 0, 16)
	hasEmailStep := false
	for _, step := range steps {
		if step.Kind == "email" {
			hasEmailStep = true
		}
		if step.Kind == "manual_task" {
			task := parseOutboundTaskPolicy(step.TaskPolicyJSON)
			if task.Title == "" {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:     "manual_task_missing_title",
					Severity: "error",
					Message:  fmt.Sprintf("Manual task step %d must have a task title.", step.Position),
					Blocking: true,
				})
			}
		}
		if strings.EqualFold(strings.TrimSpace(campaign.CampaignMode), "existing_threads") && step.Kind == "email" && step.ThreadMode != "same_thread" {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:     "existing_threads_new_thread_step",
				Severity: "warning",
				Message:  fmt.Sprintf("Step %d starts a new thread even though this campaign is configured to continue existing threads.", step.Position),
				Blocking: false,
			})
		}
	}
	if len(steps) == 0 {
		issues = append(issues, models.OutboundPreflightIssue{
			Code:     "missing_steps",
			Severity: "error",
			Message:  "Campaign must contain at least one step.",
			Blocking: true,
		})
		return issues, nil
	}
	if !hasEmailStep {
		issues = append(issues, models.OutboundPreflightIssue{
			Code:     "no_email_steps",
			Severity: "warning",
			Message:  "Campaign has no email steps and will only create manual tasks.",
			Blocking: false,
		})
	}
	enrollments, err := s.st.ListOutboundEnrollmentsByCampaign(ctx, u.ID, campaign.ID)
	if err != nil {
		return nil, err
	}
	if len(enrollments) == 0 {
		issues = append(issues, models.OutboundPreflightIssue{
			Code:     "missing_recipients",
			Severity: "error",
			Message:  "Campaign must contain at least one recipient.",
			Blocking: true,
		})
		return issues, nil
	}
	compliance := parseOutboundCompliancePolicy(campaign.CompliancePolicyJSON)
	schedule := parseOutboundSchedulePolicy(campaign.SchedulePolicyJSON)
	governance := parseOutboundGovernancePolicy(campaign.GovernancePolicyJSON)
	seen := map[string]struct{}{}
	domainCounts := map[string]int{}
	accountSends24hCache := map[string]int{}
	accountSends1hCache := map[string]int{}
	hasExistingThreadSeed := false
	for _, enrollment := range enrollments {
		if strings.TrimSpace(enrollment.RecipientDomain) != "" {
			domainCounts[enrollment.RecipientDomain]++
		}
		seed := parseOutboundSeedContext(enrollment.SeedContextJSON)
		if strings.TrimSpace(enrollment.ThreadBindingID) != "" || seed.AccountID != "" || seed.ThreadID != "" || seed.MessageID != "" {
			hasExistingThreadSeed = true
		}
	}
	if strings.EqualFold(strings.TrimSpace(campaign.CampaignMode), "existing_threads") && !hasExistingThreadSeed {
		issues = append(issues, models.OutboundPreflightIssue{
			Code:     "missing_existing_thread_context",
			Severity: "error",
			Message:  "Existing-thread campaign has no seeded mailbox threads yet. Import recipients from a saved mailbox search or thread-aware audience source first.",
			Blocking: true,
		})
	}
	for _, enrollment := range enrollments {
		key := strings.ToLower(strings.TrimSpace(enrollment.RecipientEmail))
		if _, ok := seen[key]; ok {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:      "duplicate_recipient",
				Severity:  "error",
				Recipient: enrollment.RecipientEmail,
				Domain:    enrollment.RecipientDomain,
				Message:   "Recipient appears more than once in this campaign.",
				Blocking:  true,
			})
			continue
		}
		seen[key] = struct{}{}
		if governance.MaxActivePerDomain > 0 && domainCounts[enrollment.RecipientDomain] > governance.MaxActivePerDomain {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:      "domain_active_cap",
				Severity:  "warning",
				Recipient: enrollment.RecipientEmail,
				Domain:    enrollment.RecipientDomain,
				Message:   fmt.Sprintf("Domain exceeds the configured active recipient cap (%d).", governance.MaxActivePerDomain),
				Blocking:  governance.DomainCollisionMode == "block",
			})
		}
		activeSuppression, err := s.st.GetActiveOutboundSuppression(ctx, u.ID, enrollment.RecipientEmail, enrollment.RecipientDomain, campaign.ID, time.Now().UTC())
		if err == nil {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:      "recipient_suppressed",
				Severity:  "error",
				Recipient: enrollment.RecipientEmail,
				Domain:    enrollment.RecipientDomain,
				Message:   firstNonEmptyString(activeSuppression.Reason, "Recipient or domain is suppressed."),
				Blocking:  true,
			})
		} else if err != nil && err != store.ErrNotFound {
			return nil, err
		}
		if activeElsewhere, err := s.st.ListActiveOutboundEnrollmentsByRecipient(ctx, u.ID, enrollment.RecipientEmail, campaign.ID); err == nil && len(activeElsewhere) > 0 {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:       "recipient_active_elsewhere",
				Severity:   map[bool]string{true: "error", false: "warning"}[governance.RecipientCollisionMode == "block"],
				Recipient:  enrollment.RecipientEmail,
				Domain:     enrollment.RecipientDomain,
				CampaignID: activeElsewhere[0].CampaignID,
				Message:    "Recipient is already active in another campaign.",
				Blocking:   governance.RecipientCollisionMode == "block",
			})
		}
		if activeDomainElsewhere, err := s.st.ListActiveOutboundEnrollmentsByDomain(ctx, u.ID, enrollment.RecipientDomain, campaign.ID); err == nil && len(activeDomainElsewhere) > 0 {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:       "domain_active_elsewhere",
				Severity:   map[bool]string{true: "error", false: "warning"}[governance.DomainCollisionMode == "block"],
				Recipient:  enrollment.RecipientEmail,
				Domain:     enrollment.RecipientDomain,
				CampaignID: activeDomainElsewhere[0].CampaignID,
				Message:    "Another campaign is already working this domain.",
				Blocking:   governance.DomainCollisionMode == "block",
			})
		}
		contact, _ := s.loadOutboundContact(ctx, u, enrollment)
		selection, err := s.resolveOutboundSenderSelection(ctx, u, campaign, contact, enrollment)
		if err != nil {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:      "sender_unavailable",
				Severity:  "error",
				Recipient: enrollment.RecipientEmail,
				Domain:    enrollment.RecipientDomain,
				Message:   err.Error(),
				Blocking:  true,
			})
			continue
		}
		seed := parseOutboundSeedContext(enrollment.SeedContextJSON)
		if strings.EqualFold(strings.TrimSpace(campaign.CampaignMode), "existing_threads") && seed.AccountID != "" && strings.TrimSpace(selection.Account.ID) != seed.AccountID {
			issues = append(issues, models.OutboundPreflightIssue{
				Code:      "thread_owner_mismatch",
				Severity:  "warning",
				Recipient: enrollment.RecipientEmail,
				Domain:    enrollment.RecipientDomain,
				SenderRef: selection.Account.ID,
				Message:   "Campaign will reopen an existing thread from a different mailbox than the thread owner.",
				Blocking:  false,
			})
		}
		for _, step := range steps {
			if step.Kind == "manual_task" {
				continue
			}
			rendered, err := s.renderOutboundStep(ctx, u, campaign, step, enrollment)
			if err != nil {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "sender_unavailable",
					Severity:  "error",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					SenderRef: selection.Account.ID,
					Message:   err.Error(),
					Blocking:  true,
				})
				break
			}
			if len(rendered.Missing) > 0 {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "missing_template_variables",
					Severity:  "error",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					Message:   fmt.Sprintf("Missing template values: %s", strings.Join(rendered.Missing, ", ")),
					Blocking:  true,
				})
			}
		}
		if schedule.RespectProviderCaps {
			if _, ok := accountSends24hCache[selection.Account.ID]; !ok {
				count24h, err := s.outboundAccountSendsSince(ctx, u.ID, selection.Account.ID, time.Now().UTC().Add(-24*time.Hour))
				if err != nil {
					return nil, err
				}
				accountSends24hCache[selection.Account.ID] = count24h
			}
			if _, ok := accountSends1hCache[selection.Account.ID]; !ok {
				count1h, err := s.outboundAccountSendsSince(ctx, u.ID, selection.Account.ID, time.Now().UTC().Add(-1*time.Hour))
				if err != nil {
					return nil, err
				}
				accountSends1hCache[selection.Account.ID] = count1h
			}
			pacing := outboundProviderPacingForAccount(selection.Account)
			if pacing.DailyCap > 0 && accountSends24hCache[selection.Account.ID] >= pacing.DailyCap {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "provider_daily_soft_cap",
					Severity:  "warning",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					SenderRef: selection.Account.ID,
					Message:   "Sender mailbox is already at the provider-aware daily soft cap.",
					Blocking:  false,
				})
			}
			if pacing.HourlyCap > 0 && accountSends1hCache[selection.Account.ID] >= pacing.HourlyCap {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "provider_hourly_soft_cap",
					Severity:  "warning",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					SenderRef: selection.Account.ID,
					Message:   "Sender mailbox is already at the provider-aware hourly soft cap.",
					Blocking:  false,
				})
			}
			if schedule.MaxSendsPerDay > 0 && pacing.DailyCap > 0 && schedule.MaxSendsPerDay > pacing.DailyCap {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "campaign_daily_cap_above_provider_soft_cap",
					Severity:  "warning",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					SenderRef: selection.Account.ID,
					Message:   "Campaign daily cap is above the provider-aware soft cap for this mailbox.",
					Blocking:  false,
				})
			}
			if schedule.MaxSendsPerHour > 0 && pacing.HourlyCap > 0 && schedule.MaxSendsPerHour > pacing.HourlyCap {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "campaign_hourly_cap_above_provider_soft_cap",
					Severity:  "warning",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					SenderRef: selection.Account.ID,
					Message:   "Campaign hourly cap is above the provider-aware soft cap for this mailbox.",
					Blocking:  false,
				})
			}
		}
		if schedule.MaxSendsPerDay > 0 || schedule.MaxSendsPerDomainPerDay > 0 {
			sender24h, domain24h, err := s.outboundSendCounts24h(ctx, campaign.ID, selection.Account.ID, enrollment.RecipientDomain)
			if err != nil {
				return nil, err
			}
			if schedule.MaxSendsPerDay > 0 && sender24h >= schedule.MaxSendsPerDay {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "sender_daily_cap",
					Severity:  "warning",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					SenderRef: selection.Account.ID,
					Message:   "Sender daily cap would be exceeded.",
					Blocking:  false,
				})
			}
			if schedule.MaxSendsPerDomainPerDay > 0 && domain24h >= schedule.MaxSendsPerDomainPerDay {
				issues = append(issues, models.OutboundPreflightIssue{
					Code:      "domain_daily_cap",
					Severity:  "warning",
					Recipient: enrollment.RecipientEmail,
					Domain:    enrollment.RecipientDomain,
					Message:   "Campaign domain cap would be exceeded.",
					Blocking:  false,
				})
			}
		}
	}
	if compliance.Promotional && !compliance.UnsubscribeRequired {
		issues = append(issues, models.OutboundPreflightIssue{
			Code:     "promotional_without_unsubscribe",
			Severity: "warning",
			Message:  "Promotional campaign does not require unsubscribe handling.",
			Blocking: false,
		})
	}
	return issues, nil
}

func (s *Service) LaunchOutboundCampaign(ctx context.Context, u models.User, campaignID string) ([]models.OutboundPreflightIssue, error) {
	campaign, err := s.st.GetOutboundCampaignByID(ctx, u.ID, campaignID)
	if err != nil {
		return nil, err
	}
	issues, err := s.PreflightOutboundCampaign(ctx, u, campaign)
	if err != nil {
		return nil, err
	}
	for _, item := range issues {
		if item.Blocking {
			return issues, nil
		}
	}
	enrollments, err := s.st.ListOutboundEnrollmentsByCampaign(ctx, u.ID, campaign.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, enrollment := range enrollments {
		if enrollment.Status == "completed" || enrollment.Status == "stopped" || enrollment.Status == "bounced" || enrollment.Status == "unsubscribed" {
			continue
		}
		if enrollment.Status == "waiting_reply" || enrollment.Status == "paused" || enrollment.Status == "manual_only" {
			continue
		}
		if enrollment.NextActionAt.IsZero() {
			enrollment.NextActionAt = now
		}
		enrollment.Status = "scheduled"
		if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
			return nil, err
		}
	}
	if err := s.st.SetOutboundCampaignStatus(ctx, u.ID, campaign.ID, "running", now, time.Time{}); err != nil {
		return nil, err
	}
	return issues, nil
}

func (s *Service) ApplyOutboundPlaybook(ctx context.Context, u models.User, campaignID, playbookKey string, replaceSteps bool) (models.OutboundCampaign, []models.OutboundCampaignStep, error) {
	campaign, err := s.st.GetOutboundCampaignByID(ctx, u.ID, campaignID)
	if err != nil {
		return models.OutboundCampaign{}, nil, err
	}
	playbook, ok := outboundPlaybookByKey(playbookKey)
	if !ok {
		return models.OutboundCampaign{}, nil, fmt.Errorf("playbook not found")
	}
	campaign.GoalKind = playbook.GoalKind
	campaign.PlaybookKey = playbook.Key
	campaign.CampaignMode = playbook.CampaignMode
	if strings.TrimSpace(playbook.AudienceSourceKind) != "" {
		campaign.AudienceSourceKind = playbook.AudienceSourceKind
	}
	if strings.TrimSpace(playbook.SenderPolicyKind) != "" {
		campaign.SenderPolicyKind = playbook.SenderPolicyKind
	}
	campaign.ReplyPolicyJSON = mustCompactOutboundJSON(playbook.ReplyPolicy)
	campaign.SuppressionPolicyJSON = mustCompactOutboundJSON(playbook.SuppressionPolicy)
	campaign.SchedulePolicyJSON = mustCompactOutboundJSON(playbook.SchedulePolicy)
	campaign.CompliancePolicyJSON = mustCompactOutboundJSON(playbook.CompliancePolicy)
	campaign.GovernancePolicyJSON = mustCompactOutboundJSON(playbook.GovernancePolicy)
	updated, err := s.st.UpdateOutboundCampaign(ctx, campaign)
	if err != nil {
		return models.OutboundCampaign{}, nil, err
	}
	if replaceSteps {
		if err := s.st.DeleteOutboundCampaignSteps(ctx, updated.ID); err != nil {
			return models.OutboundCampaign{}, nil, err
		}
	}
	currentSteps, err := s.st.ListOutboundCampaignSteps(ctx, updated.ID)
	if err != nil {
		return models.OutboundCampaign{}, nil, err
	}
	if replaceSteps || len(currentSteps) == 0 {
		for _, item := range playbook.Steps {
			if _, err := s.st.CreateOutboundCampaignStep(ctx, models.OutboundCampaignStep{
				CampaignID:          updated.ID,
				Position:            item.Position,
				Kind:                item.Kind,
				ThreadMode:          item.ThreadMode,
				SubjectTemplate:     item.SubjectTemplate,
				BodyTemplate:        item.BodyTemplate,
				WaitIntervalMinutes: item.WaitIntervalMinutes,
				SendWindowJSON:      "{}",
				TaskPolicyJSON:      mustCompactOutboundJSON(item.TaskPolicy),
				BranchPolicyJSON:    mustCompactOutboundJSON(item.BranchPolicy),
				StopIfReplied:       true,
				StopIfUnsubscribed:  true,
			}); err != nil {
				return models.OutboundCampaign{}, nil, err
			}
		}
	}
	steps, err := s.st.ListOutboundCampaignSteps(ctx, updated.ID)
	if err != nil {
		return models.OutboundCampaign{}, nil, err
	}
	return updated, steps, nil
}

func (s *Service) stepForOutboundSend(steps []models.OutboundCampaignStep, enrollment models.OutboundEnrollment) (models.OutboundCampaignStep, bool) {
	if len(steps) == 0 {
		return models.OutboundCampaignStep{}, false
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Position < steps[j].Position })
	if strings.TrimSpace(enrollment.LastSentMessageID) == "" {
		return steps[0], true
	}
	nextPos := enrollment.CurrentStepPosition + 1
	for _, step := range steps {
		if step.Position == nextPos {
			return step, true
		}
	}
	return models.OutboundCampaignStep{}, false
}

func (s *Service) DispatchOutboundEnrollment(ctx context.Context, enrollment models.OutboundEnrollment) error {
	campaign, err := s.st.GetOutboundCampaignByIDAny(ctx, enrollment.CampaignID)
	if err != nil {
		return err
	}
	user, err := s.st.GetUserByID(ctx, campaign.UserID)
	if err != nil {
		return err
	}
	steps, err := s.st.ListOutboundCampaignSteps(ctx, campaign.ID)
	if err != nil {
		return err
	}
	step, ok := s.stepForOutboundSend(steps, enrollment)
	if !ok {
		enrollment.Status = "completed"
		enrollment.NextActionAt = time.Time{}
		enrollment.PauseReason = ""
		enrollment.StopReason = ""
		if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
			return err
		}
		_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
			CampaignID:   campaign.ID,
			EnrollmentID: enrollment.ID,
			EventKind:    "completed_no_more_steps",
			ActorKind:    "worker",
		})
		return nil
	}
	if step.Kind == "manual_task" {
		task := parseOutboundTaskPolicy(step.TaskPolicyJSON)
		enrollment.Status = "manual_only"
		enrollment.CurrentStepPosition = step.Position
		enrollment.ManualOwnerUserID = campaign.UserID
		enrollment.PauseReason = "manual_task"
		enrollment.StopReason = ""
		enrollment.NextActionAt = time.Time{}
		if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"step_position": step.Position,
			"title":         task.Title,
			"instructions":  task.Instructions,
			"action_label":  task.ActionLabel,
		})
		_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
			CampaignID:       campaign.ID,
			EnrollmentID:     enrollment.ID,
			EventKind:        "manual_task_due",
			EventPayloadJSON: string(payload),
			ActorKind:        "worker",
		})
		return nil
	}
	rendered, err := s.renderOutboundStep(ctx, user, campaign, step, enrollment)
	if err != nil {
		enrollment.Status = "manual_only"
		enrollment.PauseReason = "send_failed"
		enrollment.NextActionAt = time.Time{}
		if _, saveErr := s.st.UpsertOutboundEnrollment(ctx, enrollment); saveErr != nil {
			return saveErr
		}
		_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
			CampaignID:       campaign.ID,
			EnrollmentID:     enrollment.ID,
			EventKind:        "preflight_blocked",
			EventPayloadJSON: fmt.Sprintf(`{"reason":%q}`, err.Error()),
			ActorKind:        "worker",
		})
		return err
	}
	if len(rendered.Missing) > 0 {
		enrollment.Status = "manual_only"
		enrollment.PauseReason = "template_missing"
		enrollment.NextActionAt = time.Time{}
		if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{
			"missing": rendered.Missing,
		})
		_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
			CampaignID:       campaign.ID,
			EnrollmentID:     enrollment.ID,
			EventKind:        "preflight_blocked",
			EventPayloadJSON: string(payload),
			ActorKind:        "worker",
		})
		return nil
	}
	if activeSuppression, err := s.st.GetActiveOutboundSuppression(ctx, campaign.UserID, enrollment.RecipientEmail, enrollment.RecipientDomain, campaign.ID, time.Now().UTC()); err == nil {
		enrollment.Status = "stopped"
		enrollment.StopReason = firstNonEmptyString(activeSuppression.Reason, "suppressed")
		enrollment.PauseReason = ""
		enrollment.NextActionAt = time.Time{}
		if _, saveErr := s.st.UpsertOutboundEnrollment(ctx, enrollment); saveErr != nil {
			return saveErr
		}
		payload, _ := json.Marshal(map[string]any{
			"scope_kind":  activeSuppression.ScopeKind,
			"scope_value": activeSuppression.ScopeValue,
			"reason":      activeSuppression.Reason,
			"source_kind": activeSuppression.SourceKind,
		})
		_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
			CampaignID:       campaign.ID,
			EnrollmentID:     enrollment.ID,
			EventKind:        "preflight_blocked",
			EventPayloadJSON: string(payload),
			ActorKind:        "worker",
			ActorRef:         "suppression",
		})
		return nil
	} else if err != nil && err != store.ErrNotFound {
		return err
	}
	schedule := parseOutboundSchedulePolicy(campaign.SchedulePolicyJSON)
	if schedule.RespectProviderCaps {
		pacing := outboundProviderPacingForAccount(rendered.Selection.Account)
		if pacing.DailyCap > 0 {
			sends24h, err := s.outboundAccountSendsSince(ctx, campaign.UserID, rendered.Selection.Account.ID, time.Now().UTC().Add(-24*time.Hour))
			if err != nil {
				return err
			}
			if sends24h >= pacing.DailyCap {
				enrollment.Status = "scheduled"
				enrollment.PauseReason = "provider_pacing"
				enrollment.NextActionAt = time.Now().UTC().Add(12 * time.Hour)
				if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{
					"sender_account_id": rendered.Selection.Account.ID,
					"window":            "24h",
					"daily_cap":         pacing.DailyCap,
				})
				_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
					CampaignID:       campaign.ID,
					EnrollmentID:     enrollment.ID,
					EventKind:        "provider_pacing_delayed",
					EventPayloadJSON: string(payload),
					ActorKind:        "worker",
				})
				return nil
			}
		}
		if pacing.HourlyCap > 0 {
			sends1h, err := s.outboundAccountSendsSince(ctx, campaign.UserID, rendered.Selection.Account.ID, time.Now().UTC().Add(-1*time.Hour))
			if err != nil {
				return err
			}
			if sends1h >= pacing.HourlyCap {
				delay := time.Duration(pacing.GapSeconds) * time.Second
				if delay < 15*time.Minute {
					delay = 15 * time.Minute
				}
				enrollment.Status = "scheduled"
				enrollment.PauseReason = "provider_pacing"
				enrollment.NextActionAt = time.Now().UTC().Add(delay)
				if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
					return err
				}
				payload, _ := json.Marshal(map[string]any{
					"sender_account_id": rendered.Selection.Account.ID,
					"window":            "1h",
					"hourly_cap":        pacing.HourlyCap,
					"gap_seconds":       pacing.GapSeconds,
				})
				_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
					CampaignID:       campaign.ID,
					EnrollmentID:     enrollment.ID,
					EventKind:        "provider_pacing_delayed",
					EventPayloadJSON: string(payload),
					ActorKind:        "worker",
				})
				return nil
			}
		}
	}
	compliance := parseOutboundCompliancePolicy(campaign.CompliancePolicyJSON)
	body := appendOutboundComplianceFooter(rendered.Body, compliance)
	subject := strings.TrimSpace(rendered.Subject)
	var binding models.MailThreadBinding
	binding, _ = s.st.GetMailThreadBindingByEnrollment(ctx, enrollment.ID)
	if strings.TrimSpace(binding.ID) == "" {
		seed := parseOutboundSeedContext(enrollment.SeedContextJSON)
		if seed.AccountID != "" || seed.ThreadID != "" || seed.MessageID != "" || seed.ThreadSubject != "" {
			binding = models.MailThreadBinding{
				ID:                    uuid.NewString(),
				AccountID:             firstNonEmptyString(seed.AccountID, rendered.Selection.Account.ID),
				ThreadID:              seed.ThreadID,
				BindingType:           "manual",
				CampaignID:            campaign.ID,
				EnrollmentID:          enrollment.ID,
				OwnerUserID:           campaign.UserID,
				RecipientEmail:        enrollment.RecipientEmail,
				RecipientDomain:       enrollment.RecipientDomain,
				RootOutboundMessageID: seed.MessageID,
				LastOutboundMessageID: seed.MessageID,
				ThreadSubject:         seed.ThreadSubject,
				CreatedAt:             time.Now().UTC(),
			}
		}
	}
	req := mail.SendRequest{
		From:            rendered.Selection.HeaderFromEmail,
		HeaderFromName:  rendered.Selection.HeaderFromName,
		HeaderFromEmail: rendered.Selection.HeaderFromEmail,
		EnvelopeFrom:    rendered.Selection.EnvelopeFrom,
		ReplyTo:         rendered.Selection.ReplyTo,
		To:              []string{enrollment.RecipientEmail},
		Subject:         subject,
		Body:            body,
		MessageID:       nextOutboundMessageID(rendered.Selection.HeaderFromEmail),
	}
	if step.ThreadMode == "same_thread" && strings.TrimSpace(binding.LastOutboundMessageID) != "" {
		req.InReplyToID = mail.NormalizeMessageIDHeader(binding.LastOutboundMessageID)
		references := make([]string, 0, 2)
		if root := mail.NormalizeMessageIDHeader(binding.RootOutboundMessageID); root != "" {
			references = append(references, root)
		}
		if last := mail.NormalizeMessageIDHeader(binding.LastOutboundMessageID); last != "" && (len(references) == 0 || references[len(references)-1] != last) {
			references = append(references, last)
		}
		req.References = mail.NormalizeMessageIDHeaders(references)
		if subject == "" {
			req.Subject = firstNonEmptyString(binding.ThreadSubject, subject)
		}
	}
	if strings.TrimSpace(req.Subject) == "" && strings.TrimSpace(binding.ThreadSubject) != "" {
		req.Subject = strings.TrimSpace(binding.ThreadSubject)
	}
	pass, err := s.decryptOutboundAccountSecret(rendered.Selection.Account)
	if err != nil {
		return err
	}
	sendClient := s.outboundMailClientForAccount(rendered.Selection.Account)
	if _, err := sendClient.Send(ctx, rendered.Selection.Account.Login, pass, req); err != nil {
		enrollment.Status = "manual_only"
		enrollment.PauseReason = "send_failed"
		enrollment.NextActionAt = time.Time{}
		if _, saveErr := s.st.UpsertOutboundEnrollment(ctx, enrollment); saveErr != nil {
			return saveErr
		}
		payload, _ := json.Marshal(map[string]any{
			"error": err.Error(),
		})
		_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
			CampaignID:       campaign.ID,
			EnrollmentID:     enrollment.ID,
			EventKind:        "send_failed",
			EventPayloadJSON: string(payload),
			ActorKind:        "worker",
		})
		return nil
	}
	now := time.Now().UTC()
	if strings.TrimSpace(binding.ID) == "" {
		binding = models.MailThreadBinding{
			ID:                    uuid.NewString(),
			AccountID:             rendered.Selection.Account.ID,
			BindingType:           "campaign",
			CampaignID:            campaign.ID,
			EnrollmentID:          enrollment.ID,
			ReplyAccountID:        firstNonEmptyString(rendered.Selection.ReplyAccountID, rendered.Selection.Account.ID),
			ReplySenderProfileID:  rendered.Selection.SenderProfileID,
			OwnerUserID:           campaign.UserID,
			RecipientEmail:        enrollment.RecipientEmail,
			RecipientDomain:       enrollment.RecipientDomain,
			RootOutboundMessageID: req.MessageID,
			ThreadSubject:         req.Subject,
			CreatedAt:             now,
		}
	}
	if strings.TrimSpace(rendered.Selection.ReplyFunnelID) != "" {
		binding.BindingType = "reply_funnel"
		binding.FunnelID = rendered.Selection.ReplyFunnelID
		binding.CollectorAccountID = rendered.Selection.CollectorAccountID
	}
	binding.AccountID = rendered.Selection.Account.ID
	binding.ReplyAccountID = firstNonEmptyString(rendered.Selection.ReplyAccountID, rendered.Selection.Account.ID)
	binding.ReplySenderProfileID = rendered.Selection.SenderProfileID
	binding.OwnerUserID = campaign.UserID
	binding.CampaignID = campaign.ID
	binding.EnrollmentID = enrollment.ID
	binding.RecipientEmail = enrollment.RecipientEmail
	binding.RecipientDomain = enrollment.RecipientDomain
	if strings.TrimSpace(binding.RootOutboundMessageID) == "" {
		binding.RootOutboundMessageID = req.MessageID
	}
	binding.LastOutboundMessageID = req.MessageID
	if strings.TrimSpace(binding.ThreadSubject) == "" {
		binding.ThreadSubject = req.Subject
	}
	binding, err = s.st.UpsertMailThreadBinding(ctx, binding)
	if err != nil {
		return err
	}
	enrollment.ThreadBindingID = binding.ID
	enrollment.SenderAccountID = rendered.Selection.Account.ID
	enrollment.SenderProfileID = rendered.Selection.SenderProfileID
	enrollment.ReplyFunnelID = rendered.Selection.ReplyFunnelID
	enrollment.LastSentMessageID = req.MessageID
	enrollment.LastSentAt = now
	enrollment.CurrentStepPosition = step.Position
	enrollment.PauseReason = ""
	enrollment.StopReason = ""
	enrollment.ReplyOutcome = ""
	enrollment.ReplyConfidence = 0
	nextStep, hasNext := s.stepForOutboundSend(steps, models.OutboundEnrollment{
		LastSentMessageID:   req.MessageID,
		CurrentStepPosition: step.Position,
	})
	if hasNext {
		waitMinutes := nextStep.WaitIntervalMinutes
		if waitMinutes < 0 {
			waitMinutes = 0
		}
		enrollment.Status = "scheduled"
		enrollment.NextActionAt = now.Add(time.Duration(waitMinutes) * time.Minute)
	} else {
		enrollment.Status = "waiting_reply"
		enrollment.NextActionAt = time.Time{}
	}
	if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"step_position":     step.Position,
		"message_id":        req.MessageID,
		"sender_account_id": rendered.Selection.Account.ID,
		"sender_profile_id": rendered.Selection.SenderProfileID,
		"recipient_email":   enrollment.RecipientEmail,
		"recipient_domain":  enrollment.RecipientDomain,
		"reply_funnel_id":   rendered.Selection.ReplyFunnelID,
		"collector_account": rendered.Selection.CollectorAccountID,
	})
	_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
		CampaignID:       campaign.ID,
		EnrollmentID:     enrollment.ID,
		EventKind:        "step_sent",
		EventPayloadJSON: string(payload),
		ActorKind:        "worker",
	})
	return nil
}

func recipientStateStatusForOutcome(outcome string) string {
	switch normalizeReplyOutcome(outcome) {
	case "positive_interest":
		return "interested"
	case "meeting_intent":
		return "meeting_booked"
	case "question", "objection", "referral", "out_of_office", "auto_reply_other", "manual_review_required":
		return "replied"
	case "wrong_person":
		return "wrong_person"
	case "not_interested":
		return "not_interested"
	case "unsubscribe_request":
		return "unsubscribed"
	case "bounce":
		return "hard_bounce"
	case "hostile":
		return "suppressed"
	default:
		return "active"
	}
}

func (s *Service) applyOutcomeDomainActions(ctx context.Context, campaign models.OutboundCampaign, enrollment models.OutboundEnrollment, outcome string) error {
	replyPolicy := parseOutboundReplyPolicy(campaign.ReplyPolicyJSON)
	suppressionPolicy := parseOutboundSuppressionPolicy(campaign.SuppressionPolicyJSON)
	governance := parseOutboundGovernancePolicy(campaign.GovernancePolicyJSON)
	action := "none"
	switch normalizeReplyOutcome(outcome) {
	case "positive_interest", "meeting_intent":
		if governance.PositiveDomainAction != "none" {
			action = governance.PositiveDomainAction
		} else if replyPolicy.StopSameDomainOnReply || replyPolicy.StopSameDomainOnPositive || suppressionPolicy.SameDomainPositiveStop {
			action = "stop_campaign"
		}
	case "not_interested", "wrong_person", "referral":
		if governance.NegativeDomainAction != "none" {
			action = governance.NegativeDomainAction
		} else if replyPolicy.StopSameDomainOnReply || replyPolicy.StopSameDomainOnNegative || suppressionPolicy.SameDomainNegativeStop {
			action = "stop_campaign"
		}
	case "unsubscribe_request":
		if governance.UnsubscribeDomainAction != "none" {
			action = governance.UnsubscribeDomainAction
		} else if suppressionPolicy.SameDomainUnsubscribeSuppress {
			if suppressionPolicy.WorkspaceSuppressUnsubscribe {
				action = "suppress_workspace"
			} else {
				action = "suppress_campaign"
			}
		} else {
			action = "stop_campaign"
		}
	case "hostile":
		if governance.HostileDomainAction != "none" {
			action = governance.HostileDomainAction
		} else if suppressionPolicy.SameDomainHostileSuppress || replyPolicy.SuppressSameDomainOnHostile {
			action = "suppress_workspace"
		} else {
			action = "stop_campaign"
		}
	}
	if action != "none" {
		others, err := s.st.ListActiveOutboundEnrollmentsByDomain(ctx, campaign.UserID, enrollment.RecipientDomain, "")
		if err != nil {
			return err
		}
		for _, other := range others {
			if other.ID == enrollment.ID {
				continue
			}
			if (action == "stop_campaign" || action == "suppress_campaign") && other.CampaignID != campaign.ID {
				continue
			}
			switch action {
			case "pause_workspace":
				other.Status = "paused"
				other.PauseReason = "same_domain_reply"
				other.StopReason = ""
				other.NextActionAt = time.Time{}
			default:
				other.Status = "stopped"
				other.StopReason = "same_domain_reply"
				other.PauseReason = ""
				other.NextActionAt = time.Time{}
			}
			if _, err := s.st.UpsertOutboundEnrollment(ctx, other); err != nil {
				return err
			}
			_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
				CampaignID:   campaign.ID,
				EnrollmentID: other.ID,
				EventKind:    "domain_governed",
				ActorKind:    "classifier",
				ActorRef:     action,
			})
		}
	}
	if action == "suppress_campaign" || action == "suppress_workspace" {
		scopeCampaignID := campaign.ID
		if action == "suppress_workspace" {
			scopeCampaignID = ""
		}
		_, err := s.st.UpsertOutboundSuppression(ctx, models.OutboundSuppression{
			UserID:     campaign.UserID,
			ScopeKind:  "domain",
			ScopeValue: enrollment.RecipientDomain,
			CampaignID: scopeCampaignID,
			Reason:     fmt.Sprintf("domain suppression due to %s", outcome),
			SourceKind: "reply_policy",
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) applyOutboundReplyOutcome(ctx context.Context, campaign models.OutboundCampaign, enrollment models.OutboundEnrollment, binding models.MailThreadBinding, indexed models.IndexedMessage, outcome string, confidence float64, resumeAt time.Time) error {
	outcome = normalizeReplyOutcome(outcome)
	replyAt := indexed.DateHeader
	if replyAt.IsZero() {
		replyAt = indexed.InternalDate
	}
	if replyAt.IsZero() {
		replyAt = time.Now().UTC()
	}
	replyPolicy := parseOutboundReplyPolicy(campaign.ReplyPolicyJSON)
	suppressionPolicy := parseOutboundSuppressionPolicy(campaign.SuppressionPolicyJSON)
	compliance := parseOutboundCompliancePolicy(campaign.CompliancePolicyJSON)
	binding.LastReplyMessageID = indexed.ID
	binding.LastReplyAt = replyAt
	if strings.TrimSpace(indexed.ThreadID) != "" {
		binding.ThreadID = indexed.ThreadID
	}
	if strings.TrimSpace(indexed.Subject) != "" {
		binding.ThreadSubject = indexed.Subject
	}
	if _, err := s.st.UpsertMailThreadBinding(ctx, binding); err != nil {
		return err
	}
	enrollment.ThreadBindingID = binding.ID
	enrollment.ReplyOutcome = outcome
	enrollment.ReplyConfidence = confidence
	enrollment.LastReplyMessageID = indexed.ID
	enrollment.NextActionAt = time.Time{}
	enrollment.PauseReason = ""
	enrollment.StopReason = ""
	steps, _ := s.st.ListOutboundCampaignSteps(ctx, campaign.ID)
	branched := s.applyOutboundReplyBranch(campaign, &enrollment, steps, outcome)
	if !branched {
		switch outcome {
		case "positive_interest", "meeting_intent":
			enrollment.Status = "stopped"
			enrollment.StopReason = outcome
		case "question":
			if replyPolicy.PauseOnQuestion {
				enrollment.Status = "manual_only"
				enrollment.ManualOwnerUserID = campaign.UserID
			} else {
				enrollment.Status = "stopped"
				enrollment.StopReason = outcome
			}
		case "objection":
			if replyPolicy.PauseOnObjection {
				enrollment.Status = "manual_only"
				enrollment.ManualOwnerUserID = campaign.UserID
			} else {
				enrollment.Status = "stopped"
				enrollment.StopReason = outcome
			}
		case "referral", "wrong_person", "not_interested", "hostile":
			enrollment.Status = "stopped"
			enrollment.StopReason = outcome
		case "unsubscribe_request":
			enrollment.Status = "unsubscribed"
			enrollment.StopReason = outcome
		case "bounce":
			enrollment.Status = "bounced"
			enrollment.StopReason = outcome
		case "out_of_office":
			if replyPolicy.PauseOnOutOfOffice {
				enrollment.Status = "paused"
				enrollment.PauseReason = "out_of_office"
				if replyPolicy.AutoResumeOutOfOffice && !resumeAt.IsZero() {
					enrollment.NextActionAt = resumeAt
				}
			} else {
				enrollment.Status = "manual_only"
				enrollment.ManualOwnerUserID = campaign.UserID
			}
		case "auto_reply_other", "manual_review_required":
			if replyPolicy.PauseOnManualReview {
				enrollment.Status = "manual_only"
				enrollment.ManualOwnerUserID = campaign.UserID
			} else {
				enrollment.Status = "paused"
				enrollment.PauseReason = "manual_review"
			}
		default:
			if replyPolicy.StopOnReply {
				enrollment.Status = "stopped"
				enrollment.StopReason = "reply"
			}
		}
	}
	if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
		return err
	}
	state := models.RecipientState{
		UserID:            campaign.UserID,
		RecipientEmail:    enrollment.RecipientEmail,
		PrimaryContactID:  enrollment.ContactID,
		RecipientDomain:   enrollment.RecipientDomain,
		Status:            recipientStateStatusForOutcome(outcome),
		Scope:             "workspace",
		LastReplyAt:       replyAt,
		LastReplyOutcome:  outcome,
		SuppressionReason: "",
		Notes:             "",
	}
	switch outcome {
	case "unsubscribe_request":
		state.SuppressionReason = "unsubscribe_request"
	case "hostile":
		state.SuppressionReason = "hostile_reply"
	case "bounce":
		state.SuppressionReason = "bounce"
	}
	if _, err := s.st.UpsertRecipientState(ctx, state); err != nil {
		return err
	}
	switch outcome {
	case "unsubscribe_request":
		scopeCampaignID := campaign.ID
		if strings.EqualFold(strings.TrimSpace(compliance.UnsubscribeScope), "workspace") || suppressionPolicy.WorkspaceSuppressUnsubscribe {
			scopeCampaignID = ""
		}
		if _, err := s.st.UpsertOutboundSuppression(ctx, models.OutboundSuppression{
			UserID:     campaign.UserID,
			ScopeKind:  "recipient",
			ScopeValue: enrollment.RecipientEmail,
			CampaignID: scopeCampaignID,
			Reason:     "recipient requested unsubscribe",
			SourceKind: "unsubscribe",
		}); err != nil {
			return err
		}
	case "bounce":
		scopeCampaignID := campaign.ID
		if suppressionPolicy.WorkspaceSuppressBounce {
			scopeCampaignID = ""
		}
		if _, err := s.st.UpsertOutboundSuppression(ctx, models.OutboundSuppression{
			UserID:     campaign.UserID,
			ScopeKind:  "recipient",
			ScopeValue: enrollment.RecipientEmail,
			CampaignID: scopeCampaignID,
			Reason:     "hard bounce",
			SourceKind: "bounce",
		}); err != nil {
			return err
		}
	case "hostile":
		if _, err := s.st.UpsertOutboundSuppression(ctx, models.OutboundSuppression{
			UserID:     campaign.UserID,
			ScopeKind:  "recipient",
			ScopeValue: enrollment.RecipientEmail,
			CampaignID: "",
			Reason:     "hostile reply",
			SourceKind: "reply_policy",
		}); err != nil {
			return err
		}
	}
	if err := s.applyOutcomeDomainActions(ctx, campaign, enrollment, outcome); err != nil {
		return err
	}
	detectedPayload, _ := json.Marshal(map[string]any{
		"message_id": indexed.ID,
		"thread_id":  indexed.ThreadID,
		"outcome":    outcome,
	})
	_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
		CampaignID:       campaign.ID,
		EnrollmentID:     enrollment.ID,
		EventKind:        "reply_detected",
		EventPayloadJSON: string(detectedPayload),
		ActorKind:        "worker",
	})
	classifiedPayload, _ := json.Marshal(map[string]any{
		"message_id":       indexed.ID,
		"reply_outcome":    outcome,
		"reply_confidence": confidence,
		"status":           enrollment.Status,
		"pause_reason":     enrollment.PauseReason,
		"stop_reason":      enrollment.StopReason,
		"next_action_at":   enrollment.NextActionAt,
	})
	_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
		CampaignID:       campaign.ID,
		EnrollmentID:     enrollment.ID,
		EventKind:        "reply_classified",
		EventPayloadJSON: string(classifiedPayload),
		ActorKind:        "classifier",
		ActorRef:         outcome,
	})
	return nil
}

func (s *Service) ProcessOutboundIndexedMessage(ctx context.Context, account models.MailAccount, indexed models.IndexedMessage) error {
	if strings.TrimSpace(indexed.MessageIDHeader) != "" {
		if binding, err := s.st.FindMailThreadBindingByOutboundMessageID(ctx, account.ID, indexed.MessageIDHeader); err == nil {
			if strings.TrimSpace(indexed.ThreadID) != "" {
				binding.ThreadID = indexed.ThreadID
			}
			if strings.TrimSpace(indexed.Subject) != "" {
				binding.ThreadSubject = indexed.Subject
			}
			if _, err := s.st.UpsertMailThreadBinding(ctx, binding); err != nil {
				return err
			}
		}
	}
	identities, err := s.st.ListMailIdentities(ctx, account.ID)
	if err != nil {
		return err
	}
	if outboundLooksLikeSelfMessage(indexed, account, identities) {
		return nil
	}
	headers := mail.ParseMessageIDList(indexed.ReferencesHeader)
	if inReplyTo := mail.NormalizeMessageIDHeader(indexed.InReplyToHeader); inReplyTo != "" {
		headers = append(headers, inReplyTo)
	}
	headers = mail.NormalizeMessageIDHeaders(headers)
	if len(headers) == 0 {
		return nil
	}
	binding, err := s.st.FindMailThreadBindingByReplyHeaders(ctx, account.ID, headers)
	if err != nil {
		if err == store.ErrNotFound {
			return nil
		}
		return err
	}
	if strings.TrimSpace(binding.LastReplyMessageID) == strings.TrimSpace(indexed.ID) {
		return nil
	}
	campaign, err := s.st.GetOutboundCampaignByIDAny(ctx, binding.CampaignID)
	if err != nil {
		return err
	}
	enrollment, err := s.st.GetOutboundEnrollmentByID(ctx, campaign.UserID, binding.EnrollmentID)
	if err != nil {
		return err
	}
	outcome, confidence, resumeAt := classifyOutboundReply(indexed)
	return s.applyOutboundReplyOutcome(ctx, campaign, enrollment, binding, indexed, outcome, confidence, resumeAt)
}

func (s *Service) ApplyManualReplyOutcome(ctx context.Context, u models.User, enrollmentID, outcome string, confidence float64) error {
	enrollment, err := s.st.GetOutboundEnrollmentByID(ctx, u.ID, enrollmentID)
	if err != nil {
		return err
	}
	campaign, err := s.st.GetOutboundCampaignByID(ctx, u.ID, enrollment.CampaignID)
	if err != nil {
		return err
	}
	binding, err := s.st.GetMailThreadBindingByEnrollment(ctx, enrollment.ID)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	indexed := models.IndexedMessage{
		ID:           binding.LastReplyMessageID,
		ThreadID:     binding.ThreadID,
		Subject:      binding.ThreadSubject,
		DateHeader:   binding.LastReplyAt,
		InternalDate: binding.LastReplyAt,
	}
	replyAccountID := strings.TrimSpace(binding.ReplyAccountID)
	if replyAccountID == "" {
		replyAccountID = strings.TrimSpace(binding.CollectorAccountID)
	}
	if replyAccountID == "" {
		replyAccountID = strings.TrimSpace(binding.AccountID)
	}
	if strings.TrimSpace(binding.LastReplyMessageID) != "" && replyAccountID != "" {
		if item, err := s.st.GetIndexedMessageByID(ctx, replyAccountID, binding.LastReplyMessageID); err == nil {
			indexed = item
		}
	}
	if confidence <= 0 {
		confidence = 1
	}
	return s.applyOutboundReplyOutcome(ctx, campaign, enrollment, binding, indexed, outcome, confidence, time.Time{})
}

func (s *Service) ApplyReplyOpsAction(ctx context.Context, u models.User, enrollmentID, action, scopeValue string, until time.Time) error {
	enrollment, err := s.st.GetOutboundEnrollmentByID(ctx, u.ID, enrollmentID)
	if err != nil {
		return err
	}
	campaign, err := s.st.GetOutboundCampaignByID(ctx, u.ID, enrollment.CampaignID)
	if err != nil {
		return err
	}
	binding, err := s.st.GetMailThreadBindingByEnrollment(ctx, enrollment.ID)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "takeover":
		enrollment.Status = "manual_only"
		enrollment.ManualOwnerUserID = u.ID
		enrollment.PauseReason = ""
		enrollment.NextActionAt = time.Time{}
	case "pause":
		enrollment.Status = "paused"
		enrollment.PauseReason = "manual_pause"
		enrollment.NextActionAt = until
	case "resume":
		enrollment.Status = "scheduled"
		enrollment.PauseReason = ""
		enrollment.NextActionAt = time.Now().UTC()
	case "stop":
		enrollment.Status = "stopped"
		enrollment.StopReason = firstNonEmptyString(scopeValue, "manual_stop")
		enrollment.PauseReason = ""
		enrollment.NextActionAt = time.Time{}
	case "suppress_recipient":
		_, err := s.st.UpsertOutboundSuppression(ctx, models.OutboundSuppression{
			UserID:     u.ID,
			ScopeKind:  "recipient",
			ScopeValue: firstNonEmptyString(scopeValue, enrollment.RecipientEmail),
			CampaignID: "",
			Reason:     "manual suppression",
			SourceKind: "manual",
			ExpiresAt:  until,
		})
		if err != nil {
			return err
		}
		enrollment.Status = "stopped"
		enrollment.StopReason = "manual_suppression"
		enrollment.NextActionAt = time.Time{}
	case "suppress_domain":
		domain := firstNonEmptyString(scopeValue, enrollment.RecipientDomain)
		_, err := s.st.UpsertOutboundSuppression(ctx, models.OutboundSuppression{
			UserID:     u.ID,
			ScopeKind:  "domain",
			ScopeValue: domain,
			CampaignID: campaign.ID,
			Reason:     "manual domain suppression",
			SourceKind: "manual",
			ExpiresAt:  until,
		})
		if err != nil {
			return err
		}
		enrollment.Status = "stopped"
		enrollment.StopReason = "manual_domain_suppression"
		enrollment.NextActionAt = time.Time{}
	default:
		return fmt.Errorf("unsupported reply-ops action")
	}
	if _, err := s.st.UpsertOutboundEnrollment(ctx, enrollment); err != nil {
		return err
	}
	if strings.TrimSpace(binding.ID) != "" && strings.TrimSpace(binding.OwnerUserID) == "" {
		binding.OwnerUserID = campaign.UserID
		if _, err := s.st.UpsertMailThreadBinding(ctx, binding); err != nil {
			return err
		}
	}
	_, _ = s.st.AppendOutboundEvent(ctx, models.OutboundEvent{
		CampaignID:   campaign.ID,
		EnrollmentID: enrollment.ID,
		EventKind:    strings.ToLower(strings.TrimSpace(action)),
		ActorKind:    "user",
		ActorRef:     u.ID,
	})
	return nil
}
