package scheduler

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sourcegraph/conc"
)

// exitFunc is called when a scheduler component panics. Defaults to
// os.Exit(1). Tests override this to verify the crash path without
// actually killing the process.
var exitFunc = func(code int) { os.Exit(code) }

// safeGo wraps a goroutine with panic recovery. If the function panics,
// the panic is logged with a stack trace, reported to Sentry, and the
// process is terminated. A silently dead scheduler component is worse
// than a restart, so we crash to let the orchestrator (systemd/k8s)
// restart us.
func safeGo(wg *conc.WaitGroup, name string, fn func()) {
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				slog.Error("scheduler component panicked, crashing process",
					"component", name,
					"panic", fmt.Sprintf("%v", r),
					"stack", stack,
				)

				if hub := sentry.CurrentHub(); hub != nil {
					hub.Recover(r)
					sentry.Flush(2 * time.Second)
				}

				exitFunc(1)
			}
		}()
		fn()
	})
}
