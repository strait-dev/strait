package pubsub

import "testing"

func TestNewRedisClient_StandardURL(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "", nil)
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
	client, err := NewRedisClient("", "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil {
		t.Error("expected nil client for empty URL")
	}
}

func TestNewRedisClient_SentinelConfig(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("", "mymaster", []string{"localhost:26379", "localhost:26380"})
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
	client, err := NewRedisClient("redis://:mypassword@localhost:6379/2", "mymaster", []string{"localhost:26379"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	client.Close()
}

func TestNewRedisClient_InvalidURL(t *testing.T) {
	t.Parallel()
	_, err := NewRedisClient("not-a-valid-url", "", nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestNewRedisClient_SentinelMasterWithoutAddrs(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "mymaster", nil)
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
	client, err := NewRedisClient("redis://localhost:59999", "", nil)
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
