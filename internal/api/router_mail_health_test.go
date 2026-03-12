package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"despatch/internal/config"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
)

type fakeMailHealthCoordinator struct {
	states map[string]models.MailHealthActionState
	calls  []string
	err    error
}

func (f *fakeMailHealthCoordinator) QueueAccountSync(ctx context.Context, account models.MailAccount) (models.MailHealthActionState, error) {
	f.calls = append(f.calls, "sync:"+account.ID)
	if f.err != nil {
		return models.MailHealthActionState{}, f.err
	}
	state := models.MailHealthActionState{Kind: "sync", Status: "queued", UpdatedAt: time.Now().UTC()}
	if f.states == nil {
		f.states = map[string]models.MailHealthActionState{}
	}
	f.states[account.ID] = state
	return state, nil
}

func (f *fakeMailHealthCoordinator) QueueQuotaRefresh(ctx context.Context, account models.MailAccount) (models.MailHealthActionState, error) {
	f.calls = append(f.calls, "quota_refresh:"+account.ID)
	if f.err != nil {
		return models.MailHealthActionState{}, f.err
	}
	state := models.MailHealthActionState{Kind: "quota_refresh", Status: "queued", UpdatedAt: time.Now().UTC()}
	if f.states == nil {
		f.states = map[string]models.MailHealthActionState{}
	}
	f.states[account.ID] = state
	return state, nil
}

func (f *fakeMailHealthCoordinator) QueueAccountReindex(ctx context.Context, account models.MailAccount) (models.MailHealthActionState, error) {
	f.calls = append(f.calls, "reindex:"+account.ID)
	if f.err != nil {
		return models.MailHealthActionState{}, f.err
	}
	state := models.MailHealthActionState{Kind: "reindex", Status: "queued", UpdatedAt: time.Now().UTC()}
	if f.states == nil {
		f.states = map[string]models.MailHealthActionState{}
	}
	f.states[account.ID] = state
	return state, nil
}

func (f *fakeMailHealthCoordinator) ActionState(accountID string) (models.MailHealthActionState, bool) {
	if f.states == nil {
		return models.MailHealthActionState{}, false
	}
	state, ok := f.states[accountID]
	return state, ok
}

func TestV2ListAccountHealthIncludesQuotaAndActionState(t *testing.T) {
	coord := &fakeMailHealthCoordinator{states: map[string]models.MailHealthActionState{}}
	router, st, _ := newV2RouterWithMailClientStoreAndHook(t, mail.NoopClient{}, nil, func(svc *service.Service, st *store.Store, _ *sql.DB, _ *config.Config) {
		svc.SetMailHealthCoordinator(coord)
	})

	sess, csrf := loginV2(t, router)
	healthy := createV2TestAccount(t, router, sess, csrf, "health-ok@example.com")
	broken := createV2TestAccount(t, router, sess, csrf, "health-bad@example.com")

	now := time.Now().UTC()
	if err := st.UpdateMailAccountSyncStatus(context.Background(), healthy.ID, now.Add(-2*time.Minute), ""); err != nil {
		t.Fatalf("update healthy sync status: %v", err)
	}
	if err := st.UpdateMailAccountSyncStatus(context.Background(), broken.ID, now.Add(-20*time.Minute), "imap down"); err != nil {
		t.Fatalf("update broken sync status: %v", err)
	}
	if _, err := st.UpsertQuotaCache(context.Background(), models.QuotaCache{
		AccountID:     healthy.ID,
		UsedBytes:     2048,
		TotalBytes:    4096,
		UsedMessages:  10,
		TotalMessages: 20,
		RefreshedAt:   now,
		LastError:     "",
	}); err != nil {
		t.Fatalf("seed quota cache: %v", err)
	}
	if _, err := st.UpsertQuotaCache(context.Background(), models.QuotaCache{
		AccountID:   broken.ID,
		RefreshedAt: now,
		LastError:   mail.ErrQuotaUnsupported.Error(),
	}); err != nil {
		t.Fatalf("seed unsupported quota cache: %v", err)
	}
	coord.states[healthy.ID] = models.MailHealthActionState{
		Kind:      "quota_refresh",
		Status:    "running",
		UpdatedAt: now,
	}

	rec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts/health", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Summary models.MailAccountHealthSummary `json:"summary"`
		Items   []models.MailAccountHealth      `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Summary.TotalAccounts != 2 || payload.Summary.HealthyAccounts != 1 || payload.Summary.ErrorAccounts != 1 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 health items, got %d", len(payload.Items))
	}
	var sawHealthy, sawBroken bool
	for _, item := range payload.Items {
		switch item.AccountID {
		case healthy.ID:
			sawHealthy = true
			if item.Status != "ok" {
				t.Fatalf("expected healthy account status ok, got %q", item.Status)
			}
			if !item.QuotaAvailable || !item.QuotaSupported || item.UsedBytes != 2048 || item.TotalMessages != 20 {
				t.Fatalf("unexpected healthy quota payload: %+v", item)
			}
			if item.ActionState == nil || item.ActionState.Kind != "quota_refresh" || item.ActionState.Status != "running" {
				t.Fatalf("expected running action state on healthy item, got %+v", item.ActionState)
			}
		case broken.ID:
			sawBroken = true
			if item.Status != "error" {
				t.Fatalf("expected broken account status error, got %q", item.Status)
			}
			if item.QuotaSupported || item.QuotaAvailable {
				t.Fatalf("expected unsupported quota payload, got %+v", item)
			}
			if item.QuotaLastError == "" {
				t.Fatalf("expected quota last error for unsupported quota")
			}
		}
	}
	if !sawHealthy || !sawBroken {
		t.Fatalf("missing expected accounts in payload: %+v", payload.Items)
	}
}

func TestV2QueueAccountHealthAction(t *testing.T) {
	coord := &fakeMailHealthCoordinator{}
	router, _, _ := newV2RouterWithMailClientStoreAndHook(t, mail.NoopClient{}, nil, func(svc *service.Service, _ *store.Store, _ *sql.DB, _ *config.Config) {
		svc.SetMailHealthCoordinator(coord)
	})
	sess, csrf := loginV2(t, router)
	account := createV2TestAccount(t, router, sess, csrf, "health-action@example.com")

	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/health/sync", map[string]any{}, sess, csrf)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(coord.calls) != 1 || coord.calls[0] != "sync:"+account.ID {
		t.Fatalf("expected sync call for %s, got %#v", account.ID, coord.calls)
	}

	coord.err = service.ErrMailHealthActionInProgress
	rec = doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/health/reindex", map[string]any{}, sess, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}
