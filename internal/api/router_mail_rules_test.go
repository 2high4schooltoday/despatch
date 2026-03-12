package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"despatch/internal/config"
	"despatch/internal/mail"
	"despatch/internal/models"
)

func TestV2AccountRulesManagedScriptLifecycle(t *testing.T) {
	client := &indexedBulkTestMailClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX"},
			{Name: "Archive"},
			{Name: "Trash"},
			{Name: "Junk"},
		},
	}
	previousFactory := mailClientFactory
	mailClientFactory = func(cfg config.Config) mail.Client { return client }
	t.Cleanup(func() {
		mailClientFactory = previousFactory
	})
	router, st := newV2RouterWithMailClientAndStore(t, client, nil)
	sess, csrf := loginV2(t, router)
	account := createV2TestAccount(t, router, sess, csrf, "rules@example.com")

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/rules", map[string]any{
		"name":       "Block billing sender",
		"enabled":    true,
		"match_mode": "all",
		"conditions": map[string]any{
			"from_contains": "billing@example.com",
		},
		"actions": map[string]any{
			"move_to_role": "junk",
			"stop":         true,
		},
	}, sess, csrf)
	if create.Code != http.StatusOK {
		t.Fatalf("expected rules create 200, got %d body=%s", create.Code, create.Body.String())
	}
	var payload struct {
		Items               []models.MailRule `json:"items"`
		ManagedScriptName   string            `json:"managed_script_name"`
		ActiveScriptName    string            `json:"active_script_name"`
		ManagedScriptActive bool              `json:"managed_script_active"`
		CustomScriptActive  bool              `json:"custom_script_active"`
		JunkMailboxName     string            `json:"junk_mailbox_name"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode rules create payload: %v body=%s", err, create.Body.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one rule after create, got %d", len(payload.Items))
	}
	if payload.ManagedScriptName != managedRulesScriptName {
		t.Fatalf("expected managed script name %q, got %q", managedRulesScriptName, payload.ManagedScriptName)
	}
	if payload.ActiveScriptName != managedRulesScriptName || !payload.ManagedScriptActive || payload.CustomScriptActive {
		t.Fatalf("unexpected managed script state: %+v", payload)
	}
	if payload.JunkMailboxName != "Junk" {
		t.Fatalf("expected junk mailbox name Junk, got %q", payload.JunkMailboxName)
	}
	script, err := st.GetSieveScript(context.Background(), account.ID, managedRulesScriptName)
	if err != nil {
		t.Fatalf("load managed sieve script: %v", err)
	}
	if !strings.Contains(script.ScriptBody, `fileinto "Junk";`) {
		t.Fatalf("expected managed script to move to Junk, got body=%s", script.ScriptBody)
	}
	active, err := st.ActiveSieveScriptName(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("load active sieve script: %v", err)
	}
	if active != managedRulesScriptName {
		t.Fatalf("expected managed script active, got %q", active)
	}

	customPut := doV2AuthedJSON(t, router, http.MethodPut, "/api/v2/rules/scripts/custom-junk?account_id="+account.ID, map[string]any{
		"body": "require [\"fileinto\"];\n\n# Custom override\n",
	}, sess, csrf)
	if customPut.Code != http.StatusOK {
		t.Fatalf("expected custom script save 200, got %d body=%s", customPut.Code, customPut.Body.String())
	}
	customActivate := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/rules/scripts/custom-junk/activate?account_id="+account.ID, map[string]any{}, sess, csrf)
	if customActivate.Code != http.StatusOK {
		t.Fatalf("expected custom activate 200, got %d body=%s", customActivate.Code, customActivate.Body.String())
	}

	createSecond := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/rules", map[string]any{
		"name":       "Mark invoices read",
		"enabled":    true,
		"match_mode": "all",
		"conditions": map[string]any{
			"subject_contains": "invoice",
		},
		"actions": map[string]any{
			"mark_read": true,
			"stop":      true,
		},
	}, sess, csrf)
	if createSecond.Code != http.StatusOK {
		t.Fatalf("expected second rules create 200, got %d body=%s", createSecond.Code, createSecond.Body.String())
	}
	var secondPayload struct {
		ActiveScriptName   string `json:"active_script_name"`
		CustomScriptActive bool   `json:"custom_script_active"`
	}
	if err := json.Unmarshal(createSecond.Body.Bytes(), &secondPayload); err != nil {
		t.Fatalf("decode second rules payload: %v body=%s", err, createSecond.Body.String())
	}
	if secondPayload.ActiveScriptName != "custom-junk" || !secondPayload.CustomScriptActive {
		t.Fatalf("expected custom script to remain active after builder save, got %+v", secondPayload)
	}
	active, err = st.ActiveSieveScriptName(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("load active custom script: %v", err)
	}
	if active != "custom-junk" {
		t.Fatalf("expected custom script to remain active, got %q", active)
	}

	activateManaged := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/rules/activate-managed", map[string]any{}, sess, csrf)
	if activateManaged.Code != http.StatusOK {
		t.Fatalf("expected activate-managed 200, got %d body=%s", activateManaged.Code, activateManaged.Body.String())
	}
	active, err = st.ActiveSieveScriptName(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("load reactivated managed script: %v", err)
	}
	if active != managedRulesScriptName {
		t.Fatalf("expected managed script reactivated, got %q", active)
	}
}
