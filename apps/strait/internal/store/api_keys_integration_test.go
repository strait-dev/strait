//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCreateAPIKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	key := &domain.APIKey{
		ProjectID:          "proj-create-apikey-" + newID(),
		OrgID:              "org-create-apikey-" + newID(),
		Name:               "test-key",
		KeyHash:            "hash-" + newID(),
		KeyPrefix:          "sk_test_",
		Scopes:             []string{"jobs:read", "jobs:trigger"},
		ExpiresAt:          &expires,
		EnvironmentID:      "env-" + newID(),
		RotationWebhookURL: "https://example.com/rotate",
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))
	require.NotEqual(t, "",

		key.ID)
	require.False(t, key.CreatedAt.
		IsZero())

	// Verify round-trip via GetAPIKeyByID.
	got, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, key.ID,

		got.ID)
	require.Equal(t, key.ProjectID,

		got.
			ProjectID,
	)
	require.Equal(t, key.OrgID,

		got.OrgID,
	)
	require.Equal(t, key.Name,

		got.Name,
	)
	require.Equal(t, key.KeyHash,

		got.
			KeyHash)
	require.Equal(t, key.KeyPrefix,

		got.
			KeyPrefix,
	)
	require.Len(t, got.Scopes,

		len(key.
			Scopes))
	require.False(t, got.ExpiresAt ==
		nil || !got.
		ExpiresAt.
		Equal(*key.
			ExpiresAt))
	require.Equal(t, key.EnvironmentID,

		got.EnvironmentID,
	)
	require.Equal(t, key.RotationWebhookURL,

		got.
			RotationWebhookURL,
	)

}

func TestCreateAPIKey_DuplicateHash(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	hash := "hash-dup-" + newID()
	key1 := &domain.APIKey{ProjectID: "proj-dup-hash", Name: "k1", KeyHash: hash, KeyPrefix: "sk_1", Scopes: []string{"read"}}
	require.NoError(t, q.CreateAPIKey(ctx, key1))

	key2 := &domain.APIKey{ProjectID: "proj-dup-hash", Name: "k2", KeyHash: hash, KeyPrefix: "sk_2", Scopes: []string{"read"}}
	require.Error(t, q.CreateAPIKey(ctx,
		key2))

}

func TestGetAPIKeyByHash(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{
		ProjectID: "proj-get-hash-" + newID(),
		Name:      "by-hash",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_h",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))

	got, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	require.NoError(t, err)
	require.Equal(t, key.ID,

		got.ID)

	// Not found.
	_, err = q.GetAPIKeyByHash(ctx, "missing-hash-"+newID())
	require.Error(t, err)

}

func TestAPIKeyCacheVersion_RoundTripAndMutationBump(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{
		ProjectID: "proj-cache-version-" + newID(),
		Name:      "cache-version",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_cv",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))
	require.EqualValues(t, 1, key.
		CacheVersion,
	)

	byHash, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	require.NoError(t, err)
	require.EqualValues(t, 1, byHash.
		CacheVersion,
	)
	require.NoError(t, q.RevokeAPIKey(ctx, key.ID))

	revoked, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, revoked.
		CacheVersion,
	)

}

func TestGetAPIKeyByID_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetAPIKeyByID(ctx, newID())
	require.Error(t, err)

}

func TestListAPIKeysByProject(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list-apikeys-" + newID()
	otherProjectID := "proj-list-apikeys-other-" + newID()

	for i := range 3 {
		key := &domain.APIKey{
			ProjectID: projectID,
			Name:      "key-" + newID(),
			KeyHash:   "hash-" + newID(),
			KeyPrefix: "sk_",
			Scopes:    []string{"jobs:read"},
		}
		_ = i
		require.NoError(t, q.CreateAPIKey(ctx, key))

		time.Sleep(5 * time.Millisecond)
	}

	// Key in other project.
	other := &domain.APIKey{
		ProjectID: otherProjectID,
		Name:      "other",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_other",
		Scopes:    []string{},
	}
	require.NoError(t, q.CreateAPIKey(ctx, other))

	keys, err := q.ListAPIKeysByProject(ctx, projectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, keys, 3)

	for i := 1; i < len(keys); i++ {
		require.False(t, keys[i-
			1].CreatedAt.
			Before(
				keys[i].CreatedAt,
			))

	}

	// Cursor pagination.
	page1, err := q.ListAPIKeysByProject(ctx, projectID, 2, nil)
	require.NoError(t, err)
	require.Len(t, page1, 2)

	cursor := page1[1].CreatedAt
	page2, err := q.ListAPIKeysByProject(ctx, projectID, 2, &cursor)
	require.NoError(t, err)
	require.Len(t, page2, 1)

}

func TestListAPIKeysByOrg(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-list-apikeys-" + newID()
	otherOrgID := "org-list-apikeys-other-" + newID()

	key1 := &domain.APIKey{
		ProjectID: "proj-orgkeys-" + newID(),
		OrgID:     orgID,
		Name:      "k1",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_1",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key1))

	key2 := &domain.APIKey{
		ProjectID: "proj-orgkeys-" + newID(),
		OrgID:     orgID,
		Name:      "k2",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_2",
		Scopes:    []string{},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key2))

	// Key in other org.
	otherKey := &domain.APIKey{
		ProjectID: "proj-orgkeys-other-" + newID(),
		OrgID:     otherOrgID,
		Name:      "other",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_other",
		Scopes:    []string{},
	}
	require.NoError(t, q.CreateAPIKey(ctx, otherKey))

	keys, err := q.ListAPIKeysByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.Len(t, keys, 2)

	for _, k := range keys {
		require.Equal(t, orgID,

			k.OrgID)

	}
}

func TestListAPIKeysByOrg_CanRunWithClearedProjectRLSContext(t *testing.T) {
	ctx := context.Background()
	admin := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-rls-apikeys-" + newID()
	key1 := &domain.APIKey{
		ProjectID: "proj-orgkeys-rls-1-" + newID(),
		OrgID:     orgID,
		Name:      "k1",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_1",
		Scopes:    []string{"api-keys:manage"},
	}
	key2 := &domain.APIKey{
		ProjectID: "proj-orgkeys-rls-2-" + newID(),
		OrgID:     orgID,
		Name:      "k2",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_2",
		Scopes:    []string{"api-keys:manage"},
	}
	for _, key := range []*domain.APIKey{key1, key2} {
		require.NoError(t, admin.
			CreateAPIKey(ctx, key))

	}

	tx, err := testDB.Pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", key1.ProjectID); err != nil {
		require.Failf(t, "test failure",

			"set project context: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE strait_app"); err != nil {
		require.Failf(t, "test failure",

			"set local role strait_app: %v", err)
	}
	routed := store.New(tx)
	projectScoped, err := routed.ListAPIKeysByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.False(t, len(projectScoped) != 1 ||
		projectScoped[0].ProjectID !=
			key1.ProjectID)
	require.NoError(t, routed.
		ClearProjectContext(ctx))

	orgScoped, err := routed.ListAPIKeysByOrg(ctx, orgID, 100, nil)
	require.NoError(t, err)
	require.Len(t, orgScoped,

		2)

}

func TestRevokeAPIKey(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{
		ProjectID: "proj-revoke-" + newID(),
		Name:      "revoke-me",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_rev",
		Scopes:    []string{"jobs:trigger"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))
	require.NoError(t, q.RevokeAPIKey(ctx, key.ID))

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)

	// Revoked keys excluded from list.
	keys, err := q.ListAPIKeysByProject(ctx, key.ProjectID, 100, nil)
	require.NoError(t, err)
	require.Len(t, keys, 0)
	require.Error(t, q.RevokeAPIKey(ctx,
		key.ID),
	)
	require.Error(t, q.RevokeAPIKey(ctx,
		newID(),
	))

	// Double revoke returns error.

	// Revoke nonexistent.

}

func TestTouchAPIKeyLastUsed(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	key := &domain.APIKey{
		ProjectID: "proj-touch-" + newID(),
		Name:      "touch",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_t",
		Scopes:    []string{"jobs:read"},
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))

	before, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, before.
		LastUsedAt,
	)
	require.NoError(t, q.TouchAPIKeyLastUsed(ctx,
		key.ID))

	after, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, after.
		LastUsedAt,
	)

}

func TestMarkAPIKeyRotated_Lifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rotate-" + newID()
	old := &domain.APIKey{ProjectID: projectID, Name: "old", KeyHash: "hash-" + newID(), KeyPrefix: "sk_old", Scopes: []string{"jobs:read"}}
	new_ := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	require.NoError(t, q.CreateAPIKey(ctx, old))
	require.NoError(t, q.CreateAPIKey(ctx, new_))

	grace := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	require.NoError(t, q.MarkAPIKeyRotated(ctx,
		old.ID, new_.
			ID, grace,
	))

	got, err := q.GetAPIKeyByID(ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, new_.ID,

		got.ReplacedByKeyID,
	)
	require.False(t, got.GraceExpiresAt ==
		nil ||
		!got.GraceExpiresAt.
			Equal(grace))

	otherNew := &domain.APIKey{ProjectID: projectID, Name: "new2", KeyHash: "hash-" + newID(), KeyPrefix: "sk_n2", Scopes: []string{"jobs:read"}}
	require.NoError(t, q.CreateAPIKey(ctx, otherNew))
	require.Error(t, q.MarkAPIKeyRotated(ctx, old.
		ID, otherNew.
		ID,
		grace,
	))

	got, err = q.GetAPIKeyByID(ctx, old.ID)
	require.NoError(t, err)
	require.Equal(t, new_.ID,

		got.ReplacedByKeyID,
	)
	require.NoError(t, q.RevokeAPIKey(ctx, old.ID))
	require.Error(t, q.MarkAPIKeyRotated(ctx, old.
		ID, new_.
		ID, grace,
	))

	// Marking an already-revoked key returns error.

}

func TestListAPIKeysDueRotation(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-due-rotation-" + newID()
	rotDays := 30
	pastRotation := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Microsecond)
	futureRotation := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)

	// Key due for rotation.
	due := &domain.APIKey{
		ProjectID:            projectID,
		Name:                 "due",
		KeyHash:              "hash-" + newID(),
		KeyPrefix:            "sk_due",
		Scopes:               []string{"jobs:read"},
		RotationIntervalDays: &rotDays,
		NextRotationAt:       &pastRotation,
	}
	require.NoError(t, q.CreateAPIKey(ctx, due))

	// Key not yet due.
	notDue := &domain.APIKey{
		ProjectID:            projectID,
		Name:                 "not-due",
		KeyHash:              "hash-" + newID(),
		KeyPrefix:            "sk_nd",
		Scopes:               []string{},
		RotationIntervalDays: &rotDays,
		NextRotationAt:       &futureRotation,
	}
	require.NoError(t, q.CreateAPIKey(ctx, notDue))

	// Key without rotation interval.
	noRot := &domain.APIKey{
		ProjectID: projectID,
		Name:      "no-rot",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_nr",
		Scopes:    []string{},
	}
	require.NoError(t, q.CreateAPIKey(ctx, noRot))

	keys, err := q.ListAPIKeysDueRotation(ctx)
	require.NoError(t, err)

	found := false
	for _, k := range keys {
		if k.ID == due.ID {
			found = true
		}
		require.NotEqual(t, notDue.
			ID, k.
			ID)
		require.NotEqual(t, noRot.
			ID, k.ID,
		)

	}
	require.True(t, found)

}

func TestDisableAPIKeyAutoRotationClearsSchedulerEligibility(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	rotDays := 30
	pastRotation := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Microsecond)
	key := &domain.APIKey{
		ProjectID:            "proj-disable-rotation-" + newID(),
		Name:                 "due-without-webhook",
		KeyHash:              "hash-" + newID(),
		KeyPrefix:            "sk_dis",
		Scopes:               []string{"jobs:read"},
		RotationIntervalDays: &rotDays,
		NextRotationAt:       &pastRotation,
	}
	require.NoError(t, q.CreateAPIKey(ctx, key))
	require.NoError(t, q.DisableAPIKeyAutoRotation(ctx, key.
		ID))

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, got.
		NextRotationAt,
	)

	keys, err := q.ListAPIKeysDueRotation(ctx)
	require.NoError(t, err)

	for _, due := range keys {
		require.NotEqual(t, key.
			ID, due.ID,
		)

	}
}

func TestListAPIKeysExpiringSoon(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-expiring-" + newID()

	// Key expiring in 3 days.
	soonExpiry := time.Now().UTC().Add(3 * 24 * time.Hour).Truncate(time.Microsecond)
	soon := &domain.APIKey{
		ProjectID: projectID,
		Name:      "soon",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_soon",
		Scopes:    []string{"jobs:read"},
		ExpiresAt: &soonExpiry,
	}
	require.NoError(t, q.CreateAPIKey(ctx, soon))

	// Key expiring far in the future.
	farExpiry := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Microsecond)
	far := &domain.APIKey{
		ProjectID: projectID,
		Name:      "far",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_far",
		Scopes:    []string{},
		ExpiresAt: &farExpiry,
	}
	require.NoError(t, q.CreateAPIKey(ctx, far))

	// Key with no expiry (included because query uses IS NULL OR).
	noExpiry := &domain.APIKey{
		ProjectID: projectID,
		Name:      "no-exp",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_ne",
		Scopes:    []string{},
	}
	require.NoError(t, q.CreateAPIKey(ctx, noExpiry))

	keys, err := q.ListAPIKeysExpiringSoon(ctx, projectID, 7)
	require.NoError(t, err)

	ids := make(map[string]bool, len(keys))
	for _, k := range keys {
		ids[k.ID] = true
	}
	require.True(t, ids[soon.
		ID])
	require.True(t, ids[noExpiry.
		ID])
	require.False(t, ids[far.
		ID])

}
