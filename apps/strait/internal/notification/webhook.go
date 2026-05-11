package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	straitcrypto "strait/internal/crypto"
	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
)

// WebhookSender sends notifications via generic HTTP webhooks with HMAC-SHA256 signing.
type WebhookSender struct {
	client                *http.Client
	retryPolicy           failsafe.Policy[*http.Response]
	allowPrivateEndpoints bool
}

// WebhookSenderOption configures a WebhookSender.
type WebhookSenderOption func(*WebhookSender)

// WithWebhookRetryPolicy overrides the default retry policy.
func WithWebhookRetryPolicy(p failsafe.Policy[*http.Response]) WebhookSenderOption {
	return func(w *WebhookSender) { w.retryPolicy = p }
}

// WithWebhookAllowPrivateEndpoints allows notification webhooks to target
// private endpoints. This should only be enabled for explicit private
// deployments or local development.
func WithWebhookAllowPrivateEndpoints(allow bool) WebhookSenderOption {
	return func(w *WebhookSender) { w.allowPrivateEndpoints = allow }
}

// NewWebhookSender creates a new WebhookSender with the given HTTP client.
func NewWebhookSender(client *http.Client, opts ...WebhookSenderOption) *WebhookSender {
	w := &WebhookSender{
		retryPolicy: retrypolicy.NewBuilder[*http.Response]().
			WithMaxRetries(2).
			WithBackoff(time.Second, 4*time.Second).
			HandleIf(func(resp *http.Response, err error) bool {
				if err != nil {
					return true
				}
				return resp.StatusCode == 429 || resp.StatusCode >= 500
			}).
			Build(),
	}

	for _, opt := range opts {
		opt(w)
	}

	if client == nil {
		client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: httputil.NewExternalTransport(w.allowPrivateEndpoints),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	w.client = client

	return w
}

type webhookConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func (w *WebhookSender) Send(ctx context.Context, channel *domain.NotificationChannel, delivery *domain.NotificationDelivery) error {
	var cfg webhookConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		return fmt.Errorf("parse webhook config: %w", err)
	}
	if cfg.URL == "" {
		return fmt.Errorf("webhook url is empty")
	}
	if err := httputil.ValidateExternalURL(cfg.URL); err != nil && !w.allowPrivateEndpoints {
		return fmt.Errorf("webhook url rejected: %w", err)
	}

	body := delivery.Payload
	if len(body) == 0 {
		body = []byte("{}")
	}

	doOnce := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Event-Type", delivery.EventType)

		if cfg.Secret != "" {
			timestamp := time.Now().UTC().Format(time.RFC3339)
			straitcrypto.SignWebhookRequest(req, []byte(cfg.Secret), body, delivery.ID, timestamp)
		}

		return w.client.Do(req)
	}

	var resp *http.Response
	var err error

	if w.retryPolicy != nil {
		resp, err = failsafe.With[*http.Response](w.retryPolicy).
			WithContext(ctx).
			GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
				// Drain and close body from previous retry attempt to release the connection.
				if prev := exec.LastResult(); prev != nil && prev.Body != nil {
					_, _ = io.Copy(io.Discard, prev.Body)
					_ = prev.Body.Close()
				}
				return doOnce()
			})
	} else {
		resp, err = doOnce()
	}

	if err != nil {
		return fmt.Errorf("send webhook notification: %s", sanitizeWebhookError(err))
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func sanitizeWebhookError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err != nil {
			var nested *url.Error
			if errors.As(urlErr.Err, &nested) {
				return sanitizeWebhookError(urlErr.Err)
			}
			return fmt.Sprintf("%s: %v", urlErr.Op, urlErr.Err)
		}
		return urlErr.Op
	}
	return err.Error()
}
