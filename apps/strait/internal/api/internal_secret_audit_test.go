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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, json.Unmarshal(ev.Details,
		&details))

	v, _ := details[key].(string)
	return v
}

func requireBypassAudit(t *testing.T, cap *auditCapture, gate, handler, resourceType, resourceID string) {
	t.Helper()
	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	require.Len(t,
		bypass, 1)

	ev := bypass[0]
	assert.Equal(
		t, gate, detailString(t, ev,
			"gate"))
	assert.Equal(
		t, handler, detailString(t, ev,
			"handler"))
	assert.Equal(
		t, resourceType,
		ev.ResourceType,
	)
	assert.Equal(
		t, resourceID, ev.
			ResourceID)
	assert.Equal(
		t, "proj-1", ev.ProjectID)
	assert.NotEmpty(t, detailString(t, ev,
		"caller"))
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
	require.Equal(t, http.StatusOK,
		w.Code)

	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	require.Len(t,
		bypass, 1)

	ev := bypass[0]
	assert.Equal(
		t, "batch_enable_jobs.project_match",

		detailString(t, ev, "gate"))
	assert.Equal(
		t, "handleBatchEnableJobs",
		detailString(t, ev, "handler"))
	assert.NotEmpty(t, detailString(t, ev,
		"caller"))
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
	require.Equal(t, http.StatusOK,
		w.Code)

	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	require.Len(t,
		bypass, 1)
	assert.Equal(
		t, "batch_disable_jobs.project_match",

		detailString(t, bypass[0], "gate"))
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
	require.Equal(t, http.StatusOK,
		w.Code)

	if got := cap.findByAction(domain.AuditActionInternalSecretBypass); len(got) != 0 {
		assert.Failf(t, "test failure",

			"project-scoped caller must not produce bypass audit; got %d", len(got))
	}
}

func TestCreateJob_InternalSecretAudit_UsesRequestProject(t *testing.T) {
	t.Parallel()

	cap := &auditCapture{}
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-created"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
		CreateAuditEventFunc: cap.record,
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"Created","slug":"created","endpoint_url":"https://example.com/hook"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
	require.Equal(t, http.StatusCreated, w.Code)

	created := cap.findByAction(domain.AuditActionJobCreated)
	require.Len(t, created, 1)
	assert.Equal(t, "proj-1", created[0].ProjectID)
	assert.Equal(t, "job-created", created[0].ResourceID)
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
	require.Len(t,
		bypass, 1)
	assert.Equal(
		t, "trig-1", bypass[0].ResourceID,
	)
	assert.Equal(
		t, "send_event.project_match",

		detailString(t, bypass[0], "gate"))
	assert.Equal(
		t, trigger.ProjectID, bypass[0].ProjectID)
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
	require.Equal(t, http.StatusOK,
		w.Code)

	bypass := cap.findByAction(domain.AuditActionInternalSecretBypass)
	require.Len(t,
		bypass, 1)
	assert.Equal(
		t, "handleGetEventTrigger",
		detailString(t, bypass[0], "handler"))
	assert.Equal(
		t, trigger.ProjectID, bypass[0].ProjectID)
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
	require.Len(t,
		bypass, 1)
	assert.Equal(
		t, "cancel_event_trigger.project_match",

		detailString(t, bypass[0], "gate"))
	assert.Equal(
		t, trigger.ProjectID, bypass[0].ProjectID)
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	requireBypassAudit(t, cap, "trigger_job.project_match", "handleTriggerJob", "job", "job-123")
	var triggered []*domain.AuditEvent
	require.Eventually(t, func() bool {
		triggered = cap.findByAction(domain.AuditActionJobTriggered)
		return len(triggered) == 1
	}, time.Second, 10*time.Millisecond)
	require.Len(t, triggered, 1)
	assert.Equal(t, "proj-1", triggered[0].ProjectID)
	assert.Equal(t, "job-123", triggered[0].ResourceID)
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
	require.NoError(t, err)

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
		require.Fail(t,

			"expected missing endpoint error")
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
	require.Equal(t, http.StatusOK,
		w.Code)

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
	require.Equal(t, http.StatusGone,
		w.Code)

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
	require.NoError(t, err)

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
		require.Failf(t, "test failure",

			"handleDeleteWebhookSubscription: %v", err)
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
		require.Failf(t, "test failure",

			"handleRotateWebhookSecret: %v", err)
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
	require.Equal(t, "unknown", got)
}

// TestBypassCallerLabel_InternalOnly pins the normal internal-secret call:
// the marker is set but no api-key principal is attached. The label should
// be "internal_secret" so reviewers can distinguish the legitimate code
// path from the truly orphaned (caller=unknown) one.
func TestBypassCallerLabel_InternalOnly(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxInternalCallerKey, true)
	got := bypassCallerLabel(ctx)
	require.Equal(t, "internal_secret",
		got)
}

// TestBypassCallerLabel_PrefersActorID pins the precedence: when an actor
// id is present, that is the most specific signal and wins over the
// project-id and internal markers.
func TestBypassCallerLabel_PrefersActorID(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), ctxActorIDKey, "user_42")
	ctx = context.WithValue(ctx, ctxInternalCallerKey, true)
	got := bypassCallerLabel(ctx)
	require.Equal(t, "user_42", got)
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
	require.Len(t,
		bypass, 1)
	assert.Equal(
		t, "unknown", detailString(t,
			bypass[0], "caller"),
	)
	assert.Equal(
		t, "thing", bypass[0].ResourceType,
	)
	assert.Equal(
		t, "thing-1", bypass[0].ResourceID,
	)
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
	require.Len(t,
		bypass, 1)

	var details map[string]any
	require.NoError(t, json.Unmarshal(bypass[0].Details, &details))

	for _, key := range []string{"gate", "caller", "handler"} {
		v, ok := details[key].(string)
		assert.False(
			t, !ok || strings.TrimSpace(v) == "")
	}
}
