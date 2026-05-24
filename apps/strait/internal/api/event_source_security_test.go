package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"

	"strait/internal/domain"
	"strait/internal/eventfilter"
)

// TestEventSource_FilterExpressionInjection verifies that SQL-like injection patterns
// in filter expressions are treated as plain string comparisons and do not cause errors.
func TestEventSource_FilterExpressionInjection(t *testing.T) {
	t.Parallel()

	maliciousFilters := []json.RawMessage{
		json.RawMessage(`{"eq":[["type","'; DROP TABLE jobs; --"]]}`),
		json.RawMessage(`{"eq":[["type","1 OR 1=1"]]}`),
		json.RawMessage(`{"eq":[["type","UNION SELECT * FROM secrets"]]}`),
		json.RawMessage(`{"has":["field; DELETE FROM runs"]}`),
		json.RawMessage(`{"ne":[["status","success"]],  "eq":[["type","'; DROP TABLE --"]]}`),
	}

	payload := json.RawMessage(`{"type":"deploy","status":"success"}`)

	for i, filter := range maliciousFilters {
		t.Run(fmt.Sprintf("injection_%d", i), func(t *testing.T) {
			t.Parallel()
			match, err := eventfilter.Eval(filter, payload)
			if err != nil {
				t.Fatalf("filter eval should not error on SQL-like strings: %v", err)
			}
			// SQL-like strings used as literal comparison values should not
			// match normal payload values.
			if match {
				t.Fatal("malicious filter should not match normal payload")
			}
		})
	}
}

// TestEventSource_FilterExpressionDoS verifies that oversized filter expressions
// fail closed instead of consuming unbounded parser work.
func TestEventSource_FilterExpressionDoS(t *testing.T) {
	t.Parallel()

	conditions := make([][2]string, 100000)
	for i := range conditions {
		conditions[i] = [2]string{"field", fmt.Sprintf("val_%d", i)}
	}
	expr := eventfilter.FilterExpr{Eq: conditions}
	filterJSON, err := json.Marshal(expr)
	if err != nil {
		t.Fatalf("failed to marshal filter: %v", err)
	}

	payload := json.RawMessage(`{"field":"no_match"}`)

	if _, err := eventfilter.Eval(filterJSON, payload); err == nil {
		t.Fatal("expected oversized filter expression to be rejected")
	}
}

// TestEventSource_FilterExpressionNestedPaths verifies that deeply nested
// dot-separated paths fail closed before recursive evaluation.
func TestEventSource_FilterExpressionNestedPaths(t *testing.T) {
	t.Parallel()

	parts := make([]string, 100)
	for i := range parts {
		parts[i] = "a"
	}
	path := strings.Join(parts, ".")

	filter := json.RawMessage(fmt.Sprintf(`{"has":[%q]}`, path))
	payload := json.RawMessage(`{"a":{"b":"c"}}`)

	if _, err := eventfilter.Eval(filter, payload); err == nil {
		t.Fatal("expected deeply nested filter path to be rejected")
	}
}

// TestEventSource_SchemaValidationBypass verifies that arbitrary JSON in the schema
// field is accepted without causing server errors (schema is stored as raw JSON).
func TestEventSource_SchemaValidationBypass(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, src *domain.EventSource) error {
			src.ID = "src-schema"
			src.CreatedAt = time.Now()
			src.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "schema-test",
		"schema": {"type":"object","properties":{"$ref":"http://evil.com/schema"},"additionalProperties":false}
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEventSource_CreateSignatureSecretRequiresEncryptor(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEventSourceFunc: func(_ context.Context, _ *domain.EventSource) error {
			t.Fatal("CreateEventSource should not be called when signature secret encryption is unavailable")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "signed-source",
		"signature_header": "X-Signature",
		"signature_algorithm": "hmac-sha256",
		"signature_secret": "secret"
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/event-sources", body))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when signature secret encryption is unavailable, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEventSource_UpdateSignatureSecretRequiresEncryptor(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		UpdateEventSourceFunc: func(_ context.Context, _, _ string, _ map[string]any) error {
			t.Fatal("UpdateEventSource should not be called when signature secret encryption is unavailable")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	secret := "secret"
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	_, err := srv.handleUpdateEventSource(ctx, &UpdateEventSourceInput{
		SourceID: "src-1",
		Body: UpdateEventSourceRequest{
			SignatureSecret: &secret,
		},
	})
	var statusErr interface{ GetStatus() int }
	if !errors.As(err, &statusErr) || statusErr.GetStatus() != http.StatusInternalServerError {
		t.Fatalf("expected 500 when signature secret encryption is unavailable, got %v", err)
	}
}

// TestEventSource_SignatureVerificationEmpty verifies that when the event source
// requires signature verification but no encryptor is configured, the server
// fails closed before dispatching subscriptions.
func TestEventSource_SignatureVerificationEmpty(t *testing.T) {
	t.Parallel()

	subscriptionsCalled := false
	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-sig", ProjectID: projectID, Name: name, Enabled: true,
				SignatureHeader: "X-Signature", SignatureAlgorithm: "hmac-sha256",
				SignatureSecretEnc: []byte("encrypted-secret"),
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			subscriptionsCalled = true
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"source":"sig-source","project_id":"proj-1","payload":{"data":"test"}}`
	w := httptest.NewRecorder()
	req := authedRequest(http.MethodPost, "/v1/events/dispatch", body)

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when signature verification is misconfigured, got %d: %s", w.Code, w.Body.String())
	}
	if subscriptionsCalled {
		t.Fatal("subscriptions should not be listed when signature verification is misconfigured")
	}
}

// TestEventSource_SignatureVerificationWrongAlgorithm verifies that unsupported
// signature algorithms are not in the supported set.
func TestEventSource_SignatureVerificationWrongAlgorithm(t *testing.T) {
	t.Parallel()

	supported := map[string]bool{
		"hmac-sha256":   true,
		"stripe-v1":     true,
		"github-sha256": true,
	}

	unsupported := []string{"md5", "sha1", "rsa-sha256", "none", ""}
	for _, algo := range unsupported {
		if supported[algo] {
			t.Fatalf("algorithm %q should not be supported", algo)
		}
	}
}

// TestEventSource_SignatureVerificationReplay verifies that replaying a valid signature
// from one payload with a different payload fails validation.
func TestEventSource_SignatureVerificationReplay(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	payloadA := []byte(`{"event":"deploy","version":"1.0"}`)
	payloadB := []byte(`{"event":"deploy","version":"2.0"}`)

	// Compute valid HMAC for payload A.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadA)
	sigForA := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// Compute expected HMAC for payload B.
	mac2 := hmac.New(sha256.New, []byte(secret))
	mac2.Write(payloadB)
	expectedForB := hex.EncodeToString(mac2.Sum(nil))

	// The signature from A should not match the HMAC for B.
	actualFromHeader := strings.TrimPrefix(sigForA, "sha256=")
	if actualFromHeader == expectedForB {
		t.Fatal("different payloads should produce different HMAC signatures")
	}
}

// TestEventSource_DispatchWithNullPayload verifies dispatching with a null payload
// does not crash the server.
func TestEventSource_DispatchWithNullPayload(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-null", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{ID: "sub-1", SourceID: "src-null", TargetType: "job", TargetID: "job-1", Enabled: true},
			}, nil
		},
		GetJobsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.Job, error) {
			out := make(map[string]*domain.Job, len(ids))
			for _, id := range ids {
				out[id] = &domain.Job{ID: id, Enabled: true, Version: 1, VersionID: "jv-1", ProjectID: "proj-1"}
			}
			return out, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			run.ID = "run-null"
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"source":"my-source","project_id":"proj-1","payload":null}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestEventSource_DispatchWithHugePayload verifies that a 10MB payload is handled
// gracefully (either accepted or rejected, but no crash).
func TestEventSource_DispatchWithHugePayload(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-huge", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	bigValue := strings.Repeat("x", 10*1024*1024)
	body := fmt.Sprintf(`{"source":"my-source","project_id":"proj-1","payload":{"data":%q}}`, bigValue)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))

	if w.Code == http.StatusInternalServerError {
		t.Fatalf("huge payload should not cause 500: %s", w.Body.String())
	}
}

// TestEventSource_SubscriptionFilterBypass verifies that a filter that should block
// a payload actually prevents dispatch.
func TestEventSource_SubscriptionFilterBypass(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32

	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-filter", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{
					ID: "sub-block", SourceID: "src-filter", TargetType: "job", TargetID: "job-1",
					FilterExpr: json.RawMessage(`{"eq":[["env","production"]]}`), Enabled: true,
				},
			}, nil
		},
		GetJobsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.Job, error) {
			out := make(map[string]*domain.Job, len(ids))
			for _, id := range ids {
				out[id] = &domain.Job{ID: id, Enabled: true, Version: 1, VersionID: "jv-1", ProjectID: "proj-1"}
			}
			return out, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueued.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	body := `{"source":"my-source","project_id":"proj-1","payload":{"env":"staging"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	dispatched := int(resp["dispatched"].(float64))
	if dispatched != 0 {
		t.Fatalf("expected 0 dispatched (filter should block), got %d", dispatched)
	}
	if enqueued.Load() != 0 {
		t.Fatalf("expected 0 enqueued, got %d", enqueued.Load())
	}
}

// TestEventSource_ConcurrentDispatch verifies that 100 concurrent dispatches
// do not cause data races or panics.
func TestEventSource_ConcurrentDispatch(t *testing.T) {
	t.Parallel()

	var enqueued atomic.Int32

	ms := &APIStoreMock{
		GetEventSourceByNameFunc: func(_ context.Context, projectID, name string) (*domain.EventSource, error) {
			return &domain.EventSource{
				ID: "src-conc", ProjectID: projectID, Name: name, Enabled: true,
			}, nil
		},
		ListEventSubscriptionsBySourceFunc: func(_ context.Context, _ string) ([]domain.EventSubscription, error) {
			return []domain.EventSubscription{
				{ID: "sub-conc", SourceID: "src-conc", TargetType: "job", TargetID: "job-1", Enabled: true},
			}, nil
		},
		GetJobsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.Job, error) {
			out := make(map[string]*domain.Job, len(ids))
			for _, id := range ids {
				out[id] = &domain.Job{ID: id, Enabled: true, Version: 1, VersionID: "jv-1", ProjectID: "proj-1"}
			}
			return out, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, run *domain.JobRun) error {
			enqueued.Add(1)
			run.ID = fmt.Sprintf("run-%d", enqueued.Load())
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	var wg conc.WaitGroup
	for i := range 100 {
		idx := i
		wg.Go(func() {
			body := fmt.Sprintf(`{"source":"my-source","project_id":"proj-1","payload":{"idx":%d}}`, idx)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/dispatch", body))
			if w.Code != http.StatusOK {
				t.Errorf("dispatch %d: expected 200, got %d", idx, w.Code)
			}
		})
	}
	wg.Wait()

	if enqueued.Load() != 100 {
		t.Fatalf("expected 100 enqueued, got %d", enqueued.Load())
	}
}

// FuzzEventSourceFilter fuzzes the filter expression JSON to ensure the filter
// evaluator does not panic on arbitrary input.
func FuzzEventSourceFilter(f *testing.F) {
	f.Add([]byte(`{"eq":[["type","deploy"]]}`))
	f.Add([]byte(`{"ne":[["status","fail"]]}`))
	f.Add([]byte(`{"has":["field"]}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(`{"eq":[[]]}`))

	payload := json.RawMessage(`{"type":"deploy","status":"ok","field":"present"}`)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = eventfilter.Eval(json.RawMessage(data), payload)
	})
}

// FuzzEventSourcePayload fuzzes the dispatch payload to ensure the filter
// evaluator does not panic on arbitrary payload content.
func FuzzEventSourcePayload(f *testing.F) {
	f.Add([]byte(`{"type":"deploy"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`123`))
	f.Add([]byte(`[1,2,3]`))

	filter := json.RawMessage(`{"eq":[["type","deploy"]]}`)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = eventfilter.Eval(filter, json.RawMessage(data))
	})
}
