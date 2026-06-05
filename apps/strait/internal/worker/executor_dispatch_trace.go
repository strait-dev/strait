package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptrace"
	"sync"
	"time"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
)

type httpDispatchTraceRecorder struct {
	mu            sync.Mutex
	dispatchStart time.Time
	connectStart  time.Time
	connectDone   time.Time
	gotFirstByte  time.Time
}

func newHTTPDispatchTraceRecorder(dispatchStart time.Time) *httpDispatchTraceRecorder {
	return &httpDispatchTraceRecorder{dispatchStart: dispatchStart}
}

func (r *httpDispatchTraceRecorder) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		ConnectStart: func(string, string) {
			r.recordConnectStart(time.Now())
		},
		ConnectDone: func(string, string, error) {
			r.recordConnectDone(time.Now())
		},
		GotFirstResponseByte: func() {
			r.recordFirstByte(time.Now())
		},
	}
}

func (r *httpDispatchTraceRecorder) recordConnectStart(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectStart = at
}

func (r *httpDispatchTraceRecorder) recordConnectDone(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectDone = at
}

func (r *httpDispatchTraceRecorder) recordFirstByte(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gotFirstByte = at
}

func (r *httpDispatchTraceRecorder) executionTrace(gotLastByte time.Time) *domain.ExecutionTrace {
	r.mu.Lock()
	connectStart := r.connectStart
	connectDone := r.connectDone
	gotFirstByte := r.gotFirstByte
	r.mu.Unlock()

	execTrace := &domain.ExecutionTrace{}
	if !connectStart.IsZero() && !connectDone.IsZero() {
		execTrace.ConnectMs = durationMillisecondsAtLeastOne(connectDone.Sub(connectStart))
	}
	if !gotFirstByte.IsZero() {
		base := r.dispatchStart
		if !connectDone.IsZero() {
			base = connectDone
		}
		execTrace.TtfbMs = durationMillisecondsAtLeastOne(gotFirstByte.Sub(base))
		execTrace.TransferMs = durationMillisecondsAtLeastOne(gotLastByte.Sub(gotFirstByte))
	}
	execTrace.DispatchMs = execTrace.ConnectMs + execTrace.TtfbMs + execTrace.TransferMs
	return execTrace
}

func (e *Executor) tracedDispatch(ctx context.Context, job *domain.Job, run *domain.JobRun) (json.RawMessage, *domain.ExecutionTrace, error) {
	traceRecorder := newHTTPDispatchTraceRecorder(time.Now())

	tracedCtx := httptrace.WithClientTrace(ctx, traceRecorder.clientTrace())

	// Secrets and checkpoint live in two independent cache entries and must
	// be resolved independently. Resume headers depend on checkpoint being
	// populated on every retry attempt, not just retries that also miss the
	// secrets cache.
	var inputs dispatchHeaderInputs
	var secretsErr error

	var dispatchWG conc.WaitGroup
	if cached, ok := dispatchCacheGet[[]domain.JobSecret](ctx, dispatchSecretsCacheKey(job)); ok {
		inputs.secrets = cached
	} else {
		dispatchWG.Go(func() {
			inputs.secrets, secretsErr = e.dispatchSecrets(tracedCtx, job)
		})
	}
	if run.Attempt > 1 {
		checkpointCacheKey := "checkpoint:" + run.ID
		if cached, ok := dispatchCacheGet[*domain.RunCheckpoint](ctx, checkpointCacheKey); ok {
			inputs.checkpoint = cached
		} else {
			dispatchWG.Go(func() {
				inputs.checkpoint, _ = e.store.GetLatestCheckpoint(tracedCtx, run.ID)
			})
		}
	}
	dispatchWG.Wait()
	if run.Attempt > 1 && inputs.checkpoint != nil {
		dispatchCacheSet(ctx, "checkpoint:"+run.ID, inputs.checkpoint)
	}

	if secretsErr != nil {
		return nil, nil, fmt.Errorf("load job %s secrets: %w", job.ID, secretsErr)
	}

	extraHeaders, err := e.buildDispatchHeaders(job, run, inputs.secrets, inputs.checkpoint)
	if err != nil {
		return nil, nil, err
	}

	result, err := e.dispatchToEndpoint(tracedCtx, job.EndpointURL, run, extraHeaders)
	gotLastByte := time.Now()

	return result, traceRecorder.executionTrace(gotLastByte), err
}
