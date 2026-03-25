package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func FuzzDecodeJSON(f *testing.F) {
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"nested":{"a":1}}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`123`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(``))
	f.Add([]byte(`{invalid`))
	f.Add([]byte(`{"a":"\u0000"}`))
	f.Add([]byte(`{"emoji":"😀"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")

		var target map[string]any
		// decodeJSON should never panic regardless of input
		srv := &Server{maxRequestBodySize: 1048576}
		_ = srv.decodeJSON(r, &target)
	})
}

func FuzzValidatePayloadAgainstSchema(f *testing.F) {
	f.Add(
		[]byte(`{"name":"test","count":1}`),
		[]byte(`{"type":"object","properties":{"name":{"type":"string"},"count":{"type":"number"}},"required":["name"]}`),
	)
	f.Add(
		[]byte(`{"items":[1,2,3]}`),
		[]byte(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"number"}}}}`),
	)
	f.Add([]byte(`{}`), []byte(`{}`))
	f.Add([]byte(``), []byte(``))
	f.Add([]byte(`null`), []byte(`{"type":"null"}`))
	f.Add([]byte(`"string"`), []byte(`{"type":"string"}`))
	f.Add([]byte(`42`), []byte(`{"type":"number"}`))
	f.Add([]byte(`true`), []byte(`{"type":"boolean"}`))
	f.Add([]byte(`[1,2]`), []byte(`{"type":"array"}`))

	f.Fuzz(func(t *testing.T, payload, schema []byte) {
		// validatePayloadAgainstSchema should never panic regardless of input
		_ = validatePayloadAgainstSchema(json.RawMessage(payload), json.RawMessage(schema))
	})
}

func FuzzValidateJobName(f *testing.F) {
	f.Add("my-job")
	f.Add("")
	f.Add(strings.Repeat("a", 300))
	f.Add("job with spaces")
	f.Add("\x00\xff")

	f.Fuzz(func(t *testing.T, name string) {
		// validateJobName should never panic regardless of input.
		_ = validateJobName(name)
	})
}

func FuzzValidateJobSlug(f *testing.F) {
	f.Add("my-job-slug")
	f.Add("")
	f.Add(strings.Repeat("x", 200))
	f.Add("slug/with/slashes")
	f.Add("\t\n")

	f.Fuzz(func(t *testing.T, slug string) {
		// validateJobSlug should never panic regardless of input.
		_ = validateJobSlug(slug)
	})
}

func FuzzValidateURL(f *testing.F) {
	f.Add("https://example.com/webhook")
	f.Add("http://localhost/secret")
	f.Add("ftp://invalid.com")
	f.Add("")
	f.Add("not-a-url")
	f.Add("http://127.0.0.1:8080/path")
	f.Add("http://[::1]/path")
	f.Add("http://metadata.google.internal/")

	f.Fuzz(func(t *testing.T, rawURL string) {
		// validateURL should never panic regardless of input.
		_ = validateURL(rawURL)
	})
}

func FuzzParsePaginationFromStrings(f *testing.F) {
	f.Add("10", "")
	f.Add("", "")
	f.Add("abc", "")
	f.Add("-1", "")
	f.Add("0", "")
	f.Add("10", "2024-01-01T00:00:00Z")
	f.Add("10", "not-a-time")
	f.Add("999999", "")
	f.Add("10", "2024-01-01T00:00:00.123456789Z")

	f.Fuzz(func(t *testing.T, limitStr, cursorStr string) {
		// parsePaginationFromStrings should never panic regardless of input.
		_, _, _ = parsePaginationFromStrings(limitStr, cursorStr)
	})
}

func FuzzValidateRetryConfig(f *testing.F) {
	f.Add("exponential", 3)
	f.Add("linear", 1)
	f.Add("fixed", 5)
	f.Add("custom", 10)
	f.Add("", 0)
	f.Add("invalid", -1)
	f.Add("exponential", 0)
	f.Add("EXPONENTIAL", 1)

	f.Fuzz(func(t *testing.T, strategy string, delay int) {
		// validateRetryConfig should never panic regardless of input.
		_ = validateRetryConfig(strategy, []int{delay})
	})
}

func FuzzCreateJobRetryPriorityBoost(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(5)
	f.Add(10)
	f.Add(11)
	f.Add(-1)
	f.Add(-100)
	f.Add(100)
	f.Add(1<<31 - 1)
	f.Add(-(1 << 31))

	f.Fuzz(func(t *testing.T, boost int) {
		ms := &APIStoreMock{
			CreateJobFunc: func(_ context.Context, job *domain.Job) error {
				// Verify invariant: if validation passed, boost must be in [1,10].
				// (0 is defaulted to 1 before reaching the store.)
				if job.RetryPriorityBoost < 0 || job.RetryPriorityBoost > 10 {
					t.Errorf("invalid retry_priority_boost %d reached store", job.RetryPriorityBoost)
				}
				job.ID = "job-fuzz"
				job.Version = 1
				return nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		body := fmt.Sprintf(`{
			"project_id": "proj-1",
			"name": "Fuzz Job",
			"slug": "fuzz-job-%d",
			"endpoint_url": "https://example.com/cb",
			"retry_priority_boost": %d
		}`, boost, boost)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		// Must never panic. Valid range [0,10] should succeed, others should 400.
		if boost >= 0 && boost <= 10 {
			if w.Code != http.StatusCreated {
				t.Errorf("boost=%d: expected 201, got %d", boost, w.Code)
			}
		} else {
			if w.Code != http.StatusBadRequest {
				t.Errorf("boost=%d: expected 400, got %d", boost, w.Code)
			}
		}
	})
}

func FuzzValidateTags(f *testing.F) {
	f.Add("env", "production")
	f.Add("", "value")
	f.Add("key", "")
	f.Add(strings.Repeat("k", 100), strings.Repeat("v", 300))
	f.Add("normal-key", "normal-value")

	f.Fuzz(func(t *testing.T, key, value string) {
		tags := map[string]string{key: value}
		// validateTags should never panic regardless of input.
		_ = validateTags(tags)
	})
}
