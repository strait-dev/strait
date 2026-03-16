package composition

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetry_SucceedsFirstAttempt(t *testing.T) {
	result, err := WithRetry(context.Background(), func() (int, error) {
		return 42, nil
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestWithRetry_SucceedsAfterRetries(t *testing.T) {
	attempt := 0
	result, err := WithRetry(context.Background(), func() (string, error) {
		attempt++
		if attempt < 3 {
			return "", errors.New("fail")
		}
		return "ok", nil
	}, &RetryOptions{Attempts: 3, DelayMs: 1, Jitter: JitterNone})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
}

func TestWithRetry_ExhaustsAttempts(t *testing.T) {
	attempt := 0
	_, err := WithRetry(context.Background(), func() (int, error) {
		attempt++
		return 0, errors.New("always fails")
	}, &RetryOptions{Attempts: 3, DelayMs: 1, Jitter: JitterNone})

	if err == nil {
		t.Fatal("expected error")
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
}

func TestWithRetry_ShouldRetryFalse(t *testing.T) {
	attempt := 0
	_, err := WithRetry(context.Background(), func() (int, error) {
		attempt++
		return 0, errors.New("no retry")
	}, &RetryOptions{
		Attempts: 5,
		DelayMs:  1,
		ShouldRetry: func(_ error, _ int, _ int) bool {
			return false
		},
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if attempt != 1 {
		t.Errorf("expected 1 attempt, got %d", attempt)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := WithRetry(ctx, func() (int, error) {
		return 0, errors.New("fail")
	}, &RetryOptions{Attempts: 5, DelayMs: 1})

	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWithRetry_DefaultOptions(t *testing.T) {
	result, err := WithRetry(context.Background(), func() (string, error) {
		return "ok", nil
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}
