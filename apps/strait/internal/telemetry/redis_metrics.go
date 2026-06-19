package telemetry

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type redisMetricsHook struct {
	metrics  redisRuntimeMetrics
	poolName string
	stats    redisPoolStatsProvider
}

type redisRuntimeMetrics struct {
	commandDuration metric.Float64Histogram
	poolActive      metric.Int64Gauge
}

type redisPoolStatsProvider interface {
	PoolStats() *redis.PoolStats
}

func NewRedisMetricsHook(poolName string, stats redisPoolStatsProvider) redis.Hook {
	return newRedisMetricsHook(newRedisRuntimeMetrics(otel.Meter("strait/redis")), poolName, stats)
}

func newRedisRuntimeMetrics(meter metric.Meter) redisRuntimeMetrics {
	commandDuration, _ := meter.Float64Histogram(
		"strait_redis_command_duration_seconds",
		metric.WithDescription("Redis command duration by command and outcome"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1),
	)
	poolActive, _ := meter.Int64Gauge(
		"strait_redis_pool_active",
		metric.WithDescription("Redis active connections in the client pool"),
		metric.WithUnit("1"),
	)
	return redisRuntimeMetrics{
		commandDuration: commandDuration,
		poolActive:      poolActive,
	}
}

func newRedisMetricsHook(metrics redisRuntimeMetrics, poolName string, stats redisPoolStatsProvider) redis.Hook {
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		poolName = "default"
	}
	return redisMetricsHook{
		metrics:  metrics,
		poolName: poolName,
		stats:    stats,
	}
}

func (h redisMetricsHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h redisMetricsHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		started := time.Now()
		err := next(ctx, cmd)
		h.recordCommand(ctx, cmd.Name(), time.Since(started), err)
		h.recordPool(ctx)
		return err
	}
}

func (h redisMetricsHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		started := time.Now()
		err := next(ctx, cmds)
		h.recordCommand(ctx, "pipeline", time.Since(started), err)
		h.recordPool(ctx)
		return err
	}
}

func (h redisMetricsHook) recordCommand(ctx context.Context, command string, duration time.Duration, err error) {
	h.metrics.commandDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("command", normalizeRedisCommand(command)),
		attribute.String("outcome", redisCommandOutcome(err)),
	))
}

func (h redisMetricsHook) recordPool(ctx context.Context) {
	if h.stats == nil {
		return
	}
	stats := h.stats.PoolStats()
	if stats == nil {
		return
	}
	active := int64(0)
	if stats.TotalConns > stats.IdleConns {
		active = int64(stats.TotalConns - stats.IdleConns)
	}
	h.metrics.poolActive.Record(ctx, active, metric.WithAttributes(attribute.String("pool", h.poolName)))
}

func normalizeRedisCommand(command string) string {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return "unknown"
	}
	if idx := strings.IndexAny(command, " \t\r\n"); idx >= 0 {
		command = command[:idx]
	}
	return command
}

func redisCommandOutcome(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, redis.Nil) {
		return "miss"
	}
	return "error"
}
