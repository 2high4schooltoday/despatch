package api

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"despatch/internal/config"
	"despatch/internal/mail"
	"despatch/internal/service"
)

var (
	mailboxCacheTTL       = 20 * time.Second
	threadCacheTTL        = 30 * time.Second
	readinessProbeTTL     = 30 * time.Second
	readinessProbeTimeout = 10 * time.Second
	probeIMAP             = mail.ProbeIMAP
	probeSMTP             = mail.ProbeSMTP
)

type mailboxListCache struct {
	mu       sync.Mutex
	items    map[string]cachedMailboxList
	inflight map[string]*mailboxesInflight
}

type cachedMailboxList struct {
	value     []mail.Mailbox
	expiresAt time.Time
}

type mailboxesInflight struct {
	done  chan struct{}
	value []mail.Mailbox
	err   error
}

type threadResponseCache struct {
	mu       sync.Mutex
	items    map[string]cachedThreadResponse
	inflight map[string]*threadInflight
}

type cachedThreadResponse struct {
	value     threadResponse
	expiresAt time.Time
}

type threadInflight struct {
	done  chan struct{}
	value threadResponse
	err   error
}

type threadResponse struct {
	ThreadID         string
	Mailbox          string
	Scope            string
	MailboxesScanned []string
	Truncated        bool
	Items            []mail.MessageSummary
}

type readinessProbeCache struct {
	mu       sync.Mutex
	cfg      config.Config
	svc      *service.Service
	result   readinessProbeResult
	hasValue bool
	inflight chan struct{}
}

type readinessProbeResult struct {
	CheckedAt time.Time
	SQLiteErr error
	IMAPErr   error
	SMTPError error
}

func newMailboxListCache() *mailboxListCache {
	return &mailboxListCache{
		items:    map[string]cachedMailboxList{},
		inflight: map[string]*mailboxesInflight{},
	}
}

func newThreadResponseCache() *threadResponseCache {
	return &threadResponseCache{
		items:    map[string]cachedThreadResponse{},
		inflight: map[string]*threadInflight{},
	}
}

func newReadinessProbeCache(cfg config.Config, svc *service.Service) *readinessProbeCache {
	return &readinessProbeCache{cfg: cfg, svc: svc}
}

func (c *mailboxListCache) get(ctx context.Context, mailLogin string, fetch func(context.Context) ([]mail.Mailbox, error)) ([]mail.Mailbox, error) {
	key := normalizeMailLoginKey(mailLogin)
	now := time.Now().UTC()

	c.mu.Lock()
	if cached, ok := c.items[key]; ok && now.Before(cached.expiresAt) {
		value := cloneMailboxes(cached.value)
		c.mu.Unlock()
		return value, nil
	}
	if wait, ok := c.inflight[key]; ok {
		done := wait.done
		c.mu.Unlock()
		select {
		case <-done:
			return cloneMailboxes(wait.value), wait.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	wait := &mailboxesInflight{done: make(chan struct{})}
	c.inflight[key] = wait
	c.mu.Unlock()

	value, err := fetch(ctx)

	c.mu.Lock()
	wait.value = cloneMailboxes(value)
	wait.err = err
	if err == nil {
		c.items[key] = cachedMailboxList{
			value:     cloneMailboxes(value),
			expiresAt: now.Add(mailboxCacheTTL),
		}
	}
	delete(c.inflight, key)
	close(wait.done)
	c.mu.Unlock()

	return cloneMailboxes(value), err
}

func (c *mailboxListCache) invalidate(mailLogin string) {
	key := normalizeMailLoginKey(mailLogin)
	if key == "" {
		return
	}
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

func (c *threadResponseCache) get(ctx context.Context, mailLogin, scope, mailbox, threadID string, page, pageSize int, fetch func(context.Context) (threadResponse, error)) (threadResponse, error) {
	key := threadCacheKey(mailLogin, scope, mailbox, threadID, page, pageSize)
	now := time.Now().UTC()

	c.mu.Lock()
	if cached, ok := c.items[key]; ok && now.Before(cached.expiresAt) {
		value := cloneThreadResponse(cached.value)
		c.mu.Unlock()
		return value, nil
	}
	if wait, ok := c.inflight[key]; ok {
		done := wait.done
		c.mu.Unlock()
		select {
		case <-done:
			return cloneThreadResponse(wait.value), wait.err
		case <-ctx.Done():
			return threadResponse{}, ctx.Err()
		}
	}
	wait := &threadInflight{done: make(chan struct{})}
	c.inflight[key] = wait
	c.mu.Unlock()

	value, err := fetch(ctx)

	c.mu.Lock()
	wait.value = cloneThreadResponse(value)
	wait.err = err
	if err == nil {
		c.items[key] = cachedThreadResponse{
			value:     cloneThreadResponse(value),
			expiresAt: now.Add(threadCacheTTL),
		}
	}
	delete(c.inflight, key)
	close(wait.done)
	c.mu.Unlock()

	return cloneThreadResponse(value), err
}

func (c *threadResponseCache) invalidateMailLogin(mailLogin string) {
	prefix := normalizeMailLoginKey(mailLogin)
	if prefix == "" {
		return
	}
	prefix += "\x00"
	c.mu.Lock()
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
}

func (c *readinessProbeCache) prime() {
	ctx, cancel := context.WithTimeout(context.Background(), readinessProbeTimeout)
	defer cancel()
	_, _ = c.snapshot(ctx)
}

func (c *readinessProbeCache) snapshot(ctx context.Context) (readinessProbeResult, error) {
	now := time.Now().UTC()

	c.mu.Lock()
	if c.hasValue {
		result := c.result
		if now.Sub(result.CheckedAt) >= readinessProbeTTL && c.inflight == nil {
			wait := make(chan struct{})
			c.inflight = wait
			go c.refreshAsync(wait)
		}
		c.mu.Unlock()
		return result, nil
	}
	if c.inflight != nil {
		wait := c.inflight
		c.mu.Unlock()
		select {
		case <-wait:
			c.mu.Lock()
			result := c.result
			ok := c.hasValue
			c.mu.Unlock()
			if ok {
				return result, nil
			}
			return readinessProbeResult{}, nil
		case <-ctx.Done():
			return readinessProbeResult{}, ctx.Err()
		}
	}
	wait := make(chan struct{})
	c.inflight = wait
	c.mu.Unlock()

	result, err := c.runRefresh(ctx)

	c.mu.Lock()
	c.result = result
	c.hasValue = true
	c.inflight = nil
	close(wait)
	c.mu.Unlock()

	return result, err
}

func (c *readinessProbeCache) refreshAsync(wait chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), readinessProbeTimeout)
	defer cancel()
	result, _ := c.runRefresh(ctx)

	c.mu.Lock()
	if c.inflight == wait {
		c.result = result
		c.hasValue = true
		c.inflight = nil
		close(wait)
	}
	c.mu.Unlock()
}

func (c *readinessProbeCache) runRefresh(ctx context.Context) (readinessProbeResult, error) {
	if readinessProbeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, readinessProbeTimeout)
		defer cancel()
	}
	result := readinessProbeResult{CheckedAt: time.Now().UTC()}
	if _, err := c.svc.SetupStatus(ctx); err != nil {
		result.SQLiteErr = err
	}
	if err := probeIMAP(ctx, c.cfg); err != nil {
		result.IMAPErr = err
	}
	if err := probeSMTP(ctx, c.cfg); err != nil {
		result.SMTPError = err
	}
	return result, nil
}

func cloneMailboxes(in []mail.Mailbox) []mail.Mailbox {
	if len(in) == 0 {
		return []mail.Mailbox{}
	}
	out := make([]mail.Mailbox, len(in))
	copy(out, in)
	return out
}

func cloneThreadResponse(in threadResponse) threadResponse {
	return threadResponse{
		ThreadID:         in.ThreadID,
		Mailbox:          in.Mailbox,
		Scope:            in.Scope,
		MailboxesScanned: append([]string(nil), in.MailboxesScanned...),
		Truncated:        in.Truncated,
		Items:            cloneMessageSummaries(in.Items),
	}
}

func cloneMessageSummaries(in []mail.MessageSummary) []mail.MessageSummary {
	if len(in) == 0 {
		return []mail.MessageSummary{}
	}
	out := make([]mail.MessageSummary, len(in))
	copy(out, in)
	return out
}

func normalizeMailLoginKey(mailLogin string) string {
	return strings.ToLower(strings.TrimSpace(mailLogin))
}

func threadCacheKey(mailLogin, scope, mailbox, threadID string, page, pageSize int) string {
	parts := []string{
		normalizeMailLoginKey(mailLogin),
		strings.ToLower(strings.TrimSpace(scope)),
		strings.TrimSpace(mailbox),
		strings.TrimSpace(threadID),
		strconv.Itoa(page),
		strconv.Itoa(pageSize),
	}
	return strings.Join(parts, "\x00")
}

func (h *Handlers) rawMailboxes(ctx context.Context, mailLogin, pass string) ([]mail.Mailbox, error) {
	return h.mailboxCache.get(ctx, mailLogin, func(ctx context.Context) ([]mail.Mailbox, error) {
		return h.svc.Mail().ListMailboxes(ctx, mailLogin, pass)
	})
}

func (h *Handlers) invalidateMailCaches(mailLogin string) {
	h.mailboxCache.invalidate(mailLogin)
	h.threadCache.invalidateMailLogin(mailLogin)
}
