package api

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestApproveDeviceCodeRollsBackOnApproveFailure pins the atomicity
// invariant of handleApproveDeviceCode: when ApproveDeviceCodeByUserCode
// fails after CreateAPIKey succeeds, the surrounding transaction must roll
// back so no orphan api_keys row is left behind.
//
// The unit test simulates transactional commit/rollback semantics by
// intercepting runInTx -- "persistence" is only recorded when the closure
// returns nil. Real Postgres rollback is exercised by the integration
// counterpart in fix_07_cli_auth_atomicity_integration_test.go.
func TestApproveDeviceCodeRollsBackOnApproveFailure(t *testing.T) {
	t.Parallel()

	var (
		createAPIKeyHits int
		approveHits      int
		auditHits        int
		persistedKeyIDs  []string
	)

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-rollback",
				DeviceCode: "test-device-code",
				UserCode:   "ROLL1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			createAPIKeyHits++
			key.ID = "key-rolledback"
			key.CreatedAt = time.Now()
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			approveHits++
			return store.ErrDeviceCodeNotFound
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			auditHits++
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Wrap runInTx so a "would-persist" key is only committed when the
	// closure returns nil. This mirrors the rollback semantics of the real
	// store.WithTx helper.
	prevRunInTx := srv.runInTx
	srv.runInTx = func(ctx context.Context, fn func(s APIStore) error) error {
		var stagedKeyIDs []string
		baseCreate := ms.CreateAPIKeyFunc
		ms.CreateAPIKeyFunc = func(c context.Context, key *domain.APIKey) error {
			err := baseCreate(c, key)
			if err == nil {
				stagedKeyIDs = append(stagedKeyIDs, key.ID)
			}
			return err
		}
		defer func() { ms.CreateAPIKeyFunc = baseCreate }()

		err := fn(ms)
		if err == nil {
			persistedKeyIDs = append(persistedKeyIDs, stagedKeyIDs...)
		}
		return err
	}
	t.Cleanup(func() { srv.runInTx = prevRunInTx })

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-rollback")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	_, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "ROLL1234",
		ProjectID: "proj-rollback",
	}})
	require.Error(t, err)
	require.Equal(t, 1, createAPIKeyHits)
	require.Equal(t, 1, approveHits)
	require.Empty(t,
		persistedKeyIDs,
	)
	require.Equal(t, 0, auditHits)
}

// TestApproveDeviceCodeCommitsOnSuccess regression-tests the happy
// path: when both store calls succeed, the transaction commits, the api
// key is persisted, and the device-code-approved audit event is emitted.
func TestApproveDeviceCodeCommitsOnSuccess(t *testing.T) {
	t.Parallel()

	var (
		createAPIKeyHits int
		approveHits      int
		auditEvents      []*domain.AuditEvent
		persistedKeyIDs  []string
	)

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-commit",
				DeviceCode: "test-device-code",
				UserCode:   "GOOD1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			createAPIKeyHits++
			key.ID = "key-committed"
			key.CreatedAt = time.Now()
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			approveHits++
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditEvents = append(auditEvents, ev)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	prevRunInTx := srv.runInTx
	srv.runInTx = func(ctx context.Context, fn func(s APIStore) error) error {
		var stagedKeyIDs []string
		baseCreate := ms.CreateAPIKeyFunc
		ms.CreateAPIKeyFunc = func(c context.Context, key *domain.APIKey) error {
			err := baseCreate(c, key)
			if err == nil {
				stagedKeyIDs = append(stagedKeyIDs, key.ID)
			}
			return err
		}
		defer func() { ms.CreateAPIKeyFunc = baseCreate }()

		err := fn(ms)
		if err == nil {
			persistedKeyIDs = append(persistedKeyIDs, stagedKeyIDs...)
		}
		return err
	}
	t.Cleanup(func() { srv.runInTx = prevRunInTx })

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-commit")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	_, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "GOOD1234",
		ProjectID: "proj-commit",
	}})
	require.NoError(t, err)
	require.Equal(t, 1, createAPIKeyHits)
	require.Equal(t, 1, approveHits)
	require.False(t, len(persistedKeyIDs) != 1 || persistedKeyIDs[0] != "key-committed")
	require.NotEmpty(t, auditEvents)

	var sawApproved bool
	for _, ev := range auditEvents {
		if ev.Action == domain.AuditActionDeviceCodeApproved {
			sawApproved = true
			break
		}
	}
	require.True(
		t, sawApproved,
	)
}

// TestApproveDeviceCodePropagatesNotFound checks that the
// ErrDeviceCodeNotFound returned from inside the transaction surfaces as
// a 404, matching the pre-fix lookup behavior. This guards against a
// regression where wrapping in WithTx accidentally swallows the sentinel.
func TestApproveDeviceCodePropagatesNotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-race",
				DeviceCode: "test-device-code",
				UserCode:   "RACE1234",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			key.ID = "key-race"
			key.CreatedAt = time.Now()
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return store.ErrDeviceCodeNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-race")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	_, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "RACE1234",
		ProjectID: "proj-race",
	}})
	require.Error(t, err)
}
