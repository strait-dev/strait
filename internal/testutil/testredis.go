//go:build integration

package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestRedis wraps a testcontainers Redis instance for integration tests.
type TestRedis struct {
	Client    *redis.Client
	Container *tcredis.RedisContainer
	Addr      string
}

// SetupTestRedis starts a Redis container and returns a connected client.
func SetupTestRedis(ctx context.Context) (*TestRedis, error) {
	container, err := tcredis.Run(ctx, "redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start redis container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get redis connection string: %w", err)
	}

	opts, err := redis.ParseURL(connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &TestRedis{
		Client:    client,
		Container: container,
		Addr:      opts.Addr,
	}, nil
}

// Cleanup closes the Redis client and terminates the container.
func (tr *TestRedis) Cleanup(ctx context.Context) {
	if tr == nil {
		return
	}
	if tr.Client != nil {
		_ = tr.Client.Close()
	}
	if tr.Container != nil {
		_ = tr.Container.Terminate(ctx)
	}
}

// FlushAll removes all keys from Redis.
func (tr *TestRedis) FlushAll(ctx context.Context) error {
	if tr == nil || tr.Client == nil {
		return nil
	}
	return tr.Client.FlushAll(ctx).Err()
}

// TestEnv combines TestDB and TestRedis for full integration test environments.
type TestEnv struct {
	DB    *TestDB
	Redis *TestRedis
}

// SetupTestEnv starts both Postgres and Redis containers.
func SetupTestEnv(ctx context.Context, migrationsPath string) (*TestEnv, error) {
	db, err := SetupTestDB(ctx, migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("setup test db: %w", err)
	}

	r, err := SetupTestRedis(ctx)
	if err != nil {
		db.Cleanup(ctx)
		return nil, fmt.Errorf("setup test redis: %w", err)
	}

	return &TestEnv{DB: db, Redis: r}, nil
}

// Cleanup tears down both containers.
func (env *TestEnv) Cleanup(ctx context.Context) {
	if env == nil {
		return
	}
	env.Redis.Cleanup(ctx)
	env.DB.Cleanup(ctx)
}

// Clean resets both database tables and Redis keys.
func (env *TestEnv) Clean(ctx context.Context) error {
	if err := env.DB.CleanTables(ctx); err != nil {
		return err
	}
	return env.Redis.FlushAll(ctx)
}
