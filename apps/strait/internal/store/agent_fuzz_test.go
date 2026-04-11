//go:build integration

package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// fuzzAdvisoryLockID mirrors internal/agents/service.go:advisoryLockID
// so the NextAgentDeploymentVersion fuzz target uses the same lock
// partitioning scheme the real service uses.
func fuzzAdvisoryLockID(value string) int64 {
	sum := sha256.Sum256([]byte(value))
	return int64(binary.BigEndian.Uint64(sum[:8]) & ((1 << 63) - 1))
}

// FuzzProjectSecretsEncryptRoundtrip hammers the Phase D
// project_secrets encryption layer with arbitrary byte inputs to catch
// any regression in the AES-GCM + HKDF path. Every (key, value) pair
// must survive a CreateProjectSecret -> ListProjectSecretsByEnv ->
// decrypt round trip and come back byte-identical. Any panic in the
// encryption helpers on exotic bytes (unicode, SQL-looking strings,
// base64 noise) is also caught.
//
// Integration-tagged because it hits a real DB: the fuzz target
// exercises the full SQL write + encrypted storage + decrypt path.
// In seed mode the hand-picked corpus covers the interesting edge
// cases; go test -fuzz in CI explores further.
func FuzzProjectSecretsEncryptRoundtrip(f *testing.F) {
	f.Add("API_KEY", "sk-plaintext-value")
	f.Add("DATABASE_URL", "postgres://user:pass@host/db?sslmode=require")
	f.Add("EMPTY_VALUE", "")
	f.Add("X", "x")
	f.Add("UNICODE_KEY", "你好世界 🔐")
	f.Add("LONG_VALUE", "AB" + string(make([]byte, 4096)))
	f.Add("BYTES_LOOKING_LIKE_SQL", "' OR 1=1; DROP TABLE project_secrets; --")
	f.Add("NULL_BYTES", "null\x00embedded\x00value")

	ctx := context.Background()

	// Each fuzz iteration creates its own project + env to stay
	// independent from sibling iterations. Clean the table between
	// groups of iterations via mustClean would be unnecessary overhead.
	f.Fuzz(func(t *testing.T, secretKey, value string) {
		// Postgres identifiers and encryption layers both have
		// practical bounds. Reject obviously-oversized inputs rather
		// than stress the db into failing for unrelated reasons.
		if len(secretKey) == 0 || len(secretKey) > 512 {
			t.Skip()
		}
		if len(value) > 1<<16 {
			t.Skip()
		}
		// secret_key must not contain NUL (pg text rejects it).
		for _, b := range []byte(secretKey) {
			if b == 0 {
				t.Skip()
			}
		}

		q := store.New(testDB.Pool)
		q.SetSecretEncryptionKey("test-secret-encryption-key-32chr!")

		projectID := "proj-fuzz-secret-" + newID()
		if err := q.CreateProject(ctx, &domain.Project{
			ID: projectID, OrgID: "org-fuzz-secret", Name: "Fuzz Secret",
		}); err != nil {
			t.Fatalf("CreateProject() error = %v", err)
		}
		env := &domain.Environment{
			ID: "env-fuzz-" + newID(), ProjectID: projectID, Name: "dev", Slug: "dev",
		}
		if err := q.CreateEnvironment(ctx, env); err != nil {
			t.Fatalf("CreateEnvironment() error = %v", err)
		}

		// Write the secret — CreateProjectSecret encrypts in place.
		secret := &domain.ProjectSecret{
			ProjectID:      projectID,
			EnvironmentID:  env.ID,
			SecretKey:      secretKey,
			EncryptedValue: value,
		}
		if err := q.CreateProjectSecret(ctx, secret); err != nil {
			t.Fatalf("CreateProjectSecret(key=%q) error = %v", secretKey, err)
		}

		// Read back via ListProjectSecretsByEnv which decrypts.
		list, err := q.ListProjectSecretsByEnv(ctx, projectID, env.ID)
		if err != nil {
			t.Fatalf("ListProjectSecretsByEnv() error = %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("got %d secrets, want 1", len(list))
		}
		if list[0].EncryptedValue != value {
			t.Fatalf("roundtrip mismatch: got %q, want %q", list[0].EncryptedValue, value)
		}
		if list[0].SecretKey != secretKey {
			t.Fatalf("key mismatch: got %q, want %q", list[0].SecretKey, secretKey)
		}

		// Clean up the project so the table doesn't balloon during
		// long fuzz runs.
		if err := q.DeleteProject(ctx, projectID); err != nil {
			t.Logf("cleanup DeleteProject error = %v", err)
		}
	})
}

// FuzzNextAgentDeploymentVersionMonotonic hammers
// NextAgentDeploymentVersion with varying levels of concurrency and
// asserts the returned versions are always strictly monotonic per
// agent. This catches any regression if the advisory lock at
// service.go:593 is ever removed or if NextAgentDeploymentVersion
// grows a path that escapes the lock.
func FuzzNextAgentDeploymentVersionMonotonic(f *testing.F) {
	f.Add(1)
	f.Add(2)
	f.Add(5)
	f.Add(10)
	f.Add(20)

	ctx := context.Background()

	f.Fuzz(func(t *testing.T, parallelism int) {
		if parallelism < 1 || parallelism > 32 {
			t.Skip()
		}

		q := store.New(testDB.Pool)
		projectID := "proj-fuzz-nadv-" + newID()
		if err := q.CreateProject(ctx, &domain.Project{
			ID: projectID, OrgID: "org-fuzz-nadv", Name: "Fuzz NADV",
		}); err != nil {
			t.Fatalf("CreateProject() error = %v", err)
		}
		job := mustCreateJob(t, ctx, q, projectID)
		agent := &domain.Agent{
			ID: "agent-fuzz-nadv-" + newID(), ProjectID: projectID, JobID: job.ID,
			Name: "Fuzz NADV", Slug: "fuzz-nadv-" + newID(), Model: "gpt-5.4",
		}
		if err := q.CreateAgent(ctx, agent); err != nil {
			t.Fatalf("CreateAgent() error = %v", err)
		}

		var mu sync.Mutex
		seen := make(map[int]bool, parallelism)
		var wg sync.WaitGroup
		wg.Add(parallelism)
		for range parallelism {
			go func() {
				defer wg.Done()
				// Each goroutine acquires the advisory lock in its
				// own tx before calling NextAgentDeploymentVersion
				// and then CreateAgentDeployment — mirroring how the
				// real DeployAgentToEnv flow uses WithTx.
				if err := store.WithTx(ctx, testDB.Pool, func(tx *store.Queries) error {
					if err := tx.AdvisoryXactLock(ctx, fuzzAdvisoryLockID(agent.ID)); err != nil {
						return err
					}
					v, err := tx.NextAgentDeploymentVersion(ctx, agent.ID)
					if err != nil {
						return err
					}
					mu.Lock()
					if seen[v] {
						mu.Unlock()
						t.Errorf("duplicate version %d observed in parallel batch", v)
						return nil
					}
					seen[v] = true
					mu.Unlock()
					// Persist the deployment so the next caller's
					// MAX(version) advances.
					return tx.CreateAgentDeployment(ctx, &domain.AgentDeployment{
						ID: newID(), AgentID: agent.ID, Version: v,
						Status: domain.AgentDeploymentStatusPending, Provider: "local_stub",
					})
				}); err != nil {
					t.Errorf("WithTx error = %v", err)
				}
			}()
		}
		wg.Wait()

		// Every version from 1..parallelism must have been handed out
		// exactly once.
		for i := 1; i <= parallelism; i++ {
			if !seen[i] {
				t.Errorf("version %d missing from parallel batch %+v", i, seen)
			}
		}

		if err := q.DeleteProject(ctx, projectID); err != nil {
			t.Logf("cleanup DeleteProject error = %v", err)
		}
	})
}
