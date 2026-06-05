package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandleCreateEnvironment_MissingName(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","slug":"staging"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments/", body, "proj-1"))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleCreateEnvironment_MissingSlug(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"Staging"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments/", body, "proj-1"))
	require.Equal(t, http.StatusUnprocessableEntity,

		w.Code)
}

func TestHandleCreateEnvironment_StoreError(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, _ *domain.Environment) error {
			return fmt.Errorf("duplicate key value violates unique constraint")
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"Staging","slug":"staging"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments/", body, "proj-1"))
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
}

func TestHandleGetEnvironment_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, _ string, _ string) (*domain.Environment, error) {
			return nil, store.ErrEnvironmentNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/env-missing/", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleGetEnvironment_CrossProject(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-other",
				Name:      "Secret",
				Slug:      "secret",
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/env-1/", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleGetEnvironment_EnvironmentScopedCallerCannotReadOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			require.Fail(t,

				"resolved variables should not be loaded for a mismatched environment")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleGetEnvironment(ctx, &GetEnvironmentInput{EnvID: "env-staging"})
	require.True(
		t, isHumaStatusError(err,
			http.StatusNotFound,
		))
}

func TestHandleListEnvironments_EmptyList(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Environment, error) {
			return []domain.Environment{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var envs []domain.Environment
	decodePaginatedList(t, w.Body.Bytes(), &envs)
	require.Empty(t,
		envs)
}

func TestHandleListEnvironments_Pagination(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, _ string, limit int, _ *time.Time) ([]domain.Environment, error) {
			// Handler requests limit+1 to detect has_more. Return limit items to trigger it.
			envs := make([]domain.Environment, 0, limit)
			for i := range limit {
				envs = append(envs, domain.Environment{
					ID:        fmt.Sprintf("env-%d", i),
					ProjectID: "proj-1",
					Name:      "Env",
					Slug:      fmt.Sprintf("env-%d", i),
					CreatedAt: now.Add(time.Duration(i) * time.Second),
					UpdatedAt: now,
				})
			}
			return envs, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/?limit=2", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var envelope struct {
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor,omitempty"`
	}
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&envelope))
	require.True(
		t, envelope.HasMore,
	)

	// Store returns limit+1 = 3 items so has_more should be true.
}

func TestHandleListEnvironments_EnvironmentScopedCallerOnlySeesOwnEnvironment(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			require.Equal(t, "proj-1", projectID)

			return []domain.Environment{
				{ID: "env-prod", ProjectID: "proj-1", Name: "Production", Slug: "production", CreatedAt: now, UpdatedAt: now},
				{ID: "env-staging", ProjectID: "proj-1", Name: "Staging", Slug: "staging", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	out, err := srv.handleListEnvironments(ctx, &ListEnvironmentsInput{Limit: "10"})
	require.NoError(t, err)

	items, ok := out.Body.Data.([]EnvironmentResponse)
	require.True(
		t, ok)
	require.False(t, len(items) !=
		1 || items[0].ID !=
		"env-prod")
}

func TestHandleUpdateEnvironment_SlugOnly(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "My Env",
				Slug:      "old-slug",
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, _ *domain.Environment) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/environments/env-1/", `{"slug":"new-slug"}`, "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var env domain.Environment
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&env))
	require.Equal(t, "new-slug", env.
		Slug,
	)
	require.Equal(t, "My Env", env.
		Name)
}

func TestHandleUpdateEnvironment_CrossProject(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-other",
				Name:      "Theirs",
				Slug:      "theirs",
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/environments/env-1/", `{"name":"Hijack"}`, "proj-1"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleUpdateEnvironment_EnvironmentScopedCallerCannotMutateOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, _ *domain.Environment) error {
			require.Fail(t,

				"UpdateEnvironment should not be called for a mismatched environment")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")
	name := "Hijack"

	_, err := srv.handleUpdateEnvironment(ctx, &UpdateEnvironmentInput{
		EnvID: "env-staging",
		Body:  UpdateEnvironmentRequest{Name: &name},
	})
	require.True(
		t, isHumaStatusError(err,
			http.StatusNotFound,
		))
}

func TestHandleDeleteEnvironment_StandardRejected(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:         id,
				ProjectID:  "proj-1",
				Name:       "Production",
				Slug:       "production",
				IsStandard: true,
				CreatedAt:  now,
				UpdatedAt:  now,
			}, nil
		},
		DeleteEnvironmentFunc: func(_ context.Context, _ string, _ string) error {
			return store.ErrStandardEnvironment
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodDelete, "/v1/environments/env-std/", "", "proj-1"))
	require.Equal(t, http.StatusForbidden,

		w.Code)
}

func TestHandleDeleteEnvironment_EnvironmentScopedCallerCannotDeleteOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		DeleteEnvironmentFunc: func(_ context.Context, _ string, _ string) error {
			require.Fail(t,

				"DeleteEnvironment should not be called for a mismatched environment")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleDeleteEnvironment(ctx, &DeleteEnvironmentInput{EnvID: "env-staging"})
	require.True(
		t, isHumaStatusError(err,
			http.StatusNotFound,
		))
}

func TestHandleGetResolvedVariables_NotFound(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, _ string, _ string) (*domain.Environment, error) {
			return nil, store.ErrEnvironmentNotFound
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/env-missing/variables", "", "proj-1"))
	require.Equal(t, http.StatusNotFound,

		w.Code)
}

func TestHandleGetResolvedVariables_EnvironmentScopedCallerCannotReadOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			require.Fail(t,

				"resolved variables should not be loaded for a mismatched environment")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleGetResolvedVariables(ctx, &GetResolvedVariablesInput{EnvID: "env-staging"})
	require.True(
		t, isHumaStatusError(err,
			http.StatusNotFound,
		))
}

func TestHandleGetResolvedVariables_InheritedVariables(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "Dev",
				Slug:      "dev",
				ParentID:  "env-parent",
				Variables: map[string]string{"LOCAL": "value"},
				CreatedAt: now,
				UpdatedAt: now,
			}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{
				"LOCAL":     "value",
				"INHERITED": "from-parent",
				"DB_HOST":   "db.example.com",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/environments/env-1/variables", "", "proj-1"))
	require.Equal(t, http.StatusOK,
		w.Code,
	)

	var resp map[string]map[string]string
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(),
		&resp))

	vars, ok := resp["variables"]
	require.True(
		t, ok)
	require.Equal(t, "from-parent",
		vars["INHERITED"],
	)
	require.Equal(t, "value", vars["LOCAL"])
	require.Len(t,
		vars, 3)
}
