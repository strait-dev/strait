package api

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminCtx returns a context that satisfies requireAdmin (isInternalCaller
// returns true) and carries an authenticated project id plus the minimum
// forensic/actor fields required by emitAuditEvent.
func adminCtx(projectID string) context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxInternalCallerKey, true)
	ctx = context.WithValue(ctx, ctxActorIDKey, "internal:admin")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "internal")
	return ctx
}

// nonAdminCtx mimics a request authenticated via API key: scopes slice is
// non-nil, so requireAdmin must return 403.
func nonAdminCtx(projectID string) context.Context {
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, projectID)
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsRead})
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:k-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	return ctx
}

type atomicDropAPIStore struct {
	*APIStoreMock
	drop func(context.Context, string, string, *domain.AuditEvent) (bool, error)
}

func (s *atomicDropAPIStore) DropAuditEventDeadletterWithAudit(ctx context.Context, id, projectID string, auditEvent *domain.AuditEvent) (bool, error) {
	return s.drop(ctx, id, projectID, auditEvent)
}

func TestListDeadletter_RequiresAdmin(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	_, err := srv.handleListDeadletter(nonAdminCtx("proj-a"), &ListDeadletterInput{})
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "admin")
}

func TestListDeadletter_ReturnsEntriesForOwnProject(t *testing.T) {
	t.Parallel()

	var listProject atomic.Value
	listProject.Store("")
	ms := &APIStoreMock{
		ListAuditEventsDeadletterByProjectFunc: func(_ context.Context, projectID string, _ int, _ string) ([]domain.AuditEvent, []string, []string, error) {
			listProject.Store(projectID)
			return []domain.AuditEvent{
				{ID: "dlq-1", ProjectID: projectID, Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC(), Details: json.RawMessage(`{"run_id":"r-1"}`)},
				{ID: "dlq-2", ProjectID: projectID, Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC(), Details: json.RawMessage(`{"run_id":"r-2"}`)},
			}, []string{"dlq-1", "dlq-2"}, []string{"2026-04-01T00:00:00Z", "2026-04-01T00:00:01Z"}, nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}
	srv := newTestServer(t, ms, nil, nil)

	out, err := srv.handleListDeadletter(adminCtx("proj-a"), &ListDeadletterInput{})
	require.NoError(t, err)
	assert.Equal(
		t, "proj-a", listProject.
			Load().(string))
	assert.Len(t,
		out.Body.Entries,
		2)
}

func TestListDeadletter_RejectsCrossTenantProjectID(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, nil, nil)

	// Admin of proj-a tries to list proj-b's DLQ via the query param.
	// Cross-tenant must surface as 404 (not 403) so a probing admin
	// cannot enumerate other projects by watching error codes.
	_, err := srv.handleListDeadletter(adminCtx("proj-a"), &ListDeadletterInput{ProjectID: "proj-b"})
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "not found")
}

func TestReplayDeadletter_MovesEventToChain(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{
		ID:        "dlq-1",
		ProjectID: "proj-a",
		Action:    domain.AuditActionJobTriggered,
		Details:   json.RawMessage(`{"run_id":"r-1"}`),
		CreatedAt: time.Now().UTC(),
	}

	var (
		mu               sync.Mutex
		createdEvents    []domain.AuditEvent
		deletedDLQID     string
		selfAuditFound   bool
		selfAuditEventID string
		selfAuditDLQID   string
	)

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			createdEvents = append(createdEvents, *ev)
			if ev.Action == domain.AuditActionDeadletterReplayed {
				selfAuditFound = true
				var d map[string]any
				_ = json.Unmarshal(ev.Details, &d)
				if s, ok := d["new_event_id"].(string); ok {
					selfAuditEventID = s
				}
				if s, ok := d["deadletter_id"].(string); ok {
					selfAuditDLQID = s
				}
			}
			return nil
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, id, _ string) error {
			mu.Lock()
			defer mu.Unlock()
			deletedDLQID = id
			return nil
		},
		MarkAuditDeadletterReclaimedFunc: func(_ context.Context, dlqID, newEventID string) error {
			require.Equal(t, seed.ID, dlqID)
			require.False(t, newEventID ==
				"" || newEventID ==
				seed.ID)

			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	out, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "dlq-1"})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(
		t, "dlq-1", deletedDLQID,
	)
	require.GreaterOrEqual(t, len(
		createdEvents,
	), 2)
	assert.True(t,
		selfAuditFound)
	assert.Equal(
		t, "dlq-1", selfAuditDLQID,
	)
	assert.Equal(
		t, out.Body.NewEventID,
		selfAuditEventID,
	)
	assert.NotEqual(t, seed.ID, out.
		Body.NewEventID,
	)
}

func TestReplayDeadletter_NotFound_404(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, _, _ string) (*domain.AuditEvent, error) {
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "missing"})
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "not found")
}

func TestDropDeadletter_EmitsAuditAndRemoves(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		droppedID      string
		droppedProject string
		selfAuditHit   bool
		seenReason     string
	)

	ms := &atomicDropAPIStore{
		APIStoreMock: &APIStoreMock{},
		drop: func(_ context.Context, id, projectID string, ev *domain.AuditEvent) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			droppedID = id
			droppedProject = projectID
			if ev.Action == domain.AuditActionDeadletterDropped {
				selfAuditHit = true
				var d map[string]any
				_ = json.Unmarshal(ev.Details, &d)
				if s, ok := d["reason"].(string); ok {
					seenReason = s
				}
			}
			return true, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleDropDeadletter(adminCtx("proj-a"), &DropDeadletterInput{ID: "dlq-1", Reason: "corrupt_payload"})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(
		t, "dlq-1", droppedID,
	)
	assert.Equal(
		t, "proj-a", droppedProject,
	)
	assert.True(t,
		selfAuditHit)
	assert.Equal(
		t, "corrupt_payload",
		seenReason,
	)
}

func TestDropDeadletter_CrossTenant_404(t *testing.T) {
	t.Parallel()

	ms := &atomicDropAPIStore{
		APIStoreMock: &APIStoreMock{},
		drop: func(_ context.Context, _, projectID string, _ *domain.AuditEvent) (bool, error) {
			require.Equal(t, "proj-a", projectID)

			return false, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleDropDeadletter(adminCtx("proj-a"), &DropDeadletterInput{ID: "dlq-1", Reason: "x"})
	require.Error(t, err)
	assert.Contains(t,
		err.Error(), "not found")
}

func TestReplayDeadletter_ChainInsertFailure_LeavesInDLQ(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{ID: "dlq-1", ProjectID: "proj-a", Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC()}
	var (
		mu           sync.Mutex
		deleteCalled bool
		selfAuditHit bool
	)

	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			mu.Lock()
			defer mu.Unlock()
			if ev.Action == domain.AuditActionDeadletterReplayed {
				selfAuditHit = true
				return nil
			}
			// Simulate a broken chain insert for the replayed event.
			return errors.New("chain down")
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, _, _ string) error {
			mu.Lock()
			defer mu.Unlock()
			deleteCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "dlq-1"})
	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.False(
		t, deleteCalled)
	assert.False(
		t, selfAuditHit)
}

func TestReplayDeadletter_MarkFailureFailsClosed(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{ID: "dlq-1", ProjectID: "proj-a", Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC()}
	var deleteCalled bool
	var selfAuditHit bool
	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.Action == domain.AuditActionDeadletterReplayed {
				selfAuditHit = true
			}
			return nil
		},
		MarkAuditDeadletterReclaimedFunc: func(_ context.Context, _, _ string) error {
			return errors.New("mark failed")
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, _, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "dlq-1"})
	require.Error(t, err)
	require.False(t, deleteCalled)
	require.False(t, selfAuditHit)
}

func TestReplayDeadletter_DeleteFailureFailsClosed(t *testing.T) {
	t.Parallel()

	seed := &domain.AuditEvent{ID: "dlq-1", ProjectID: "proj-a", Action: domain.AuditActionJobTriggered, CreatedAt: time.Now().UTC()}
	var selfAuditHit bool
	ms := &APIStoreMock{
		GetAuditEventDeadletterFunc: func(_ context.Context, id, projectID string) (*domain.AuditEvent, error) {
			if id == seed.ID && projectID == seed.ProjectID {
				clone := *seed
				return &clone, nil
			}
			return nil, nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			if ev.Action == domain.AuditActionDeadletterReplayed {
				selfAuditHit = true
			}
			return nil
		},
		MarkAuditDeadletterReclaimedFunc: func(_ context.Context, _, _ string) error {
			return nil
		},
		DeleteAuditEventDeadletterFunc: func(_ context.Context, _, _ string) error {
			return errors.New("delete failed")
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	_, err := srv.handleReplayDeadletter(adminCtx("proj-a"), &ReplayDeadletterInput{ID: "dlq-1"})
	require.Error(t, err)
	require.False(t, selfAuditHit)
}

// TestRedactDeadletterFilter_StripsSecretShapes asserts the filter now
// redacts by shape rather than by key-name allow-list. The previous
// implementation treated a bare Stripe secret key as "safe" because
// neither the key name nor the containing string contained the words
// "secret" or "token"; scanAndRedact catches it by regex.
func TestRedactDeadletterFilter_StripsSecretShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		projectID string
		limit     string
		cursor    string
		mustHide  string
	}{
		{"stripe-secret-in-cursor", "proj-a", "50", "sk_live_abcdefghijklmnop", "sk_live_abcdefghijklmnop"},
		{"bearer-in-projectid", "Bearer abcdefghijklmnop1234", "50", "", "abcdefghijklmnop1234"},
		{"whsec-in-limit", "proj-a", "whsec_abcdefghijklmnop", "", "whsec_abcdefghijklmnop"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := redactDeadletterFilter(tc.projectID, tc.limit, tc.cursor)
			assert.NotContains(
				t, out, tc.
					mustHide)
			assert.Contains(t,
				out, "[redacted:")
		})
	}
}

// TestRedactDeadletterFilter_PassesThroughPlainValues asserts normal
// query params survive unchanged — the scanner must not false-positive
// on UUID-shaped cursors, numeric limits, or short project ids.
func TestRedactDeadletterFilter_PassesThroughPlainValues(t *testing.T) {
	t.Parallel()

	out := redactDeadletterFilter("proj-a", "50", "2026-04-01T00:00:00Z")
	want := "project_id=proj-a&limit=50&cursor=2026-04-01T00:00:00Z"
	assert.Equal(
		t, want, out)
}
