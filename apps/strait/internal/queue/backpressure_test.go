package queue

import (
	"context"
	"errors"
	"testing"
)

func TestThrottledError_Unwrap(t *testing.T) {
	e := &ThrottledError{ProjectID: "p", RetryAfter: 0}
	if !errors.Is(e, ErrEnqueueThrottled) {
		t.Error("ThrottledError should unwrap to ErrEnqueueThrottled")
	}
}

func TestAsThrottled_Positive(t *testing.T) {
	e := &ThrottledError{ProjectID: "p"}
	if tthrot, ok := AsThrottled(e); !ok || tthrot.ProjectID != "p" {
		t.Errorf("AsThrottled failed: %+v %v", tthrot, ok)
	}
}

func TestAsThrottled_Negative(t *testing.T) {
	if _, ok := AsThrottled(errors.New("other")); ok {
		t.Error("non-throttled should not match")
	}
}

func TestBackpressure_NilSafeAndDisabled(t *testing.T) {
	var b *Backpressure
	if err := b.TryConsume(context.Background(), "p"); err != nil {
		t.Errorf("nil should be no-op, got %v", err)
	}

	b2 := NewBackpressure(nil, BackpressureConfig{}, false)
	if err := b2.TryConsume(context.Background(), "p"); err != nil {
		t.Errorf("disabled should be no-op, got %v", err)
	}
}

func TestBackpressure_EmptyProjectID(t *testing.T) {
	b := NewBackpressure(nil, BackpressureConfig{}, true)
	if err := b.TryConsume(context.Background(), ""); err != nil {
		t.Errorf("empty project id should pass, got %v", err)
	}
}

func TestBackpressure_DefaultConfig(t *testing.T) {
	b := NewBackpressure(nil, BackpressureConfig{}, true)
	if b.cfg.DefaultMaxTokens != 1000 || b.cfg.DefaultRefillPerSec != 100 {
		t.Errorf("defaults = %+v", b.cfg)
	}
}
