//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
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

	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	if key.ID == "" {
		t.Fatal("CreateAPIKey() did not set ID")
	}
	if key.CreatedAt.IsZero() {
		t.Fatal("CreateAPIKey() did not set CreatedAt")
	}

	// Verify round-trip via GetAPIKeyByID.
	got, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID() error = %v", err)
	}
	if got.ID != key.ID {
		t.Fatalf("ID = %q, want %q", got.ID, key.ID)
	}
	if got.ProjectID != key.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, key.ProjectID)
	}
	if got.OrgID != key.OrgID {
		t.Fatalf("OrgID = %q, want %q", got.OrgID, key.OrgID)
	}
	if got.Name != key.Name {
		t.Fatalf("Name = %q, want %q", got.Name, key.Name)
	}
	if got.KeyHash != key.KeyHash {
		t.Fatalf("KeyHash = %q, want %q", got.KeyHash, key.KeyHash)
	}
	if got.KeyPrefix != key.KeyPrefix {
		t.Fatalf("KeyPrefix = %q, want %q", got.KeyPrefix, key.KeyPrefix)
	}
	if len(got.Scopes) != len(key.Scopes) {
		t.Fatalf("Scopes len = %d, want %d", len(got.Scopes), len(key.Scopes))
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(*key.ExpiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", got.ExpiresAt, key.ExpiresAt)
	}
	if got.EnvironmentID != key.EnvironmentID {
		t.Fatalf("EnvironmentID = %q, want %q", got.EnvironmentID, key.EnvironmentID)
	}
	if got.RotationWebhookURL != key.RotationWebhookURL {
		t.Fatalf("RotationWebhookURL = %q, want %q", got.RotationWebhookURL, key.RotationWebhookURL)
	}
}

func TestCreateAPIKey_DuplicateHash(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	hash := "hash-dup-" + newID()
	key1 := &domain.APIKey{ProjectID: "proj-dup-hash", Name: "k1", KeyHash: hash, KeyPrefix: "sk_1", Scopes: []string{"read"}}
	if err := q.CreateAPIKey(ctx, key1); err != nil {
		t.Fatalf("CreateAPIKey(key1) error = %v", err)
	}

	key2 := &domain.APIKey{ProjectID: "proj-dup-hash", Name: "k2", KeyHash: hash, KeyPrefix: "sk_2", Scopes: []string{"read"}}
	if err := q.CreateAPIKey(ctx, key2); err == nil {
		t.Fatal("CreateAPIKey(duplicate hash) error = nil, want error")
	}
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
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	got, err := q.GetAPIKeyByHash(ctx, key.KeyHash)
	if err != nil {
		t.Fatalf("GetAPIKeyByHash() error = %v", err)
	}
	if got.ID != key.ID {
		t.Fatalf("ID = %q, want %q", got.ID, key.ID)
	}

	// Not found.
	_, err = q.GetAPIKeyByHash(ctx, "missing-hash-"+newID())
	if err == nil {
		t.Fatal("GetAPIKeyByHash(missing) error = nil, want error")
	}
}

func TestGetAPIKeyByID_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetAPIKeyByID(ctx, newID())
	if err == nil {
		t.Fatal("GetAPIKeyByID(missing) error = nil, want error")
	}
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
		if err := q.CreateAPIKey(ctx, key); err != nil {
			t.Fatalf("CreateAPIKey(%d) error = %v", i, err)
		}
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
	if err := q.CreateAPIKey(ctx, other); err != nil {
		t.Fatalf("CreateAPIKey(other) error = %v", err)
	}

	keys, err := q.ListAPIKeysByProject(ctx, projectID, 100, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject() error = %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("len = %d, want 3", len(keys))
	}

	// Verify DESC ordering.
	for i := 1; i < len(keys); i++ {
		if keys[i-1].CreatedAt.Before(keys[i].CreatedAt) {
			t.Fatalf("keys not DESC at index %d", i)
		}
	}

	// Cursor pagination.
	page1, err := q.ListAPIKeysByProject(ctx, projectID, 2, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject(page1) error = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}

	cursor := page1[1].CreatedAt
	page2, err := q.ListAPIKeysByProject(ctx, projectID, 2, &cursor)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject(page2) error = %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2))
	}
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
	if err := q.CreateAPIKey(ctx, key1); err != nil {
		t.Fatalf("CreateAPIKey(k1) error = %v", err)
	}

	key2 := &domain.APIKey{
		ProjectID: "proj-orgkeys-" + newID(),
		OrgID:     orgID,
		Name:      "k2",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_2",
		Scopes:    []string{},
	}
	if err := q.CreateAPIKey(ctx, key2); err != nil {
		t.Fatalf("CreateAPIKey(k2) error = %v", err)
	}

	// Key in other org.
	otherKey := &domain.APIKey{
		ProjectID: "proj-orgkeys-other-" + newID(),
		OrgID:     otherOrgID,
		Name:      "other",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_other",
		Scopes:    []string{},
	}
	if err := q.CreateAPIKey(ctx, otherKey); err != nil {
		t.Fatalf("CreateAPIKey(other) error = %v", err)
	}

	keys, err := q.ListAPIKeysByOrg(ctx, orgID, 100, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByOrg() error = %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("len = %d, want 2", len(keys))
	}
	for _, k := range keys {
		if k.OrgID != orgID {
			t.Fatalf("OrgID = %q, want %q", k.OrgID, orgID)
		}
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
		if err := admin.CreateAPIKey(ctx, key); err != nil {
			t.Fatalf("CreateAPIKey(%s) error = %v", key.Name, err)
		}
	}

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_project_id', $1, true)", key1.ProjectID); err != nil {
		t.Fatalf("set project context: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE strait_app"); err != nil {
		t.Fatalf("set local role strait_app: %v", err)
	}
	routed := store.New(tx)
	projectScoped, err := routed.ListAPIKeysByOrg(ctx, orgID, 100, nil)
	if err != nil {
		t.Fatalf("project-scoped ListAPIKeysByOrg() error = %v", err)
	}
	if len(projectScoped) != 1 || projectScoped[0].ProjectID != key1.ProjectID {
		t.Fatalf("project-scoped keys = %+v, want only %s", projectScoped, key1.ProjectID)
	}

	if err := routed.ClearProjectContext(ctx); err != nil {
		t.Fatalf("ClearProjectContext() error = %v", err)
	}
	orgScoped, err := routed.ListAPIKeysByOrg(ctx, orgID, 100, nil)
	if err != nil {
		t.Fatalf("org-scoped ListAPIKeysByOrg() error = %v", err)
	}
	if len(orgScoped) != 2 {
		t.Fatalf("org-scoped keys = %d, want 2", len(orgScoped))
	}
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
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	if err := q.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("RevokeAPIKey() error = %v", err)
	}

	got, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(revoked) error = %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("RevokedAt = nil, want non-nil")
	}

	// Revoked keys excluded from list.
	keys, err := q.ListAPIKeysByProject(ctx, key.ProjectID, 100, nil)
	if err != nil {
		t.Fatalf("ListAPIKeysByProject() error = %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("len = %d, want 0 (revoked excluded)", len(keys))
	}

	// Double revoke returns error.
	if err := q.RevokeAPIKey(ctx, key.ID); err == nil {
		t.Fatal("RevokeAPIKey(already) error = nil, want error")
	}

	// Revoke nonexistent.
	if err := q.RevokeAPIKey(ctx, newID()); err == nil {
		t.Fatal("RevokeAPIKey(missing) error = nil, want error")
	}
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
	if err := q.CreateAPIKey(ctx, key); err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}

	before, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(before) error = %v", err)
	}
	if before.LastUsedAt != nil {
		t.Fatalf("LastUsedAt before = %v, want nil", before.LastUsedAt)
	}

	if err := q.TouchAPIKeyLastUsed(ctx, key.ID); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed() error = %v", err)
	}

	after, err := q.GetAPIKeyByID(ctx, key.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(after) error = %v", err)
	}
	if after.LastUsedAt == nil {
		t.Fatal("LastUsedAt after = nil, want non-nil")
	}
}

func TestMarkAPIKeyRotated_Lifecycle(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-rotate-" + newID()
	old := &domain.APIKey{ProjectID: projectID, Name: "old", KeyHash: "hash-" + newID(), KeyPrefix: "sk_old", Scopes: []string{"jobs:read"}}
	new_ := &domain.APIKey{ProjectID: projectID, Name: "new", KeyHash: "hash-" + newID(), KeyPrefix: "sk_new", Scopes: []string{"jobs:read"}}
	if err := q.CreateAPIKey(ctx, old); err != nil {
		t.Fatalf("CreateAPIKey(old) error = %v", err)
	}
	if err := q.CreateAPIKey(ctx, new_); err != nil {
		t.Fatalf("CreateAPIKey(new) error = %v", err)
	}

	grace := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Microsecond)
	if err := q.MarkAPIKeyRotated(ctx, old.ID, new_.ID, grace); err != nil {
		t.Fatalf("MarkAPIKeyRotated() error = %v", err)
	}

	got, err := q.GetAPIKeyByID(ctx, old.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(old) error = %v", err)
	}
	if got.ReplacedByKeyID != new_.ID {
		t.Fatalf("ReplacedByKeyID = %q, want %q", got.ReplacedByKeyID, new_.ID)
	}
	if got.GraceExpiresAt == nil || !got.GraceExpiresAt.Equal(grace) {
		t.Fatalf("GraceExpiresAt = %v, want %v", got.GraceExpiresAt, grace)
	}

	otherNew := &domain.APIKey{ProjectID: projectID, Name: "new2", KeyHash: "hash-" + newID(), KeyPrefix: "sk_n2", Scopes: []string{"jobs:read"}}
	if err := q.CreateAPIKey(ctx, otherNew); err != nil {
		t.Fatalf("CreateAPIKey(otherNew) error = %v", err)
	}
	if err := q.MarkAPIKeyRotated(ctx, old.ID, otherNew.ID, grace); err == nil {
		t.Fatal("MarkAPIKeyRotated(already rotated) error = nil, want error")
	}
	got, err = q.GetAPIKeyByID(ctx, old.ID)
	if err != nil {
		t.Fatalf("GetAPIKeyByID(old after duplicate) error = %v", err)
	}
	if got.ReplacedByKeyID != new_.ID {
		t.Fatalf("ReplacedByKeyID overwritten = %q, want original %q", got.ReplacedByKeyID, new_.ID)
	}

	// Marking an already-revoked key returns error.
	if err := q.RevokeAPIKey(ctx, old.ID); err != nil {
		t.Fatalf("RevokeAPIKey(old) error = %v", err)
	}
	if err := q.MarkAPIKeyRotated(ctx, old.ID, new_.ID, grace); err == nil {
		t.Fatal("MarkAPIKeyRotated(revoked) error = nil, want error")
	}
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
	if err := q.CreateAPIKey(ctx, due); err != nil {
		t.Fatalf("CreateAPIKey(due) error = %v", err)
	}

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
	if err := q.CreateAPIKey(ctx, notDue); err != nil {
		t.Fatalf("CreateAPIKey(not-due) error = %v", err)
	}

	// Key without rotation interval.
	noRot := &domain.APIKey{
		ProjectID: projectID,
		Name:      "no-rot",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_nr",
		Scopes:    []string{},
	}
	if err := q.CreateAPIKey(ctx, noRot); err != nil {
		t.Fatalf("CreateAPIKey(no-rot) error = %v", err)
	}

	keys, err := q.ListAPIKeysDueRotation(ctx)
	if err != nil {
		t.Fatalf("ListAPIKeysDueRotation() error = %v", err)
	}

	found := false
	for _, k := range keys {
		if k.ID == due.ID {
			found = true
		}
		if k.ID == notDue.ID {
			t.Fatal("ListAPIKeysDueRotation() returned not-due key")
		}
		if k.ID == noRot.ID {
			t.Fatal("ListAPIKeysDueRotation() returned no-rotation key")
		}
	}
	if !found {
		t.Fatal("ListAPIKeysDueRotation() did not return due key")
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
	if err := q.CreateAPIKey(ctx, soon); err != nil {
		t.Fatalf("CreateAPIKey(soon) error = %v", err)
	}

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
	if err := q.CreateAPIKey(ctx, far); err != nil {
		t.Fatalf("CreateAPIKey(far) error = %v", err)
	}

	// Key with no expiry (included because query uses IS NULL OR).
	noExpiry := &domain.APIKey{
		ProjectID: projectID,
		Name:      "no-exp",
		KeyHash:   "hash-" + newID(),
		KeyPrefix: "sk_ne",
		Scopes:    []string{},
	}
	if err := q.CreateAPIKey(ctx, noExpiry); err != nil {
		t.Fatalf("CreateAPIKey(no-exp) error = %v", err)
	}

	keys, err := q.ListAPIKeysExpiringSoon(ctx, projectID, 7)
	if err != nil {
		t.Fatalf("ListAPIKeysExpiringSoon() error = %v", err)
	}

	ids := make(map[string]bool, len(keys))
	for _, k := range keys {
		ids[k.ID] = true
	}

	if !ids[soon.ID] {
		t.Fatal("ListAPIKeysExpiringSoon() missing soon-expiring key")
	}
	if !ids[noExpiry.ID] {
		t.Fatal("ListAPIKeysExpiringSoon() missing no-expiry key")
	}
	if ids[far.ID] {
		t.Fatal("ListAPIKeysExpiringSoon() included far-expiry key")
	}
}
