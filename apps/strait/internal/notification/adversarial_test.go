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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helpers

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

type recordingSender struct {
	calls atomic.Int32
}

func (r *recordingSender) Send(_ context.Context, _ *domain.NotificationChannel, _ *domain.NotificationDelivery) error {
	r.calls.Add(1)
	return nil
}

type ChannelSenderFunc func(context.Context, *domain.NotificationChannel, *domain.NotificationDelivery) error

func (f ChannelSenderFunc) Send(ctx context.Context, ch *domain.NotificationChannel, d *domain.NotificationDelivery) error {
	return f(ctx, ch, d)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// 1. Malformed notification payloads

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
			require.Error(t, err)
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
			require.Error(t, err)
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
			require.NoError(t, err)
			assert.Equal(t, tt.payload,
				string(capturedBody))

			// Payload must arrive verbatim.
		})
	}
}

// 2. Delivery failures (worker errors, timeouts, partial delivery)

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
				Enabled:     true,
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
	require.NotNil(t, updatedDelivery)
	assert.Equal(t, "failed",
		updatedDelivery.
			Status)
	assert.Equal(t, 1, updatedDelivery.
		Attempts,
	)
	assert.Contains(t, updatedDelivery.
		LastError, "unsupported channel type")
}

func TestWorker_RedactsSenderURLSecretsFromLastError(t *testing.T) {
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
				MaxAttempts: 1,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeSlack,
				Config:      json.RawMessage(`{"webhook_url":"https://hooks.slack.com/services/T00/B00/secret-token"}`),
				Enabled:     true,
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			clone := *d
			updatedDelivery = &clone
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeSlack, &failingSender{
		err: fmt.Errorf("Post \"https://hooks.slack.com/services/T00/B00/secret-token\": dial tcp failed"),
	})

	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)
	require.NotNil(t, updatedDelivery)
	require.False(t, strings.Contains(updatedDelivery.
		LastError,
		"secret-token",
	) || strings.Contains(updatedDelivery.
		LastError,
		"hooks.slack.com/services",
	))
	require.Contains(t, updatedDelivery.
		LastError, "[redacted-url]")
}

func TestSlackSender_RedactsWebhookURLInTransportError(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s", req.URL.String())
	})}
	sender := NewSlackSender(client)

	err := sender.Send(context.Background(),
		&domain.NotificationChannel{Config: json.RawMessage(`{"webhook_url":"https://hooks.slack.com/services/T00/B00/secret-token"}`)},
		&domain.NotificationDelivery{EventType: "test.event", Payload: json.RawMessage(`{}`)},
	)
	require.Error(t, err)

	msg := err.Error()
	require.False(t, strings.Contains(msg,
		"secret-token",
	) ||
		strings.Contains(msg, "hooks.slack.com/services"))
	require.Contains(t, msg, "[redacted-url]")
}

func TestDiscordSender_RedactsWebhookURLInTransportError(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s", req.URL.String())
	})}
	sender := NewDiscordSender(client)

	err := sender.Send(context.Background(),
		&domain.NotificationChannel{Config: json.RawMessage(`{"webhook_url":"https://discord.com/api/webhooks/123/secret-token"}`)},
		&domain.NotificationDelivery{EventType: "test.event", Payload: json.RawMessage(`{}`)},
	)
	require.Error(t, err)

	msg := err.Error()
	require.False(t, strings.Contains(msg,
		"secret-token",
	) ||
		strings.Contains(msg, "discord.com/api/webhooks"))
	require.Contains(t, msg, "[redacted-url]")
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
	require.NotNil(t, updatedDelivery)
	assert.Equal(t, "failed",
		updatedDelivery.
			Status)
	assert.Equal(t, 1, updatedDelivery.
		Attempts,
	)
}

func TestWorker_DispatchSkipsDisabledChannel(t *testing.T) {
	t.Parallel()

	var updatedDelivery *domain.NotificationDelivery
	var claimCount atomic.Int32
	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			if claimCount.Add(1) > 1 {
				return nil, nil
			}
			return []domain.NotificationDelivery{{
				ID:          "del-disabled",
				ChannelID:   "ch-disabled",
				ProjectID:   "proj-1",
				EventType:   "test.event",
				Payload:     json.RawMessage(`{"secret":"queued-before-disable"}`),
				MaxAttempts: 3,
			}}, nil
		},
		getChanFn: func(_ context.Context, _, _ string) (*domain.NotificationChannel, error) {
			return &domain.NotificationChannel{
				ID:          "ch-disabled",
				ProjectID:   "proj-1",
				ChannelType: domain.ChannelTypeWebhook,
				Config:      json.RawMessage(`{"url":"https://example.com/hook"}`),
				Enabled:     false,
			}, nil
		},
		updateFn: func(_ context.Context, d *domain.NotificationDelivery) (bool, error) {
			cp := *d
			updatedDelivery = &cp
			return true, nil
		},
	}
	sender := &recordingSender{}

	w := NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeWebhook, sender)
	processCtx, processCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer processCancel()
	w.process(processCtx)
	require.Equal(t, int32(0), sender.
		calls.Load(),
	)
	require.NotNil(t, updatedDelivery)
	require.Equal(t, "failed",
		updatedDelivery.
			Status,
	)
	require.Equal(t, 0, updatedDelivery.
		Attempts,
	)
	require.Nil(t,
		updatedDelivery.NextRetryAt,
	)
	require.Contains(t, updatedDelivery.
		LastError, "disabled")
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
				Enabled:     true,
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
	require.NotEmpty(t, updates)

	d := updates[0]
	assert.Equal(t, "pending",
		d.Status)
	require.NotNil(t, d.NextRetryAt)
	assert.Equal(t, 1, d.Attempts)
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
				Enabled:     true,
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
	require.NotNil(t, updatedDelivery)
	assert.Equal(t, "failed",
		updatedDelivery.
			Status)
	assert.Nil(t, updatedDelivery.
		NextRetryAt,
	)
	assert.Equal(t, 3, updatedDelivery.
		Attempts,
	)
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
				Enabled:     true,
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
				Enabled:     true,
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

// 3. Concurrent notification sending

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
	assert.Equal(t, int64(goroutines),
		hits.Load(),
	)
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
	assert.Equal(t, int64(goroutines),
		hits.Load(),
	)
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
	assert.Equal(t, int64(goroutines),
		hits.Load(),
	)
}

// 4. Retry exhaustion and backoff edge cases

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
						Enabled:     true,
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
			require.NotNil(t, updatedDelivery)
			assert.Equal(t, tt.wantStatus,
				updatedDelivery.
					Status,
			)

			if tt.wantStatus == "pending" {
				require.NotNil(t, updatedDelivery.
					NextRetryAt,
				)

				backoff := time.Until(*updatedDelivery.NextRetryAt)
				assert.False(t, backoff <
					tt.wantMinBackoff ||
					backoff >
						tt.wantMaxBackoff,
				)
			}
			assert.False(t, tt.wantStatus ==
				"failed" &&
				updatedDelivery.
					NextRetryAt !=
					nil)
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
				Enabled:     true,
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
	require.NotNil(t, updatedDelivery)
	assert.Equal(t, "failed",
		updatedDelivery.
			Status)

	// With MaxAttempts=0, Attempts(1) >= MaxAttempts(0), so it should fail.
}

// 5. Duplicate notification prevention (delivery deduplication at worker level)

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
				Enabled:     true,
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
	require.NotNil(t, updatedDelivery)
	assert.Equal(t, "delivered",
		updatedDelivery.
			Status,
	)
	assert.NotNil(t, updatedDelivery.
		DeliveredAt,
	)
	assert.Empty(t, updatedDelivery.
		LastError,
	)
	assert.Nil(t, updatedDelivery.
		NextRetryAt,
	)
}

func TestWorker_ClaimsBatches(t *testing.T) {
	t.Parallel()

	// Verify that the worker claims a bounded batch so one slow delivery does
	// not monopolize the global worker loop before unrelated deliveries are
	// even attempted.
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
				Enabled:     true,
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

	for _, limit := range claimedLimits {
		assert.Equal(t, notificationClaimBatchSize, limit)
	}
}

func TestWorker_DispatchesClaimedDeliveriesConcurrently(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	t.Parallel()

	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	fastSent := make(chan struct{})
	done := make(chan struct{})

	st := &controllableNotificationStore{
		claimFn: func(_ context.Context, _ int, _ time.Duration) ([]domain.NotificationDelivery, error) {
			return []domain.NotificationDelivery{
				{
					ID:          "del-slow",
					ChannelID:   "ch-1",
					ProjectID:   "proj-slow",
					EventType:   "test.event",
					Payload:     json.RawMessage(`{}`),
					MaxAttempts: 3,
				},
				{
					ID:          "del-fast",
					ChannelID:   "ch-1",
					ProjectID:   "proj-fast",
					EventType:   "test.event",
					Payload:     json.RawMessage(`{}`),
					MaxAttempts: 3,
				},
			}, nil
		},
		getChanFn: func(_ context.Context, _, projectID string) (*domain.NotificationChannel, error) {
			cfg, _ := json.Marshal(webhookConfig{URL: "https://example.com/hook"})
			return &domain.NotificationChannel{
				ID:          "ch-1",
				ProjectID:   projectID,
				ChannelType: domain.ChannelTypeWebhook,
				Config:      cfg,
				Enabled:     true,
			}, nil
		},
		updateFn: func(_ context.Context, _ *domain.NotificationDelivery) (bool, error) {
			return true, nil
		},
	}

	w := NewWorker(st, &http.Client{})
	w.RegisterSender(domain.ChannelTypeWebhook, ChannelSenderFunc(func(_ context.Context, _ *domain.NotificationChannel, d *domain.NotificationDelivery) error {
		switch d.ID {
		case "del-slow":
			close(slowStarted)
			<-releaseSlow
		case "del-fast":
			close(fastSent)
		}
		return nil
	}))
	concWG.Go(func() {
		defer close(done)
		w.dispatchBatch(context.Background(), []domain.NotificationDelivery{
			{ID: "del-slow", ChannelID: "ch-1", ProjectID: "proj-slow", MaxAttempts: 3},
			{ID: "del-fast", ChannelID: "ch-1", ProjectID: "proj-fast", MaxAttempts: 3},
		})
	})

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "slow delivery was not started")
	}
	select {
	case <-fastSent:
	case <-time.After(250 * time.Millisecond):
		require.FailNow(t, "fast delivery was blocked behind slow delivery")
	}
	close(releaseSlow)
	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "dispatch batch did not finish after slow delivery released")
	}
}

// Email sender adversarial tests

func TestEmailSender_XSSInPayloadFields(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{}
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
	require.NoError(t, err)
	require.Len(t, mock.calls,
		1)

	assert.Equal(t, "notification.spending_limit_warning", string(mock.calls[0].Template))
	props := transactionalPropsMap(t, mock.calls[0].Props)
	assert.Equal(t, `<script>alert("xss")</script>`, props["orgId"])
}

func TestEmailSender_InvalidPayloadJSON(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{}
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
	require.NoError(t, err)
	require.Len(t, mock.calls,
		1)
	assert.Equal(t, "notification.generic", string(mock.calls[0].Template))
	props := transactionalPropsMap(t, mock.calls[0].Props)
	assert.Equal(t, "{not valid json", props["payload"])
}

func TestEmailSender_NilPayload(t *testing.T) {
	t.Parallel()

	mock := &mockNotificationEmailClient{}
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
	require.NoError(t, err)
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
	require.Error(t, err)
}

// Worker with concurrent Start/Stop

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
