package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/golang-jwt/jwt/v5"
)

// safePathEscape escapes a fuzzed string so it is safe for use in a URL path.
// httptest.NewRequest panics on control characters (including \x00) in the URL.
func safePathEscape(s string) string {
	return url.PathEscape(s)
}

// newFuzzIsolationStore extends newIsolationStore with proper "not found" errors
// for unknown IDs, preventing nil-pointer dereferences in handlers.
func newFuzzIsolationStore() *APIStoreMock {
	ms := newIsolationStore()

	origGetJob := ms.GetJobFunc
	ms.GetJobFunc = func(ctx context.Context, id string) (*domain.Job, error) {
		job, err := origGetJob(ctx, id)
		if job == nil && err == nil {
			return nil, store.ErrJobNotFound
		}
		return job, err
	}

	origGetRun := ms.GetRunFunc
	ms.GetRunFunc = func(ctx context.Context, id string) (*domain.JobRun, error) {
		run, err := origGetRun(ctx, id)
		if run == nil && err == nil {
			return nil, store.ErrRunNotFound
		}
		return run, err
	}

	origGetWorkflow := ms.GetWorkflowFunc
	ms.GetWorkflowFunc = func(ctx context.Context, id string) (*domain.Workflow, error) {
		wf, err := origGetWorkflow(ctx, id)
		if wf == nil && err == nil {
			return nil, store.ErrWorkflowNotFound
		}
		return wf, err
	}

	origGetEnv := ms.GetEnvironmentFunc
	ms.GetEnvironmentFunc = func(ctx context.Context, id string, projectID string) (*domain.Environment, error) {
		env, err := origGetEnv(ctx, id, projectID)
		if env == nil && err == nil {
			return nil, store.ErrEnvironmentNotFound
		}
		return env, err
	}

	return ms
}

// 1. FuzzCrossProjectJobAccess
// Fuzz job IDs to ensure cross-project access always returns 404, never 200
// with data from a different project.

func FuzzCrossProjectJobAccess(f *testing.F) {
	f.Add("job-a")
	f.Add("job-b")
	f.Add("")
	f.Add("../../../etc/passwd")
	f.Add("job-a%00")
	f.Add("nonexistent-job")

	f.Fuzz(func(t *testing.T, jobID string) {
		ms := newFuzzIsolationStore()
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/jobs/"+safePathEscape(jobID), "", projectB))
		if w.Code == http.StatusOK {
			var job domain.Job
			if json.Unmarshal(w.Body.Bytes(), &job) == nil && job.ProjectID == projectA {
				t.Errorf("cross-project access: got project A job %q via project B", job.ID)
			}
		}
	})
}

// 2. FuzzCrossProjectRunAccess
// Fuzz run IDs to ensure cross-project access always returns 404.

func FuzzCrossProjectRunAccess(f *testing.F) {
	f.Add("run-a")
	f.Add("run-b")
	f.Add("")
	f.Add("../run-a")
	f.Add("run-a%00trailing")

	f.Fuzz(func(t *testing.T, runID string) {
		ms := newFuzzIsolationStore()
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/runs/"+safePathEscape(runID), "", projectB))
		if w.Code == http.StatusOK {
			var run domain.JobRun
			if json.Unmarshal(w.Body.Bytes(), &run) == nil && run.ProjectID == projectA {
				t.Errorf("cross-project access: got project A run %q via project B", run.ID)
			}
		}
	})
}

// 3. FuzzCrossProjectWorkflowAccess
// Fuzz workflow IDs to ensure cross-project access always returns 404.

func FuzzCrossProjectWorkflowAccess(f *testing.F) {
	f.Add("wf-a")
	f.Add("wf-b")
	f.Add("")
	f.Add("wf-a/../wf-b")
	f.Add("%00wf-a")

	f.Fuzz(func(t *testing.T, workflowID string) {
		ms := newFuzzIsolationStore()
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/workflows/"+safePathEscape(workflowID), "", projectB))
		if w.Code == http.StatusOK {
			var wf domain.Workflow
			if json.Unmarshal(w.Body.Bytes(), &wf) == nil && wf.ProjectID == projectA {
				t.Errorf("cross-project access: got project A workflow %q via project B", wf.ID)
			}
		}
	})
}

// 4. FuzzCrossProjectEnvironmentAccess
// Fuzz environment IDs to ensure cross-project access always returns 404.

func FuzzCrossProjectEnvironmentAccess(f *testing.F) {
	f.Add("env-a")
	f.Add("env-b")
	f.Add("")
	f.Add("env-a%00")
	f.Add("../env-a")

	f.Fuzz(func(t *testing.T, envID string) {
		ms := newFuzzIsolationStore()
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, requestForProject(http.MethodGet, "/v1/environments/"+safePathEscape(envID), "", projectB))
		if w.Code == http.StatusOK {
			var env domain.Environment
			if json.Unmarshal(w.Body.Bytes(), &env) == nil && env.ProjectID == projectA {
				t.Errorf("cross-project access: got project A environment %q via project B", env.ID)
			}
		}
	})
}

// 5. FuzzSDKTokenRunIDMismatch
// Generate valid JWTs with random run IDs, then try to use them against a
// different run ID endpoint. Should always get 403.

func FuzzSDKTokenRunIDMismatch(f *testing.F) {
	f.Add("run-x", "run-y")
	f.Add("run-a", "run-b")
	f.Add("", "run-b")
	f.Add("run-a-extra", "run-a")

	f.Fuzz(func(t *testing.T, tokenRunID, pathRunID string) {
		if tokenRunID == pathRunID {
			return // same run ID is not a mismatch test
		}
		if pathRunID == "" || tokenRunID == "" {
			return // empty IDs cause routing ambiguity, not a security issue
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
			Subject:   tokenRunID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		})
		signed, err := token.SignedString([]byte(testJWTSigningKey))
		if err != nil {
			t.Skip("could not sign token")
		}

		ms := newFuzzIsolationStore()
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/sdk/v1/runs/"+safePathEscape(pathRunID)+"/heartbeat", nil)
		req.Header.Set("Authorization", "Bearer "+signed)
		req.Header.Set("Content-Type", "application/json")
		srv.ServeHTTP(w, req)

		if w.Code == http.StatusOK {
			t.Errorf("mismatched token (sub=%q) used on run %q returned 200", tokenRunID, pathRunID)
		}
	})
}

// 6. FuzzURLValidationErrorCasing
// Fuzz validateURL with various URLs and verify the error message never
// contains the malformed "uRL" casing.

func FuzzURLValidationErrorCasing(f *testing.F) {
	f.Add("http://localhost/secret")
	f.Add("http://127.0.0.1:8080/path")
	f.Add("http://[::1]/path")
	f.Add("http://metadata.google.internal/")
	f.Add("ftp://invalid.com")
	f.Add("")
	f.Add("not-a-url")
	f.Add("http://10.0.0.1/internal")
	f.Add("https://example.com")

	f.Fuzz(func(t *testing.T, rawURL string) {
		err := validateURL(rawURL)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "uRL") {
				t.Errorf("error message contains malformed casing 'uRL': %s", msg)
			}
		}
	})
}

// 7. FuzzNullByteStripping
// Fuzz stripNullBytesFromStruct with structs containing strings with
// embedded null bytes. Verify output never contains \x00.

func FuzzNullByteStripping(f *testing.F) {
	f.Add("hello\x00world", "test\x00data")
	f.Add("", "")
	f.Add("\x00", "\x00\x00\x00")
	f.Add("clean", "also clean")
	f.Add("mixed\x00\x00end", "\x00start")

	type testStruct struct {
		Name  string
		Value string
	}

	f.Fuzz(func(t *testing.T, name, value string) {
		s := testStruct{Name: name, Value: value}
		v := reflect.ValueOf(&s).Elem()
		stripNullBytesFromStruct(v)

		if strings.ContainsRune(s.Name, 0) {
			t.Errorf("Name still contains null byte after stripping: %q", s.Name)
		}
		if strings.ContainsRune(s.Value, 0) {
			t.Errorf("Value still contains null byte after stripping: %q", s.Value)
		}
	})
}

// 8. FuzzCronFieldCount
// Fuzz validateCronFieldCount with random strings. Verify it only accepts
// exactly 5 field expressions (the parser does not support seconds).

func FuzzCronFieldCount(f *testing.F) {
	f.Add("* * * * *")
	f.Add("0 * * * * *")
	f.Add("* * * *")
	f.Add("* * * * * * *")
	f.Add("")
	f.Add("*/5 * * * *")
	f.Add("0 0 1 1 *")
	f.Add("   ")
	f.Add("\t\n")
	f.Add("a b c d e f g h i j")

	f.Fuzz(func(t *testing.T, expr string) {
		err := validateCronFieldCount(expr)
		fields := strings.Fields(expr)
		fieldCount := len(fields)

		if fieldCount == 5 {
			if err != nil {
				t.Errorf("valid field count %d rejected: %v", fieldCount, err)
			}
		} else {
			if err == nil {
				t.Errorf("invalid field count %d accepted for expr: %q", fieldCount, expr)
			}
		}
	})
}

// 9. FuzzWebhookEventTypes
// Fuzz webhook event type validation with random strings. Ensure invalid
// types are always rejected.

func FuzzWebhookEventTypes(f *testing.F) {
	f.Add("run.completed")
	f.Add("run.failed")
	f.Add("run.timed_out")
	f.Add("run.canceled")
	f.Add("workflow.completed")
	f.Add("workflow.failed")
	f.Add("compute_budget_warning")
	f.Add("slo.budget_warning")
	f.Add("")
	f.Add("invalid.type")
	f.Add("run.completed; DROP TABLE")
	f.Add("RUN.COMPLETED")

	f.Fuzz(func(t *testing.T, eventType string) {
		isValid := validWebhookEventTypes[eventType]

		knownTypes := map[string]bool{
			domain.WebhookEventRunCompleted:      true,
			domain.WebhookEventRunFailed:         true,
			domain.WebhookEventRunTimedOut:       true,
			domain.WebhookEventRunCanceled:       true,
			domain.WebhookEventWorkflowCompleted: true,
			domain.WebhookEventWorkflowFailed:    true,
			domain.WebhookEventSLOBudgetWarning:  true,
		}

		if isValid && !knownTypes[eventType] {
			t.Errorf("unknown event type %q was accepted", eventType)
		}
		if !isValid && knownTypes[eventType] {
			t.Errorf("known event type %q was rejected", eventType)
		}
	})
}

// 10. FuzzTriggerScheduledAt
// Fuzz the scheduled_at validation with random time strings. Ensure past
// dates and dates >30 days out are rejected.

func FuzzTriggerScheduledAt(f *testing.F) {
	f.Add("2020-01-01T00:00:00Z")
	f.Add("2099-01-01T00:00:00Z")
	f.Add("")
	f.Add("not-a-time")
	f.Add("2024-06-15T12:00:00Z")
	f.Add("2024-06-15T12:00:00+05:00")

	f.Fuzz(func(t *testing.T, scheduledAtStr string) {
		parsed, parseErr := time.Parse(time.RFC3339, scheduledAtStr)
		if parseErr != nil {
			return // skip unparseable times -- the JSON decoder would reject them
		}

		ms := newFuzzIsolationStore()
		ms.GetJobFunc = func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:        id,
				ProjectID: projectA,
				Name:      "Test Job",
				Slug:      "test-job",
				Enabled:   true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		}
		ms.CreateRunFunc = func(_ context.Context, _ *domain.JobRun) error {
			return nil
		}
		ms.GetProjectQuotaFunc = func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return nil, nil
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		body, _ := json.Marshal(map[string]any{
			"payload":      map[string]string{"key": "val"},
			"scheduled_at": parsed.Format(time.RFC3339Nano),
		})

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, requestForProject(http.MethodPost, "/v1/jobs/test-job/trigger", string(body), projectA))

		now := time.Now()
		delay := time.Until(parsed)

		if delay < 0 && w.Code == http.StatusCreated {
			t.Errorf("past scheduled_at %v was accepted (status %d)", parsed, w.Code)
		}
		if delay > 30*24*time.Hour && w.Code == http.StatusCreated {
			t.Errorf("scheduled_at %v (>30 days from %v) was accepted", parsed, now)
		}
	})
}

// 11. FuzzProjectMatchHelper
// Fuzz requireProjectMatch with random project IDs. Ensure it never returns
// nil when IDs differ and context has a project.

func FuzzProjectMatchHelper(f *testing.F) {
	f.Add("proj-aaa", "proj-bbb")
	f.Add("proj-aaa", "proj-aaa")
	f.Add("", "proj-aaa")
	f.Add("proj-aaa", "")
	f.Add("", "")
	f.Add("proj-a-extra", "proj-a")
	f.Add("PROJ-AAA", "proj-aaa")

	f.Fuzz(func(t *testing.T, ctxProjectID, resourceProjectID string) {
		ctx := context.Background()
		if ctxProjectID != "" {
			ctx = context.WithValue(ctx, ctxProjectIDKey, ctxProjectID)
		}

		err := requireProjectMatch(ctx, resourceProjectID)

		if ctxProjectID != "" && ctxProjectID != resourceProjectID && err == nil {
			t.Errorf("requireProjectMatch returned nil for ctx=%q resource=%q", ctxProjectID, resourceProjectID)
		}
		if ctxProjectID == "" && err != nil {
			t.Errorf("requireProjectMatch returned error for empty context project: %v", err)
		}
		if ctxProjectID == resourceProjectID && err != nil {
			t.Errorf("requireProjectMatch returned error for matching projects: %v", err)
		}
	})
}

// 12. FuzzNullByteReader
// Fuzz the nullByteStrippingReader with random byte sequences. Ensure
// output never contains \x00.

func FuzzNullByteReader(f *testing.F) {
	f.Add([]byte("hello\x00world"))
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte("clean data"))
	f.Add([]byte(""))
	f.Add([]byte{0, 1, 2, 3, 0, 255, 0})
	f.Add([]byte("mixed\x00content\x00here"))

	f.Fuzz(func(t *testing.T, input []byte) {
		reader := &nullByteStrippingReader{r: bytes.NewReader(input)}
		output, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("unexpected read error: %v", err)
		}
		if bytes.ContainsRune(output, 0) {
			t.Errorf("output still contains null byte: %q", output)
		}
		// Verify length is preserved (null bytes become spaces, not removed).
		if len(output) != len(input) {
			t.Errorf("length changed: input=%d output=%d", len(input), len(output))
		}
	})
}
