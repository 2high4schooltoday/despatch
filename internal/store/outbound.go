package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/models"
)

type outboundScanner interface {
	Scan(dest ...any) error
}

func normalizeOutboundCampaignStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running":
		return "running"
	case "paused":
		return "paused"
	case "completed":
		return "completed"
	case "archived":
		return "archived"
	default:
		return "draft"
	}
}

func normalizeOutboundGoalKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "thread_revival":
		return "thread_revival"
	case "find_owner":
		return "find_owner"
	case "referral_request":
		return "referral_request"
	case "reengage_dormant":
		return "reengage_dormant"
	default:
		return "general_outreach"
	}
}

func normalizeOutboundCampaignMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "existing_threads":
		return "existing_threads"
	default:
		return "new_threads"
	}
}

func normalizeOutboundAudienceSourceKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "contact_group":
		return "contact_group"
	case "saved_search":
		return "saved_search"
	case "csv_import":
		return "csv_import"
	default:
		return "manual"
	}
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

func normalizeOutboundThreadMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "new_thread":
		return "new_thread"
	default:
		return "same_thread"
	}
}

func normalizeOutboundStepKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "manual_task":
		return "manual_task"
	case "email":
		return "email"
	default:
		return "email"
	}
}

func normalizeOutboundEnrollmentStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "scheduled":
		return "scheduled"
	case "sending":
		return "sending"
	case "waiting_reply":
		return "waiting_reply"
	case "paused":
		return "paused"
	case "stopped":
		return "stopped"
	case "completed":
		return "completed"
	case "bounced":
		return "bounced"
	case "unsubscribed":
		return "unsubscribed"
	case "manual_only":
		return "manual_only"
	default:
		return "pending"
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

func replyOutcomeBucket(outcome, status string) string {
	switch normalizeReplyOutcome(outcome) {
	case "positive_interest", "meeting_intent":
		return "interested"
	case "question":
		return "questions"
	case "objection":
		return "objections"
	case "wrong_person", "referral":
		return "wrong_person"
	case "out_of_office", "auto_reply_other":
		return "out_of_office"
	case "bounce":
		return "bounces"
	case "unsubscribe_request":
		return "unsubscribed"
	case "hostile":
		return "hostile"
	case "not_interested":
		return "needs_review"
	}
	switch normalizeOutboundEnrollmentStatus(status) {
	case "bounced":
		return "bounces"
	case "unsubscribed":
		return "unsubscribed"
	}
	return "needs_review"
}

func normalizeRecipientStateStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "replied":
		return "replied"
	case "interested":
		return "interested"
	case "not_interested":
		return "not_interested"
	case "wrong_person":
		return "wrong_person"
	case "meeting_booked":
		return "meeting_booked"
	case "unsubscribed":
		return "unsubscribed"
	case "hard_bounce":
		return "hard_bounce"
	case "suppressed":
		return "suppressed"
	default:
		return "active"
	}
}

func normalizeRecipientScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "campaign_only":
		return "campaign_only"
	default:
		return "workspace"
	}
}

func normalizeThreadBindingType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "reply_funnel":
		return "reply_funnel"
	case "manual":
		return "manual"
	default:
		return "campaign"
	}
}

func normalizeSuppressionScopeKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "domain":
		return "domain"
	default:
		return "recipient"
	}
}

func normalizeSuppressionSourceKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "unsubscribe":
		return "unsubscribe"
	case "bounce":
		return "bounce"
	case "reply_policy":
		return "reply_policy"
	case "compliance":
		return "compliance"
	default:
		return "manual"
	}
}

func normalizeOutboundRecipientEmail(raw string) (string, error) {
	value, err := mail.NormalizeMailboxAddress(raw)
	if err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(value)), nil
}

func outboundRecipientDomain(email string) string {
	value := strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(value, "@")
	if at < 0 || at >= len(value)-1 {
		return ""
	}
	return strings.TrimSpace(value[at+1:])
}

func outboundCampaignSelectSQL() string {
	return `SELECT c.id,c.user_id,c.name,c.status,c.goal_kind,c.playbook_key,c.campaign_mode,c.audience_source_kind,c.audience_source_ref,c.sender_policy_kind,c.sender_policy_ref,c.reply_policy_json,c.suppression_policy_json,c.schedule_policy_json,c.compliance_policy_json,c.governance_policy_json,c.created_at,c.updated_at,c.launched_at,c.completed_at,
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.last_sent_message_id<>''),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.reply_outcome<>''),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.reply_outcome IN ('positive_interest','meeting_intent')),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.reply_outcome IN ('not_interested','unsubscribe_request','hostile','wrong_person')),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.status='paused'),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.status='bounced'),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.status='unsubscribed'),0),
		COALESCE((SELECT COUNT(1) FROM outbound_enrollments e WHERE e.campaign_id=c.id AND e.status IN ('manual_only','paused') AND e.reply_outcome IN ('question','objection','referral','wrong_person','manual_review_required','out_of_office','auto_reply_other')),0)
		FROM outbound_campaigns c`
}

func scanOutboundCampaign(row outboundScanner) (models.OutboundCampaign, error) {
	var item models.OutboundCampaign
	var launchedAt sql.NullTime
	var completedAt sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.Status,
		&item.GoalKind,
		&item.PlaybookKey,
		&item.CampaignMode,
		&item.AudienceSourceKind,
		&item.AudienceSourceRef,
		&item.SenderPolicyKind,
		&item.SenderPolicyRef,
		&item.ReplyPolicyJSON,
		&item.SuppressionPolicyJSON,
		&item.SchedulePolicyJSON,
		&item.CompliancePolicyJSON,
		&item.GovernancePolicyJSON,
		&item.CreatedAt,
		&item.UpdatedAt,
		&launchedAt,
		&completedAt,
		&item.EnrollmentCount,
		&item.SentCount,
		&item.RepliedCount,
		&item.PositiveCount,
		&item.NegativeCount,
		&item.PausedCount,
		&item.BouncedCount,
		&item.UnsubscribedCount,
		&item.WaitingHumanCount,
	); err != nil {
		return models.OutboundCampaign{}, err
	}
	item.Status = normalizeOutboundCampaignStatus(item.Status)
	item.GoalKind = normalizeOutboundGoalKind(item.GoalKind)
	item.PlaybookKey = strings.TrimSpace(item.PlaybookKey)
	item.CampaignMode = normalizeOutboundCampaignMode(item.CampaignMode)
	item.AudienceSourceKind = normalizeOutboundAudienceSourceKind(item.AudienceSourceKind)
	item.SenderPolicyKind = normalizeOutboundSenderPolicyKind(item.SenderPolicyKind)
	item.Name = strings.TrimSpace(item.Name)
	item.AudienceSourceRef = strings.TrimSpace(item.AudienceSourceRef)
	item.SenderPolicyRef = strings.TrimSpace(item.SenderPolicyRef)
	item.ReplyPolicyJSON = strings.TrimSpace(item.ReplyPolicyJSON)
	item.SuppressionPolicyJSON = strings.TrimSpace(item.SuppressionPolicyJSON)
	item.SchedulePolicyJSON = strings.TrimSpace(item.SchedulePolicyJSON)
	item.CompliancePolicyJSON = strings.TrimSpace(item.CompliancePolicyJSON)
	item.GovernancePolicyJSON = strings.TrimSpace(item.GovernancePolicyJSON)
	if launchedAt.Valid {
		item.LaunchedAt = launchedAt.Time
	}
	if completedAt.Valid {
		item.CompletedAt = completedAt.Time
	}
	return item, nil
}

func (s *Store) ListOutboundCampaigns(ctx context.Context, userID string) ([]models.OutboundCampaign, error) {
	rows, err := s.db.QueryContext(ctx,
		outboundCampaignSelectSQL()+` WHERE c.user_id=? ORDER BY c.updated_at DESC, c.name COLLATE NOCASE ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundCampaign, 0, 16)
	for rows.Next() {
		item, err := scanOutboundCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetOutboundCampaignByID(ctx context.Context, userID, id string) (models.OutboundCampaign, error) {
	row := s.db.QueryRowContext(ctx,
		outboundCampaignSelectSQL()+` WHERE c.user_id=? AND c.id=?`,
		userID,
		strings.TrimSpace(id),
	)
	item, err := scanOutboundCampaign(row)
	if err == sql.ErrNoRows {
		return models.OutboundCampaign{}, ErrNotFound
	}
	if err != nil {
		return models.OutboundCampaign{}, err
	}
	return item, nil
}

func (s *Store) GetOutboundCampaignByIDAny(ctx context.Context, id string) (models.OutboundCampaign, error) {
	row := s.db.QueryRowContext(ctx,
		outboundCampaignSelectSQL()+` WHERE c.id=?`,
		strings.TrimSpace(id),
	)
	item, err := scanOutboundCampaign(row)
	if err == sql.ErrNoRows {
		return models.OutboundCampaign{}, ErrNotFound
	}
	if err != nil {
		return models.OutboundCampaign{}, err
	}
	return item, nil
}

func (s *Store) CreateOutboundCampaign(ctx context.Context, in models.OutboundCampaign) (models.OutboundCampaign, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Status = normalizeOutboundCampaignStatus(in.Status)
	in.GoalKind = normalizeOutboundGoalKind(in.GoalKind)
	in.PlaybookKey = strings.TrimSpace(in.PlaybookKey)
	in.CampaignMode = normalizeOutboundCampaignMode(in.CampaignMode)
	in.AudienceSourceKind = normalizeOutboundAudienceSourceKind(in.AudienceSourceKind)
	in.AudienceSourceRef = strings.TrimSpace(in.AudienceSourceRef)
	in.SenderPolicyKind = normalizeOutboundSenderPolicyKind(in.SenderPolicyKind)
	in.SenderPolicyRef = strings.TrimSpace(in.SenderPolicyRef)
	in.ReplyPolicyJSON = strings.TrimSpace(in.ReplyPolicyJSON)
	if in.ReplyPolicyJSON == "" {
		in.ReplyPolicyJSON = "{}"
	}
	in.SuppressionPolicyJSON = strings.TrimSpace(in.SuppressionPolicyJSON)
	if in.SuppressionPolicyJSON == "" {
		in.SuppressionPolicyJSON = "{}"
	}
	in.SchedulePolicyJSON = strings.TrimSpace(in.SchedulePolicyJSON)
	if in.SchedulePolicyJSON == "" {
		in.SchedulePolicyJSON = "{}"
	}
	in.CompliancePolicyJSON = strings.TrimSpace(in.CompliancePolicyJSON)
	if in.CompliancePolicyJSON == "" {
		in.CompliancePolicyJSON = "{}"
	}
	in.GovernancePolicyJSON = strings.TrimSpace(in.GovernancePolicyJSON)
	if in.GovernancePolicyJSON == "" {
		in.GovernancePolicyJSON = "{}"
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO outbound_campaigns(id,user_id,name,status,goal_kind,playbook_key,campaign_mode,audience_source_kind,audience_source_ref,sender_policy_kind,sender_policy_ref,reply_policy_json,suppression_policy_json,schedule_policy_json,compliance_policy_json,governance_policy_json,created_at,updated_at,launched_at,completed_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		in.Name,
		in.Status,
		in.GoalKind,
		in.PlaybookKey,
		in.CampaignMode,
		in.AudienceSourceKind,
		in.AudienceSourceRef,
		in.SenderPolicyKind,
		in.SenderPolicyRef,
		in.ReplyPolicyJSON,
		in.SuppressionPolicyJSON,
		in.SchedulePolicyJSON,
		in.CompliancePolicyJSON,
		in.GovernancePolicyJSON,
		in.CreatedAt,
		in.UpdatedAt,
		nullTimeValue(in.LaunchedAt),
		nullTimeValue(in.CompletedAt),
	)
	if err != nil {
		return models.OutboundCampaign{}, err
	}
	return s.GetOutboundCampaignByID(ctx, in.UserID, in.ID)
}

func (s *Store) UpdateOutboundCampaign(ctx context.Context, in models.OutboundCampaign) (models.OutboundCampaign, error) {
	current, err := s.GetOutboundCampaignByID(ctx, in.UserID, in.ID)
	if err != nil {
		return models.OutboundCampaign{}, err
	}
	now := time.Now().UTC()
	current.Name = strings.TrimSpace(in.Name)
	current.Status = normalizeOutboundCampaignStatus(in.Status)
	current.GoalKind = normalizeOutboundGoalKind(in.GoalKind)
	current.PlaybookKey = strings.TrimSpace(in.PlaybookKey)
	current.CampaignMode = normalizeOutboundCampaignMode(in.CampaignMode)
	current.AudienceSourceKind = normalizeOutboundAudienceSourceKind(in.AudienceSourceKind)
	current.AudienceSourceRef = strings.TrimSpace(in.AudienceSourceRef)
	current.SenderPolicyKind = normalizeOutboundSenderPolicyKind(in.SenderPolicyKind)
	current.SenderPolicyRef = strings.TrimSpace(in.SenderPolicyRef)
	current.ReplyPolicyJSON = strings.TrimSpace(in.ReplyPolicyJSON)
	if current.ReplyPolicyJSON == "" {
		current.ReplyPolicyJSON = "{}"
	}
	current.SuppressionPolicyJSON = strings.TrimSpace(in.SuppressionPolicyJSON)
	if current.SuppressionPolicyJSON == "" {
		current.SuppressionPolicyJSON = "{}"
	}
	current.SchedulePolicyJSON = strings.TrimSpace(in.SchedulePolicyJSON)
	if current.SchedulePolicyJSON == "" {
		current.SchedulePolicyJSON = "{}"
	}
	current.CompliancePolicyJSON = strings.TrimSpace(in.CompliancePolicyJSON)
	if current.CompliancePolicyJSON == "" {
		current.CompliancePolicyJSON = "{}"
	}
	current.GovernancePolicyJSON = strings.TrimSpace(in.GovernancePolicyJSON)
	if current.GovernancePolicyJSON == "" {
		current.GovernancePolicyJSON = "{}"
	}
	current.UpdatedAt = now
	res, err := s.db.ExecContext(ctx,
		`UPDATE outbound_campaigns
		 SET name=?, status=?, goal_kind=?, playbook_key=?, campaign_mode=?, audience_source_kind=?, audience_source_ref=?, sender_policy_kind=?, sender_policy_ref=?, reply_policy_json=?, suppression_policy_json=?, schedule_policy_json=?, compliance_policy_json=?, governance_policy_json=?, updated_at=?, launched_at=?, completed_at=?
		 WHERE user_id=? AND id=?`,
		current.Name,
		current.Status,
		current.GoalKind,
		current.PlaybookKey,
		current.CampaignMode,
		current.AudienceSourceKind,
		current.AudienceSourceRef,
		current.SenderPolicyKind,
		current.SenderPolicyRef,
		current.ReplyPolicyJSON,
		current.SuppressionPolicyJSON,
		current.SchedulePolicyJSON,
		current.CompliancePolicyJSON,
		current.GovernancePolicyJSON,
		current.UpdatedAt,
		nullTimeValue(in.LaunchedAt),
		nullTimeValue(in.CompletedAt),
		current.UserID,
		current.ID,
	)
	if err != nil {
		return models.OutboundCampaign{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.OutboundCampaign{}, ErrNotFound
	}
	return s.GetOutboundCampaignByID(ctx, current.UserID, current.ID)
}

func (s *Store) SetOutboundCampaignStatus(ctx context.Context, userID, id, status string, launchedAt, completedAt time.Time) error {
	status = normalizeOutboundCampaignStatus(status)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE outbound_campaigns
		 SET status=?, updated_at=?, launched_at=COALESCE(?, launched_at), completed_at=CASE WHEN ? IS NULL THEN completed_at ELSE ? END
		 WHERE user_id=? AND id=?`,
		status,
		now,
		nullTimeValue(launchedAt),
		nullTimeValue(completedAt),
		nullTimeValue(completedAt),
		userID,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanOutboundCampaignStep(row outboundScanner) (models.OutboundCampaignStep, error) {
	var item models.OutboundCampaignStep
	var stopIfReplied int
	var stopIfClicked int
	var stopIfBooked int
	var stopIfUnsubscribed int
	if err := row.Scan(
		&item.ID,
		&item.CampaignID,
		&item.Position,
		&item.Kind,
		&item.ThreadMode,
		&item.SubjectTemplate,
		&item.BodyTemplate,
		&item.WaitIntervalMinutes,
		&item.SendWindowJSON,
		&item.TaskPolicyJSON,
		&item.BranchPolicyJSON,
		&stopIfReplied,
		&stopIfClicked,
		&stopIfBooked,
		&stopIfUnsubscribed,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.OutboundCampaignStep{}, err
	}
	item.Kind = normalizeOutboundStepKind(item.Kind)
	item.ThreadMode = normalizeOutboundThreadMode(item.ThreadMode)
	item.SubjectTemplate = strings.TrimSpace(item.SubjectTemplate)
	item.BodyTemplate = strings.TrimSpace(item.BodyTemplate)
	item.SendWindowJSON = strings.TrimSpace(item.SendWindowJSON)
	item.TaskPolicyJSON = strings.TrimSpace(item.TaskPolicyJSON)
	item.BranchPolicyJSON = strings.TrimSpace(item.BranchPolicyJSON)
	item.StopIfReplied = stopIfReplied == 1
	item.StopIfClicked = stopIfClicked == 1
	item.StopIfBooked = stopIfBooked == 1
	item.StopIfUnsubscribed = stopIfUnsubscribed == 1
	return item, nil
}

func (s *Store) ListOutboundCampaignSteps(ctx context.Context, campaignID string) ([]models.OutboundCampaignStep, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,campaign_id,position,kind,thread_mode,subject_template,body_template,wait_interval_minutes,send_window_json,task_policy_json,branch_policy_json,stop_if_replied,stop_if_clicked,stop_if_booked,stop_if_unsubscribed,created_at,updated_at
		 FROM outbound_campaign_steps
		 WHERE campaign_id=?
		 ORDER BY position ASC, created_at ASC`,
		strings.TrimSpace(campaignID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundCampaignStep, 0, 8)
	for rows.Next() {
		item, err := scanOutboundCampaignStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetOutboundCampaignStepByID(ctx context.Context, stepID string) (models.OutboundCampaignStep, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,campaign_id,position,kind,thread_mode,subject_template,body_template,wait_interval_minutes,send_window_json,task_policy_json,branch_policy_json,stop_if_replied,stop_if_clicked,stop_if_booked,stop_if_unsubscribed,created_at,updated_at
		 FROM outbound_campaign_steps
		 WHERE id=?`,
		strings.TrimSpace(stepID),
	)
	item, err := scanOutboundCampaignStep(row)
	if err == sql.ErrNoRows {
		return models.OutboundCampaignStep{}, ErrNotFound
	}
	if err != nil {
		return models.OutboundCampaignStep{}, err
	}
	return item, nil
}

func (s *Store) CreateOutboundCampaignStep(ctx context.Context, in models.OutboundCampaignStep) (models.OutboundCampaignStep, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.CampaignID = strings.TrimSpace(in.CampaignID)
	in.Kind = normalizeOutboundStepKind(in.Kind)
	in.ThreadMode = normalizeOutboundThreadMode(in.ThreadMode)
	in.SubjectTemplate = strings.TrimSpace(in.SubjectTemplate)
	in.BodyTemplate = strings.TrimSpace(in.BodyTemplate)
	in.SendWindowJSON = strings.TrimSpace(in.SendWindowJSON)
	if in.SendWindowJSON == "" {
		in.SendWindowJSON = "{}"
	}
	in.TaskPolicyJSON = strings.TrimSpace(in.TaskPolicyJSON)
	if in.TaskPolicyJSON == "" {
		in.TaskPolicyJSON = "{}"
	}
	in.BranchPolicyJSON = strings.TrimSpace(in.BranchPolicyJSON)
	if in.BranchPolicyJSON == "" {
		in.BranchPolicyJSON = "{}"
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO outbound_campaign_steps(id,campaign_id,position,kind,thread_mode,subject_template,body_template,wait_interval_minutes,send_window_json,task_policy_json,branch_policy_json,stop_if_replied,stop_if_clicked,stop_if_booked,stop_if_unsubscribed,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.CampaignID,
		in.Position,
		in.Kind,
		in.ThreadMode,
		in.SubjectTemplate,
		in.BodyTemplate,
		in.WaitIntervalMinutes,
		in.SendWindowJSON,
		in.TaskPolicyJSON,
		in.BranchPolicyJSON,
		boolToInt(in.StopIfReplied),
		boolToInt(in.StopIfClicked),
		boolToInt(in.StopIfBooked),
		boolToInt(in.StopIfUnsubscribed),
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.OutboundCampaignStep{}, err
	}
	return s.GetOutboundCampaignStepByID(ctx, in.ID)
}

func (s *Store) UpdateOutboundCampaignStep(ctx context.Context, in models.OutboundCampaignStep) (models.OutboundCampaignStep, error) {
	current, err := s.GetOutboundCampaignStepByID(ctx, in.ID)
	if err != nil {
		return models.OutboundCampaignStep{}, err
	}
	current.Position = in.Position
	current.Kind = normalizeOutboundStepKind(in.Kind)
	current.ThreadMode = normalizeOutboundThreadMode(in.ThreadMode)
	current.SubjectTemplate = strings.TrimSpace(in.SubjectTemplate)
	current.BodyTemplate = strings.TrimSpace(in.BodyTemplate)
	current.WaitIntervalMinutes = in.WaitIntervalMinutes
	current.SendWindowJSON = strings.TrimSpace(in.SendWindowJSON)
	if current.SendWindowJSON == "" {
		current.SendWindowJSON = "{}"
	}
	current.TaskPolicyJSON = strings.TrimSpace(in.TaskPolicyJSON)
	if current.TaskPolicyJSON == "" {
		current.TaskPolicyJSON = "{}"
	}
	current.BranchPolicyJSON = strings.TrimSpace(in.BranchPolicyJSON)
	if current.BranchPolicyJSON == "" {
		current.BranchPolicyJSON = "{}"
	}
	current.StopIfReplied = in.StopIfReplied
	current.StopIfClicked = in.StopIfClicked
	current.StopIfBooked = in.StopIfBooked
	current.StopIfUnsubscribed = in.StopIfUnsubscribed
	current.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE outbound_campaign_steps
		 SET position=?, kind=?, thread_mode=?, subject_template=?, body_template=?, wait_interval_minutes=?, send_window_json=?, task_policy_json=?, branch_policy_json=?, stop_if_replied=?, stop_if_clicked=?, stop_if_booked=?, stop_if_unsubscribed=?, updated_at=?
		 WHERE id=?`,
		current.Position,
		current.Kind,
		current.ThreadMode,
		current.SubjectTemplate,
		current.BodyTemplate,
		current.WaitIntervalMinutes,
		current.SendWindowJSON,
		current.TaskPolicyJSON,
		current.BranchPolicyJSON,
		boolToInt(current.StopIfReplied),
		boolToInt(current.StopIfClicked),
		boolToInt(current.StopIfBooked),
		boolToInt(current.StopIfUnsubscribed),
		current.UpdatedAt,
		current.ID,
	)
	if err != nil {
		return models.OutboundCampaignStep{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.OutboundCampaignStep{}, ErrNotFound
	}
	return s.GetOutboundCampaignStepByID(ctx, current.ID)
}

func (s *Store) DeleteOutboundCampaignStep(ctx context.Context, stepID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM outbound_campaign_steps WHERE id=?`, strings.TrimSpace(stepID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteOutboundCampaignSteps(ctx context.Context, campaignID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM outbound_campaign_steps WHERE campaign_id=?`, strings.TrimSpace(campaignID))
	return err
}

func (s *Store) ReorderOutboundCampaignSteps(ctx context.Context, campaignID string, stepIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for idx, stepID := range stepIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE outbound_campaign_steps SET position=?, updated_at=? WHERE campaign_id=? AND id=?`,
			idx+1,
			time.Now().UTC(),
			strings.TrimSpace(campaignID),
			strings.TrimSpace(stepID),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func outboundEnrollmentSelectSQL() string {
	return `SELECT e.id,e.campaign_id,e.contact_id,e.recipient_email,e.recipient_domain,e.sender_account_id,e.sender_profile_id,e.reply_funnel_id,e.thread_binding_id,e.status,e.current_step_position,e.last_sent_message_id,e.last_sent_at,e.next_action_at,e.pause_reason,e.stop_reason,e.reply_outcome,e.reply_confidence,e.manual_owner_user_id,e.seed_context_json,e.created_at,e.updated_at,
		COALESCE(c.name,''),COALESCE(ct.name,''),COALESCE(a.display_name,''),COALESCE(a.login,''),COALESCE(b.thread_subject,''),COALESCE(b.last_reply_message_id,''),b.last_reply_at
		FROM outbound_enrollments e
		JOIN outbound_campaigns c ON c.id=e.campaign_id
		LEFT JOIN contacts ct ON ct.id=e.contact_id
		LEFT JOIN mail_accounts a ON a.id=e.sender_account_id
		LEFT JOIN mail_thread_bindings b ON b.id=e.thread_binding_id`
}

func scanOutboundEnrollment(row outboundScanner) (models.OutboundEnrollment, error) {
	var item models.OutboundEnrollment
	var contactID sql.NullString
	var senderAccountID sql.NullString
	var senderProfileID sql.NullString
	var replyFunnelID sql.NullString
	var manualOwner sql.NullString
	var lastSentAt sql.NullTime
	var nextActionAt sql.NullTime
	var threadBindingID string
	var campaignName string
	var contactName string
	var senderAccountLabel string
	var senderAccountLogin string
	var threadSubject string
	var lastReplyMessageID string
	var lastReplyAt sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.CampaignID,
		&contactID,
		&item.RecipientEmail,
		&item.RecipientDomain,
		&senderAccountID,
		&senderProfileID,
		&replyFunnelID,
		&threadBindingID,
		&item.Status,
		&item.CurrentStepPosition,
		&item.LastSentMessageID,
		&lastSentAt,
		&nextActionAt,
		&item.PauseReason,
		&item.StopReason,
		&item.ReplyOutcome,
		&item.ReplyConfidence,
		&manualOwner,
		&item.SeedContextJSON,
		&item.CreatedAt,
		&item.UpdatedAt,
		&campaignName,
		&contactName,
		&senderAccountLabel,
		&senderAccountLogin,
		&threadSubject,
		&lastReplyMessageID,
		&lastReplyAt,
	); err != nil {
		return models.OutboundEnrollment{}, err
	}
	item.ContactID = strings.TrimSpace(contactID.String)
	item.SenderAccountID = strings.TrimSpace(senderAccountID.String)
	item.SenderProfileID = strings.TrimSpace(senderProfileID.String)
	item.ReplyFunnelID = strings.TrimSpace(replyFunnelID.String)
	item.ThreadBindingID = strings.TrimSpace(threadBindingID)
	item.Status = normalizeOutboundEnrollmentStatus(item.Status)
	item.ReplyOutcome = normalizeReplyOutcome(item.ReplyOutcome)
	item.ManualOwnerUserID = strings.TrimSpace(manualOwner.String)
	item.SeedContextJSON = strings.TrimSpace(item.SeedContextJSON)
	item.PauseReason = strings.TrimSpace(item.PauseReason)
	item.StopReason = strings.TrimSpace(item.StopReason)
	item.LastSentMessageID = strings.TrimSpace(item.LastSentMessageID)
	item.CampaignName = strings.TrimSpace(campaignName)
	item.ContactName = strings.TrimSpace(contactName)
	item.SenderAccountLabel = strings.TrimSpace(senderAccountLabel)
	item.SenderAccountLogin = strings.TrimSpace(senderAccountLogin)
	item.ThreadSubject = strings.TrimSpace(threadSubject)
	item.LastReplyMessageID = strings.TrimSpace(lastReplyMessageID)
	item.LastReplyBucket = replyOutcomeBucket(item.ReplyOutcome, item.Status)
	if item.SenderAccountLabel == "" {
		item.SenderAccountLabel = item.SenderAccountLogin
	}
	if lastSentAt.Valid {
		item.LastSentAt = lastSentAt.Time
	}
	if nextActionAt.Valid {
		item.NextActionAt = nextActionAt.Time
	}
	if lastReplyAt.Valid {
		item.LastReplyAt = lastReplyAt.Time
	}
	return item, nil
}

func (s *Store) ListOutboundEnrollmentsByCampaign(ctx context.Context, userID, campaignID string) ([]models.OutboundEnrollment, error) {
	rows, err := s.db.QueryContext(ctx,
		outboundEnrollmentSelectSQL()+` WHERE c.user_id=? AND e.campaign_id=? ORDER BY e.updated_at DESC, e.recipient_email COLLATE NOCASE ASC`,
		userID,
		strings.TrimSpace(campaignID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundEnrollment, 0, 32)
	for rows.Next() {
		item, err := scanOutboundEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetOutboundEnrollmentByID(ctx context.Context, userID, id string) (models.OutboundEnrollment, error) {
	row := s.db.QueryRowContext(ctx,
		outboundEnrollmentSelectSQL()+` WHERE c.user_id=? AND e.id=?`,
		userID,
		strings.TrimSpace(id),
	)
	item, err := scanOutboundEnrollment(row)
	if err == sql.ErrNoRows {
		return models.OutboundEnrollment{}, ErrNotFound
	}
	if err != nil {
		return models.OutboundEnrollment{}, err
	}
	return item, nil
}

func (s *Store) GetOutboundEnrollmentByCampaignEmail(ctx context.Context, campaignID, recipientEmail string) (models.OutboundEnrollment, error) {
	row := s.db.QueryRowContext(ctx,
		outboundEnrollmentSelectSQL()+` WHERE e.campaign_id=? AND e.recipient_email=?`,
		strings.TrimSpace(campaignID),
		strings.ToLower(strings.TrimSpace(recipientEmail)),
	)
	item, err := scanOutboundEnrollment(row)
	if err == sql.ErrNoRows {
		return models.OutboundEnrollment{}, ErrNotFound
	}
	if err != nil {
		return models.OutboundEnrollment{}, err
	}
	return item, nil
}

func (s *Store) UpsertOutboundEnrollment(ctx context.Context, in models.OutboundEnrollment) (models.OutboundEnrollment, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	recipientEmail, err := normalizeOutboundRecipientEmail(in.RecipientEmail)
	if err != nil {
		return models.OutboundEnrollment{}, err
	}
	in.RecipientEmail = recipientEmail
	in.RecipientDomain = outboundRecipientDomain(recipientEmail)
	in.ContactID = strings.TrimSpace(in.ContactID)
	in.SenderAccountID = strings.TrimSpace(in.SenderAccountID)
	in.SenderProfileID = strings.TrimSpace(in.SenderProfileID)
	in.ReplyFunnelID = strings.TrimSpace(in.ReplyFunnelID)
	in.ThreadBindingID = strings.TrimSpace(in.ThreadBindingID)
	in.Status = normalizeOutboundEnrollmentStatus(in.Status)
	in.PauseReason = strings.TrimSpace(in.PauseReason)
	in.StopReason = strings.TrimSpace(in.StopReason)
	in.ReplyOutcome = normalizeReplyOutcome(in.ReplyOutcome)
	in.ManualOwnerUserID = strings.TrimSpace(in.ManualOwnerUserID)
	in.SeedContextJSON = strings.TrimSpace(in.SeedContextJSON)
	if in.SeedContextJSON == "" {
		in.SeedContextJSON = "{}"
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO outbound_enrollments(id,campaign_id,contact_id,recipient_email,recipient_domain,sender_account_id,sender_profile_id,reply_funnel_id,thread_binding_id,status,current_step_position,last_sent_message_id,last_sent_at,next_action_at,pause_reason,stop_reason,reply_outcome,reply_confidence,manual_owner_user_id,seed_context_json,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(campaign_id,recipient_email) DO UPDATE SET
		   contact_id=excluded.contact_id,
		   recipient_domain=excluded.recipient_domain,
		   sender_account_id=excluded.sender_account_id,
		   sender_profile_id=excluded.sender_profile_id,
		   reply_funnel_id=excluded.reply_funnel_id,
		   thread_binding_id=excluded.thread_binding_id,
		   status=excluded.status,
		   current_step_position=excluded.current_step_position,
		   last_sent_message_id=excluded.last_sent_message_id,
		   last_sent_at=excluded.last_sent_at,
		   next_action_at=excluded.next_action_at,
		   pause_reason=excluded.pause_reason,
		   stop_reason=excluded.stop_reason,
		   reply_outcome=excluded.reply_outcome,
		   reply_confidence=excluded.reply_confidence,
		   manual_owner_user_id=excluded.manual_owner_user_id,
		   seed_context_json=excluded.seed_context_json,
		   updated_at=excluded.updated_at`,
		in.ID,
		strings.TrimSpace(in.CampaignID),
		nullableTrimmedString(in.ContactID),
		in.RecipientEmail,
		in.RecipientDomain,
		nullableTrimmedString(in.SenderAccountID),
		nullableTrimmedString(in.SenderProfileID),
		nullableTrimmedString(in.ReplyFunnelID),
		in.ThreadBindingID,
		in.Status,
		in.CurrentStepPosition,
		strings.TrimSpace(in.LastSentMessageID),
		nullTimeValue(in.LastSentAt),
		nullTimeValue(in.NextActionAt),
		in.PauseReason,
		in.StopReason,
		in.ReplyOutcome,
		in.ReplyConfidence,
		nullableTrimmedString(in.ManualOwnerUserID),
		in.SeedContextJSON,
		coalesceTime(in.CreatedAt, now),
		in.UpdatedAt,
	)
	if err != nil {
		return models.OutboundEnrollment{}, err
	}
	return s.GetOutboundEnrollmentByCampaignEmail(ctx, in.CampaignID, in.RecipientEmail)
}

func (s *Store) ListDueOutboundEnrollments(ctx context.Context, now time.Time, limit int) ([]models.OutboundEnrollment, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		outboundEnrollmentSelectSQL()+` WHERE c.status='running'
		   AND e.status IN ('pending','scheduled')
		   AND (e.next_action_at IS NULL OR e.next_action_at<=?)
		 ORDER BY COALESCE(e.next_action_at, e.created_at) ASC
		 LIMIT ?`,
		now.UTC(),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundEnrollment, 0, limit)
	for rows.Next() {
		item, err := scanOutboundEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListOutboundOOOResumeDue(ctx context.Context, now time.Time, limit int) ([]models.OutboundEnrollment, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		outboundEnrollmentSelectSQL()+` WHERE c.status='running'
		   AND e.status='paused'
		   AND e.pause_reason='out_of_office'
		   AND e.next_action_at IS NOT NULL
		   AND e.next_action_at<=?
		 ORDER BY e.next_action_at ASC
		 LIMIT ?`,
		now.UTC(),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundEnrollment, 0, limit)
	for rows.Next() {
		item, err := scanOutboundEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListActiveOutboundEnrollmentsByRecipient(ctx context.Context, userID, recipientEmail, excludeCampaignID string) ([]models.OutboundEnrollment, error) {
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	args := []any{userID, recipientEmail}
	query := outboundEnrollmentSelectSQL() + ` WHERE c.user_id=? AND e.recipient_email=? AND e.status IN ('pending','scheduled','sending','waiting_reply','paused','manual_only')`
	if strings.TrimSpace(excludeCampaignID) != "" {
		query += ` AND e.campaign_id<>?`
		args = append(args, strings.TrimSpace(excludeCampaignID))
	}
	query += ` ORDER BY e.updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundEnrollment, 0, 8)
	for rows.Next() {
		item, err := scanOutboundEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListActiveOutboundEnrollmentsByDomain(ctx context.Context, userID, domain, excludeCampaignID string) ([]models.OutboundEnrollment, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	args := []any{userID, domain}
	query := outboundEnrollmentSelectSQL() + ` WHERE c.user_id=? AND e.recipient_domain=? AND e.status IN ('pending','scheduled','sending','waiting_reply','paused','manual_only')`
	if strings.TrimSpace(excludeCampaignID) != "" {
		query += ` AND e.campaign_id<>?`
		args = append(args, strings.TrimSpace(excludeCampaignID))
	}
	query += ` ORDER BY e.updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundEnrollment, 0, 16)
	for rows.Next() {
		item, err := scanOutboundEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanOutboundEvent(row outboundScanner) (models.OutboundEvent, error) {
	var item models.OutboundEvent
	if err := row.Scan(
		&item.ID,
		&item.CampaignID,
		&item.EnrollmentID,
		&item.EventKind,
		&item.EventPayloadJSON,
		&item.ActorKind,
		&item.ActorRef,
		&item.CreatedAt,
	); err != nil {
		return models.OutboundEvent{}, err
	}
	item.EventKind = strings.TrimSpace(item.EventKind)
	item.EventPayloadJSON = strings.TrimSpace(item.EventPayloadJSON)
	item.ActorKind = strings.TrimSpace(item.ActorKind)
	item.ActorRef = strings.TrimSpace(item.ActorRef)
	return item, nil
}

func (s *Store) AppendOutboundEvent(ctx context.Context, in models.OutboundEvent) (models.OutboundEvent, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if in.EventPayloadJSON == "" {
		in.EventPayloadJSON = "{}"
	}
	in.ActorKind = strings.TrimSpace(in.ActorKind)
	if in.ActorKind == "" {
		in.ActorKind = "system"
	}
	in.ActorRef = strings.TrimSpace(in.ActorRef)
	in.CreatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO outbound_events(id,campaign_id,enrollment_id,event_kind,event_payload_json,actor_kind,actor_ref,created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		in.ID,
		strings.TrimSpace(in.CampaignID),
		strings.TrimSpace(in.EnrollmentID),
		strings.TrimSpace(in.EventKind),
		strings.TrimSpace(in.EventPayloadJSON),
		in.ActorKind,
		in.ActorRef,
		in.CreatedAt,
	)
	if err != nil {
		return models.OutboundEvent{}, err
	}
	return in, nil
}

func (s *Store) ListOutboundCampaignEvents(ctx context.Context, campaignID string, limit int) ([]models.OutboundEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,campaign_id,enrollment_id,event_kind,event_payload_json,actor_kind,actor_ref,created_at
		 FROM outbound_events
		 WHERE campaign_id=?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		strings.TrimSpace(campaignID),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundEvent, 0, limit)
	for rows.Next() {
		item, err := scanOutboundEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanRecipientState(row outboundScanner) (models.RecipientState, error) {
	var item models.RecipientState
	var primaryContactID sql.NullString
	var lastReplyAt sql.NullTime
	var suppressedUntil sql.NullTime
	if err := row.Scan(
		&item.UserID,
		&item.RecipientEmail,
		&primaryContactID,
		&item.RecipientDomain,
		&item.Status,
		&item.Scope,
		&lastReplyAt,
		&item.LastReplyOutcome,
		&suppressedUntil,
		&item.SuppressionReason,
		&item.Notes,
		&item.UpdatedAt,
	); err != nil {
		return models.RecipientState{}, err
	}
	item.PrimaryContactID = strings.TrimSpace(primaryContactID.String)
	item.RecipientEmail = strings.ToLower(strings.TrimSpace(item.RecipientEmail))
	item.RecipientDomain = strings.ToLower(strings.TrimSpace(item.RecipientDomain))
	item.Status = normalizeRecipientStateStatus(item.Status)
	item.Scope = normalizeRecipientScope(item.Scope)
	item.LastReplyOutcome = normalizeReplyOutcome(item.LastReplyOutcome)
	item.SuppressionReason = strings.TrimSpace(item.SuppressionReason)
	item.Notes = strings.TrimSpace(item.Notes)
	if lastReplyAt.Valid {
		item.LastReplyAt = lastReplyAt.Time
	}
	if suppressedUntil.Valid {
		item.SuppressedUntil = suppressedUntil.Time
	}
	return item, nil
}

func (s *Store) GetRecipientState(ctx context.Context, userID, recipientEmail string) (models.RecipientState, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id,recipient_email,primary_contact_id,recipient_domain,status,scope,last_reply_at,last_reply_outcome,suppressed_until,suppression_reason,notes,updated_at
		 FROM recipient_state
		 WHERE user_id=? AND recipient_email=?`,
		userID,
		strings.ToLower(strings.TrimSpace(recipientEmail)),
	)
	item, err := scanRecipientState(row)
	if err == sql.ErrNoRows {
		return models.RecipientState{}, ErrNotFound
	}
	if err != nil {
		return models.RecipientState{}, err
	}
	return item, nil
}

func (s *Store) UpsertRecipientState(ctx context.Context, in models.RecipientState) (models.RecipientState, error) {
	now := time.Now().UTC()
	recipientEmail, err := normalizeOutboundRecipientEmail(in.RecipientEmail)
	if err != nil {
		return models.RecipientState{}, err
	}
	in.RecipientEmail = recipientEmail
	in.RecipientDomain = outboundRecipientDomain(recipientEmail)
	in.PrimaryContactID = strings.TrimSpace(in.PrimaryContactID)
	in.Status = normalizeRecipientStateStatus(in.Status)
	in.Scope = normalizeRecipientScope(in.Scope)
	in.LastReplyOutcome = normalizeReplyOutcome(in.LastReplyOutcome)
	in.SuppressionReason = strings.TrimSpace(in.SuppressionReason)
	in.Notes = strings.TrimSpace(in.Notes)
	in.UpdatedAt = now
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO recipient_state(user_id,recipient_email,primary_contact_id,recipient_domain,status,scope,last_reply_at,last_reply_outcome,suppressed_until,suppression_reason,notes,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(user_id,recipient_email) DO UPDATE SET
		   primary_contact_id=excluded.primary_contact_id,
		   recipient_domain=excluded.recipient_domain,
		   status=excluded.status,
		   scope=excluded.scope,
		   last_reply_at=excluded.last_reply_at,
		   last_reply_outcome=excluded.last_reply_outcome,
		   suppressed_until=excluded.suppressed_until,
		   suppression_reason=excluded.suppression_reason,
		   notes=excluded.notes,
		   updated_at=excluded.updated_at`,
		in.UserID,
		in.RecipientEmail,
		nullableTrimmedString(in.PrimaryContactID),
		in.RecipientDomain,
		in.Status,
		in.Scope,
		nullTimeValue(in.LastReplyAt),
		in.LastReplyOutcome,
		nullTimeValue(in.SuppressedUntil),
		in.SuppressionReason,
		in.Notes,
		in.UpdatedAt,
	)
	if err != nil {
		return models.RecipientState{}, err
	}
	return s.GetRecipientState(ctx, in.UserID, in.RecipientEmail)
}

func (s *Store) ListRecipientStates(ctx context.Context, userID, query string) ([]models.RecipientState, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	args := []any{userID}
	sqlQuery := `SELECT user_id,recipient_email,primary_contact_id,recipient_domain,status,scope,last_reply_at,last_reply_outcome,suppressed_until,suppression_reason,notes,updated_at
		 FROM recipient_state
		 WHERE user_id=?`
	if query != "" {
		sqlQuery += ` AND (recipient_email LIKE ? OR recipient_domain LIKE ? OR notes LIKE ?)`
		like := "%" + query + "%"
		args = append(args, like, like, like)
	}
	sqlQuery += ` ORDER BY updated_at DESC, recipient_email COLLATE NOCASE ASC`
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.RecipientState, 0, 32)
	for rows.Next() {
		item, err := scanRecipientState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanMailThreadBinding(row outboundScanner) (models.MailThreadBinding, error) {
	var item models.MailThreadBinding
	var campaignID sql.NullString
	var enrollmentID sql.NullString
	var funnelID sql.NullString
	var replyAccountID sql.NullString
	var replySenderProfileID sql.NullString
	var collectorAccountID sql.NullString
	var ownerUserID sql.NullString
	var lastReplyAt sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.AccountID,
		&item.ThreadID,
		&item.BindingType,
		&campaignID,
		&enrollmentID,
		&funnelID,
		&replyAccountID,
		&replySenderProfileID,
		&collectorAccountID,
		&ownerUserID,
		&item.RecipientEmail,
		&item.RecipientDomain,
		&item.RootOutboundMessageID,
		&item.LastOutboundMessageID,
		&item.LastReplyMessageID,
		&item.ThreadSubject,
		&lastReplyAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.MailThreadBinding{}, err
	}
	item.BindingType = normalizeThreadBindingType(item.BindingType)
	item.CampaignID = strings.TrimSpace(campaignID.String)
	item.EnrollmentID = strings.TrimSpace(enrollmentID.String)
	item.FunnelID = strings.TrimSpace(funnelID.String)
	item.ReplyAccountID = strings.TrimSpace(replyAccountID.String)
	item.ReplySenderProfileID = strings.TrimSpace(replySenderProfileID.String)
	item.CollectorAccountID = strings.TrimSpace(collectorAccountID.String)
	item.OwnerUserID = strings.TrimSpace(ownerUserID.String)
	item.ThreadID = strings.TrimSpace(item.ThreadID)
	item.RecipientEmail = strings.ToLower(strings.TrimSpace(item.RecipientEmail))
	item.RecipientDomain = strings.ToLower(strings.TrimSpace(item.RecipientDomain))
	item.RootOutboundMessageID = mail.NormalizeMessageIDHeader(item.RootOutboundMessageID)
	item.LastOutboundMessageID = mail.NormalizeMessageIDHeader(item.LastOutboundMessageID)
	item.LastReplyMessageID = strings.TrimSpace(item.LastReplyMessageID)
	item.ThreadSubject = strings.TrimSpace(item.ThreadSubject)
	if lastReplyAt.Valid {
		item.LastReplyAt = lastReplyAt.Time
	}
	return item, nil
}

func (s *Store) GetMailThreadBindingByID(ctx context.Context, id string) (models.MailThreadBinding, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,thread_id,binding_type,campaign_id,enrollment_id,funnel_id,reply_account_id,reply_sender_profile_id,collector_account_id,owner_user_id,recipient_email,recipient_domain,root_outbound_message_id,last_outbound_message_id,last_reply_message_id,thread_subject,last_reply_at,created_at,updated_at
		 FROM mail_thread_bindings
		 WHERE id=?`,
		strings.TrimSpace(id),
	)
	item, err := scanMailThreadBinding(row)
	if err == sql.ErrNoRows {
		return models.MailThreadBinding{}, ErrNotFound
	}
	if err != nil {
		return models.MailThreadBinding{}, err
	}
	return item, nil
}

func (s *Store) GetMailThreadBindingByEnrollment(ctx context.Context, enrollmentID string) (models.MailThreadBinding, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,thread_id,binding_type,campaign_id,enrollment_id,funnel_id,reply_account_id,reply_sender_profile_id,collector_account_id,owner_user_id,recipient_email,recipient_domain,root_outbound_message_id,last_outbound_message_id,last_reply_message_id,thread_subject,last_reply_at,created_at,updated_at
		 FROM mail_thread_bindings
		 WHERE enrollment_id=?`,
		strings.TrimSpace(enrollmentID),
	)
	item, err := scanMailThreadBinding(row)
	if err == sql.ErrNoRows {
		return models.MailThreadBinding{}, ErrNotFound
	}
	if err != nil {
		return models.MailThreadBinding{}, err
	}
	return item, nil
}

func (s *Store) UpsertMailThreadBinding(ctx context.Context, in models.MailThreadBinding) (models.MailThreadBinding, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if normalized, err := normalizeOutboundRecipientEmail(in.RecipientEmail); err == nil {
		in.RecipientEmail = normalized
		in.RecipientDomain = outboundRecipientDomain(normalized)
	}
	if strings.TrimSpace(in.RecipientDomain) == "" && strings.TrimSpace(in.RecipientEmail) != "" {
		in.RecipientDomain = outboundRecipientDomain(in.RecipientEmail)
	}
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.ThreadID = strings.TrimSpace(in.ThreadID)
	in.BindingType = normalizeThreadBindingType(in.BindingType)
	in.CampaignID = strings.TrimSpace(in.CampaignID)
	in.EnrollmentID = strings.TrimSpace(in.EnrollmentID)
	in.FunnelID = strings.TrimSpace(in.FunnelID)
	in.ReplyAccountID = strings.TrimSpace(in.ReplyAccountID)
	in.ReplySenderProfileID = strings.TrimSpace(in.ReplySenderProfileID)
	in.CollectorAccountID = strings.TrimSpace(in.CollectorAccountID)
	in.OwnerUserID = strings.TrimSpace(in.OwnerUserID)
	in.RootOutboundMessageID = mail.NormalizeMessageIDHeader(in.RootOutboundMessageID)
	in.LastOutboundMessageID = mail.NormalizeMessageIDHeader(in.LastOutboundMessageID)
	in.LastReplyMessageID = strings.TrimSpace(in.LastReplyMessageID)
	in.ThreadSubject = strings.TrimSpace(in.ThreadSubject)
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_thread_bindings(id,account_id,thread_id,binding_type,campaign_id,enrollment_id,funnel_id,reply_account_id,reply_sender_profile_id,collector_account_id,owner_user_id,recipient_email,recipient_domain,root_outbound_message_id,last_outbound_message_id,last_reply_message_id,thread_subject,last_reply_at,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
		   account_id=excluded.account_id,
		   thread_id=excluded.thread_id,
		   binding_type=excluded.binding_type,
		   campaign_id=excluded.campaign_id,
		   enrollment_id=excluded.enrollment_id,
		   funnel_id=excluded.funnel_id,
		   reply_account_id=excluded.reply_account_id,
		   reply_sender_profile_id=excluded.reply_sender_profile_id,
		   collector_account_id=excluded.collector_account_id,
		   owner_user_id=excluded.owner_user_id,
		   recipient_email=excluded.recipient_email,
		   recipient_domain=excluded.recipient_domain,
		   root_outbound_message_id=excluded.root_outbound_message_id,
		   last_outbound_message_id=excluded.last_outbound_message_id,
		   last_reply_message_id=excluded.last_reply_message_id,
		   thread_subject=excluded.thread_subject,
		   last_reply_at=excluded.last_reply_at,
		   updated_at=excluded.updated_at`,
		in.ID,
		in.AccountID,
		in.ThreadID,
		in.BindingType,
		nullableTrimmedString(in.CampaignID),
		nullableTrimmedString(in.EnrollmentID),
		nullableTrimmedString(in.FunnelID),
		nullableTrimmedString(in.ReplyAccountID),
		nullableTrimmedString(in.ReplySenderProfileID),
		nullableTrimmedString(in.CollectorAccountID),
		nullableTrimmedString(in.OwnerUserID),
		in.RecipientEmail,
		in.RecipientDomain,
		in.RootOutboundMessageID,
		in.LastOutboundMessageID,
		in.LastReplyMessageID,
		in.ThreadSubject,
		nullTimeValue(in.LastReplyAt),
		coalesceTime(in.CreatedAt, now),
		in.UpdatedAt,
	)
	if err != nil {
		return models.MailThreadBinding{}, err
	}
	return s.GetMailThreadBindingByID(ctx, in.ID)
}

func (s *Store) FindMailThreadBindingByReplyHeaders(ctx context.Context, accountID string, messageIDs []string) (models.MailThreadBinding, error) {
	normalized := mail.NormalizeMessageIDHeaders(messageIDs)
	if len(normalized) == 0 {
		return models.MailThreadBinding{}, ErrNotFound
	}
	args := make([]any, 0, len(normalized)*2+3)
	args = append(args, strings.TrimSpace(accountID))
	args = append(args, strings.TrimSpace(accountID))
	args = append(args, strings.TrimSpace(accountID))
	for _, id := range normalized {
		args = append(args, id)
	}
	for _, id := range normalized {
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`SELECT id,account_id,thread_id,binding_type,campaign_id,enrollment_id,funnel_id,reply_account_id,reply_sender_profile_id,collector_account_id,owner_user_id,recipient_email,recipient_domain,root_outbound_message_id,last_outbound_message_id,last_reply_message_id,thread_subject,last_reply_at,created_at,updated_at
		 FROM mail_thread_bindings
		 WHERE (account_id=? OR reply_account_id=? OR collector_account_id=?)
		   AND (root_outbound_message_id IN (%s) OR last_outbound_message_id IN (%s))
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		placeholders(len(normalized)),
		placeholders(len(normalized)),
	)
	row := s.db.QueryRowContext(ctx, query, args...)
	item, err := scanMailThreadBinding(row)
	if err == sql.ErrNoRows {
		return models.MailThreadBinding{}, ErrNotFound
	}
	if err != nil {
		return models.MailThreadBinding{}, err
	}
	return item, nil
}

func (s *Store) FindMailThreadBindingByOutboundMessageID(ctx context.Context, accountID, messageID string) (models.MailThreadBinding, error) {
	messageID = mail.NormalizeMessageIDHeader(messageID)
	if messageID == "" {
		return models.MailThreadBinding{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,thread_id,binding_type,campaign_id,enrollment_id,funnel_id,reply_account_id,reply_sender_profile_id,collector_account_id,owner_user_id,recipient_email,recipient_domain,root_outbound_message_id,last_outbound_message_id,last_reply_message_id,thread_subject,last_reply_at,created_at,updated_at
		 FROM mail_thread_bindings
		 WHERE account_id=? AND (root_outbound_message_id=? OR last_outbound_message_id=?)
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		strings.TrimSpace(accountID),
		messageID,
		messageID,
	)
	item, err := scanMailThreadBinding(row)
	if err == sql.ErrNoRows {
		return models.MailThreadBinding{}, ErrNotFound
	}
	if err != nil {
		return models.MailThreadBinding{}, err
	}
	return item, nil
}

func scanOutboundSuppression(row outboundScanner) (models.OutboundSuppression, error) {
	var item models.OutboundSuppression
	var campaignID sql.NullString
	var expiresAt sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.ScopeKind,
		&item.ScopeValue,
		&campaignID,
		&item.Reason,
		&item.SourceKind,
		&expiresAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.OutboundSuppression{}, err
	}
	item.ScopeKind = normalizeSuppressionScopeKind(item.ScopeKind)
	item.ScopeValue = strings.ToLower(strings.TrimSpace(item.ScopeValue))
	item.CampaignID = strings.TrimSpace(campaignID.String)
	item.Reason = strings.TrimSpace(item.Reason)
	item.SourceKind = normalizeSuppressionSourceKind(item.SourceKind)
	if expiresAt.Valid {
		item.ExpiresAt = expiresAt.Time
	}
	return item, nil
}

func (s *Store) GetActiveOutboundSuppression(ctx context.Context, userID, recipientEmail, recipientDomain, campaignID string, now time.Time) (models.OutboundSuppression, error) {
	recipientEmail = strings.ToLower(strings.TrimSpace(recipientEmail))
	recipientDomain = strings.ToLower(strings.TrimSpace(recipientDomain))
	query := `SELECT id,user_id,scope_kind,scope_value,campaign_id,reason,source_kind,expires_at,created_at,updated_at
		 FROM outbound_suppressions
		 WHERE user_id=?
		   AND (expires_at IS NULL OR expires_at>?)
		   AND (
		     (scope_kind='recipient' AND scope_value=?)
		     OR (scope_kind='domain' AND scope_value=?)
		   )
		   AND (campaign_id IS NULL OR campaign_id='' OR campaign_id=?) 
		 ORDER BY CASE WHEN campaign_id=? THEN 0 ELSE 1 END, updated_at DESC
		 LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, userID, now.UTC(), recipientEmail, recipientDomain, strings.TrimSpace(campaignID), strings.TrimSpace(campaignID))
	item, err := scanOutboundSuppression(row)
	if err == sql.ErrNoRows {
		return models.OutboundSuppression{}, ErrNotFound
	}
	if err != nil {
		return models.OutboundSuppression{}, err
	}
	return item, nil
}

func (s *Store) ListOutboundSuppressions(ctx context.Context, userID string) ([]models.OutboundSuppression, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,scope_kind,scope_value,campaign_id,reason,source_kind,expires_at,created_at,updated_at
		 FROM outbound_suppressions
		 WHERE user_id=?
		 ORDER BY updated_at DESC, scope_kind ASC, scope_value COLLATE NOCASE ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OutboundSuppression, 0, 32)
	for rows.Next() {
		item, err := scanOutboundSuppression(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertOutboundSuppression(ctx context.Context, in models.OutboundSuppression) (models.OutboundSuppression, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.UserID = strings.TrimSpace(in.UserID)
	in.ScopeKind = normalizeSuppressionScopeKind(in.ScopeKind)
	in.ScopeValue = strings.ToLower(strings.TrimSpace(in.ScopeValue))
	in.CampaignID = strings.TrimSpace(in.CampaignID)
	in.Reason = strings.TrimSpace(in.Reason)
	in.SourceKind = normalizeSuppressionSourceKind(in.SourceKind)
	in.CreatedAt = coalesceTime(in.CreatedAt, now)
	in.UpdatedAt = now
	var existingID string
	row := s.db.QueryRowContext(ctx,
		`SELECT id
		 FROM outbound_suppressions
		 WHERE user_id=?
		   AND scope_kind=?
		   AND scope_value=?
		   AND IFNULL(campaign_id,'')=IFNULL(?, '')
		 LIMIT 1`,
		in.UserID,
		in.ScopeKind,
		in.ScopeValue,
		nullableTrimmedString(in.CampaignID),
	)
	switch err := row.Scan(&existingID); err {
	case nil:
		in.ID = strings.TrimSpace(existingID)
		_, err = s.db.ExecContext(ctx,
			`UPDATE outbound_suppressions
			 SET reason=?, source_kind=?, expires_at=?, updated_at=?
			 WHERE id=?`,
			in.Reason,
			in.SourceKind,
			nullTimeValue(in.ExpiresAt),
			in.UpdatedAt,
			in.ID,
		)
		if err != nil {
			return models.OutboundSuppression{}, err
		}
	case sql.ErrNoRows:
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO outbound_suppressions(id,user_id,scope_kind,scope_value,campaign_id,reason,source_kind,expires_at,created_at,updated_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?)`,
			in.ID,
			in.UserID,
			in.ScopeKind,
			in.ScopeValue,
			nullableTrimmedString(in.CampaignID),
			in.Reason,
			in.SourceKind,
			nullTimeValue(in.ExpiresAt),
			in.CreatedAt,
			in.UpdatedAt,
		)
		if err != nil {
			return models.OutboundSuppression{}, err
		}
	default:
		return models.OutboundSuppression{}, err
	}
	row = s.db.QueryRowContext(ctx,
		`SELECT id,user_id,scope_kind,scope_value,campaign_id,reason,source_kind,expires_at,created_at,updated_at
		 FROM outbound_suppressions
		 WHERE user_id=? AND scope_kind=? AND scope_value=? AND IFNULL(campaign_id,'')=?`,
		in.UserID,
		in.ScopeKind,
		in.ScopeValue,
		in.CampaignID,
	)
	item, err := scanOutboundSuppression(row)
	if err != nil {
		return models.OutboundSuppression{}, err
	}
	return item, nil
}

func (s *Store) DeleteOutboundSuppression(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM outbound_suppressions WHERE user_id=? AND id=?`, userID, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
