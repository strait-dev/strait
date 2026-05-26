package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrCacheMiss = errors.New("cache miss")

// Codec serializes Redis L2 cache values. JSON is the v1 codec so the wire
// format stays inspectable and does not add another runtime dependency.
type Codec[T any] interface {
	Marshal(T) ([]byte, error)
	Unmarshal([]byte, *T) error
}

type JSONCodec[T any] struct{}

func (JSONCodec[T]) Marshal(v T) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec[T]) Unmarshal(b []byte, dst *T) error {
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("decode cache value: %w", err)
	}
	return nil
}

type cacheEntry[V any] struct {
	Version  int64 `json:"version"`
	Negative bool  `json:"negative,omitempty"`
	Barrier  bool  `json:"barrier,omitempty"`
	Value    V     `json:"value,omitempty"`
}

// L2 is the shared cache tier. Implementations must treat ErrCacheMiss as a
// normal miss and all other errors as fail-open candidates.
type L2[K comparable, V any] interface {
	Get(ctx context.Context, key K) (cacheEntry[V], error)
	Set(ctx context.Context, key K, entry cacheEntry[V], ttl time.Duration) error
	CompareAndSet(ctx context.Context, key K, entry cacheEntry[V], ttl time.Duration) (bool, error)
	Delete(ctx context.Context, key K) error
}
