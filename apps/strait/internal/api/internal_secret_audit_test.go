package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

// auditCapture is a tiny recorder for audit events produced via the API
// store mock. Tests use it to assert the bypass-specific audit row was
// emitted with the expected metadata.
type auditCapture struct {
	mu     sync.Mutex
	events []*domain.AuditEvent
}

func (c *auditCapture) record(_ context.Context, ev *domain.AuditEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	clone := *ev
	c.events = append(c.events, &clone)
	return nil
}

func (c *auditCapture) findByAction(action string) []*domain.AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*domain.AuditEvent, 0)
	for _, ev := range c.events {
		if ev.Action == action {
			out = append(out, ev)
		}
	}
	return out
}

func detailString(t *testing.T, ev *domain.AuditEvent, key string) string {
	t.Helper()
	var details map[string]any
	if err := json.Unmarshal(ev.Details, &details); err != nil {
		t.Fatalf("decode audit details: %v\nraw: %s", err, string(ev.Details))
	}
	v, _ := details[key].(string)
	return v
}

func requireBypassAudit(t *testing.T, cap *auditCapture, gate, handler, resourceType, resourceID string) {
	t.Helper()
	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d (all=%d)", len(bypass), len(cap.events))
	}
	ev := bypass[0]
	if got := detailString(t, ev, "gate"); got != gate {
		t.Errorf("gate = %q, want %s", got, gate)
	}
	if got := detailString(t, ev, "handler"); got != handler {
		t.Errorf("handler = %q, want %s", got, handler)
	}
	if got := ev.ResourceType; got != resourceType {
		t.Errorf("resource_type = %q, want %s", got, resourceType)
	}
	if got := ev.ResourceID; got != resourceID {
		t.Errorf("resource_id = %q, want %s", got, resourceID)
	}
	if got := detailString(t, ev, "caller"); got == "" {
		t.Error("caller must be populated")
	}
}

// TestBatchEnableJobs_InternalSecretBypass_EmitsAudit walks the documented
// bypass: an internal-secret-authenticated caller with no project context
// hits the batch-enable handler. The handler is allowed through, but the
// bypass-audit row must be recorded so the leak leaves a forensic trail.
func TestBatchEnableJobs_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, ids []string, _ bool, _ string) (int64, error) {
			return int64(len(ids)), nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	body := `{"ids":["job-1","job-2"]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-enable", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d: %s", w.Code, w.Body.String())
	}
	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d (all=%d)", len(bypass), len(cap.events))
	}
	ev := bypass[0]
	if got := detailString(t, ev, "gate"); got != "batch_enable_jobs.project_match" {
		t.Errorf("gate = %q, want batch_enable_jobs.project_match", got)
	}
	if got := detailString(t, ev, "handler"); got != "handleBatchEnableJobs" {
		t.Errorf("handler = %q, want handleBatchEnableJobs", got)
	}
	if got := detailString(t, ev, "caller"); got == "" {
		t.Error("caller must be populated")
	}
}

// TestBatchDisableJobs_InternalSecretBypass_EmitsAudit mirrors the enable
// counterpart for the disable path.
func TestBatchDisableJobs_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, ids []string, _ bool, _ string) (int64, error) {
			return int64(len(ids)), nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	body := `{"ids":["job-1"]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/batch-disable", body))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d: %s", w.Code, w.Body.String())
	}
	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d", len(bypass))
	}
	if got := detailString(t, bypass[0], "gate"); got != "batch_disable_jobs.project_match" {
		t.Errorf("gate = %q, want batch_disable_jobs.project_match", got)
	}
}

// TestBatchEnableJobs_ProjectScopedCaller_NoBypassAudit confirms a normal
// API-key caller that includes a project context does NOT emit the bypass
// audit row — the bypass event must fire only on the internal-secret path.
func TestBatchEnableJobs_ProjectScopedCaller_NoBypassAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		BatchUpdateJobsEnabledFunc: func(_ context.Context, ids []string, _ bool, _ string) (int64, error) {
			return int64(len(ids)), nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	body := `{"ids":["job-1"]}`
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/jobs/batch-enable", body, "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d: %s", w.Code, w.Body.String())
	}
	if got := cap.findByAction(domain.AuditActionInternalSecretBypass); len(got) != 0 {
		t.Errorf("project-scoped caller must not produce bypass audit; got %d", len(got))
	}
}

// TestSendEvent_InternalSecretBypass_EmitsAudit walks the same bypass
// pattern for the event-trigger send path. The trigger.ID must end up
// in the audit row's resource_id so a reviewer can pivot back to the
// resource that was touched.
func TestSendEvent_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	trigger := &domain.EventTrigger{
		ID:            "trig-1",
		ProjectID:     "proj-1",
		EventKey:      "evt.test",
		Status:        domain.EventTriggerStatusWaiting,
		SourceType:    "manual",
		RequestedAt:   time.Now(),
		EnvironmentID: "",
	}
	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return trigger, nil
		},
		UpdateEventTriggerStatusFunc: func(_ context.Context, _, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		SetEventTriggerSentByFunc: func(_ context.Context, _, _ string) error { return nil },
		CreateAuditEventFunc:      cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/events/evt.test/send", `{"payload":{}}`))

	// The handler may return 200 or 500 depending on resume path, but the
	// bypass audit must fire BEFORE the body of the handler runs the resume
	// logic. So regardless of exit code, the bypass row must be present.
	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d (status=%d body=%s)", len(bypass), w.Code, w.Body.String())
	}
	if got := bypass[0].ResourceID; got != "trig-1" {
		t.Errorf("resource_id = %q, want trig-1", got)
	}
	if got := detailString(t, bypass[0], "gate"); got != "send_event.project_match" {
		t.Errorf("gate = %q, want send_event.project_match", got)
	}
}

// TestGetEventTrigger_InternalSecretBypass_EmitsAudit covers the read path.
func TestGetEventTrigger_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	trigger := &domain.EventTrigger{
		ID: "trig-2", ProjectID: "proj-1", EventKey: "evt.read",
		Status: domain.EventTriggerStatusWaiting, SourceType: "manual",
	}
	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return trigger, nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/events/evt-read", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d: %s", w.Code, w.Body.String())
	}
	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d", len(bypass))
	}
	if got := detailString(t, bypass[0], "handler"); got != "handleGetEventTrigger" {
		t.Errorf("handler = %q", got)
	}
}

// TestCancelEventTrigger_InternalSecretBypass_EmitsAudit covers the
// cancel path.
func TestCancelEventTrigger_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	trigger := &domain.EventTrigger{
		ID: "trig-3", ProjectID: "proj-1", EventKey: "evt.cancel",
		Status: domain.EventTriggerStatusWaiting, SourceType: "manual",
	}
	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return trigger, nil
		},
		UpdateEventTriggerStatusFunc: func(_ context.Context, _, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			return nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/events/evt-cancel", ""))

	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d (status=%d)", len(bypass), w.Code)
	}
	if got := detailString(t, bypass[0], "gate"); got != "cancel_event_trigger.project_match" {
		t.Errorf("gate = %q", got)
	}
}

func TestTriggerJob_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		CreateAuditEventFunc: cap.record,
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201; got %d: %s", w.Code, w.Body.String())
	}
	requireBypassAudit(t, cap, "trigger_job.project_match", "handleTriggerJob", "job", "job-123")
}

func TestSetJobEndpoint_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	job := &domain.Job{
		ID:          "job-endpoint",
		ProjectID:   "proj-1",
		EndpointURL: "https://example.com/hook",
	}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		UpdateJobEndpointFunc: func(context.Context, string, string, string, string) error {
			return nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	_, err := srv.handleSetJobEndpoint(ctx, &SetJobEndpointInput{
		JobID: job.ID,
		Body:  SetJobEndpointRequest{EndpointURL: job.EndpointURL},
	})
	if err != nil {
		t.Fatalf("handleSetJobEndpoint: %v", err)
	}
	requireBypassAudit(t, cap, "set_job_endpoint.project_match", "handleSetJobEndpoint", "job", job.ID)
}

func TestVerifyJobEndpoint_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	job := &domain.Job{ID: "job-verify", ProjectID: "proj-1"}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return job, nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	if _, err := srv.handleVerifyJobEndpoint(ctx, &VerifyJobEndpointInput{JobID: job.ID}); err == nil {
		t.Fatal("expected missing endpoint error")
	}
	requireBypassAudit(t, cap, "verify_job_endpoint.project_match", "handleVerifyJobEndpoint", "job", job.ID)
}

func TestEventTriggerStream_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	trigger := &domain.EventTrigger{
		ID:         "trig-stream",
		ProjectID:  "proj-1",
		EventKey:   "evt.stream",
		Status:     domain.EventTriggerStatusReceived,
		SourceType: "manual",
	}
	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return trigger, nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := authedRequest(http.MethodGet, "/v1/events/evt.stream/stream/", "")
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("eventKey", trigger.EventKey)
	ctx := context.WithValue(r.Context(), ctxInternalCallerKey, true)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeCtx)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.handleEventTriggerStream(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200; got %d: %s", w.Code, w.Body.String())
	}
	requireBypassAudit(t, cap, "event_trigger_stream.project_match", "handleEventTriggerStream", "event_trigger", trigger.ID)
}

func TestStreamSSE_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	run := &domain.JobRun{ID: "run-stream", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return run, nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	r := authedRequest(http.MethodGet, "/v1/runs/run-stream/stream/", "")
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("runID", run.ID)
	ctx := context.WithValue(r.Context(), ctxInternalCallerKey, true)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeCtx)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.streamSSE(w, r, sseStreamOptions{channel: "run:%s", rejectIfTerminal: true})

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410; got %d: %s", w.Code, w.Body.String())
	}
	requireBypassAudit(t, cap, "stream_sse.project_match", "streamSSE", "run", run.ID)
}

func TestCreateWebhookSubscription_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		CreateWebhookSubscriptionFunc: func(_ context.Context, sub *domain.WebhookSubscription) error {
			sub.ID = "sub-create"
			return nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	_, err := srv.handleCreateWebhookSubscription(ctx, &CreateWebhookSubscriptionInput{Body: CreateWebhookSubscriptionRequest{
		ProjectID:  "proj-1",
		WebhookURL: "https://example.com/hook",
		EventTypes: []string{domain.WebhookEventRunCompleted},
	}})
	if err != nil {
		t.Fatalf("handleCreateWebhookSubscription: %v", err)
	}
	requireBypassAudit(t, cap, "create_webhook_subscription.project_match", "handleCreateWebhookSubscription", "project", "proj-1")
}

func TestDeleteWebhookSubscription_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, _ string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{
				ID:         "sub-delete",
				ProjectID:  "proj-1",
				WebhookURL: "https://example.com/hook",
				EventTypes: []string{domain.WebhookEventRunFailed},
			}, nil
		},
		DeleteWebhookSubscriptionFunc: func(context.Context, string) error { return nil },
		CreateAuditEventFunc:          cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	if _, err := srv.handleDeleteWebhookSubscription(ctx, &DeleteWebhookSubscriptionInput{ID: "sub-delete"}); err != nil {
		t.Fatalf("handleDeleteWebhookSubscription: %v", err)
	}
	requireBypassAudit(t, cap, "delete_webhook_subscription.project_match", "handleDeleteWebhookSubscription", "webhook_subscription", "sub-delete")
}

func TestRotateWebhookSecret_InternalSecretBypass_EmitsAudit(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		GetWebhookSubscriptionFunc: func(_ context.Context, _ string) (*domain.WebhookSubscription, error) {
			return &domain.WebhookSubscription{ID: "sub-rotate", ProjectID: "proj-1"}, nil
		},
		RotateWebhookSecretFunc: func(context.Context, string, string, time.Time) error { return nil },
		CreateAuditEventFunc:    cap.record,
	}
	srv := newTestServerWithEncryptor(t, ms, &mockQueue{}, &mockEncryptor{})

	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	if _, err := srv.handleRotateWebhookSecret(ctx, &RotateWebhookSecretInput{ID: "sub-rotate"}); err != nil {
		t.Fatalf("handleRotateWebhookSecret: %v", err)
	}
	requireBypassAudit(t, cap, "rotate_webhook_secret.project_match", "handleRotateWebhookSecret", "webhook_subscription", "sub-rotate")
}

// TestBypassCallerLabel_NoPrincipal pins the leaked-internal-secret
// adversarial path: when the audit emit is called with no auth principal
// in context, the helper records caller="unknown" rather than crashing
// or leaving the field empty.
func TestBypassCallerLabel_NoPrincipal(t *testing.T) {
	t.Parallel()
	got := bypassCallerLabel(context.Background())
	if got != "unknown" {
		t.Fatalf("bypassCallerLabel(empty ctx) = %q, want unknown", got)
	}
}

// TestBypassCallerLabel_InternalOnly pins the normal internal-secret call:
// the marker is set but no api-key principal is attached. The label should
// be "internal_secret" so reviewers can distinguish the legitimate code
// path from the truly orphaned (caller=unknown) one.
func TestBypassCallerLabel_InternalOnly(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	got := bypassCallerLabel(ctx)
	if got != "internal_secret" {
		t.Fatalf("bypassCallerLabel(internal-only) = %q, want internal_secret", got)
	}
}

// TestBypassCallerLabel_PrefersActorID pins the precedence: when an actor
// id is present, that is the most specific signal and wins over the
// project-id and internal markers.
func TestBypassCallerLabel_PrefersActorID(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "user_42")
	ctx = context.WithValue(ctx, ctxInternalCallerKey, true)
	got := bypassCallerLabel(ctx)
	if got != "user_42" {
		t.Fatalf("bypassCallerLabel(actor+internal) = %q, want user_42", got)
	}
}

// TestEmitInternalSecretBypassAudit_LeakedSecret_NoPrincipal walks the
// adversarial scenario from the plan: an attacker has captured the
// internal secret and is replaying it without any auth principal. The
// emit path must still record the audit row, with caller="unknown" so
// the SOC review can flag the orphaned bypass for investigation.
func TestEmitInternalSecretBypassAudit_LeakedSecret_NoPrincipal(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{CreateAuditEventFunc: cap.record}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Bare context — simulate the worst case: no actor, no project, not
	// even the internal-caller marker (this represents a malformed test
	// fixture or a future code path that calls the helper without
	// staging context properly). The audit row must still be produced.
	srv.emitInternalSecretBypassAudit(context.Background(),
		"hypothetical_gate", "hypothetical_handler", "thing", "thing-1")

	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d", len(bypass))
	}
	if got := detailString(t, bypass[0], "caller"); got != "unknown" {
		t.Errorf("caller = %q, want unknown", got)
	}
	if got := bypass[0].ResourceType; got != "thing" {
		t.Errorf("resource_type = %q", got)
	}
	if got := bypass[0].ResourceID; got != "thing-1" {
		t.Errorf("resource_id = %q", got)
	}
}

// TestEmitInternalSecretBypassAudit_DetailKeysMatchSchema is a tight
// regression guard: the schema declares `gate`, `caller`, and `handler`
// as REQUIRED fields. A future refactor that renames or drops one of
// them would silently break the audit_detail_schema_test guard. This
// test pins the actual emit shape locally so the failure surfaces here
// at the call site.
func TestEmitInternalSecretBypassAudit_DetailKeysMatchSchema(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{CreateAuditEventFunc: cap.record}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	srv.emitInternalSecretBypassAudit(ctx, "g", "h", "r", "")

	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	if len(bypass) != 1 {
		t.Fatalf("expected 1 bypass audit row; got %d", len(bypass))
	}
	var details map[string]any
	if err := json.Unmarshal(bypass[0].Details, &details); err != nil {
		t.Fatalf("decode details: %v", err)
	}
	for _, key := range []string{"gate", "caller", "handler"} {
		v, ok := details[key].(string)
		if !ok || strings.TrimSpace(v) == "" {
			t.Errorf("details[%q] missing/empty: %#v", key, details[key])
		}
	}
}
