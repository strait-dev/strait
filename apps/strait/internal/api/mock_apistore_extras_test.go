package api

import (
	"context"
	"sync"
	"time"

	"strait/internal/domain"
)

// runTokenStateOverrides is a per-mock map of overrides for the manually-added
// runTokenStateGetter shim. The map is keyed by the *APIStoreMock pointer so
// the override remains scoped to a single mock instance.
var (
	runTokenStateMu        sync.Mutex
	runTokenStateOverrides = make(map[*APIStoreMock]func(context.Context, string) (domain.RunStatus, int, string, error))
)

// SetRunTokenStateFunc lets tests opt in to the runTokenStateGetter contract
// on APIStoreMock without regenerating moq. Production stores implement this
// directly; runTokenAuth fails closed when neither path is available.
func (mock *APIStoreMock) SetRunTokenStateFunc(fn func(context.Context, string) (domain.RunStatus, int, string, error)) {
	runTokenStateMu.Lock()
	defer runTokenStateMu.Unlock()
	runTokenStateOverrides[mock] = fn
}

// GetRunTokenState is the manual implementation of the runTokenStateGetter
// interface on the moq-generated APIStoreMock. The default returns a
// non-terminal status with attempt=0 so tests that do not configure token
// state keep the same staleness behavior as the SDK guard in sdk.go.
func (mock *APIStoreMock) GetRunTokenState(ctx context.Context, id string) (domain.RunStatus, int, string, error) {
	runTokenStateMu.Lock()
	fn := runTokenStateOverrides[mock]
	runTokenStateMu.Unlock()
	if fn == nil {
		return domain.StatusExecuting, 0, "", nil
	}
	return fn(ctx, id)
}

// ReplayAuditEventDeadletter is the manual implementation of the atomic
// dead-letter replayer on the moq-generated APIStoreMock. The production store
// runs the steps in one transaction; the mock mirrors the sequence by delegating
// to the configured Get/Create/Mark/Delete funcs so existing tests keep working.
// Any step's error surfaces as the method's error (matching the all-or-nothing
// transactional contract).
func (mock *APIStoreMock) ReplayAuditEventDeadletter(ctx context.Context, id, projectID, newEventID string) (*domain.AuditEvent, bool, error) {
	ev, err := mock.GetAuditEventDeadletter(ctx, id, projectID)
	if err != nil {
		return nil, false, err
	}
	if ev == nil {
		return nil, false, nil
	}
	newEvent := *ev
	newEvent.ID = newEventID
	if err := mock.CreateAuditEvent(ctx, &newEvent); err != nil {
		return nil, false, err
	}
	if err := mock.MarkAuditDeadletterReclaimed(ctx, id, newEvent.ID); err != nil {
		return nil, false, err
	}
	if err := mock.DeleteAuditEventDeadletter(ctx, id, projectID); err != nil {
		return nil, false, err
	}
	return &newEvent, true, nil
}

// CreateRotatedAPIKey is the manual implementation of the atomic rotation method
// on the moq-generated APIStoreMock. The production store runs CreateAPIKey and
// MarkAPIKeyRotated in a single transaction; the mock mirrors that by delegating
// to the configured CreateAPIKeyFunc and MarkAPIKeyRotatedFunc so existing
// rotation tests keep working. A MarkAPIKeyRotated failure surfaces as this
// method's error with no compensating revoke, matching the transactional
// rollback (no orphaned active key).
func (mock *APIStoreMock) CreateRotatedAPIKey(ctx context.Context, oldKeyID string, newKey *domain.APIKey, graceExpiresAt time.Time) error {
	if err := mock.CreateAPIKey(ctx, newKey); err != nil {
		return err
	}
	return mock.MarkAPIKeyRotated(ctx, oldKeyID, newKey.ID, graceExpiresAt)
}
