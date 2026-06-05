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
)

func TestHandleCreateEnvironment_MissingName(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","slug":"staging"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments/", body, "proj-1"))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing name, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateEnvironment_MissingSlug(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"Staging"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/environments/", body, "proj-1"))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing slug, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for store error, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetEnvironment_EnvironmentScopedCallerCannotReadOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			t.Fatal("resolved variables should not be loaded for a mismatched environment")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleGetEnvironment(ctx, &GetEnvironmentInput{EnvID: "env-staging"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var envs []domain.Environment
	decodePaginatedList(t, w.Body.Bytes(), &envs)
	if len(envs) != 0 {
		t.Fatalf("expected empty list, got %d items", len(envs))
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var envelope struct {
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor,omitempty"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Store returns limit+1 = 3 items so has_more should be true.
	if !envelope.HasMore {
		t.Fatal("expected has_more=true when store returns limit+1 items")
	}
}

func TestHandleListEnvironments_EnvironmentScopedCallerOnlySeesOwnEnvironment(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			if projectID != "proj-1" {
				t.Fatalf("projectID = %q, want proj-1", projectID)
			}
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
	if err != nil {
		t.Fatalf("handleListEnvironments returned error: %v", err)
	}
	items, ok := out.Body.Data.([]EnvironmentResponse)
	if !ok {
		t.Fatalf("items type = %T, want []EnvironmentResponse", out.Body.Data)
	}
	if len(items) != 1 || items[0].ID != "env-prod" {
		t.Fatalf("items = %+v, want only env-prod", items)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var env domain.Environment
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.Slug != "new-slug" {
		t.Fatalf("expected new-slug, got %s", env.Slug)
	}
	if env.Name != "My Env" {
		t.Fatalf("expected name unchanged, got %s", env.Name)
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-project update, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleUpdateEnvironment_EnvironmentScopedCallerCannotMutateOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, _ *domain.Environment) error {
			t.Fatal("UpdateEnvironment should not be called for a mismatched environment")
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
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
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

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for standard env deletion, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteEnvironment_EnvironmentScopedCallerCannotDeleteOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		DeleteEnvironmentFunc: func(_ context.Context, _ string, _ string) error {
			t.Fatal("DeleteEnvironment should not be called for a mismatched environment")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleDeleteEnvironment(ctx, &DeleteEnvironmentInput{EnvID: "env-staging"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
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

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetResolvedVariables_EnvironmentScopedCallerCannotReadOtherEnvironment(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			t.Fatal("resolved variables should not be loaded for a mismatched environment")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-prod")

	_, err := srv.handleGetResolvedVariables(ctx, &GetResolvedVariablesInput{EnvID: "env-staging"})
	if !isHumaStatusError(err, http.StatusNotFound) {
		t.Fatalf("expected 404 for environment mismatch, got %v", err)
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	vars, ok := resp["variables"]
	if !ok {
		t.Fatal("expected 'variables' key in response")
	}
	if vars["INHERITED"] != "from-parent" {
		t.Fatalf("expected inherited variable from-parent, got %v", vars["INHERITED"])
	}
	if vars["LOCAL"] != "value" {
		t.Fatalf("expected local variable value, got %v", vars["LOCAL"])
	}
	if len(vars) != 3 {
		t.Fatalf("expected 3 resolved variables, got %d", len(vars))
	}
}
