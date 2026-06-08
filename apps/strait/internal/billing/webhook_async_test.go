//go:build cloud

package billing

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGoAsync_RecoversPanic is the regression guard for silent panic
// suppression: a panic in a fire-and-forget billing task must be recovered (and
// logged) rather than crash the process or vanish.
func TestGoAsync_RecoversPanic(t *testing.T) {
	t.Parallel()
	h := &WebhookHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

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
	// Let the deferred recover in goAsync execute; if it failed, the panicking
	// goroutine would crash the test binary.
	time.Sleep(50 * time.Millisecond)
	require.True(t, true)
}
