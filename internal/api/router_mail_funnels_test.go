package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"despatch/internal/models"
	"despatch/internal/service"
)

func TestV2ReplyFunnelsManagedRulesLifecycle(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	collector := createV2TestAccount(t, router, sess, csrf, "collector@example.com")
	sourceA := createV2TestAccount(t, router, sess, csrf, "john@example.com")
	sourceB := createV2TestAccount(t, router, sess, csrf, "maybelin@example.com")

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/funnels", map[string]any{
		"name":                 "Libero Replies",
		"sender_name":          "John",
		"collector_account_id": collector.ID,
		"source_account_ids":   []string{sourceA.ID, sourceB.ID},
		"reply_mode":           "collector",
		"routing_mode":         "managed_rules",
		"include_collector":    true,
		"target_reply_count":   100,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", create.Code, create.Body.String())
	}

	var funnel models.ReplyFunnel
	if err := json.Unmarshal(create.Body.Bytes(), &funnel); err != nil {
		t.Fatalf("decode funnel create: %v body=%s", err, create.Body.String())
	}
	if got := strings.TrimSpace(funnel.SavedSearchID); got == "" {
		t.Fatalf("expected saved search id on funnel response")
	}
	if got := len(funnel.SourceAccountIDs); got != 2 {
		t.Fatalf("expected 2 source accounts, got %d", got)
	}

	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	savedList, err := st.ListSavedSearches(context.Background(), admin.ID)
	if err != nil {
		t.Fatalf("list saved searches: %v", err)
	}
	if len(savedList) != 1 {
		t.Fatalf("expected one saved search, got %d", len(savedList))
	}
	if !strings.Contains(savedList[0].Name, "Libero Replies") {
		t.Fatalf("expected saved search name to include funnel name, got %q", savedList[0].Name)
	}

	rows, err := st.ListReplyFunnelAccounts(context.Background(), funnel.ID)
	if err != nil {
		t.Fatalf("list funnel accounts: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected collector + 2 sources, got %d", len(rows))
	}
	var collectorRow models.ReplyFunnelAccount
	sourceRows := make([]models.ReplyFunnelAccount, 0, 2)
	for _, row := range rows {
		if row.Role == "collector" {
			collectorRow = row
			continue
		}
		sourceRows = append(sourceRows, row)
	}
	if collectorRow.AccountID != collector.ID {
		t.Fatalf("expected collector row for collector account, got %+v", collectorRow)
	}
	if strings.TrimSpace(collectorRow.SenderIdentityID) == "" {
		t.Fatalf("expected collector sender identity id to be populated")
	}
	for _, row := range sourceRows {
		if strings.TrimSpace(row.SenderIdentityID) == "" {
			t.Fatalf("expected source sender identity id to be populated for %+v", row)
		}
		if strings.TrimSpace(row.RedirectRuleID) == "" {
			t.Fatalf("expected managed redirect rule id for %+v", row)
		}
		rule, err := st.GetMailRuleByID(context.Background(), row.AccountID, row.RedirectRuleID)
		if err != nil {
			t.Fatalf("load managed rule: %v", err)
		}
		if got := strings.TrimSpace(rule.Actions.Redirect); got != collector.Login {
			t.Fatalf("expected redirect to %q got %q", collector.Login, got)
		}
	}

	update := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/funnels/"+funnel.ID, map[string]any{
		"name":                 "Collector Only",
		"sender_name":          "Maybelin",
		"collector_account_id": collector.ID,
		"source_account_ids":   []string{sourceA.ID},
		"reply_mode":           "source",
		"routing_mode":         "virtual_inbox",
		"include_collector":    false,
		"target_reply_count":   50,
	}, sess, csrf)
	if update.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", update.Code, update.Body.String())
	}
	var updated models.ReplyFunnel
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode funnel update: %v body=%s", err, update.Body.String())
	}
	if updated.RoutingMode != "virtual_inbox" {
		t.Fatalf("expected virtual_inbox routing mode, got %q", updated.RoutingMode)
	}
	if updated.ReplyMode != "source" {
		t.Fatalf("expected source reply mode, got %q", updated.ReplyMode)
	}
	if got := len(updated.SourceAccountIDs); got != 1 {
		t.Fatalf("expected 1 source account after update, got %d", got)
	}

	rows, err = st.ListReplyFunnelAccounts(context.Background(), funnel.ID)
	if err != nil {
		t.Fatalf("list updated funnel accounts: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected collector + 1 source after update, got %d", len(rows))
	}
	for _, row := range rows {
		if row.Role != "source" {
			continue
		}
		if row.AccountID != sourceA.ID {
			t.Fatalf("unexpected remaining source account %q", row.AccountID)
		}
		if strings.TrimSpace(row.RedirectRuleID) != "" {
			t.Fatalf("expected redirect rule removed in virtual mode, got %q", row.RedirectRuleID)
		}
	}
	rulesA, err := st.ListMailRules(context.Background(), sourceA.ID)
	if err != nil {
		t.Fatalf("list sourceA rules: %v", err)
	}
	for _, rule := range rulesA {
		if strings.Contains(rule.Name, "Collector Only") {
			t.Fatalf("did not expect managed funnel rule to remain after virtual mode update")
		}
	}
	rulesB, err := st.ListMailRules(context.Background(), sourceB.ID)
	if err != nil {
		t.Fatalf("list sourceB rules: %v", err)
	}
	if len(rulesB) != 0 {
		t.Fatalf("expected sourceB rules removed after source deletion, got %d", len(rulesB))
	}

	del := doV2AuthedJSON(t, router, http.MethodDelete, "/api/v2/funnels/"+funnel.ID, map[string]any{}, sess, csrf)
	if del.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d body=%s", del.Code, del.Body.String())
	}
	if _, err := st.GetReplyFunnelByID(context.Background(), admin.ID, funnel.ID); err == nil {
		t.Fatalf("expected funnel to be deleted")
	}
	savedList, err = st.ListSavedSearches(context.Background(), admin.ID)
	if err != nil {
		t.Fatalf("list saved searches after delete: %v", err)
	}
	if len(savedList) != 0 {
		t.Fatalf("expected funnel saved search to be deleted, got %d items", len(savedList))
	}
}

func TestV2ReplyFunnelsAssistedForwardingLifecycle(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	collector, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:             uuid.NewString(),
		UserID:         admin.ID,
		DisplayName:    "Collector",
		Login:          "collector@example.com",
		SecretEnc:      "enc",
		IMAPHost:       "imap.example.com",
		IMAPPort:       993,
		IMAPTLS:        true,
		SMTPHost:       "smtp.example.com",
		SMTPPort:       587,
		SMTPStartTLS:   true,
		ProviderType:   service.MailProviderTypeGeneric,
		ProviderLabel:  "Generic IMAP/SMTP",
		AuthKind:       service.MailAccountAuthKindPassword,
		ConnectionMode: service.MailConnectionModeIMAPSMTP,
		IsDefault:      true,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	source, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:             uuid.NewString(),
		UserID:         admin.ID,
		DisplayName:    "Libero Source",
		Login:          "john@libero.it",
		SecretEnc:      "enc",
		IMAPHost:       "imapmail.libero.it",
		IMAPPort:       993,
		IMAPTLS:        true,
		SMTPHost:       "smtp.libero.it",
		SMTPPort:       465,
		SMTPTLS:        true,
		ProviderType:   service.MailProviderTypeLibero,
		ProviderLabel:  "Libero Mail",
		AuthKind:       service.MailAccountAuthKindAppPassword,
		ConnectionMode: service.MailConnectionModeIMAPSMTP,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/funnels", map[string]any{
		"name":                 "Libero Assisted",
		"sender_name":          "John",
		"collector_account_id": collector.ID,
		"source_account_ids":   []string{source.ID},
		"reply_mode":           "collector",
		"routing_mode":         "assisted_forwarding",
		"include_collector":    true,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var funnel models.ReplyFunnel
	if err := json.Unmarshal(create.Body.Bytes(), &funnel); err != nil {
		t.Fatalf("decode funnel: %v body=%s", err, create.Body.String())
	}
	if len(funnel.Accounts) != 2 {
		t.Fatalf("expected collector + source, got %d", len(funnel.Accounts))
	}
	var sourceRow models.ReplyFunnelAccount
	for _, row := range funnel.Accounts {
		if row.Role == "source" {
			sourceRow = row
			break
		}
	}
	if sourceRow.AssistedForwardingState != "pending" {
		t.Fatalf("expected pending assisted forwarding state, got %+v", sourceRow)
	}
	if got := strings.TrimSpace(sourceRow.AssistedForwardingWarning); got == "" {
		t.Fatalf("expected Mail Plus warning for non-Libero collector, got %+v", sourceRow)
	}
	if strings.TrimSpace(sourceRow.RedirectRuleID) != "" {
		t.Fatalf("did not expect redirect rule for assisted forwarding, got %+v", sourceRow)
	}

	update := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/funnels/"+funnel.ID+"/assisted-forwarding/"+source.ID, map[string]any{
		"state": "confirmed",
	}, sess, csrf)
	if update.Code != http.StatusOK {
		t.Fatalf("expected assisted forwarding update 200, got %d body=%s", update.Code, update.Body.String())
	}
	var updated models.ReplyFunnel
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated funnel: %v body=%s", err, update.Body.String())
	}
	for _, row := range updated.Accounts {
		if row.Role != "source" {
			continue
		}
		if row.AssistedForwardingState != "confirmed" {
			t.Fatalf("expected confirmed assisted forwarding state, got %+v", row)
		}
		if row.AssistedForwardingConfirmed.IsZero() {
			t.Fatalf("expected confirmed timestamp, got %+v", row)
		}
	}
}

func TestV2ReplyFunnelsGmailWorkspaceAssistedForwardingUsesWorkspaceGuide(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	collector, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:             uuid.NewString(),
		UserID:         admin.ID,
		DisplayName:    "Collector",
		Login:          "collector@example.com",
		SecretEnc:      "enc",
		IMAPHost:       "imap.example.com",
		IMAPPort:       993,
		IMAPTLS:        true,
		SMTPHost:       "smtp.example.com",
		SMTPPort:       587,
		SMTPStartTLS:   true,
		ProviderType:   service.MailProviderTypeGeneric,
		ProviderLabel:  "Generic IMAP/SMTP",
		AuthKind:       service.MailAccountAuthKindPassword,
		ConnectionMode: service.MailConnectionModeIMAPSMTP,
		IsDefault:      true,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	source, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:             uuid.NewString(),
		UserID:         admin.ID,
		DisplayName:    "Workspace Source",
		Login:          "john@example.com",
		SecretEnc:      "enc",
		IMAPHost:       "imap.gmail.com",
		IMAPPort:       993,
		IMAPTLS:        true,
		SMTPHost:       "smtp.gmail.com",
		SMTPPort:       587,
		SMTPStartTLS:   true,
		ProviderType:   service.MailProviderTypeGmail,
		ProviderLabel:  "Gmail / Google Workspace",
		AuthKind:       service.MailAccountAuthKindAppPassword,
		ConnectionMode: service.MailConnectionModeIMAPSMTP,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/funnels", map[string]any{
		"name":                 "Workspace Assisted",
		"sender_name":          "John",
		"collector_account_id": collector.ID,
		"source_account_ids":   []string{source.ID},
		"reply_mode":           "collector",
		"routing_mode":         "assisted_forwarding",
		"include_collector":    true,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var funnel models.ReplyFunnel
	if err := json.Unmarshal(create.Body.Bytes(), &funnel); err != nil {
		t.Fatalf("decode funnel: %v body=%s", err, create.Body.String())
	}
	var sourceRow models.ReplyFunnelAccount
	for _, row := range funnel.Accounts {
		if row.Role == "source" {
			sourceRow = row
			break
		}
	}
	if sourceRow.AssistedForwardingState != "pending" {
		t.Fatalf("expected pending assisted forwarding state, got %+v", sourceRow)
	}
	if !strings.Contains(sourceRow.AssistedForwardingURL, "14724207") {
		t.Fatalf("expected Workspace forwarding guide URL, got %+v", sourceRow)
	}
	if !strings.Contains(strings.ToLower(sourceRow.AssistedForwardingWarning), "admins may need to allow automatic forwarding") {
		t.Fatalf("expected Workspace forwarding warning, got %+v", sourceRow)
	}
}

func TestV2ReplyFunnelsSupportMoreThanOneHundredSources(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	collector, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:           uuid.NewString(),
		UserID:       admin.ID,
		DisplayName:  "Collector",
		Login:        "collector@example.com",
		SecretEnc:    "enc",
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPTLS:      true,
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPStartTLS: true,
		IsDefault:    true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create collector: %v", err)
	}
	sourceIDs := make([]string, 0, 120)
	for i := 0; i < 120; i++ {
		account, err := st.CreateMailAccount(context.Background(), models.MailAccount{
			ID:           uuid.NewString(),
			UserID:       admin.ID,
			DisplayName:  "Source",
			Login:        "source" + strings.TrimSpace(uuid.NewString())[:8] + "@example.com",
			SecretEnc:    "enc",
			IMAPHost:     "imap.example.com",
			IMAPPort:     993,
			IMAPTLS:      true,
			SMTPHost:     "smtp.example.com",
			SMTPPort:     587,
			SMTPStartTLS: true,
			Status:       "active",
		})
		if err != nil {
			t.Fatalf("create source %d: %v", i, err)
		}
		sourceIDs = append(sourceIDs, account.ID)
	}
	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/funnels", map[string]any{
		"name":                 "Unlimited Sources",
		"sender_name":          "John",
		"collector_account_id": collector.ID,
		"source_account_ids":   sourceIDs,
		"reply_mode":           "collector",
		"routing_mode":         "virtual_inbox",
		"include_collector":    true,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var funnel models.ReplyFunnel
	if err := json.Unmarshal(create.Body.Bytes(), &funnel); err != nil {
		t.Fatalf("decode funnel: %v body=%s", err, create.Body.String())
	}
	if got := len(funnel.SourceAccountIDs); got != 120 {
		t.Fatalf("expected 120 source accounts, got %d", got)
	}
	rows, err := st.ListReplyFunnelAccounts(context.Background(), funnel.ID)
	if err != nil {
		t.Fatalf("list funnel accounts: %v", err)
	}
	if got := len(rows); got != 121 {
		t.Fatalf("expected 121 funnel rows including collector, got %d", got)
	}
}
