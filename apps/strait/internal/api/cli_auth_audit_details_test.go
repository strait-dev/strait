package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

// TestApproveDeviceCodeAuditCarriesAPIKeyID asserts that the
// device_code_approved audit event records the same api_key_id as the
// row persisted by CreateAPIKey. The handler pre-assigns the UUID so
// the audit map serialized inside buildAuditEvent matches the row that
// CreateAPIKey will commit (the store honors a non-empty key.ID).
func TestApproveDeviceCodeAuditCarriesAPIKeyID(t *testing.T) {
	t.Parallel()

	var (
		persistedAPIKeyID string
		auditEvents       []*domain.AuditEvent
	)

	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:         "dc-audit",
				DeviceCode: "test-device-code",
				UserCode:   "AUDIT123",
				Status:     "pending",
				ExpiresAt:  time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			// Mirror the production store: only assign an ID when the
			// caller didn't pre-assign one. This is what makes the
			// handler's pre-assignment observable.
			if key.ID == "" {
				key.ID = "store-assigned-id"
			}
			persistedAPIKeyID = key.ID
			key.CreatedAt = time.Now()
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, ev *domain.AuditEvent) error {
			auditEvents = append(auditEvents, ev)
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-audit")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	if _, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "AUDIT123",
		ProjectID: "proj-audit",
	}}); err != nil {
		require.Failf(t, "test failure",

			"handleApproveDeviceCode() error = %v", err)
	}
	require.NotEqual(t, "", persistedAPIKeyID)
	require.NotEqual(t, "store-assigned-id",

		persistedAPIKeyID,
	)

	var approvedEvent *domain.AuditEvent
	for _, ev := range auditEvents {
		if ev.Action == domain.AuditActionDeviceCodeApproved {
			approvedEvent = ev
			break
		}
	}
	require.NotNil(t, approvedEvent)

	var details map[string]any
	require.NoError(t, json.Unmarshal(approvedEvent.
		Details,

		&details))

	got, _ := details["api_key_id"].(string)
	require.NotEqual(t, "", got)
	require.Equal(t, persistedAPIKeyID,
		got)

}

// TestApproveDeviceCodeAuditAPIKeyIDIsValidUUIDv7 ensures the
// pre-assigned ID looks like a UUIDv7 (36 chars, 4 hyphens, version
// nibble = 7). This is the format the rest of the system relies on.
func TestApproveDeviceCodeAuditAPIKeyIDIsValidUUIDv7(t *testing.T) {
	t.Parallel()

	var captured *domain.APIKey
	ms := &APIStoreMock{
		GetDeviceCodeByUserCodeFunc: func(_ context.Context, _ string) (*store.DeviceCodeRow, error) {
			return &store.DeviceCodeRow{
				ID:        "dc-uuid",
				UserCode:  "UUIDV7AB",
				Status:    "pending",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: projectID}, nil
		},
		CreateAPIKeyFunc: func(_ context.Context, key *domain.APIKey) error {
			captured = key
			key.CreatedAt = time.Now()
			return nil
		},
		ApproveDeviceCodeByUserCodeFunc: func(_ context.Context, _, _, _, _ string, _ []string) error {
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error { return nil },
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-uuid")
	ctx = context.WithValue(ctx, ctxScopesKey, domain.CLIDefaultScopes)

	if _, err := srv.handleApproveDeviceCode(ctx, &ApproveDeviceCodeInput{Body: approveDeviceCodeRequest{
		UserCode:  "UUIDV7AB",
		ProjectID: "proj-uuid",
	}}); err != nil {
		require.Failf(t, "test failure",

			"handleApproveDeviceCode() error = %v", err)
	}
	require.False(t, captured == nil ||
		captured.
			ID ==

			"")
	require.Len(t,
		captured.ID, 36,
	)
	require.Equal(t, byte('7'), captured.
		ID[14])

	// UUIDv7 layout: xxxxxxxx-xxxx-7xxx-xxxx-xxxxxxxxxxxx (version nibble at index 14).

}
