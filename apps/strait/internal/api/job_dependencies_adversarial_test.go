package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDependencyCondition_Valid verifies that the three allowed condition
// strings are accepted by isValidDependencyCondition.
func TestDependencyCondition_Valid(t *testing.T) {
	t.Parallel()

	for _, cond := range []string{"completed", "failed", "any"} {
		assert.True(t,
			isValidDependencyCondition(
				cond))

	}
}

// TestDependencyCondition_Invalid verifies that unknown or empty condition
// strings are rejected by isValidDependencyCondition.
func TestDependencyCondition_Invalid(t *testing.T) {
	t.Parallel()

	cases := []string{
		"success",
		"",
		"COMPLETED",
		"pending",
		"cancelled",
		"done",
		" completed",
		"completed ",
		"completed\n",
	}

	for _, cond := range cases {
		assert.False(
			t, isValidDependencyCondition(cond))

	}
}

// TestDependencyCondition_SQLInjection verifies that SQL injection payloads
// in the condition field are rejected.
func TestDependencyCondition_SQLInjection(t *testing.T) {
	t.Parallel()

	payloads := []string{
		"completed'; DROP TABLE jobs; --",
		"' OR '1'='1",
		"completed UNION SELECT * FROM api_keys",
		"1; DELETE FROM job_dependencies",
		"completed\x00; DROP TABLE jobs",
	}

	for _, payload := range payloads {
		assert.False(
			t, isValidDependencyCondition(payload))

	}
}

// TestDependencyCondition_NullBytes verifies that null bytes embedded in
// condition strings are rejected.
func TestDependencyCondition_NullBytes(t *testing.T) {
	t.Parallel()

	payloads := []string{
		"completed\x00",
		"\x00completed",
		"comp\x00leted",
		"\x00",
		"\x00\x00\x00",
	}

	for _, payload := range payloads {
		assert.False(
			t, isValidDependencyCondition(payload))

	}
}

// TestDependencyCondition_CaseSensitive verifies that condition matching is
// case-sensitive; uppercase or mixed-case variants must be rejected.
func TestDependencyCondition_CaseSensitive(t *testing.T) {
	t.Parallel()

	cases := []string{
		"Completed",
		"COMPLETED",
		"Failed",
		"FAILED",
		"Any",
		"ANY",
		"cOMPLETED",
	}

	for _, cond := range cases {
		assert.False(
			t, isValidDependencyCondition(cond))

	}
}

// TestDependency_SelfReference verifies that a job cannot declare a dependency
// on itself via the HTTP handler.
func TestDependency_SelfReference(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			if id == "job-1" {
				return &domain.Job{ID: "job-1", ProjectID: "proj-1"}, nil
			}
			return nil, store.ErrJobNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"depends_on_job_id":"job-1"}`
	req := authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", body)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

}

// TestDependency_EmptyDependsOnID verifies that an empty depends_on_job_id is
// rejected by validation.
func TestDependency_EmptyDependsOnID(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", ProjectID: "proj-1"}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"depends_on_job_id":""}`
	req := authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", body)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.False(t, w.Code == http.
		StatusOK ||
		w.Code == http.StatusCreated,
	)

	// Empty depends_on_job_id should fail validation (required field).

}

// TestDependency_CrossProject verifies that creating a dependency between jobs
// in different projects is rejected.
func TestDependency_CrossProject(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			switch id {
			case "job-1":
				return &domain.Job{ID: "job-1", ProjectID: "proj-1"}, nil
			case "job-2":
				return &domain.Job{ID: "job-2", ProjectID: "proj-2"}, nil
			default:
				return nil, store.ErrJobNotFound
			}
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{"depends_on_job_id":"job-2","condition":"completed"}`
	req := authedRequest(http.MethodPost, "/v1/jobs/job-1/dependencies", body)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest,
		w.
			Code)

}

// FuzzDependencyCondition fuzzes the isValidDependencyCondition function with
// arbitrary strings to ensure it never panics and only accepts the known set.
func FuzzDependencyCondition(f *testing.F) {
	f.Add("completed")
	f.Add("failed")
	f.Add("any")
	f.Add("")
	f.Add("COMPLETED")
	f.Add("success")
	f.Add("completed'; DROP TABLE jobs; --")
	f.Add("\x00")
	f.Add(strings.Repeat("a", 10000))

	valid := map[string]bool{
		"completed": true,
		"failed":    true,
		"any":       true,
	}

	f.Fuzz(func(t *testing.T, cond string) {
		result := isValidDependencyCondition(cond)
		assert.False(
			t, result && !valid[cond])
		assert.False(
			t, !result && valid[cond])

	})
}

// FuzzDependencyIDs fuzzes job ID pairs through the create-dependency handler
// to verify it never panics regardless of input.
func FuzzDependencyIDs(f *testing.F) {
	f.Add("job-1", "job-2")
	f.Add("", "")
	f.Add("job-1", "job-1")
	f.Add("\x00", "job-2")
	f.Add("job-1", "\x00")
	f.Add(strings.Repeat("x", 5000), "job-2")
	f.Add("../job-1", "job-2")

	f.Fuzz(func(t *testing.T, jobID, depJobID string) {
		now := time.Now().UTC()
		ms := &APIStoreMock{
			GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
				if id == jobID || id == depJobID {
					return &domain.Job{ID: id, ProjectID: "proj-1", CreatedAt: now}, nil
				}
				return nil, store.ErrJobNotFound
			},
			CreateJobDependencyFunc: func(_ context.Context, _ *domain.JobDependency) error {
				return nil
			},
		}

		srv := newTestServer(t, ms, &mockQueue{}, nil)
		bodyJSON, _ := json.Marshal(map[string]string{
			"depends_on_job_id": depJobID,
			"condition":         "completed",
		})
		req := authedRequest(http.MethodPost, "/v1/jobs/"+url.PathEscape(jobID)+"/dependencies", string(bodyJSON))
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)
		require.NotEqual(t, 0, w.Code)

		// We only care that the server does not panic; any status is fine.

	})
}
