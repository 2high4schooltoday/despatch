package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"despatch/internal/config"
	"despatch/internal/mail"
)

func TestHealthReadyMemoizesAndRefreshesProbes(t *testing.T) {
	oldIMAP := probeIMAP
	oldSMTP := probeSMTP
	oldTTL := readinessProbeTTL
	oldTimeout := readinessProbeTimeout
	probeIMAPCalls := 0
	probeSMTPCalls := 0
	var mu sync.Mutex
	probeIMAP = func(_ context.Context, _ config.Config) error {
		mu.Lock()
		defer mu.Unlock()
		probeIMAPCalls++
		return nil
	}
	probeSMTP = func(_ context.Context, _ config.Config) error {
		mu.Lock()
		defer mu.Unlock()
		probeSMTPCalls++
		return nil
	}
	readinessProbeTTL = 20 * time.Millisecond
	readinessProbeTimeout = time.Second
	t.Cleanup(func() {
		probeIMAP = oldIMAP
		probeSMTP = oldSMTP
		readinessProbeTTL = oldTTL
		readinessProbeTimeout = oldTimeout
	})

	router := newSendRouter(t, mail.NoopClient{}, "")

	mu.Lock()
	if probeIMAPCalls != 1 || probeSMTPCalls != 1 {
		mu.Unlock()
		t.Fatalf("expected readiness prime to run once, got imap=%d smtp=%d", probeIMAPCalls, probeSMTPCalls)
	}
	mu.Unlock()

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", first.Code, first.Body.String())
	}
	firstCheckedAt := decodeReadyCheckedAt(t, first)

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", second.Code, second.Body.String())
	}
	mu.Lock()
	if probeIMAPCalls != 1 || probeSMTPCalls != 1 {
		mu.Unlock()
		t.Fatalf("expected cached readiness probes within ttl, got imap=%d smtp=%d", probeIMAPCalls, probeSMTPCalls)
	}
	mu.Unlock()

	time.Sleep(30 * time.Millisecond)
	stale := httptest.NewRecorder()
	router.ServeHTTP(stale, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if stale.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", stale.Code, stale.Body.String())
	}
	if got := decodeReadyCheckedAt(t, stale); !got.Equal(firstCheckedAt) {
		t.Fatalf("expected stale response to serve cached probe timestamp, got %s want %s", got, firstCheckedAt)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		mu.Lock()
		imapCalls := probeIMAPCalls
		smtpCalls := probeSMTPCalls
		mu.Unlock()
		if imapCalls >= 2 && smtpCalls >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected background readiness refresh to run, got imap=%d smtp=%d", imapCalls, smtpCalls)
		}
		time.Sleep(10 * time.Millisecond)
	}

	refreshed := httptest.NewRecorder()
	router.ServeHTTP(refreshed, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if refreshed.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", refreshed.Code, refreshed.Body.String())
	}
	if got := decodeReadyCheckedAt(t, refreshed); got.Before(firstCheckedAt) {
		t.Fatalf("expected refreshed readiness timestamp to stay at or after cached value, got %s want >= %s", got, firstCheckedAt)
	}
}

func decodeReadyCheckedAt(t *testing.T, rec *httptest.ResponseRecorder) time.Time {
	t.Helper()
	var payload struct {
		CheckedAt string `json:"checked_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode readiness payload: %v body=%s", err, rec.Body.String())
	}
	parsed, err := time.Parse(time.RFC3339, payload.CheckedAt)
	if err != nil {
		t.Fatalf("parse checked_at %q: %v", payload.CheckedAt, err)
	}
	return parsed
}
