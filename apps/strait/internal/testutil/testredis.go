//go:build integration

package testutil

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
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
	DB        int
}

func (tr *TestRedis) Options() *redis.Options {
	if tr == nil {
		return &redis.Options{}
	}
	return &redis.Options{Addr: tr.Addr, DB: tr.DB}
}

// SetupTestRedis returns an isolated Redis logical DB. By default it reuses a
// shared Redis container and allocates a DB index for the caller. Set
// STRAIT_TEST_FRESH_CONTAINER=1 to force the historical fresh-container path.
func SetupTestRedis(ctx context.Context) (*TestRedis, error) {
	if os.Getenv("STRAIT_TEST_FRESH_CONTAINER") == "1" {
		return SetupFreshTestRedis(ctx)
	}
	return SetupSharedTestRedis(ctx, "default-"+randomHex(6))
}

func SetupSharedTestRedis(ctx context.Context, namespace string) (*TestRedis, error) {
	defer testTiming("SetupSharedTestRedis " + namespace)()

	shared, err := getSharedRedis(ctx)
	if err != nil {
		return nil, err
	}

	admin := redis.NewClient(shared.Options)
	defer func() {
		_ = admin.Close()
	}()
	if err := waitForRedisReady(ctx, admin); err != nil {
		return nil, fmt.Errorf("wait for shared redis readiness: %w", err)
	}

	dbIndex, err := allocateRedisDB(ctx, admin, namespace)
	if err != nil {
		return nil, err
	}

	opts := *shared.Options
	opts.DB = dbIndex
	client := redis.NewClient(&opts)
	if err := waitForRedisReady(ctx, client); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("wait for shared redis db %d: %w", dbIndex, err)
	}
	return &TestRedis{
		Client: client,
		Addr:   opts.Addr,
		DB:     dbIndex,
	}, nil
}

func SetupFreshTestRedis(ctx context.Context) (*TestRedis, error) {
	defer testTiming("SetupFreshTestRedis")()

	container, err := tcredis.Run(ctx, "redis:8-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("6379/tcp").
				SkipInternalCheck().
				WithStartupTimeout(60*time.Second),
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

type sharedRedis struct {
	Container *tcredis.RedisContainer
	Options   *redis.Options
}

var (
	sharedRedisMu  sync.Mutex
	sharedRedisVal *sharedRedis
)

func getSharedRedis(ctx context.Context) (*sharedRedis, error) {
	sharedRedisMu.Lock()
	defer sharedRedisMu.Unlock()
	if sharedRedisVal != nil {
		return sharedRedisVal, nil
	}
	if err := configurePersistentTestcontainers(); err != nil {
		return nil, err
	}

	container, err := tcredis.Run(ctx, "redis:8-alpine",
		testcontainers.WithCmdArgs("--databases", "4096"),
		testcontainers.WithReuseByName(sharedContainerName("redis")),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("6379/tcp").
				SkipInternalCheck().
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start shared redis container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		return nil, fmt.Errorf("get shared redis connection string: %w", err)
	}
	opts, err := redis.ParseURL(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse shared redis url: %w", err)
	}
	opts.DB = 0

	sharedRedisVal = &sharedRedis{Container: container, Options: opts}
	return sharedRedisVal, nil
}

func waitForRedisReady(ctx context.Context, client *redis.Client) error {
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for {
		if err := client.Ping(ctx).Err(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("ping redis: %w", lastErr)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func allocateRedisDB(ctx context.Context, client *redis.Client, namespace string) (int, error) {
	script := redis.NewScript(`
local existing = redis.call("HGET", KEYS[1], ARGV[1])
if existing then
	return existing
end
local next = redis.call("INCR", KEYS[2])
local db = (next % tonumber(ARGV[2])) + 1
redis.call("HSET", KEYS[1], ARGV[1], db)
return db
`)
	value, err := script.Run(ctx, client, []string{"strait:test:redis:dbs", "strait:test:redis:seq"}, namespace, "4095").Result()
	if err != nil {
		return 0, fmt.Errorf("allocate shared redis db: %w", err)
	}
	switch v := value.(type) {
	case int64:
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("parse shared redis db %q: %w", v, err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unexpected shared redis db value %T", value)
	}
}

// Cleanup closes the Redis client and terminates the container.
func (tr *TestRedis) Cleanup(ctx context.Context) {
	if tr == nil {
		return
	}
	if tr.Client != nil {
		if tr.DB > 0 {
			_ = tr.Client.FlushDB(ctx).Err()
		}
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
	if tr.DB > 0 {
		return tr.Client.FlushDB(ctx).Err()
	}
	return tr.Client.FlushAll(ctx).Err()
}

// TestEnv combines TestDB and TestRedis for full integration test environments.
type TestEnv struct {
	DB    *TestDB
	Redis *TestRedis
}

// SetupTestEnv returns isolated Postgres and Redis fixtures.
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

func SetupSharedTestEnv(ctx context.Context, migrationsPath, namespace string) (*TestEnv, error) {
	db, err := SetupSharedTestDB(ctx, migrationsPath, namespace)
	if err != nil {
		return nil, fmt.Errorf("setup shared test db: %w", err)
	}

	r, err := SetupSharedTestRedis(ctx, namespace)
	if err != nil {
		db.Cleanup(ctx)
		return nil, fmt.Errorf("setup shared test redis: %w", err)
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
