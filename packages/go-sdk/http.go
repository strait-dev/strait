package strait

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// HTTPDoer abstracts the http.Client for testability.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RequestOptions describes the parameters for an API request.
type RequestOptions struct {
	Method        string
	Path          string
	Query         map[string]string
	Headers       map[string]string
	Body          any
	SuccessStatus int
}

func (c *Client) doRequest(ctx context.Context, opts RequestOptions, result any) error {
	reqURL, err := url.Parse(c.config.BaseURL + opts.Path)
	if err != nil {
		return &TransportError{Message: fmt.Sprintf("invalid URL: %s", err), Cause: err}
	}

	if len(opts.Query) > 0 {
		q := reqURL.Query()
		for k, v := range opts.Query {
			q.Set(k, v)
		}
		reqURL.RawQuery = q.Encode()
	}

	var bodyReader io.Reader
	if opts.Body != nil {
		data, err := json.Marshal(opts.Body)
		if err != nil {
			return &TransportError{Message: fmt.Sprintf("failed to marshal request body: %s", err), Cause: err}
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, reqURL.String(), bodyReader)
	if err != nil {
		return &TransportError{Message: fmt.Sprintf("failed to create request: %s", err), Cause: err}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", GetAuthorizationHeader(c.config.Auth))

	for k, v := range c.config.DefaultHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &TransportError{Message: fmt.Sprintf("request failed: %s", err), Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TransportError{Message: fmt.Sprintf("failed to read response body: %s", err), Cause: err}
	}

	successStatus := opts.SuccessStatus
	if successStatus == 0 {
		successStatus = http.StatusOK
	}

	if resp.StatusCode != successStatus && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		var errBody any
		_ = json.Unmarshal(respBody, &errBody)

		msg := fmt.Sprintf("HTTP %d: %s %s", resp.StatusCode, opts.Method, opts.Path)
		if errBody != nil {
			if m, ok := errBody.(map[string]any); ok {
				if errMsg, ok := m["message"].(string); ok {
					msg = errMsg
				}
			}
		}
		return MapHttpError(resp.StatusCode, msg, errBody)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return &DecodeError{
				Message: fmt.Sprintf("failed to decode response: %s", err),
				Body:    string(respBody),
				Cause:   err,
			}
		}
	}

	return nil
}
