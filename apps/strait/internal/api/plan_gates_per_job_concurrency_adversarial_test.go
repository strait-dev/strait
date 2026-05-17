package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

// TestUpdateJob_PerJobConcurrencyBypass_AfterDowngrade walks the after-downgrade
// vector for max_concurrency: a job was created with a Pro-tier-permitted
// concurrency, the org has been downgraded to Free, and the customer attempts
// to PATCH the field. The update path must enforce the new ceiling.
func TestUpdateJob_PerJobConcurrencyBypass_AfterDowngrade(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:             "job-1",
		ProjectID:      "proj-1",
		Name:           "Job",
		Slug:           "job",
		EndpointURL:    "https://example.com",
		Enabled:        true,
		MaxConcurrency: 50, // Pro-era value
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
	body := `{"max_concurrency":50}` // over Free cap
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-1", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from per-job concurrency cap; got %d: %s", w.Code, w.Body.String())
	}
	if updateCalled {
		t.Fatal("UpdateJob must not be called when the gate rejects")
	}
	if !strings.Contains(w.Body.String(), "max_concurrency") {
		t.Errorf("response should mention max_concurrency; got %s", w.Body.String())
	}
}

// TestUpdateJob_OnlyPerKeyChanged_ChecksWithExistingMaxConcurrency catches a
// sloppy implementation that only validates the field present in the patch.
// When PATCH supplies max_concurrency_per_key but not max_concurrency, the
// gate must combine the new per-key value with the unchanged max_concurrency
// from the source row, since the cap applies to BOTH.
func TestUpdateJob_OnlyPerKeyChanged_ChecksWithExistingMaxConcurrency(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		Name:                 "Job",
		Slug:                 "job",
		EndpointURL:          "https://example.com",
		Enabled:              true,
		MaxConcurrency:       1, // already within Free cap
		MaxConcurrencyPerKey: 1,
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
	body := `{"max_concurrency_per_key":50}` // only this field changes
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-1", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when patched per-key field exceeds cap; got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "max_concurrency_per_key") {
		t.Errorf("response should mention max_concurrency_per_key; got %s", w.Body.String())
	}
}

// TestUpdateJob_NoConcurrencyChange_NotGated documents the deliberate
// carve-out: a PATCH that omits the concurrency fields does not re-evaluate
// the cap against the existing values. This mirrors the platform-wide
// "block new, leave existing" downgrade policy.
func TestUpdateJob_NoConcurrencyChange_NotGated(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:             "job-1",
		ProjectID:      "proj-1",
		Name:           "Job",
		Slug:           "job",
		EndpointURL:    "https://example.com",
		Enabled:        true,
		MaxConcurrency: 50, // far above Free cap, but not being changed
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when concurrency unchanged; got %d: %s", w.Code, w.Body.String())
	}
}

// TestCloneJob_PerJobConcurrencyBypass_FromHighSource walks the clone vector:
// a Pro-era job sitting at max_concurrency=50 is being cloned by a Free-tier
// project. The clone must reject — there is no "inherit grandfather" loophole.
func TestCloneJob_PerJobConcurrencyBypass_FromHighSource(t *testing.T) {
	t.Parallel()

	source := &domain.Job{
		ID:             "job-source",
		ProjectID:      "proj-1",
		Name:           "Source",
		Slug:           "source",
		EndpointURL:    "https://example.com",
		MaxAttempts:    3,
		Enabled:        true,
		MaxConcurrency: 50,
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
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-source/clone", `{"name":"Cloned","slug":"cloned"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 from per-job concurrency cap on clone; got %d: %s", w.Code, w.Body.String())
	}
	if createCalled {
		t.Fatal("CreateJob must not be called when the gate rejects the clone")
	}
}
