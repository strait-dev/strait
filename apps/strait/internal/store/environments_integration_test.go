//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	env := &domain.Environment{
		ProjectID: "proj-create-env-" + newID(),
		Name:      "Production",
		Slug:      "production",
		Variables: map[string]string{"DB_HOST": "db.prod", "LOG_LEVEL": "info"},
	}

	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if env.ID == "" {
		t.Fatal("CreateEnvironment() did not set ID")
	}
	if env.CreatedAt.IsZero() {
		t.Fatal("CreateEnvironment() did not set CreatedAt")
	}
	if env.UpdatedAt.IsZero() {
		t.Fatal("CreateEnvironment() did not set UpdatedAt")
	}

	// Round-trip.
	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got.ID != env.ID {
		t.Fatalf("ID = %q, want %q", got.ID, env.ID)
	}
	if got.ProjectID != env.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, env.ProjectID)
	}
	if got.Name != env.Name {
		t.Fatalf("Name = %q, want %q", got.Name, env.Name)
	}
	if got.Slug != env.Slug {
		t.Fatalf("Slug = %q, want %q", got.Slug, env.Slug)
	}
	if got.ParentID != "" {
		t.Fatalf("ParentID = %q, want empty", got.ParentID)
	}
	if got.IsStandard {
		t.Fatal("IsStandard = true, want false")
	}
	if len(got.Variables) != 2 {
		t.Fatalf("Variables len = %d, want 2", len(got.Variables))
	}
	for k, want := range env.Variables {
		if got.Variables[k] != want {
			t.Fatalf("Variable %q = %q, want %q", k, got.Variables[k], want)
		}
	}
}

func TestCreateEnvironment_WithParent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-env-parent-" + newID()
	parent := &domain.Environment{ProjectID: projectID, Name: "Base", Slug: "base"}
	if err := q.CreateEnvironment(ctx, parent); err != nil {
		t.Fatalf("CreateEnvironment(parent) error = %v", err)
	}

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
	}
	if err := q.CreateEnvironment(ctx, child); err != nil {
		t.Fatalf("CreateEnvironment(child) error = %v", err)
	}

	got, err := q.GetEnvironment(ctx, child.ID, child.ProjectID)
	if err != nil {
		t.Fatalf("GetEnvironment(child) error = %v", err)
	}
	if got.ParentID != parent.ID {
		t.Fatalf("ParentID = %q, want %q", got.ParentID, parent.ID)
	}
}

func TestGetEnvironment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetEnvironment(ctx, newID(), "proj-missing-"+newID())
	if !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment(missing) error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestListEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list-envs-" + newID()

	for i := range 3 {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "env-" + strconv.Itoa(i),
			Slug:      "env-slug-" + newID(),
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment(%d) error = %v", i, err)
		}
	}

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) != 3 {
		t.Fatalf("len = %d, want 3", len(envs))
	}

	for i := 1; i < len(envs); i++ {
		if envs[i-1].CreatedAt.Before(envs[i].CreatedAt) {
			t.Fatalf("envs not DESC at index %d", i)
		}
	}

	// Cursor pagination.
	page1, err := q.ListEnvironments(ctx, projectID, 2, nil)
	if err != nil {
		t.Fatalf("ListEnvironments(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	cursor := page1[1].CreatedAt
	page2, err := q.ListEnvironments(ctx, projectID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListEnvironments(page2) error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}

	// Empty project.
	empty, err := q.ListEnvironments(ctx, "proj-envs-empty-"+newID(), 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments(empty) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty len = %d, want 0", len(empty))
	}
}

func TestUpdateEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-update-env-" + newID()
	env := &domain.Environment{
		ProjectID: projectID,
		Name:      "Original",
		Slug:      "original",
		Variables: map[string]string{"A": "1"},
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	origUpdatedAt := env.UpdatedAt

	env.Name = "Updated"
	env.Slug = "updated"
	env.Variables = map[string]string{"A": "2", "B": "3"}
	if err := q.UpdateEnvironment(ctx, env); err != nil {
		t.Fatalf("UpdateEnvironment() error = %v", err)
	}
	if !env.UpdatedAt.After(origUpdatedAt) {
		t.Fatal("UpdatedAt was not advanced")
	}

	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	if err != nil {
		t.Fatalf("GetEnvironment(updated) error = %v", err)
	}
	if got.Name != "Updated" {
		t.Fatalf("Name = %q, want %q", got.Name, "Updated")
	}
	if got.Slug != "updated" {
		t.Fatalf("Slug = %q, want %q", got.Slug, "updated")
	}
	if len(got.Variables) != 2 {
		t.Fatalf("Variables len = %d, want 2", len(got.Variables))
	}
	if got.Variables["B"] != "3" {
		t.Fatalf("Variable B = %q, want %q", got.Variables["B"], "3")
	}
}

func TestEnvironmentVariablesRequireEncryptionKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	env := &domain.Environment{
		ProjectID: "proj-env-require-encryption-" + newID(),
		Name:      "Secrets",
		Slug:      "secrets",
		Variables: map[string]string{"API_TOKEN": "plaintext-must-not-persist"},
	}
	if err := q.CreateEnvironment(ctx, env); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		t.Fatalf("CreateEnvironment() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
	var count int
	if err := testDB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM environments WHERE slug = $1`, env.Slug).Scan(&count); err != nil {
		t.Fatalf("count rejected environment: %v", err)
	}
	if count != 0 {
		t.Fatalf("environment with unencrypted variables was persisted, count = %d", count)
	}

	empty := &domain.Environment{ProjectID: env.ProjectID, Name: "Empty", Slug: "empty"}
	if err := q.CreateEnvironment(ctx, empty); err != nil {
		t.Fatalf("CreateEnvironment(empty variables) error = %v", err)
	}
	empty.Variables = map[string]string{"API_TOKEN": "plaintext-must-not-persist"}
	if err := q.UpdateEnvironment(ctx, empty); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		t.Fatalf("UpdateEnvironment() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
	var variablesRaw []byte
	if err := testDB.Pool.QueryRow(ctx, `SELECT variables FROM environments WHERE id = $1`, empty.ID).Scan(&variablesRaw); err != nil {
		t.Fatalf("read variables after rejected update: %v", err)
	}
	if string(variablesRaw) != "{}" {
		t.Fatalf("rejected update persisted plaintext variables = %s", variablesRaw)
	}
}

func TestUpdateEnvironment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	env := &domain.Environment{ID: newID(), ProjectID: "proj-missing", Name: "ghost", Slug: "ghost"}
	if err := q.UpdateEnvironment(ctx, env); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("UpdateEnvironment(missing) error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestDeleteEnvironment(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	env := &domain.Environment{
		ProjectID: "proj-delete-env-" + newID(),
		Name:      "to-delete",
		Slug:      "to-delete",
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	if err := q.DeleteEnvironment(ctx, env.ID, env.ProjectID); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}

	_, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	if !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetEnvironment(deleted) error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestDeleteEnvironment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.DeleteEnvironment(ctx, newID(), "proj-missing-"+newID()); !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("DeleteEnvironment(missing) error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestDeleteEnvironment_StandardProtected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-env-std-protect-" + newID()
	if err := q.CreateStandardEnvironments(ctx, projectID); err != nil {
		t.Fatalf("CreateStandardEnvironments() error = %v", err)
	}

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) == 0 {
		t.Fatal("no standard environments created")
	}

	// Deleting a standard environment should return ErrStandardEnvironment.
	if err := q.DeleteEnvironment(ctx, envs[0].ID, projectID); !errors.Is(err, store.ErrStandardEnvironment) {
		t.Fatalf("DeleteEnvironment(standard) error = %v, want ErrStandardEnvironment", err)
	}
}

func TestCreateStandardEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-std-envs-" + newID()
	if err := q.CreateStandardEnvironments(ctx, projectID); err != nil {
		t.Fatalf("CreateStandardEnvironments() error = %v", err)
	}

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListEnvironments() error = %v", err)
	}
	if len(envs) != len(domain.StandardEnvironmentSlugs) {
		t.Fatalf("len = %d, want %d", len(envs), len(domain.StandardEnvironmentSlugs))
	}

	slugs := make(map[string]bool, len(envs))
	for _, e := range envs {
		slugs[e.Slug] = true
		if !e.IsStandard {
			t.Fatalf("env %q IsStandard = false, want true", e.Slug)
		}
		wantName := domain.StandardEnvironmentNames[e.Slug]
		if e.Name != wantName {
			t.Fatalf("env %q Name = %q, want %q", e.Slug, e.Name, wantName)
		}
	}
	for _, slug := range domain.StandardEnvironmentSlugs {
		if !slugs[slug] {
			t.Fatalf("missing standard environment %q", slug)
		}
	}
}

func TestGetResolvedEnvironmentVariables(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-resolve-vars-" + newID()

	parent := &domain.Environment{
		ProjectID: projectID,
		Name:      "Parent",
		Slug:      "parent",
		Variables: map[string]string{"A": "1", "SHARED": "parent"},
	}
	if err := q.CreateEnvironment(ctx, parent); err != nil {
		t.Fatalf("CreateEnvironment(parent) error = %v", err)
	}

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"B": "2", "SHARED": "child"},
	}
	if err := q.CreateEnvironment(ctx, child); err != nil {
		t.Fatalf("CreateEnvironment(child) error = %v", err)
	}

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables() error = %v", err)
	}

	want := map[string]string{"A": "1", "B": "2", "SHARED": "child"}
	if len(resolved) != len(want) {
		t.Fatalf("resolved len = %d, want %d", len(resolved), len(want))
	}
	for k, v := range want {
		if resolved[k] != v {
			t.Fatalf("resolved[%q] = %q, want %q", k, resolved[k], v)
		}
	}

	// Root only.
	rootVars, err := q.GetResolvedEnvironmentVariables(ctx, parent.ID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables(parent) error = %v", err)
	}
	if len(rootVars) != 2 {
		t.Fatalf("root vars len = %d, want 2", len(rootVars))
	}
}

func TestEnvironmentVariablesEncryptedWithoutPlaintextCopy(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	env := &domain.Environment{
		ProjectID: "proj-env-encrypted-" + newID(),
		Name:      "Encrypted",
		Slug:      "encrypted",
		Variables: map[string]string{"API_TOKEN": "super-secret-token"},
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}

	var variablesRaw []byte
	var encryptedLen int
	if err := testDB.Pool.QueryRow(ctx, `
		SELECT variables, length(variables_encrypted)
		FROM environments
		WHERE id = $1`, env.ID).Scan(&variablesRaw, &encryptedLen); err != nil {
		t.Fatalf("read raw environment variables: %v", err)
	}
	if string(variablesRaw) != "{}" {
		t.Fatalf("plaintext variables = %s, want empty object", variablesRaw)
	}
	if encryptedLen == 0 {
		t.Fatal("variables_encrypted was not populated")
	}

	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if got.Variables["API_TOKEN"] != "super-secret-token" {
		t.Fatalf("decrypted API_TOKEN = %q", got.Variables["API_TOKEN"])
	}
}

func TestEnvironmentVariablesDoNotFallbackToPlaintextOnDecryptFailure(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	env := &domain.Environment{
		ProjectID: "proj-env-no-plaintext-fallback-" + newID(),
		Name:      "No Fallback",
		Slug:      "no-fallback",
		Variables: map[string]string{"API_TOKEN": "encrypted-token"},
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE environments
		SET variables = $2, variables_encrypted = $3
		WHERE id = $1`,
		env.ID,
		[]byte(`{"API_TOKEN":"plaintext-fallback"}`),
		[]byte(`not-valid-ciphertext`),
	); err != nil {
		t.Fatalf("tamper environment variables: %v", err)
	}

	if _, err := q.GetEnvironment(ctx, env.ID, env.ProjectID); err == nil {
		t.Fatal("GetEnvironment() error = nil, want decrypt failure instead of plaintext fallback")
	}
	qWithoutKey := mustStore(t)
	if _, err := qWithoutKey.GetEnvironment(ctx, env.ID, env.ProjectID); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		t.Fatalf("GetEnvironment(without key) error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
}

func TestEnvironmentVariablesRejectLegacyPlaintextWithoutCiphertext(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	env := &domain.Environment{
		ProjectID: "proj-env-legacy-plaintext-" + newID(),
		Name:      "Legacy Plaintext",
		Slug:      "legacy-plaintext",
	}
	if err := q.CreateEnvironment(ctx, env); err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE environments
		SET variables = $2, variables_encrypted = NULL
		WHERE id = $1`,
		env.ID,
		[]byte(`{"API_TOKEN":"legacy-plaintext"}`),
	); err != nil {
		t.Fatalf("tamper environment variables: %v", err)
	}

	if _, err := q.GetEnvironment(ctx, env.ID, env.ProjectID); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		t.Fatalf("GetEnvironment() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
	if _, err := q.GetResolvedEnvironmentVariables(ctx, env.ID); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		t.Fatalf("GetResolvedEnvironmentVariables() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
}

func TestResolvedEnvironmentVariablesUseEncryptedParentChain(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-env-encrypted-chain-" + newID()
	parent := &domain.Environment{
		ProjectID: projectID,
		Name:      "Parent",
		Slug:      "parent",
		Variables: map[string]string{"API_TOKEN": "parent-token", "SHARED": "parent"},
	}
	if err := q.CreateEnvironment(ctx, parent); err != nil {
		t.Fatalf("CreateEnvironment(parent) error = %v", err)
	}
	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"SHARED": "child"},
	}
	if err := q.CreateEnvironment(ctx, child); err != nil {
		t.Fatalf("CreateEnvironment(child) error = %v", err)
	}

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables() error = %v", err)
	}
	want := map[string]string{"API_TOKEN": "parent-token", "SHARED": "child"}
	if gotJSON, _ := json.Marshal(resolved); len(resolved) != len(want) {
		t.Fatalf("resolved = %s, want %v", gotJSON, want)
	}
	for key, value := range want {
		if resolved[key] != value {
			t.Fatalf("resolved[%q] = %q, want %q", key, resolved[key], value)
		}
	}
}

func TestGetResolvedEnvironmentVariables_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetResolvedEnvironmentVariables(ctx, newID())
	if !errors.Is(err, store.ErrEnvironmentNotFound) {
		t.Fatalf("GetResolvedEnvironmentVariables(missing) error = %v, want ErrEnvironmentNotFound", err)
	}
}

// TestGetResolvedEnvironmentVariables_MaxDepthChainResolves builds a chain of
// exactly the resolver's CTE depth ceiling (10) and confirms it resolves
// without error. Previously, the truncation guard inspected the leaf's
// parent_id instead of the deepest returned ancestor's, so any chain whose
// leaf had a parent — including valid full-length ones — wrongly tripped
// "exceeded max inheritance depth".
func TestGetResolvedEnvironmentVariables_MaxDepthChainResolves(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	projectID := "proj-env-maxdepth-ok-" + newID()
	const chainLen = 10 // matches the maxDepth constant in environments.go

	var parentID string
	var leafID string
	for i := range chainLen {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "depth-" + strconv.Itoa(i),
			Slug:      "depth-" + strconv.Itoa(i) + "-" + newID(),
			ParentID:  parentID,
			Variables: map[string]string{"LEVEL": strconv.Itoa(i)},
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment(level %d) error = %v", i, err)
		}
		parentID = env.ID
		leafID = env.ID
	}

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, leafID)
	if err != nil {
		t.Fatalf("GetResolvedEnvironmentVariables(leaf) error = %v, want nil for a chain at exactly maxDepth", err)
	}
	// The leaf's overlay wins; the variables map should reflect the deepest
	// child's LEVEL value.
	if got, want := resolved["LEVEL"], strconv.Itoa(chainLen-1); got != want {
		t.Fatalf("resolved[LEVEL] = %q, want %q (leaf overlay)", got, want)
	}
}

// TestGetResolvedEnvironmentVariables_TruncatedChainErrors builds a chain
// longer than maxDepth and confirms the resolver detects the truncation by
// inspecting the deepest returned ancestor (whose parent_id is non-null
// because the CTE stopped before reaching its parent).
func TestGetResolvedEnvironmentVariables_TruncatedChainErrors(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-env-maxdepth-overflow-" + newID()
	const chainLen = 12 // strictly longer than maxDepth=10

	var parentID string
	var leafID string
	for i := range chainLen {
		env := &domain.Environment{
			ProjectID: projectID,
			Name:      "depth-" + strconv.Itoa(i),
			Slug:      "depth-" + strconv.Itoa(i) + "-" + newID(),
			ParentID:  parentID,
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment(level %d) error = %v", i, err)
		}
		parentID = env.ID
		leafID = env.ID
	}

	_, err := q.GetResolvedEnvironmentVariables(ctx, leafID)
	if err == nil {
		t.Fatal("GetResolvedEnvironmentVariables(overflow) error = nil, want exceeded max inheritance depth")
	}
}
