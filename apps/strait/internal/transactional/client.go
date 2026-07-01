package transactional

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 5 * time.Second

// Attachment is an email attachment encoded for the internal app endpoint.
type Attachment struct {
	Filename      string `json:"filename"`
	ContentBase64 string `json:"contentBase64"`
	ContentType   string `json:"contentType,omitempty"`
}

// Request is the contract Go uses to ask apps/app to render and send a
// transactional email.
type Request struct {
	Template       string         `json:"template"`
	To             []string       `json:"to"`
	From           string         `json:"from,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Props          map[string]any `json:"props"`
	Attachments    []Attachment   `json:"attachments,omitempty"`
}

// Response is the successful response from the internal transactional email API.
type Response struct {
	ID *string `json:"id"`
}

// Client calls the apps/app internal transactional email endpoint.
type Client struct {
	endpoint       string
	internalSecret string
	httpClient     *http.Client
	logger         *slog.Logger
}

// NewClient creates a transactional email client. It returns nil when appURL or
// internalSecret is empty so callers can keep email delivery optional.
func NewClient(appURL, internalSecret string, timeout time.Duration, logger *slog.Logger) *Client {
	appURL = strings.TrimSpace(appURL)
	internalSecret = strings.TrimSpace(internalSecret)
	if appURL == "" || internalSecret == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	endpoint, err := transactionalEmailEndpoint(appURL)
	if err != nil {
		logger.Warn("transactional email client disabled: invalid app internal URL", "url", appURL, "error", err)
		return nil
	}
	return &Client{
		endpoint:       endpoint,
		internalSecret: internalSecret,
		httpClient:     &http.Client{Timeout: timeout},
		logger:         logger,
	}
}

func transactionalEmailEndpoint(appURL string) (string, error) {
	u, err := url.Parse(appURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("missing scheme or host")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/internal/transactional-email"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// Send posts req to apps/app. Non-2xx responses are returned as errors.
func (c *Client) Send(ctx context.Context, req Request) error {
	if c == nil {
		return fmt.Errorf("transactional email client is not configured")
	}
	if len(req.To) == 0 {
		return fmt.Errorf("transactional email requires at least one recipient")
	}
	if strings.TrimSpace(req.Template) == "" {
		return fmt.Errorf("transactional email template is required")
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("transactional email idempotency key is required")
	}
	if req.Props == nil {
		req.Props = map[string]any{}
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal transactional email request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create transactional email request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Secret", c.internalSecret)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send transactional email request: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		return fmt.Errorf("read transactional email response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("transactional email endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out Response
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("decode transactional email response: %w", err)
	}
	return nil
}
