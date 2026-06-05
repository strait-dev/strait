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
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
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
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tdb, err := testutil.SetupSharedTestDB(ctx, "../../migrations", "api-cli-auth")
	require.NoError(
		t,
		err)

	t.Cleanup(func() { tdb.Cleanup(context.Background()) })

	q := store.New(tdb.Pool)
	q.SetSecretEncryptionKey("test-device-flow-key")
	require.NoError(
		t,
		tdb.CleanTables(ctx))

	const (
		userCode  = "RACE1234"
		projectID = "proj-fix-07-race"
	)
	deviceCode := uuid.Must(uuid.NewV7()).String()
	require.NoError(
		t,
		q.CreateDeviceCode(ctx, deviceCode,
			userCode,

			"", []string{}, time.Now().
				UTC().Add(15*time.Minute)))

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
		concWG.Go(func() {
			defer wg.Done()
			err := approveOnce(label)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				errsOK++
			} else {
				errsNG++
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, errsOK)
	require.EqualValues(t, 1, errsNG)

	var keyCount int
	require.NoError(
		t,
		tdb.Pool.
			QueryRow(
				ctx, `SELECT COUNT(*) FROM api_keys WHERE project_id = $1`,

				projectID).Scan(&keyCount))
	require.EqualValues(t, 1, keyCount)

	var approvedCount int
	require.NoError(
		t,
		tdb.Pool.
			QueryRow(
				ctx, `SELECT COUNT(*) FROM cli_device_codes WHERE user_code = $1 AND status = 'approved'`,

				userCode).Scan(&approvedCount))
	require.EqualValues(t, 1, approvedCount)

}
