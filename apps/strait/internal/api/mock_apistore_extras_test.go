package api

import (
	"context"
	"sync"

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
// non-terminal status with attempt=0; this preserves the pre-Phase-4 test
// behavior where the `attempt > 0` guard at sdk.go silently bypasses
// staleness when the test does not explicitly configure it.
func (mock *APIStoreMock) GetRunTokenState(ctx context.Context, id string) (domain.RunStatus, int, string, error) {
	runTokenStateMu.Lock()
	fn := runTokenStateOverrides[mock]
	runTokenStateMu.Unlock()
	if fn == nil {
		return domain.StatusExecuting, 0, "", nil
	}
	return fn(ctx, id)
}
