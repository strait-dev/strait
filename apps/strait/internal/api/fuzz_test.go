package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func FuzzUpdateWorkflowRequest(f *testing.F) {
	f.Add(`{"name":"new"}`)
	f.Add(`{"name":"new","breaking_change":true}`)
	f.Add(`{"name":"new","breaking_change":false}`)
	f.Add(`{"name":"new","breaking_change":null}`)
	f.Add(`{"breaking_change":true}`)
	f.Add(`{"breaking_change":"not-a-bool"}`)
	f.Add(`{"breaking_change":1}`)
	f.Add(`{"breaking_change":0}`)
	f.Add(`{}`)
	f.Add(`{"name":"","slug":"","enabled":false,"breaking_change":true}`)
	f.Add(`{"steps":[],"breaking_change":true}`)
	f.Add(`{"steps":null,"breaking_change":true}`)
	f.Add(`null`)
	f.Add(``)
	f.Add(`{invalid`)
	f.Add(`{"name":"\x00\xff","breaking_change":true}`)

	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{
				ID: id, Name: "old", Slug: "old", Enabled: true,
				VersionID: "v-prev", Version: 3,
			}, nil
		},
		UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
			return nil
		},
		ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
			return nil, nil
		},
		CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
			return nil
		},
		CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
			return 3, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}

	f.Fuzz(func(t *testing.T, body string) {
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", body))
		require.NotEqual(t, 0,
			w.Code)

		// Handler must never panic. Any status code is acceptable.

		// If 200, the response must be valid JSON.
		if w.Code == http.StatusOK {
			var resp map[string]any
			require.NoError(t, json.
				Unmarshal(w.Body.
					Bytes(), &resp))

		}
	})
}

func FuzzActiveVersionsResponse(f *testing.F) {
	f.Add("wf-1")
	f.Add("")
	f.Add(strings.Repeat("a", 1000))
	f.Add("wf-with-special-chars")
	f.Add("\x00")

	ms := &APIStoreMock{
		ListActiveWorkflowVersionsFunc: func(_ context.Context, _ string) ([]store.ActiveVersion, error) {
			return []store.ActiveVersion{
				{VersionID: "v-1", Version: 1, Pending: 0, Running: 2, Paused: 0, Total: 2},
			}, nil
		},
	}

	f.Fuzz(func(t *testing.T, workflowID string) {
		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/workflows/"+url.PathEscape(workflowID)+"/active-versions", ""))
		require.NotEqual(t, 0,
			w.Code)

		// Handler must never panic.

		// If 200, the response must be valid JSON with a versions array.
		if w.Code == http.StatusOK {
			var resp map[string]any
			require.NoError(t, json.
				Unmarshal(w.Body.
					Bytes(), &resp))

			if _, ok := resp["versions"]; !ok {
				require.Fail(t,

					"200 response missing versions field")
			}
		}
	})
}

func FuzzBreakingChangeDetectionLogic(f *testing.F) {
	// Fuzz the combination of version state and active run count
	// to ensure the breaking change detection never panics.
	f.Add("v-old", 2, 5, true)
	f.Add("v-old", 2, 0, true)
	f.Add("v-old", 2, 5, false)
	f.Add("", 0, 0, false)
	f.Add("", 0, 5, true)
	f.Add("v-1", 1, 1, true)
	f.Add("v-1", 0, 3, true) // version 0 with non-empty ID (edge case)
	f.Add("", 1, 3, true)    // version 1 with empty ID (edge case)
	f.Add("v-old", -1, 5, true)
	f.Add("v-old", 2, -1, true)

	f.Fuzz(func(t *testing.T, versionID string, version int, activeCount int, breakingChange bool) {
		var capturedAction string
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{
					ID: id, Name: "old", Slug: "old",
					VersionID: versionID, Version: version,
				}, nil
			},
			UpdateWorkflowFunc: func(_ context.Context, _ *domain.Workflow) error {
				return nil
			},
			ListStepsByWorkflowFunc: func(_ context.Context, _ string) ([]domain.WorkflowStep, error) {
				return nil, nil
			},
			CreateWorkflowVersionSnapshotFunc: func(_ context.Context, _ string, _ int) error {
				return nil
			},
			CountActiveWorkflowRunsByVersionFunc: func(_ context.Context, _, _ string) (int, error) {
				return activeCount, nil
			},
			CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
				capturedAction = ev.Action
				return nil
			},
		}

		body := `{"name":"updated"}`
		if breakingChange {
			body = `{"name":"updated","breaking_change":true}`
		}

		srv := newWorkflowTestServer(t, ms, &mockQueue{}, nil, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/workflows/wf-1", body))
		require.Equal(t, http.
			StatusOK, w.Code)

		// Must never panic and must return 200.

		// Verify invariants:
		var resp map[string]any
		require.NoError(t, json.
			Unmarshal(w.Body.
				Bytes(), &resp))

		hasCount := resp["active_runs_on_previous_version"] != nil

		// active_runs_on_previous_version should only appear when:
		// versionID != "" AND version >= 1 AND activeCount > 0
		shouldHaveCount := versionID != "" && version >= 1 && activeCount > 0
		require.Equal(t, shouldHaveCount,
			hasCount,
		)

		// Every successful workflow update emits an audit event. The action is
		// workflow.updated_breaking only when breaking_change=true AND the
		// version guard passes AND there are active runs on the previous
		// version; otherwise it is workflow.updated.
		shouldBeBreaking := breakingChange && versionID != "" && version >= 1 && activeCount > 0
		wantAction := "workflow.updated"
		if shouldBeBreaking {
			wantAction = "workflow.updated_breaking"
		}
		require.Equal(t, wantAction,
			capturedAction,
		)

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
				assert.False(t, job.RetryPriorityBoost <
					0 ||
					job.RetryPriorityBoost >
						10)

				// Verify invariant: if validation passed, boost must be in [1,10].
				// (0 is defaulted to 1 before reaching the store.)

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
			assert.Equal(t, http.StatusCreated,
				w.Code,
			)

		} else {
			assert.Equal(t, http.StatusUnprocessableEntity,

				w.Code)

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
