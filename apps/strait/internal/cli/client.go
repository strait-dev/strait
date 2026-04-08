package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a thin HTTP client for the Strait API.
type Client struct {
	baseURL    string
	apiKey     string
	projectID  string
	httpClient *http.Client
}

// APIError is returned when the server responds with a non-2xx status.
type APIError struct {
	Status  int
	Code    string `json:"error"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("API error %d: %s", e.Status, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("API error %d: %s", e.Status, e.Code)
	}
	return fmt.Sprintf("API error %d", e.Status)
}

// NewClient creates a Client from the given profile.
func NewClient(p *Profile) *Client {
	return &Client{
		baseURL:   p.APIURL,
		apiKey:    p.APIKey,
		projectID: p.ProjectID,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// newRequest builds an authenticated HTTP request.
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// Do performs a JSON request. If out is non-nil the response body is decoded
// into it. Returns an *APIError for non-2xx responses.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := c.newRequest(ctx, method, path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		apiErr.Status = resp.StatusCode
		// Try to parse the huma error envelope: {"errors":[{"message":"..."}]} or {"error":"...","message":"..."}
		var humaEnvelope struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if jsonErr := json.Unmarshal(respBody, &humaEnvelope); jsonErr == nil {
			if len(humaEnvelope.Errors) > 0 {
				apiErr.Message = humaEnvelope.Errors[0].Message
			} else if humaEnvelope.Message != "" {
				apiErr.Message = humaEnvelope.Message
			}
			apiErr.Code = humaEnvelope.Error
		}
		return &apiErr
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Upload performs a raw PUT request with the given reader to a presigned URL.
// No Authorization header is sent — the presigned URL carries its own auth.
func (c *Client) Upload(ctx context.Context, presignedURL string, r io.Reader, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignedURL, r)
	if err != nil {
		return fmt.Errorf("build upload request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")

	uploadClient := &http.Client{Timeout: 30 * time.Minute} // uploads can be slow
	resp, err := uploadClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// Stream opens a long-lived GET request and returns the response body for the
// caller to read (e.g. for SSE). The caller is responsible for closing the body.
// No Timeout is set on the underlying client for streaming connections.
func (c *Client) Stream(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("build stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	streamClient := &http.Client{} // no timeout — stream lives until server closes it
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, &APIError{Status: resp.StatusCode}
	}
	return resp.Body, nil
}
