// Package client provides an HTTP client for the strait REST API.
// It handles authentication, JSON encoding/decoding, retry with exponential
// backoff for transient failures, and structured error responses.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base URL must be http(s)")
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: parsed.String(),
		apiKey:  strings.TrimSpace(apiKey),
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, query url.Values, body any, out any) error {
	return c.doJSONWithHeaders(ctx, method, endpoint, query, body, nil, out)
}

// paginatedResponse wraps the paginated API envelope.
type paginatedResponse struct {
	Data       json.RawMessage `json:"data"`
	NextCursor *string         `json:"next_cursor,omitempty"`
	HasMore    bool            `json:"has_more"`
}

// doListJSON performs a GET request and unwraps the paginated response envelope,
// decoding only the "data" field into out.
func (c *Client) doListJSON(ctx context.Context, endpoint string, query url.Values, out any) error {
	var envelope paginatedResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, query, nil, &envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, out)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, method, endpoint string, query url.Values, body any, headers map[string]string, out any) error {
	fullURL, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	fullURL.Path = path.Join(fullURL.Path, endpoint)
	if query != nil {
		fullURL.RawQuery = query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		var marshalErr error
		bodyBytes, marshalErr = json.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
	}

	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, reqErr := http.NewRequestWithContext(ctx, method, fullURL.String(), bodyReader)
		if reqErr != nil {
			return reqErr
		}
		req.Header.Set("Accept", "application/json")
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, doErr := c.http.Do(req)
		if doErr != nil {
			return doErr
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("request failed with status %d", resp.StatusCode)
			if attempt < maxRetries-1 {
				backoff := time.Duration(1<<uint(attempt)) * time.Second // 1s, 2s, 4s
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
			}
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode >= http.StatusBadRequest {
			errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			var apiErr map[string]any
			if err := json.Unmarshal(errBody, &apiErr); err == nil {
				if msg, ok := apiErr["error"].(string); ok && msg != "" {
					return fmt.Errorf("request failed (%d): %s", resp.StatusCode, msg)
				}
			}
			return fmt.Errorf("request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
		}

		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}

	return lastErr
}
