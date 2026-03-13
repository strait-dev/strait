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

	"go.opentelemetry.io/otel"
)

// Client is an HTTP client for the Sequin Stream API.
type Client struct {
	httpClient   *http.Client
	baseURL      *url.URL
	consumerName string
	apiToken     string
}

// NewClient creates a new Sequin Stream API client.
func NewClient(baseURL, consumerName, apiToken string) *Client {
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		parsedBaseURL = &url.URL{}
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		baseURL:      parsedBaseURL,
		consumerName: consumerName,
		apiToken:     apiToken,
	}
}

type receiveRequest struct {
	BatchSize int `json:"batch_size"`
	WaitFor   int `json:"wait_for,omitempty"`
}

type receiveResponse struct {
	Data []Message `json:"data"`
}

type ackRequest struct {
	AckIDs []string `json:"ack_ids"`
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

	for i := range result.Data {
		result.Data[i].Metadata.ConsumerName = c.consumerName
	}

	return result.Data, nil
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

// doRequest sends an HTTP request to the Sequin Stream API.
func (c *Client) doRequest(ctx context.Context, method, endpointPath string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	endpoint := *c.baseURL
	endpoint.Path = stdpath.Join(endpoint.Path, "/api/http_pull_consumers", c.consumerName, endpointPath)
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid base url")
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	return c.httpClient.Do(req)
}

// readError reads the error body from a non-200 response.
func (c *Client) readError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("sequin api error (status %d): %s", resp.StatusCode, string(body))
}
