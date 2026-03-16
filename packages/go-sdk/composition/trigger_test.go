package composition

import (
	"context"
	"errors"
	"testing"
)

func TestTriggerAndWait_Success(t *testing.T) {
	run, err := TriggerAndWait(
		context.Background(),
		func(_ context.Context, input string) (fakeRun, error) {
			return fakeRun{ID: "run_1", Status: "queued"}, nil
		},
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{ID: "run_1", Status: "completed"}, nil
		},
		func(r fakeRun) string { return r.ID },
		func(r fakeRun) string { return r.Status },
		"payload",
		&WaitForRunOptions{InitialDelayMs: 1, MaxDelayMs: 1},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "completed" {
		t.Errorf("expected 'completed', got %q", run.Status)
	}
}

func TestTriggerAndWait_TriggerError(t *testing.T) {
	_, err := TriggerAndWait(
		context.Background(),
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{}, errors.New("trigger failed")
		},
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{}, nil
		},
		func(r fakeRun) string { return r.ID },
		func(r fakeRun) string { return r.Status },
		"payload",
		nil,
	)

	if err == nil || err.Error() != "trigger failed" {
		t.Errorf("expected 'trigger failed', got %v", err)
	}
}
