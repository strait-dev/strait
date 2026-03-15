package composition

import "context"

// TriggerAndWait triggers a run and polls until it reaches a terminal status.
func TriggerAndWait[TInput any, TRun any](
	ctx context.Context,
	triggerFn func(ctx context.Context, input TInput) (TRun, error),
	getRun func(ctx context.Context, runID string) (TRun, error),
	getID func(TRun) string,
	getStatus func(TRun) string,
	input TInput,
	opts *WaitForRunOptions,
) (TRun, error) {
	run, err := triggerFn(ctx, input)
	if err != nil {
		var zero TRun
		return zero, err
	}
	return WaitForRun(ctx, getRun, getStatus, getID(run), opts)
}
