package scheduler

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/sourcegraph/conc"
)

// safeGo wraps a goroutine with panic recovery. If the function panics,
// the panic is logged with a stack trace and the goroutine exits cleanly
// instead of crashing the process.
func safeGo(wg *conc.WaitGroup, name string, fn func()) {
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduler component panicked",
					"component", name,
					"panic", fmt.Sprintf("%v", r),
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	})
}
