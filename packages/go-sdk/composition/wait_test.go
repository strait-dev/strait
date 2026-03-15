package composition

import (
	"context"
	"errors"
	"testing"

	strait "github.com/strait-dev/go-sdk"
)

type fakeRun struct {
	ID     string
	Status string
}

func TestWaitForRun_AlreadyTerminal(t *testing.T) {
	run, err := WaitForRun(
		context.Background(),
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{ID: "run_1", Status: "completed"}, nil
		},
		func(r fakeRun) string { return r.Status },
		"run_1",
		nil,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "completed" {
		t.Errorf("expected 'completed', got %q", run.Status)
	}
}

func TestWaitForRun_TransitionsToTerminal(t *testing.T) {
	call := 0
	run, err := WaitForRun(
		context.Background(),
		func(_ context.Context, _ string) (fakeRun, error) {
			call++
			if call < 3 {
				return fakeRun{ID: "run_1", Status: "executing"}, nil
			}
			return fakeRun{ID: "run_1", Status: "completed"}, nil
		},
		func(r fakeRun) string { return r.Status },
		"run_1",
		&WaitForRunOptions{TimeoutMs: 5000, InitialDelayMs: 1, MaxDelayMs: 1},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "completed" {
		t.Errorf("expected 'completed', got %q", run.Status)
	}
}

func TestWaitForRun_Timeout(t *testing.T) {
	_, err := WaitForRun(
		context.Background(),
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{ID: "run_1", Status: "executing"}, nil
		},
		func(r fakeRun) string { return r.Status },
		"run_1",
		&WaitForRunOptions{TimeoutMs: 10, InitialDelayMs: 1, MaxDelayMs: 1},
	)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	var te *strait.TimeoutError
	if !errors.As(err, &te) {
		t.Errorf("expected TimeoutError, got %T", err)
	}
	if te.RunID != "run_1" {
		t.Errorf("expected RunID 'run_1', got %q", te.RunID)
	}
}

func TestWaitForRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := WaitForRun(
		ctx,
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{ID: "run_1", Status: "executing"}, nil
		},
		func(r fakeRun) string { return r.Status },
		"run_1",
		nil,
	)

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWaitForRun_GetRunError(t *testing.T) {
	_, err := WaitForRun(
		context.Background(),
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{}, errors.New("api error")
		},
		func(r fakeRun) string { return r.Status },
		"run_1",
		nil,
	)

	if err == nil || err.Error() != "api error" {
		t.Errorf("expected 'api error', got %v", err)
	}
}

func TestWaitForRun_CustomIsTerminal(t *testing.T) {
	run, err := WaitForRun(
		context.Background(),
		func(_ context.Context, _ string) (fakeRun, error) {
			return fakeRun{ID: "run_1", Status: "custom_done"}, nil
		},
		func(r fakeRun) string { return r.Status },
		"run_1",
		&WaitForRunOptions{
			IsTerminal: func(s string) bool { return s == "custom_done" },
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Status != "custom_done" {
		t.Errorf("expected 'custom_done', got %q", run.Status)
	}
}
