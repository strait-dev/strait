package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/getsentry/sentry-go"
)

type redactedHTTPDispatchError struct {
	message string
	err     error
}

func (e *redactedHTTPDispatchError) Error() string {
	return e.message
}

func (e *redactedHTTPDispatchError) Unwrap() error {
	return e.err
}

func (e *Executor) dispatchToEndpoint(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (json.RawMessage, error) {
	recordDispatchPayloadBytes(ctx, dispatchModeHTTP, len(run.Payload))
	req, err := newDispatchRequest(ctx, endpointURL, run, extraHeaders)
	if err != nil {
		return nil, err
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		if e.metrics != nil {
			e.metrics.DispatchErrors.Add(ctx, 1)
		}
		recordDispatchAttempt(ctx, dispatchModeHTTP, dispatchOutcomeError)
		return nil, &redactedHTTPDispatchError{
			message: "http dispatch: " + httputil.SanitizeHTTPClientError(err),
			err:     err,
		}
	}
	defer resp.Body.Close()
	return readDispatchResponse(ctx, resp)
}

func readDispatchResponse(ctx context.Context, resp *http.Response) (json.RawMessage, error) {
	recordDispatchResponseStatus(ctx, dispatchModeHTTP, resp.StatusCode)

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)-2))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		recordDispatchAttempt(ctx, dispatchModeHTTP, dispatchOutcomeError)
		return nil, &domain.EndpointError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	recordDispatchAttempt(ctx, dispatchModeHTTP, dispatchOutcomeSuccess)

	if len(respBody) == 0 {
		return nil, nil
	}
	return normalizeDispatchResult(respBody), nil
}

func newDispatchRequest(ctx context.Context, endpointURL string, run *domain.JobRun, extraHeaders map[string]string) (*http.Request, error) {
	var body io.Reader
	if len(run.Payload) > 0 {
		body = bytes.NewReader(run.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: invalid endpoint URL")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Run-ID", run.ID)
	req.Header.Set("X-Job-ID", run.JobID)
	req.Header.Set("X-Attempt", strconv.Itoa(run.Attempt))
	addRunTraceHeaders(req.Header, run.Metadata)

	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	return req, nil
}

func addRunTraceHeaders(headers http.Header, metadata map[string]string) {
	if tp, ok := metadata[domain.RunMetadataTraceParent]; ok && tp != "" {
		headers.Set("Traceparent", tp)
		if ts, ok := metadata[domain.RunMetadataTraceState]; ok && ts != "" {
			headers.Set("Tracestate", ts)
		}
	}
	if traceparent, ok := metadata[domain.RunMetadataSentryTrace]; ok && traceparent != "" {
		headers.Set(sentry.SentryTraceHeader, traceparent)
		if baggage, ok := metadata[domain.RunMetadataSentryBaggage]; ok && baggage != "" {
			headers.Set(sentry.SentryBaggageHeader, baggage)
		}
	}
}

func normalizeDispatchResult(body []byte) json.RawMessage {
	if json.Valid(body) {
		return json.RawMessage(body)
	}
	encoded, err := json.Marshal(string(body))
	if err != nil {
		return nil
	}
	return json.RawMessage(encoded)
}
