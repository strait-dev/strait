//go:build cloud

package billing

import (
	"log/slog"
	"testing"
	"time"
)

// TestGoAsync_RecoversPanic is the regression guard for silent panic
// suppression: a panic in a fire-and-forget billing task must be recovered (and
// logged) rather than crash the process or vanish.
func TestGoAsync_RecoversPanic(t *testing.T) {
	t.Parallel()
	h := &WebhookHandler{logger: slog.New(slog.DiscardHandler)}

	ran := make(chan struct{})
	h.goAsync(func() {
		defer close(ran)
		panic("boom")
	})

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("async task did not run")
	}
	// Let the deferred recover in goAsync execute. The assertion is implicit: if
	// recovery failed, the panicking goroutine would crash the test binary.
	time.Sleep(50 * time.Millisecond)
}
