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

	"github.com/stretchr/testify/require"
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
	require.NoError(t, q.CreateEnvironment(ctx,
		env))
	require.NotEqual(t, "",

		env.ID)
	require.False(t, env.CreatedAt.
		IsZero())
	require.False(t, env.UpdatedAt.
		IsZero())

	// Round-trip.
	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	require.NoError(t, err)
	require.Equal(t, env.ID,

		got.ID)
	require.Equal(t, env.ProjectID,

		got.
			ProjectID,
	)
	require.Equal(t, env.Name,

		got.Name,
	)
	require.Equal(t, env.Slug,

		got.Slug,
	)
	require.Equal(t, "", got.
		ParentID,
	)
	require.False(t, got.IsStandard)
	require.Len(t, got.Variables,

		2)

	for k, want := range env.Variables {
		require.Equal(t, want,
			got.
				Variables[k])

	}
}

func TestCreateEnvironment_WithParent(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-env-parent-" + newID()
	parent := &domain.Environment{ProjectID: projectID, Name: "Base", Slug: "base"}
	require.NoError(t, q.CreateEnvironment(ctx,
		parent))

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		child))

	got, err := q.GetEnvironment(ctx, child.ID, projectID)
	require.NoError(t, err)
	require.Equal(t, parent.
		ID, got.ParentID,
	)

}

func TestGetEnvironment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetEnvironment(ctx, newID(), "proj-missing")
	require.True(t, errors.Is(err, store.
		ErrEnvironmentNotFound,
	))

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
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

	}

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, envs, 3)

	for i := 1; i < len(envs); i++ {
		require.False(t, envs[i-
			1].CreatedAt.
			Before(
				envs[i].CreatedAt,
			))

	}

	// Cursor pagination.
	page1, err := q.ListEnvironments(ctx, projectID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	cursor := page1[1].CreatedAt
	page2, err := q.ListEnvironments(ctx, projectID, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)

	// Empty project.
	empty, err := q.ListEnvironments(ctx, "proj-envs-empty-"+newID(), 100, nil)
	require.NoError(t, err)
	require.Len(t, empty, 0)

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
	require.NoError(t, q.CreateEnvironment(ctx,
		env))

	origUpdatedAt := env.UpdatedAt

	env.Name = "Updated"
	env.Slug = "updated"
	env.Variables = map[string]string{"A": "2", "B": "3"}
	require.NoError(t, q.UpdateEnvironment(ctx,
		env))
	require.True(t, env.UpdatedAt.
		After(origUpdatedAt))

	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	require.NoError(t, err)
	require.Equal(t, "Updated",

		got.Name,
	)
	require.Equal(t, "updated",

		got.Slug,
	)
	require.Len(t, got.Variables,

		2)
	require.Equal(t, "3", got.
		Variables["B"])

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
		require.Failf(t, "test failure",

			"CreateEnvironment() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
	var count int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM environments WHERE slug = $1`,

		env.Slug).Scan(&count),
	)
	require.EqualValues(t, 0, count)

	empty := &domain.Environment{ProjectID: env.ProjectID, Name: "Empty", Slug: "empty"}
	require.NoError(t, q.CreateEnvironment(ctx,
		empty))

	empty.Variables = map[string]string{"API_TOKEN": "plaintext-must-not-persist"}
	if err := q.UpdateEnvironment(ctx, empty); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		require.Failf(t, "test failure",

			"UpdateEnvironment() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
	var variablesRaw []byte
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`SELECT variables FROM environments WHERE id = $1`,

		empty.ID).Scan(&variablesRaw))
	require.Equal(t, "{}",
		string(variablesRaw))

}

func TestUpdateEnvironment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	env := &domain.Environment{ID: newID(), ProjectID: "proj-missing", Name: "ghost", Slug: "ghost"}
	if err := q.UpdateEnvironment(ctx, env); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"UpdateEnvironment(missing) error = %v, want ErrEnvironmentNotFound", err)
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
	require.NoError(t, q.CreateEnvironment(ctx,
		env))
	require.NoError(t, q.DeleteEnvironment(ctx, env.ID, env.ProjectID))

	_, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	require.True(t, errors.Is(err, store.
		ErrEnvironmentNotFound,
	))

}

func TestDeleteEnvironment_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	if err := q.DeleteEnvironment(ctx, newID(), "proj-missing"); !errors.Is(err, store.ErrEnvironmentNotFound) {
		require.Failf(t, "test failure",

			"DeleteEnvironment(missing) error = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestDeleteEnvironment_StandardProtected(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-env-std-protect-" + newID()
	require.NoError(t, q.CreateStandardEnvironments(ctx, projectID))

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.NotEmpty(t, envs)

	// Deleting a standard environment should return ErrStandardEnvironment.
	if err := q.DeleteEnvironment(ctx, envs[0].ID, projectID); !errors.Is(err, store.ErrStandardEnvironment) {
		require.Failf(t, "test failure",

			"DeleteEnvironment(standard) error = %v, want ErrStandardEnvironment", err)
	}
}

func TestCreateStandardEnvironments(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-std-envs-" + newID()
	require.NoError(t, q.CreateStandardEnvironments(ctx, projectID))

	envs, err := q.ListEnvironments(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, envs, len(domain.StandardEnvironmentSlugs))

	slugs := make(map[string]bool, len(envs))
	for _, e := range envs {
		slugs[e.Slug] = true
		require.True(t, e.IsStandard)

		wantName := domain.StandardEnvironmentNames[e.Slug]
		require.Equal(t, wantName,

			e.Name,
		)

	}
	for _, slug := range domain.StandardEnvironmentSlugs {
		require.True(t, slugs[slug])

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
	require.NoError(t, q.CreateEnvironment(ctx,
		parent))

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"B": "2", "SHARED": "child"},
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		child))

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, child.ProjectID, child.ID)
	require.NoError(t, err)

	want := map[string]string{"A": "1", "B": "2", "SHARED": "child"}
	require.Len(t, resolved,

		len(want))

	for k, v := range want {
		require.Equal(t, v, resolved[k])

	}

	// Root only.
	rootVars, err := q.GetResolvedEnvironmentVariables(ctx, parent.ProjectID, parent.ID)
	require.NoError(t, err)
	require.Len(t, rootVars,

		2)

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
	require.NoError(t, q.CreateEnvironment(ctx,
		env))

	var variablesRaw []byte
	var encryptedLen int
	require.NoError(t, testDB.
		Pool.QueryRow(ctx,
		`
		SELECT variables, length(variables_encrypted)
		FROM environments
		WHERE id = $1`,

		env.ID).Scan(&variablesRaw,
		&encryptedLen))
	require.Equal(t, "{}",
		string(variablesRaw))
	require.NotEqual(t, 0,
		encryptedLen,
	)

	got, err := q.GetEnvironment(ctx, env.ID, env.ProjectID)
	require.NoError(t, err)
	require.Equal(t, "super-secret-token",

		got.Variables["API_TOKEN"])

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
	require.NoError(t, q.CreateEnvironment(ctx,
		env))

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE environments
		SET variables = $2, variables_encrypted = $3
		WHERE id = $1`,
		env.ID,
		[]byte(`{"API_TOKEN":"plaintext-fallback"}`),
		[]byte(`not-valid-ciphertext`),
	); err != nil {
		require.Failf(t, "test failure",

			"tamper environment variables: %v", err)
	}

	if _, err := q.GetEnvironment(ctx, env.ID, env.ProjectID); err == nil {
		require.Fail(t,

			"GetEnvironment() error = nil, want decrypt failure instead of plaintext fallback")
	}
	qWithoutKey := mustStore(t)
	if _, err := qWithoutKey.GetEnvironment(ctx, env.ID, env.ProjectID); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		require.Failf(t, "test failure",

			"GetEnvironment(without key) error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
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
	require.NoError(t, q.CreateEnvironment(ctx,
		env))

	if _, err := testDB.Pool.Exec(ctx, `
		UPDATE environments
		SET variables = $2, variables_encrypted = NULL
		WHERE id = $1`,
		env.ID,
		[]byte(`{"API_TOKEN":"legacy-plaintext"}`),
	); err != nil {
		require.Failf(t, "test failure",

			"tamper environment variables: %v", err)
	}

	if _, err := q.GetEnvironment(ctx, env.ID, env.ProjectID); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		require.Failf(t, "test failure",

			"GetEnvironment() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
	}
	if _, err := q.GetResolvedEnvironmentVariables(ctx, env.ProjectID, env.ID); !errors.Is(err, store.ErrEnvironmentVariableEncryptionRequired) {
		require.Failf(t, "test failure",

			"GetResolvedEnvironmentVariables() error = %v, want ErrEnvironmentVariableEncryptionRequired", err)
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
	require.NoError(t, q.CreateEnvironment(ctx,
		parent))

	child := &domain.Environment{
		ProjectID: projectID,
		Name:      "Child",
		Slug:      "child",
		ParentID:  parent.ID,
		Variables: map[string]string{"SHARED": "child"},
	}
	require.NoError(t, q.CreateEnvironment(ctx,
		child))

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, child.ProjectID, child.ID)
	require.NoError(t, err)

	want := map[string]string{"API_TOKEN": "parent-token", "SHARED": "child"}
	if gotJSON, _ := json.Marshal(resolved); len(resolved) != len(want) {
		require.Failf(t, "test failure",

			"resolved = %s, want %v", gotJSON, want)
	}
	for key, value := range want {
		require.Equal(t, value,

			resolved[key])

	}
}

func TestGetResolvedEnvironmentVariables_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetResolvedEnvironmentVariables(ctx, newID(), newID())
	require.True(t, errors.Is(err, store.
		ErrEnvironmentNotFound,
	))

}

// TestGetResolvedEnvironmentVariables_CrossTenantSeedReturnsNotFound is the
// regression guard for the unscoped-resolve finding: resolving an environment
// id while scoped to a different project must not return or decrypt that
// environment's variables. The seed row is filtered by project_id so a snapshot
// id leaked across tenants cannot recover another tenant's secrets.
func TestGetResolvedEnvironmentVariables_CrossTenantSeedReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	q.SetSecretEncryptionKey("0123456789abcdef0123456789abcdef")
	mustClean(t, ctx)

	ownerProject := "proj-owner-" + newID()
	attackerProject := "proj-attacker-" + newID()

	env := &domain.Environment{
		ProjectID: ownerProject,
		Name:      "Secrets",
		Slug:      "secrets",
		Variables: map[string]string{"API_TOKEN": "owner-secret"},
	}
	require.NoError(t, q.CreateEnvironment(ctx, env))

	// Same id, attacker's project: must not resolve the owner's chain.
	_, err := q.GetResolvedEnvironmentVariables(ctx, attackerProject, env.ID)
	require.ErrorIs(t, err, store.ErrEnvironmentNotFound)

	// Sanity: the owning project still resolves correctly.
	resolved, err := q.GetResolvedEnvironmentVariables(ctx, ownerProject, env.ID)
	require.NoError(t, err)
	require.Equal(t, "owner-secret", resolved["API_TOKEN"])
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
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

		parentID = env.ID
		leafID = env.ID
	}

	resolved, err := q.GetResolvedEnvironmentVariables(ctx, projectID, leafID)
	require.NoError(t, err)

	// The leaf's overlay wins; the variables map should reflect the deepest
	// child's LEVEL value.
	if got, want := resolved["LEVEL"], strconv.Itoa(chainLen-1); got != want {
		require.Failf(t, "test failure",

			"resolved[LEVEL] = %q, want %q (leaf overlay)", got, want)
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
		require.NoError(t, q.CreateEnvironment(ctx,
			env))

		parentID = env.ID
		leafID = env.ID
	}

	_, err := q.GetResolvedEnvironmentVariables(ctx, projectID, leafID)
	require.Error(t, err)

}
