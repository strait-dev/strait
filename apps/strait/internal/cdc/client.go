package cdc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	stdpath "path"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"go.opentelemetry.io/otel"
)

// Client is an HTTP client for the Sequin Stream API.
type Client struct {
	httpClient     *http.Client
	baseURL        *url.URL
	consumerName   string
	databaseName   string
	apiToken       string
	retryPolicy    failsafe.Policy[*http.Response]
	circuitBreaker failsafe.Policy[*http.Response]
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithRetryPolicy sets a custom retry policy for the client.
func WithRetryPolicy(p failsafe.Policy[*http.Response]) ClientOption {
	return func(c *Client) { c.retryPolicy = p }
}

// WithCircuitBreaker sets a custom circuit breaker for the client.
func WithCircuitBreaker(p failsafe.Policy[*http.Response]) ClientOption {
	return func(c *Client) { c.circuitBreaker = p }
}

func WithDatabaseName(name string) ClientOption {
	return func(c *Client) {
		if name != "" {
			c.databaseName = name
		}
	}
}

// isServerErrorOrNetworkFailure returns true for 5xx status codes or network errors.
func isServerErrorOrNetworkFailure(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return resp.StatusCode >= 500
}

// NewClient creates a new Sequin Stream API client.
func NewClient(baseURL, consumerName, apiToken string, opts ...ClientOption) *Client {
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		parsedBaseURL = &url.URL{}
	}

	c := &Client{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		baseURL:      parsedBaseURL,
		consumerName: consumerName,
		databaseName: "strait-db",
		apiToken:     apiToken,
		retryPolicy: retrypolicy.NewBuilder[*http.Response]().
			WithMaxRetries(2).
			WithBackoff(time.Second, 4*time.Second).
			HandleIf(isServerErrorOrNetworkFailure).
			ReturnLastFailure().
			Build(),
		circuitBreaker: circuitbreaker.NewBuilder[*http.Response]().
			WithFailureThresholdPeriod(5, 60*time.Second).
			WithDelay(30 * time.Second).
			HandleIf(isServerErrorOrNetworkFailure).
			Build(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

type receiveRequest struct {
	BatchSize int `json:"batch_size"`
	WaitFor   int `json:"wait_for,omitempty"`
}

type receiveResponse struct {
	Data []json.RawMessage `json:"data"`
}

type ackRequest struct {
	AckIDs []string `json:"ack_ids"`
}

type sinkConsumerHealthResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Health struct {
		Status string `json:"status"`
	} `json:"health"`
}

// Receive pulls a batch of messages from the Sequin Stream.
func (c *Client) Receive(ctx context.Context, batchSize, waitForMs int) ([]Message, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.Receive")
	defer span.End()

	body := receiveRequest{BatchSize: batchSize}
	if waitForMs > 0 {
		body.WaitFor = waitForMs
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/receive", body)
	if err != nil {
		return nil, fmt.Errorf("receive request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var result receiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode receive response: %w", err)
	}

	messages := make([]Message, 0, len(result.Data))
	for i, raw := range result.Data {
		msg, err := decodeReceiveMessage(raw)
		if err != nil {
			return nil, fmt.Errorf("decode receive message %d: %w", i, err)
		}
		msg.Metadata.ConsumerName = c.consumerName
		messages = append(messages, msg)
	}

	return messages, nil
}

func decodeReceiveMessage(raw json.RawMessage) (Message, error) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Message{}, err
	}
	if msg.Metadata.TableName != "" || msg.Action != "" || len(msg.Record) > 0 {
		return msg, nil
	}

	var wrapped struct {
		AckID string `json:"ack_id"`
		Data  struct {
			Record   json.RawMessage `json:"record"`
			Changes  json.RawMessage `json:"changes,omitempty"`
			Action   Action          `json:"action"`
			Metadata Metadata        `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return Message{}, err
	}
	if wrapped.Data.Metadata.TableName == "" && wrapped.Data.Action == "" && len(wrapped.Data.Record) == 0 {
		return msg, nil
	}

	return Message{
		AckID:    wrapped.AckID,
		Record:   wrapped.Data.Record,
		Changes:  wrapped.Data.Changes,
		Action:   wrapped.Data.Action,
		Metadata: wrapped.Data.Metadata,
	}, nil
}

// Ack acknowledges successfully processed messages.
func (c *Client) Ack(ctx context.Context, ackIDs []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.Ack")
	defer span.End()

	if len(ackIDs) == 0 {
		return nil
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/ack", ackRequest{AckIDs: ackIDs})
	if err != nil {
		return fmt.Errorf("ack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	return nil
}

// Nack negatively acknowledges messages, making them available for redelivery.
func (c *Client) Nack(ctx context.Context, ackIDs []string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "cdc.Nack")
	defer span.End()

	if len(ackIDs) == 0 {
		return nil
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/nack", ackRequest{AckIDs: ackIDs})
	if err != nil {
		return fmt.Errorf("nack request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	return nil
}

// Health verifies the Sequin service is reachable.
func (c *Client) Health(ctx context.Context) error {
	endpoint := *c.baseURL
	endpoint.Path = stdpath.Join(endpoint.Path, "/health")
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return fmt.Errorf("invalid base url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sequin health request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("sequin health returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// SinkConsumerHealth verifies the configured pull consumer is active without
// leasing stream messages from Sequin.
func (c *Client) SinkConsumerHealth(ctx context.Context) error {
	endpoint := *c.baseURL
	endpoint.Path = stdpath.Join(endpoint.Path, "/api/sinks", c.consumerName)
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return fmt.Errorf("invalid base url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create sink consumer health request: %w", err)
	}
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sequin sink consumer health request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	var result sinkConsumerHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode sink consumer health response: %w", err)
	}
	if result.Name != "" && result.Name != c.consumerName {
		return fmt.Errorf("sequin sink consumer name mismatch: got %q, want %q", result.Name, c.consumerName)
	}
	if result.Status != "active" {
		return fmt.Errorf("sequin sink consumer %q is %s", c.consumerName, result.Status)
	}
	switch result.Health.Status {
	case "healthy", "waiting":
	default:
		return fmt.Errorf("sequin sink consumer %q health is %s", c.consumerName, result.Health.Status)
	}
	return nil
}

// doRequest sends an HTTP request to the Sequin Stream API.
func (c *Client) doRequest(ctx context.Context, method, endpointPath string, body any) (*http.Response, error) {
	var bodyData []byte
	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	endpoint := *c.baseURL
	endpoint.Path = stdpath.Join(endpoint.Path, "/api/http_pull_consumers", c.consumerName, endpointPath)
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid base url")
	}

	buildRequest := func() (*http.Request, error) {
		var bodyReader io.Reader
		if bodyData != nil {
			bodyReader = bytes.NewReader(bodyData)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiToken)
		}
		return req, nil
	}

	// failsafe-go evaluates policies outermost-first: circuit breaker should be
	// first (outermost) so it short-circuits before retry attempts are wasted.
	var policies []failsafe.Policy[*http.Response]
	if c.circuitBreaker != nil {
		policies = append(policies, c.circuitBreaker)
	}
	if c.retryPolicy != nil {
		policies = append(policies, c.retryPolicy)
	}
	if len(policies) > 0 {
		return failsafe.With[*http.Response](policies...).WithContext(ctx).GetWithExecution(func(exec failsafe.Execution[*http.Response]) (*http.Response, error) {
			// Drain and close body from previous retry attempt to release the connection.
			if prev := exec.LastResult(); prev != nil && prev.Body != nil {
				_, _ = io.Copy(io.Discard, prev.Body)
				_ = prev.Body.Close()
			}
			req, err := buildRequest()
			if err != nil {
				return nil, err
			}
			return c.httpClient.Do(req)
		})
	}

	req, err := buildRequest()
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// readError reads the error body from a non-200 response.
func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("sequin api error (status %d): %s", resp.StatusCode, string(body))
}

// EnsureConsumer checks if the named consumer exists and creates it if not.
// Idempotent and safe to call on every startup.
func (c *Client) EnsureConsumer(ctx context.Context, tables []string) error {
	// Probe: try a non-blocking receive to check if the consumer exists.
	// Sequin rejects batch_size=0, so use the smallest valid batch.
	_, err := c.Receive(ctx, 1, 0)
	if err == nil {
		return nil
	}

	// Create sink via Sequin management API.
	endpoint := *c.baseURL
	endpoint.Path = stdpath.Join(endpoint.Path, "/api/sinks")

	sinkBody := map[string]any{
		"name":        c.consumerName,
		"database":    c.databaseName,
		"source":      map[string]any{"include_tables": tables},
		"destination": map[string]any{"type": "sequin_stream"},
	}
	bodyData, _ := json.Marshal(sinkBody)

	req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(bodyData))
	if reqErr != nil {
		return fmt.Errorf("create ensure consumer request: %w", reqErr)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, doErr := c.httpClient.Do(req)
	if doErr != nil {
		return fmt.Errorf("create sequin sink: %w", doErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if (resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity) &&
			bytes.Contains(respBody, []byte("has already been taken")) {
			return c.waitForConsumerReady(ctx)
		}
		return fmt.Errorf("create sequin sink (status %d): %s", resp.StatusCode, respBody)
	}
	return c.waitForConsumerReady(ctx)
}

func (c *Client) waitForConsumerReady(ctx context.Context) error {
	var lastErr error
	for attempt := range 30 {
		_, err := c.Receive(ctx, 1, 0)
		if err == nil {
			return nil
		}
		lastErr = err

		delay := min(time.Duration(attempt+1)*250*time.Millisecond, time.Second)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for sequin consumer: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("wait for sequin consumer: %w", lastErr)
}
