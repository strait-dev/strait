package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestStatusReadModel_CASRejectsOutOfOrderUpdate(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := NewReadModel[string](ReadModelConfig[string]{
		Client:    rdb,
		Namespace: "status_test",
		TTL:       time.Minute,
	})

	ok, err := model.CompareAndSet(context.Background(), "run-1", "running", 5)
	if err != nil {
		t.Fatalf("CompareAndSet(v5) error = %v", err)
	}
	if !ok {
		t.Fatal("CompareAndSet(v5) = false, want true")
	}
	ok, err = model.CompareAndSet(context.Background(), "run-1", "queued", 4)
	if err != nil {
		t.Fatalf("CompareAndSet(v4) error = %v", err)
	}
	if ok {
		t.Fatal("CompareAndSet(v4) = true, want false")
	}
	got, err := model.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Version != 5 || got.Value != "running" {
		t.Fatalf("Get() = %+v, want version 5 running", got)
	}
}

func TestStatusReadModel_SetIfColdDoesNotOverwriteNewerCDCValue(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	model := NewReadModel[string](ReadModelConfig[string]{
		Client:    rdb,
		Namespace: "status_fill_test",
		TTL:       time.Minute,
	})

	if ok, err := model.CompareAndSet(context.Background(), "run-1", "completed", 9); err != nil || !ok {
		t.Fatalf("CompareAndSet(v9) = %v, %v; want true, nil", ok, err)
	}
	if err := model.SetIfCold(context.Background(), "run-1", "queued"); err != nil {
		t.Fatalf("SetIfCold() error = %v", err)
	}
	got, err := model.Get(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Version != 9 || got.Value != "completed" {
		t.Fatalf("Get() = %+v, want version 9 completed", got)
	}
}
