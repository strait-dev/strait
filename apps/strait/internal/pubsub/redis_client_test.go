package pubsub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisClient_StandardURL(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "", nil, RedisPoolOptions{})
	require.NoError(
		t, err)
	require.NotNil(t,
		client,
	)

	client.Close()
}

func TestNewRedisClient_EmptyURL(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("", "", nil, RedisPoolOptions{})
	require.Error(t,
		err)
	assert.Nil(t, client)
}

func TestNewRedisClient_SentinelConfig(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("", "mymaster", []string{"localhost:26379", "localhost:26380"}, RedisPoolOptions{})
	require.NoError(
		t, err)
	require.NotNil(t,
		client,
	)

	client.Close()
}

func TestNewRedisClient_SentinelWithPassword(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://:mypassword@localhost:6379/2", "mymaster", []string{"localhost:26379"}, RedisPoolOptions{})
	require.NoError(
		t, err)
	require.NotNil(t,
		client,
	)

	client.Close()
}

func TestNewRedisClient_SentinelWithRedissEnablesTLS(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("rediss://:mypassword@localhost:6379/2", "mymaster", []string{"localhost:26379"}, RedisPoolOptions{})
	require.NoError(
		t, err)

	defer client.Close()

	opts := client.Options()
	require.Equal(t,
		"mypassword",

		opts.Password)
	require.Equal(t,
		2, opts.
			DB,
	)
	require.NotNil(t,
		opts.TLSConfig,
	)
}

func TestNewRedisClient_SentinelInvalidURLFailsClosed(t *testing.T) {
	t.Parallel()
	_, err := NewRedisClient("not-a-valid-url", "mymaster", []string{"localhost:26379"}, RedisPoolOptions{})
	require.Error(t,
		err)
}

func TestNewRedisClient_InvalidURL(t *testing.T) {
	t.Parallel()
	_, err := NewRedisClient("not-a-valid-url", "", nil, RedisPoolOptions{})
	require.Error(t,
		err)
}

func TestNewRedisClient_SentinelMasterWithoutAddrs(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "mymaster", nil, RedisPoolOptions{})
	require.NoError(
		t, err)
	require.NotNil(t,
		client,
	)

	client.Close()
}

func TestRedisPublisher_Ping(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:59999", "", nil, RedisPoolOptions{})
	require.NoError(
		t, err)

	defer client.Close()

	pub := NewRedisPublisher(client)
	ctx := t.Context()
	err = pub.Ping(ctx)
	assert.Error(t,
		err)
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
	require.NoError(
		t, err)

	defer client.Close()

	opts := client.Options()
	assert.Equal(t,
		pool.PoolSize,

		opts.PoolSize)
	assert.Equal(t,
		pool.MinIdleConns,

		opts.MinIdleConns)
	assert.Equal(t,
		pool.ReadTimeout,

		opts.ReadTimeout)
	assert.Equal(t,
		pool.WriteTimeout,

		opts.WriteTimeout)
	assert.Equal(t,
		pool.ConnMaxLifetime,

		opts.ConnMaxLifetime)
}

func TestNewRedisClient_EmptyPoolOptions_KeepsURLDefaults(t *testing.T) {
	t.Parallel()
	client, err := NewRedisClient("redis://localhost:6379", "", nil, RedisPoolOptions{})
	require.NoError(
		t, err)

	defer client.Close()

	opts := client.Options()
	assert.NotEqual(
		t, 0, opts.
			PoolSize)
	assert.Positive(t,
		opts.ReadTimeout)

	// go-redis defaults: PoolSize = 10*GOMAXPROCS, MinIdleConns = 0.
}

func TestNewRedisClient_SentinelAppliesPoolOptions(t *testing.T) {
	t.Parallel()
	pool := RedisPoolOptions{
		PoolSize:    15,
		ReadTimeout: 500 * time.Millisecond,
	}

	client, err := NewRedisClient("", "mymaster", []string{"localhost:26379"}, pool)
	require.NoError(
		t, err)

	defer client.Close()

	opts := client.Options()
	assert.Equal(t,
		pool.PoolSize,

		opts.PoolSize)
	assert.Equal(t,
		pool.ReadTimeout,

		opts.ReadTimeout)
}
