package pubsub

import (
	"testing"
	"time"
)

func TestNewRedisClient_StandardURL(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "", nil, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	client.Close()
}

func TestNewRedisClient_EmptyURL(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("", "", nil, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil {
		t.Error("expected nil client for empty URL")
	}
}

func TestNewRedisClient_SentinelConfig(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("", "mymaster", []string{"localhost:26379", "localhost:26380"}, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client for sentinel config")
	}
	client.Close()
}

func TestNewRedisClient_SentinelWithPassword(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://:mypassword@localhost:6379/2", "mymaster", []string{"localhost:26379"}, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	client.Close()
}

func TestNewRedisClient_SentinelWithRedissEnablesTLS(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("rediss://:mypassword@localhost:6379/2", "mymaster", []string{"localhost:26379"}, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	opts := client.Options()
	if opts.Password != "mypassword" {
		t.Fatalf("Password: got %q, want %q", opts.Password, "mypassword")
	}
	if opts.DB != 2 {
		t.Fatalf("DB: got %d, want 2", opts.DB)
	}
	if opts.TLSConfig == nil {
		t.Fatal("expected Sentinel rediss:// configuration to enable TLS")
	}
}

func TestNewRedisClient_SentinelInvalidURLFailsClosed(t *testing.T) {
	t.Parallel()
	_, err := NewRedisClient("not-a-valid-url", "mymaster", []string{"localhost:26379"}, RedisPoolOptions{})
	if err == nil {
		t.Fatal("expected error for invalid Sentinel Redis URL")
	}
}

func TestNewRedisClient_InvalidURL(t *testing.T) {
	t.Parallel()
	_, err := NewRedisClient("not-a-valid-url", "", nil, RedisPoolOptions{})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestNewRedisClient_SentinelMasterWithoutAddrs(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "mymaster", nil, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client (should fall back to standard)")
	}
	client.Close()
}

func TestRedisPublisher_Ping(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:59999", "", nil, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	pub := NewRedisPublisher(client)
	ctx := t.Context()
	err = pub.Ping(ctx)
	if err == nil {
		t.Error("expected error pinging non-existent Redis")
	}
}

func TestNewRedisClient_AppliesPoolOptions(t *testing.T) {
	t.Parallel()
	pool := RedisPoolOptions{
		PoolSize:        42,
		MinIdleConns:    7,
		ReadTimeout:     750 * time.Millisecond,
		WriteTimeout:    900 * time.Millisecond,
		ConnMaxLifetime: 17 * time.Minute,
	}

	client, err := NewRedisClient("redis://localhost:6379", "", nil, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	opts := client.Options()
	if opts.PoolSize != pool.PoolSize {
		t.Errorf("PoolSize: got %d, want %d", opts.PoolSize, pool.PoolSize)
	}
	if opts.MinIdleConns != pool.MinIdleConns {
		t.Errorf("MinIdleConns: got %d, want %d", opts.MinIdleConns, pool.MinIdleConns)
	}
	if opts.ReadTimeout != pool.ReadTimeout {
		t.Errorf("ReadTimeout: got %s, want %s", opts.ReadTimeout, pool.ReadTimeout)
	}
	if opts.WriteTimeout != pool.WriteTimeout {
		t.Errorf("WriteTimeout: got %s, want %s", opts.WriteTimeout, pool.WriteTimeout)
	}
	if opts.ConnMaxLifetime != pool.ConnMaxLifetime {
		t.Errorf("ConnMaxLifetime: got %s, want %s", opts.ConnMaxLifetime, pool.ConnMaxLifetime)
	}
}

func TestNewRedisClient_EmptyPoolOptions_KeepsURLDefaults(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "", nil, RedisPoolOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	opts := client.Options()
	// go-redis defaults: PoolSize = 10*GOMAXPROCS, MinIdleConns = 0.
	if opts.PoolSize == 0 {
		t.Error("expected go-redis default PoolSize, got 0")
	}
	if opts.ReadTimeout <= 0 {
		t.Error("expected go-redis default ReadTimeout, got non-positive")
	}
}

func TestNewRedisClient_SentinelAppliesPoolOptions(t *testing.T) {
	t.Parallel()
	pool := RedisPoolOptions{
		PoolSize:    15,
		ReadTimeout: 500 * time.Millisecond,
	}

	client, err := NewRedisClient("", "mymaster", []string{"localhost:26379"}, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	opts := client.Options()
	if opts.PoolSize != pool.PoolSize {
		t.Errorf("Sentinel PoolSize: got %d, want %d", opts.PoolSize, pool.PoolSize)
	}
	if opts.ReadTimeout != pool.ReadTimeout {
		t.Errorf("Sentinel ReadTimeout: got %s, want %s", opts.ReadTimeout, pool.ReadTimeout)
	}
}
