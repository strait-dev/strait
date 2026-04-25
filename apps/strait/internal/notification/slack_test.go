package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jarcoal/httpmock"
)

func newSlackChannel(webhookURL string) *domain.NotificationChannel {
	cfg, _ := json.Marshal(slackConfig{WebhookURL: webhookURL})
	return &domain.NotificationChannel{
		ID:          "ch-slack-1",
		ChannelType: domain.ChannelTypeSlack,
		Config:      cfg,
		Enabled:     true,
	}
}

func TestSlackSender_Success(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewStringResponder(200, "ok"))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.completed", json.RawMessage(`{"run_id":"r-1"}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestSlackSender_EmptyWebhookURL(t *testing.T) {
	t.Parallel()
	sender := NewSlackSender(&http.Client{})
	ch := newSlackChannel("")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for empty webhook URL")
	}
}

func TestSlackSender_InvalidConfig(t *testing.T) {
	t.Parallel()
	sender := NewSlackSender(&http.Client{})
	ch := &domain.NotificationChannel{
		ID:     "ch-slack-bad",
		Config: json.RawMessage(`not-json`),
	}
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for invalid config JSON")
	}
}

func TestSlackSender_StatusBoundary200(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewStringResponder(200, "ok"))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("status 200 should succeed, got: %v", err)
	}
}

func TestSlackSender_StatusBoundary199(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewStringResponder(199, ""))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("status 199 should be rejected")
	}
}

func TestSlackSender_StatusBoundary299(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewStringResponder(299, ""))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err != nil {
		t.Fatalf("status 299 should succeed, got: %v", err)
	}
}

func TestSlackSender_StatusBoundary300(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewStringResponder(300, ""))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("status 300 should be rejected")
	}
}

func TestSlackSender_NilClient(t *testing.T) {
	t.Parallel()
	sender := NewSlackSender(nil)
	if sender.client == nil {
		t.Fatal("expected default client when nil is passed")
	}
}

func TestSlackSender_ContextCancellation(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		func(req *http.Request) (*http.Response, error) {
			if err := req.Context().Err(); err != nil {
				return nil, err
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSlackSender_Non2xxStatus(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewStringResponder(400, "bad request"))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
}

func TestSlackSender_NetworkError(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://hooks.slack.com/services/T00/B00/xxx",
		httpmock.NewErrorResponder(http.ErrHandlerTimeout))

	sender := NewSlackSender(client)
	ch := newSlackChannel("https://hooks.slack.com/services/T00/B00/xxx")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}
