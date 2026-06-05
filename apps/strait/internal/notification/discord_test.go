package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

func newDiscordChannel(webhookURL string) *domain.NotificationChannel {
	cfg, _ := json.Marshal(discordConfig{WebhookURL: webhookURL})
	return &domain.NotificationChannel{
		ID:          "ch-discord-1",
		ChannelType: domain.ChannelTypeDiscord,
		Config:      cfg,
		Enabled:     true,
	}
}

func TestDiscordSender_Success(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewStringResponder(200, "ok"))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.completed", json.RawMessage(`{"run_id":"r-1"}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.NoError(t, err)
}

func TestDiscordSender_EmptyWebhookURL(t *testing.T) {
	t.Parallel()
	sender := NewDiscordSender(&http.Client{})
	ch := newDiscordChannel("")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}

func TestDiscordSender_InvalidConfig(t *testing.T) {
	t.Parallel()
	sender := NewDiscordSender(&http.Client{})
	ch := &domain.NotificationChannel{
		ID:     "ch-discord-bad",
		Config: json.RawMessage(`not-json`),
	}
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}

func TestDiscordSender_StatusBoundary200(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewStringResponder(200, "ok"))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.NoError(t, err)
}

func TestDiscordSender_StatusBoundary199(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewStringResponder(199, ""))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}

func TestDiscordSender_StatusBoundary299(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewStringResponder(299, ""))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.NoError(t, err)
}

func TestDiscordSender_StatusBoundary300(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewStringResponder(300, ""))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}

func TestDiscordSender_NilClient(t *testing.T) {
	t.Parallel()
	sender := NewDiscordSender(nil)
	require.NotNil(t, sender.
		client)
}

func TestDiscordSender_ContextCancellation(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		func(req *http.Request) (*http.Response, error) {
			if err := req.Context().Err(); err != nil {
				return nil, err
			}
			return httpmock.NewStringResponse(200, "ok"), nil
		})

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.completed", json.RawMessage(`{}`))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}

func TestDiscordSender_Non2xxStatus(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewStringResponder(400, "bad request"))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}

func TestDiscordSender_NetworkError(t *testing.T) {
	t.Parallel()
	client, transport := newMockClient(t)

	transport.RegisterResponder("POST", "https://discord.com/api/webhooks/123/abc",
		httpmock.NewErrorResponder(http.ErrHandlerTimeout))

	sender := NewDiscordSender(client)
	ch := newDiscordChannel("https://discord.com/api/webhooks/123/abc")
	del := newTestDelivery("run.failed", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.Send(ctx, ch, del)
	require.Error(t, err)
}
