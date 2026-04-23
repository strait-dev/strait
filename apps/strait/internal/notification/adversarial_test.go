package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jarcoal/httpmock"
	"github.com/sourcegraph/conc"
)

// ---------------------------------------------------------------------------.
// Helpers
// ---------------------------------------------------------------------------.

// controllableNotificationStore implements store.NotificationStore with
// configurable behavior for adversarial testing of the Worker.
type controllableNotificationStore struct {
	stubNotificationStore

	claimFn   func(ctx context.Context, limit int, lease time.Duration) ([]domain.NotificationDelivery, error)
	getChanFn func(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error)
	updateFn  func(ctx context.Context, d *domain.NotificationDelivery) (bool, error)
}

func (s *controllableNotificationStore) ClaimPendingNotificationDeliveries(ctx context.Context, limit int, lease time.Duration) ([]domain.NotificationDelivery, error) {
	if s.claimFn != nil {
		return s.claimFn(ctx, limit, lease)
	}
	return nil, nil
}

func (s *controllableNotificationStore) GetNotificationChannel(ctx context.Context, id, projectID string) (*domain.NotificationChannel, error) {
	if s.getChanFn != nil {
		return s.getChanFn(ctx, id, projectID)
	}
	return nil, store.ErrNotificationChannelNotFound
}

func (s *controllableNotificationStore) UpdateClaimedNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) (bool, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, d)
	}
	return true, nil
}

// failingSender is a ChannelSender that always returns an error.
type failingSender struct {
	err error
}

func (f *failingSender) Send(_ context.Context, _ *domain.NotificationChannel, _ *domain.NotificationDelivery) error {
	return f.err
}

// ---------------------------------------------------------------------------.
// 1. Malformed notification payloads
// ---------------------------------------------------------------------------.

func TestWebhookSender_MalformedPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload json.RawMessage
	}{
		{"nil payload", nil},
		{"empty slice payload", json.RawMessage{}},
		{"invalid json", json.RawMessage(`{not valid}`)},
		{"just whitespace", json.RawMessage(`   `)},
		{"nested nulls", json.RawMessage(`{"a":null,"b":{"c":null}}`)},
		{"unicode escapes", json.RawMessage(`{"key":"\u0000\uFFFF"}`)},
		{"bare null", json.RawMessage(`null`)},
		{"bare string", json.RawMessage(`"just a string"`)},
		{"bare number", json.RawMessage(`42`)},
		{"bare boolean", json.RawMessage(`true`)},
		{"array payload", json.RawMessage(`[1,2,3]`)},
		{"deeply nested", json.RawMessage(`{"a":{"b":{"c":{"d":{"e":"deep"}}}}}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, transport := newMockClient(t)
			transport.RegisterResponder("POST", "https://example.com/hook",
				httpmock.NewStringResponder(200, "ok"))

			sender := NewWebhookSender(client)
			ch := newTestChannel("https://example.com/hook", "")
			del := newTestDelivery("test.event", tt.payload)

			// Must not panic regardless of payload content.
			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			_ = sender.Send(sendCtx, ch, del)
		})
	}
}

func TestSlackSender_MalformedConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config json.RawMessage
	}{
		{"nil config", nil},
		{"empty config", json.RawMessage(`{}`)},
		{"missing webhook_url", json.RawMessage(`{"other":"field"}`)},
		{"invalid json config", json.RawMessage(`not-json`)},
		{"null config", json.RawMessage(`null`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sender := NewSlackSender(&http.Client{})
			ch := &domain.NotificationChannel{
				ID:          "ch-test",
				ChannelType: domain.ChannelTypeSlack,
				Config:      tt.config,
			}
			del := newTestDelivery("test.event", json.RawMessage(`{}`))

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			err := sender.Send(sendCtx, ch, del)
			if err == nil {
				t.Fatal("expected error for malformed config")
			}
		})
	}
}

func TestDiscordSender_MalformedConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config json.RawMessage
	}{
		{"nil config", nil},
		{"empty config", json.RawMessage(`{}`)},
		{"missing webhook_url", json.RawMessage(`{"other":"field"}`)},
		{"invalid json config", json.RawMessage(`not-json`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sender := NewDiscordSender(&http.Client{})
			ch := &domain.NotificationChannel{
				ID:          "ch-test",
				ChannelType: domain.ChannelTypeDiscord,
				Config:      tt.config,
			}
			del := newTestDelivery("test.event", json.RawMessage(`{}`))

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			err := sender.Send(sendCtx, ch, del)
			if err == nil {
				t.Fatal("expected error for malformed config")
			}
		})
	}
}

func TestWebhookSender_SpecialCharacterPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{"null bytes", `{"data":"` + "\x00\x00\x00" + `"}`},
		{"emoji", `{"data":"` + string(rune(0x1F600)) + `"}`},
		{"backslashes", `{"path":"C:\\Users\\test"}`},
		{"html entities", `{"msg":"<b>&amp;&lt;&gt;</b>"}`},
		{"sql injection", `{"name":"'; DROP TABLE users;--"}`},
		{"very long key", `{"` + strings.Repeat("k", 10000) + `":"v"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, transport := newMockClient(t)
			var capturedBody []byte
			transport.RegisterResponder("POST", "https://example.com/hook",
				func(req *http.Request) (*http.Response, error) {
					capturedBody, _ = io.ReadAll(req.Body)
					return httpmock.NewStringResponse(200, "ok"), nil
				})

			sender := NewWebhookSender(client)
			ch := newTestChannel("https://example.com/hook", "test-secret-value")
			del := newTestDelivery("test.event", json.RawMessage(tt.payload))

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			err := sender.Send(sendCtx, ch, del)
			if err != nil {
				t.Fatalf("Send failed: %v", err)
			}

			// Payload must arrive verbatim.
			if string(capturedBody) != tt.payload {
				t.Errorf("body mismatch:\n  got:  %q\n  want: %q", capturedBody, tt.payload)
			}
		})
	}
}

// ---------------------------------------------------------------------------.
// 2. Delivery failures (worker errors, timeouts, partial delivery)
// ---------------------------------------------------------------------------.

func TestWorker_DispatchUnsupportedChannelType(t *testing.T) {
	t.Parallel()

	var updatedDelivery *domain.NotificationDelivery
	var claimCount atomic.Int32
	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 3,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: "unsupported_type",
				Config:      json.RawMessage(`{}`),
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			updatedDelivery = d
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	if updatedDelivery == nil {
		t.Fatal("expected delivery to be updated")
	}
	if updatedDelivery.Status != "failed" {
		t.Errorf("status = %q, want %q", updatedDelivery.Status, "failed")
	}
	if updatedDelivery.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", updatedDelivery.Attempts)
	}
	if !strings.Contains(updatedDelivery.LastError, "unsupported channel type") {
		t.Errorf("LastError = %q, want it to contain 'unsupported channel type'", updatedDelivery.LastError)
	}
}

func TestWorker_DispatchChannelNotFound(t *testing.T) {
	t.Parallel()

	var updatedDelivery *domain.NotificationDelivery
	var claimCount atomic.Int32
	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-missing",
				ProjectID:   "proj-1",
				MaxAttempts: 3,
				Payload:     json.RawMessage(`{}`),
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			return nil, store.ErrNotificationChannelNotFound
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			updatedDelivery = d
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	if updatedDelivery == nil {
		t.Fatal("expected delivery to be updated after channel-not-found")
	}
	if updatedDelivery.Status != "failed" {
		t.Errorf("status = %q, want %q", updatedDelivery.Status, "failed")
	}
	if updatedDelivery.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", updatedDelivery.Attempts)
	}
}

func TestWorker_DispatchSenderError_RetriesWithBackoff(t *testing.T) {
	t.Parallel()

	var updates []*domain.NotificationDelivery
	var mu sync.Mutex
	var claimCount atomic.Int32

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			n := claimCount.Add(1)
			if n > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 5,
				Attempts:    0,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			mu.Lock()
			cp := *d
			updates = append(updates, &cp)
			mu.Unlock()
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeWebhook, &failingSender{err: errors.New("connection refused")})
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	mu.Lock()
	defer mu.Unlock()

	if len(updates) == 0 {
		t.Fatal("expected at least one delivery update")
	}

	d := updates[0]
	if d.Status != "pending" {
		t.Errorf("status = %q, want %q (should retry)", d.Status, "pending")
	}
	if d.NextRetryAt == nil {
		t.Fatal("expected NextRetryAt to be set for retry")
	}
	if d.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", d.Attempts)
	}
}

func TestWorker_DispatchSenderError_ExhaustsMaxAttempts(t *testing.T) {
	t.Parallel()

	var updatedDelivery *domain.NotificationDelivery
	var claimCount atomic.Int32

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 3,
				Attempts:    2, // Already at 2, next attempt is the 3rd (final).
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			updatedDelivery = d
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeWebhook, &failingSender{err: errors.New("still failing")})
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	if updatedDelivery == nil {
		t.Fatal("expected delivery to be updated")
	}
	if updatedDelivery.Status != "failed" {
		t.Errorf("status = %q, want %q after exhausting retries", updatedDelivery.Status, "failed")
	}
	if updatedDelivery.NextRetryAt != nil {
		t.Error("NextRetryAt should be nil after exhausting retries")
	}
	if updatedDelivery.Attempts != 3 {
		t.Errorf("attempts = %d, want 3", updatedDelivery.Attempts)
	}
}

func TestWorker_LeaseLostDuringUpdate(t *testing.T) {
	t.Parallel()

	var claimCount atomic.Int32
	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 3,
				Attempts:    0,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfgBytes, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfgBytes,
			}, nil
		},
		updateFn: func(_ context.Context, _ *domain.NotificationDelivery) (bool, error) {
			// Simulate lease loss: update returns false.
			return false, nil
		},
	}

	client, transport := newMockClient(t)
	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(200, "ok"))

	w := NewWorker(st, client)
	// Should not panic when lease is lost.
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)
}

func TestWorker_UpdateClaimReturnsError(t *testing.T) {
	t.Parallel()

	var claimCount atomic.Int32
	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 3,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
			}, nil
		},
		updateFn: func(_ context.Context, _ *domain.NotificationDelivery) (bool, error) {
			return false, errors.New("database connection lost")
		},
	}

	client, transport := newMockClient(t)
	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(200, "ok"))

	w := NewWorker(st, client)
	// Should not panic on update error.
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)
}

func TestWorker_ClaimReturnsError(t *testing.T) {
	t.Parallel()

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			return nil, errors.New("database down")
		},
	}

	w := NewWorker(st, &http.Client{})
	// Should not panic; should log and return gracefully.
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)
}

func TestWorker_ContextCancelledDuringProcess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	st := &controllableNotificationStore{
		claimFn: func(ctx context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			// The claim function receives a cancelled context.
			return nil, ctx.Err()
		},
	}

	w := NewWorker(st, &http.Client{})
	// Must not hang or panic.
	w.process(ctx)
}

// ---------------------------------------------------------------------------.
// 3. Concurrent notification sending
// ---------------------------------------------------------------------------.

func TestWebhookSender_ConcurrentSends(t *testing.T) {
	t.Parallel()

	client, transport := newMockClient(t)
	var hits atomic.Int64
	transport.RegisterResponder("POST", "https://example.com/hook",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewWebhookSender(client)
	ch := newTestChannel("https://example.com/hook", "secret")

	const goroutines = 50
	var wg conc.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			del := newTestDelivery(
				fmt.Sprintf("event.%d", i),
				json.RawMessage(fmt.Sprintf(`{"idx":%d}`, i)),
			)
			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			_ = sender.Send(sendCtx, ch, del)
		})
	}
	wg.Wait()

	if hits.Load() != goroutines {
		t.Errorf("hits = %d, want %d", hits.Load(), goroutines)
	}
}

func TestSlackSender_ConcurrentSends(t *testing.T) {
	t.Parallel()

	client, transport := newMockClient(t)
	var hits atomic.Int64
	transport.RegisterResponder("POST", "https://hooks.slack.com/test",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewSlackSender(client)
	cfg, _ := json.Marshal(slackConfig{WebhookURL: "https://hooks.slack.com/test"})
	ch := &domain.NotificationChannel{
		ID:          "ch-1",
		ChannelType: domain.ChannelTypeSlack,
		Config:      cfg,
	}

	const goroutines = 50
	var wg conc.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			del := newTestDelivery("test.event", json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)))
			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			_ = sender.Send(sendCtx, ch, del)
		})
	}
	wg.Wait()

	if hits.Load() != goroutines {
		t.Errorf("hits = %d, want %d", hits.Load(), goroutines)
	}
}

func TestDiscordSender_ConcurrentSends(t *testing.T) {
	t.Parallel()

	client, transport := newMockClient(t)
	var hits atomic.Int64
	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/test",
		func(_ *http.Request) (*http.Response, error) {
			hits.Add(1)
			return httpmock.NewStringResponse(204, ""), nil
		})

	sender := NewDiscordSender(client)
	cfg, _ := json.Marshal(discordConfig{WebhookURL: "https://discord.com/api/webhooks/test"})
	ch := &domain.NotificationChannel{
		ID:          "ch-1",
		ChannelType: domain.ChannelTypeDiscord,
		Config:      cfg,
	}

	const goroutines = 50
	var wg conc.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			del := newTestDelivery("test.event", json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)))
			sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sendCancel()
			_ = sender.Send(sendCtx, ch, del)
		})
	}
	wg.Wait()

	if hits.Load() != goroutines {
		t.Errorf("hits = %d, want %d", hits.Load(), goroutines)
	}
}

// ---------------------------------------------------------------------------.
// 4. Retry exhaustion and backoff edge cases
// ---------------------------------------------------------------------------.

func TestWorker_BackoffCalculation(t *testing.T) {
	t.Parallel()

	// Test that backoff increases exponentially: 30s * 4^(attempt-1).
	tests := []struct {
		name            string
		currentAttempts int
		maxAttempts     int
		wantStatus      string
		wantMinBackoff  time.Duration
		wantMaxBackoff  time.Duration
	}{
		{
			name:            "first failure (attempt 0->1)",
			currentAttempts: 0,
			maxAttempts:     5,
			wantStatus:      "pending",
			wantMinBackoff:  25 * time.Second,
			wantMaxBackoff:  35 * time.Second,
		},
		{
			name:            "second failure (attempt 1->2)",
			currentAttempts: 1,
			maxAttempts:     5,
			wantStatus:      "pending",
			wantMinBackoff:  110 * time.Second,
			wantMaxBackoff:  130 * time.Second,
		},
		{
			name:            "third failure (attempt 2->3)",
			currentAttempts: 2,
			maxAttempts:     5,
			wantStatus:      "pending",
			wantMinBackoff:  450 * time.Second,
			wantMaxBackoff:  510 * time.Second,
		},
		{
			name:            "max attempts reached",
			currentAttempts: 4,
			maxAttempts:     5,
			wantStatus:      "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var updatedDelivery *domain.NotificationDelivery
			var claimCount atomic.Int32

			st := &controllableNotificationStore{
				claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
					if claimCount.Add(1) > 1 {
						return nil, nil
					}
					return []domain.NotificationDelivery{{
						ID:          "del-1",
						ChannelID:   "ch-1",
						ProjectID:   "proj-1",
						EventType:   "test.event",
						Payload:     json.RawMessage(`{}`),
						MaxAttempts: tt.maxAttempts,
						Attempts:    tt.currentAttempts,
					}}, nil
				},
				getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
					cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
					return &domain.NotificationChannel{
						ID:          "ch-1",
						ProjectID:   "proj-1",
						ChannelType: domain.ChannelTypeWebhook,
						Config:      cfg,
					}, nil
				},
				updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
					cp := *d
					updatedDelivery = &cp
					return true, nil
				},
			}

			w := NewWorker(st, &http.Client{})
			w.RegisterSender(domain.ChannelTypeWebhook, &failingSender{err: errors.New("timeout")})
			processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer processCancel()
			w.process(processCtx)

			if updatedDelivery == nil {
				t.Fatal("expected delivery to be updated")
			}
			if updatedDelivery.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", updatedDelivery.Status, tt.wantStatus)
			}

			if tt.wantStatus == "pending" {
				if updatedDelivery.NextRetryAt == nil {
					t.Fatal("expected NextRetryAt to be set")
				}
				backoff := time.Until(*updatedDelivery.NextRetryAt)
				if backoff < tt.wantMinBackoff || backoff > tt.wantMaxBackoff {
					t.Errorf("backoff = %v, want between %v and %v", backoff, tt.wantMinBackoff, tt.wantMaxBackoff)
				}
			}

			if tt.wantStatus == "failed" && updatedDelivery.NextRetryAt != nil {
				t.Error("NextRetryAt should be nil when status is failed")
			}
		})
	}
}

func TestWorker_ZeroMaxAttempts_FailsImmediately(t *testing.T) {
	t.Parallel()

	var updatedDelivery *domain.NotificationDelivery
	var claimCount atomic.Int32

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 0,
				Attempts:    0,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			updatedDelivery = d
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeWebhook, &failingSender{err: errors.New("fail")})
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	if updatedDelivery == nil {
		t.Fatal("expected delivery to be updated")
	}
	// With MaxAttempts=0, Attempts(1) >= MaxAttempts(0), so it should fail.
	if updatedDelivery.Status != "failed" {
		t.Errorf("status = %q, want %q for zero max attempts", updatedDelivery.Status, "failed")
	}
}

// ---------------------------------------------------------------------------.
// 5. Duplicate notification prevention (delivery deduplication at worker level)
// ---------------------------------------------------------------------------.

func TestWorker_SuccessfulDelivery_SetsDeliveredAt(t *testing.T) {
	t.Parallel()

	var updatedDelivery *domain.NotificationDelivery
	var claimCount atomic.Int32

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-1",
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 3,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			cp := *d
			updatedDelivery = &cp
			return true, nil
		},
	}

	client, transport := newMockClient(t)
	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(200, "ok"))

	w := NewWorker(st, client)
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	if updatedDelivery == nil {
		t.Fatal("expected delivery to be updated")
	}
	if updatedDelivery.Status != "delivered" {
		t.Errorf("status = %q, want %q", updatedDelivery.Status, "delivered")
	}
	if updatedDelivery.DeliveredAt == nil {
		t.Error("DeliveredAt should be set on successful delivery")
	}
	if updatedDelivery.LastError != "" {
		t.Errorf("LastError = %q, want empty on success", updatedDelivery.LastError)
	}
	if updatedDelivery.NextRetryAt != nil {
		t.Error("NextRetryAt should be nil on success")
	}
}

func TestWorker_ProcessesOneAtATime(t *testing.T) {
	t.Parallel()

	// Verify that the worker claims exactly 1 delivery at a time.
	var claimedLimits []int
	var mu sync.Mutex
	var callCount atomic.Int32

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, limit int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			mu.Lock()
			claimedLimits = append(claimedLimits, limit)
			mu.Unlock()
			if callCount.Add(1) > 3 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          fmt.Sprintf("del-%d", callCount.Load()),
				ChannelID:   "ch-1",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{}`),
				MaxAttempts: 3,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
			}, nil
		},
		updateFn: func(_ context.Context, _ *domain.NotificationDelivery) (bool, error) {
			return true, nil
		},
	}

	client, transport := newMockClient(t)
	transport.RegisterResponder("POST", "https://example.com/hook",
		httpmock.NewStringResponder(200, "ok"))

	w := NewWorker(st, client)
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)

	mu.Lock()
	defer mu.Unlock()

	for i, limit := range claimedLimits {
		if limit != 1 {
			t.Errorf("claim call %d: limit = %d, want 1", i, limit)
		}
	}
}

// ---------------------------------------------------------------------------.
// Email sender adversarial tests
// ---------------------------------------------------------------------------.

func TestEmailSender_XSSInPayloadFields(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "alerts@strait.dev")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":"user@example.com"}`),
	}

	// Inject XSS via payload fields that get rendered in HTML.
	payload, _ := json.Marshal(map[string]any{
		"org_id":             `<script>alert("xss")</script>`,
		"overage_pct":        80.0,
		"spending_limit_usd": 100.0,
		"current_spend_usd":  80.0,
	})

	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventSpendingLimitWarning,
		Payload:   payload,
	}

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	err := sender.Send(sendCtx, channel, delivery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}

	// The HTML body must escape the XSS attempt.
	body := mock.calls[0].Html
	if strings.Contains(body, "<script>") {
		t.Error("HTML body contains unescaped <script> tag -- XSS vulnerability")
	}
}

func TestEmailSender_InvalidPayloadJSON(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "alerts@strait.dev")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":"user@example.com"}`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventSpendingLimitWarning,
		Payload:   json.RawMessage(`{not valid json`),
	}

	// Should not panic; the email sender has a fallback for unparseable payloads.
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	err := sender.Send(sendCtx, channel, delivery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}

	// The fallback should still produce some HTML body.
	if mock.calls[0].Html == "" {
		t.Error("expected non-empty HTML body for invalid payload JSON")
	}
}

func TestEmailSender_NilPayload(t *testing.T) {
	t.Parallel()

	mock := &mockResendClient{}
	sender := NewEmailSenderWithClient(mock, "alerts@strait.dev")

	channel := &domain.NotificationChannel{
		Config: json.RawMessage(`{"to":"user@example.com"}`),
	}
	delivery := &domain.NotificationDelivery{
		EventType: domain.NotificationEventBudgetThreshold,
		Payload:   nil,
	}

	// Must not panic on nil payload.
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	err := sender.Send(sendCtx, channel, delivery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookSender_InvalidJSON_Config(t *testing.T) {
	t.Parallel()

	sender := NewWebhookSender(&http.Client{})
	ch := &domain.NotificationChannel{
		ID:     "ch-1",
		Config: json.RawMessage(`not-json-at-all`),
	}
	del := newTestDelivery("test.event", json.RawMessage(`{}`))

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sendCancel()
	err := sender.Send(sendCtx, ch, del)
	if err == nil {
		t.Fatal("expected error for invalid config JSON")
	}
}

// ---------------------------------------------------------------------------.
// Worker with concurrent Start/Stop
// ---------------------------------------------------------------------------.

func TestWorker_ConcurrentStartStop(t *testing.T) {
	t.Parallel()

	st := &stubNotificationStore{}
	w := NewWorker(st, &http.Client{})

	w.Start(t.Context())

	// Concurrently stop the worker from multiple goroutines.
	var wg conc.WaitGroup
	for range 20 {
		wg.Go(func() {
			w.Stop()
		})
	}
	wg.Wait()
}
