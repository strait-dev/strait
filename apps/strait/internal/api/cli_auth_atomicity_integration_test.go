//go:build integration

package api

import (
	"context"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
)

// TestApproveDeviceCodeRaceNoOrphans drives the canonical race the
// fix is designed to prevent: two callers approve the same user_code at
// the same time, each writing an api_keys row inside the same transaction
// that approves the device code. Real Postgres serializes the two
// transactions; whichever loses the UPDATE-with-status='pending' filter
// gets ErrDeviceCodeNotFound and the surrounding tx must roll back its
// CreateAPIKey insert. After the race finishes, exactly one api_keys row
// must remain for the project.
func TestApproveDeviceCodeRaceNoOrphans(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tdb, err := testutil.SetupTestDB(ctx, "../../migrations")
	if err != nil {
		t.Fatalf("SetupTestDB() error = %v", err)
	}
	t.Cleanup(func() { tdb.Cleanup(context.Background()) })

	q := store.New(tdb.Pool)
	q.SetSecretEncryptionKey("test-device-flow-key")

	if err := tdb.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}

	const (
		userCode  = "RACE1234"
		projectID = "proj-fix-07-race"
	)
	deviceCode := uuid.Must(uuid.NewV7()).String()

	if err := q.CreateDeviceCode(ctx, deviceCode, userCode, "", []string{}, time.Now().UTC().Add(15*time.Minute)); err != nil {
		t.Fatalf("CreateDeviceCode() error = %v", err)
	}

	approveOnce := func(label string) error {
		return store.WithTx(ctx, tdb.Pool, func(txq *store.Queries) error {
			txq.SetSecretEncryptionKey("test-device-flow-key")
			rawKey, err := generateAPIKey()
			if err != nil {
				return err
			}
			apiKey := &domain.APIKey{
				ID:        uuid.Must(uuid.NewV7()).String(),
				ProjectID: projectID,
				Name:      "CLI race " + label,
				KeyHash:   hashAPIKey(rawKey),
				KeyPrefix: rawKey[:12],
				Scopes:    domain.CLIDefaultScopes,
			}
			if err := txq.CreateAPIKey(ctx, apiKey); err != nil {
				return err
			}
			return txq.ApproveDeviceCodeByUserCode(ctx, userCode, apiKey.ID, rawKey, projectID, domain.CLIDefaultScopes)
		})
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errsOK int
		errsNG int
	)
	wg.Add(2)
	for i := range 2 {
		label := []string{"a", "b"}[i]
		go func() {
			defer wg.Done()
			err := approveOnce(label)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				errsOK++
			} else {
				errsNG++
			}
		}()
	}
	wg.Wait()

	if errsOK != 1 {
		t.Fatalf("expected exactly one successful approval, got success=%d failure=%d", errsOK, errsNG)
	}
	if errsNG != 1 {
		t.Fatalf("expected exactly one failed approval (rolled back), got success=%d failure=%d", errsOK, errsNG)
	}

	var keyCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE project_id = $1`, projectID).Scan(&keyCount); err != nil {
		t.Fatalf("count api_keys: %v", err)
	}
	if keyCount != 1 {
		t.Fatalf("api_keys for project after race = %d, want 1 (failed tx must rollback its key)", keyCount)
	}

	var approvedCount int
	if err := tdb.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM cli_device_codes WHERE user_code = $1 AND status = 'approved'`, userCode).Scan(&approvedCount); err != nil {
		t.Fatalf("count approved device codes: %v", err)
	}
	if approvedCount != 1 {
		t.Fatalf("approved device codes = %d, want 1", approvedCount)
	}
}
