package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type RuntimeCallbackClient interface {
	Send(ctx context.Context, runID, runToken string, event RuntimeEvent) (bool, error)
}

type HTTPCallbackClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPCallbackClient(baseURL string, client *http.Client) *HTTPCallbackClient {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPCallbackClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (c *HTTPCallbackClient) Send(ctx context.Context, runID, runToken string, event RuntimeEvent) (bool, error) {
	path, body, terminal, err := c.eventRequest(event)
	if err != nil {
		return false, err
	}
	if err := c.postJSON(ctx, runID, runToken, path, body); err != nil {
		return false, err
	}
	return terminal, nil
}

func (c *HTTPCallbackClient) eventRequest(event RuntimeEvent) (string, any, bool, error) {
	switch event.Type {
	case RuntimeEventCheckpoint:
		return "/checkpoint", map[string]any{
			"source": "agents_runtime",
			"state":  rawMessageBody(event.State, `{}`),
		}, false, nil
	case RuntimeEventUsage:
		return "/usage", map[string]any{
			"provider":          event.Provider,
			"model":             event.Model,
			"prompt_tokens":     event.PromptTokens,
			"completion_tokens": event.CompletionTokens,
			"total_tokens":      event.TotalTokens,
			"cost_microusd":     event.CostMicrousd,
		}, false, nil
	case RuntimeEventToolCall:
		return "/tool-call", map[string]any{
			"tool_name":   event.ToolName,
			"input":       rawMessageBody(event.Input, `{}`),
			"output":      rawMessageBody(event.Output, `{}`),
			"duration_ms": event.DurationMs,
			"status":      event.Status,
		}, false, nil
	case RuntimeEventStream:
		streamID := event.StreamID
		if strings.TrimSpace(streamID) == "" {
			streamID = "default"
		}
		return "/stream", map[string]any{
			"chunk":     event.Chunk,
			"stream_id": streamID,
			"done":      event.Done,
		}, false, nil
	case RuntimeEventComplete:
		return "/complete", map[string]any{
			"result": rawMessageBody(event.Result, `null`),
		}, true, nil
	case RuntimeEventFail:
		return "/fail", map[string]any{
			"error": event.Error,
		}, true, nil
	default:
		return "", nil, false, fmt.Errorf("unsupported runtime callback event %q", event.Type)
	}
}

func (c *HTTPCallbackClient) postJSON(ctx context.Context, runID, runToken, path string, body any) error {
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("runtime callback base URL is not configured")
	}
	endpoint, err := url.JoinPath(c.baseURL, "sdk/v1/runs", runID, strings.TrimPrefix(path, "/"))
	if err != nil {
		return fmt.Errorf("build runtime callback URL: %w", err)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal runtime callback body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create runtime callback request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+runToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send runtime callback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("runtime callback %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
}

func rawMessageBody(raw json.RawMessage, fallback string) any {
	if len(raw) == 0 {
		return json.RawMessage(fallback)
	}
	return raw
}
