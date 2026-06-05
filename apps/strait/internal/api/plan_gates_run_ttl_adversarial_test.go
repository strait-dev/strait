package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateJob_RunTTLBypass_AfterDowngrade simulates the scenario where a job
// was created with a Pro-tier-permitted run_ttl_secs (30 days), the org has
// since been downgraded to Free (7-day cap), and the customer attempts to
// update the same field. The update path must enforce the new ceiling — a
// pre-existing high TTL does not grant a forever-write loophole.
func TestUpdateJob_RunTTLBypass_AfterDowngrade(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:          "job-1",
		ProjectID:   "proj-1",
		Name:        "Job",
		Slug:        "job",
		EndpointURL: "https://example.com",
		Enabled:     true,
		RunTTLSecs:  30 * 86400, // Pro-era value
	}
	updateCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return source, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			updateCalled = true
			return nil
		},
	}

	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	body := `{"run_ttl_secs":2592000}` // 30 days
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-1", body))
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.False(t, updateCalled)
	assert.Contains(t,
		w.Body.String(), "run_ttl_secs")
}

// TestUpdateJob_NoRunTTLChange_NotGated documents the deliberate carve-out: a
// PATCH that omits run_ttl_secs does not re-evaluate the cap against the
// existing value. This mirrors the platform-wide "block new, leave existing"
// downgrade policy. The test fails if a future change starts gating updates
// that don't touch the field.
func TestUpdateJob_NoRunTTLChange_NotGated(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:          "job-1",
		ProjectID:   "proj-1",
		Name:        "Job",
		Slug:        "job",
		EndpointURL: "https://example.com",
		Enabled:     true,
		RunTTLSecs:  30 * 86400,
	}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return source, nil
		},
		UpdateJobFunc: func(_ context.Context, _ *domain.Job) error {
			return nil
		},
	}

	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-1", `{"name":"renamed"}`))
	require.Equal(t, http.StatusOK,
		w.Code)
}

// TestCloneJob_RunTTLBypass_FromHighTTLSource walks the clone vector: a Pro-era
// job sitting at 30 days is being cloned by a Free-tier project. The clone
// must reject because the new job would inherit an over-cap TTL — there is no
// "inherit grandfather" loophole.
func TestCloneJob_RunTTLBypass_FromHighTTLSource(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:          "job-source",
		ProjectID:   "proj-1",
		Name:        "Source",
		Slug:        "source",
		EndpointURL: "https://example.com",
		MaxAttempts: 3,
		Enabled:     true,
		RunTTLSecs:  30 * 86400,
	}
	createCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return source, nil
		},
		CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
			createCalled = true
			return nil
		},
	}

	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	body := `{"name":"Cloned","slug":"cloned"}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-source/clone", body))
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.False(t, createCalled)
}

// TestCloneJob_RunTTLAtLimit_Allows verifies the clone gate is inclusive at
// the boundary — if the source's TTL equals the destination plan's cap
// exactly, the clone proceeds.
func TestCloneJob_RunTTLAtLimit_Allows(t *testing.T) {
	t.Parallel()

	limits := freeLimits()
	source := &domain.Job{
		ID:          "job-source",
		ProjectID:   "proj-1",
		Name:        "Source",
		Slug:        "source",
		EndpointURL: "https://example.com",
		MaxAttempts: 3,
		Enabled:     true,
		RunTTLSecs:  limits.RetentionDays * 86400,
	}
	created := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return source, nil
		},
		CreateJobFunc: func(_ context.Context, _ *domain.Job) error {
			created = true
			return nil
		},
	}

	enforcer := &tunableLimitsEnforcer{limits: limits}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-source/clone", `{"name":"Cloned","slug":"cloned"}`))
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.True(
		t, created)
}
