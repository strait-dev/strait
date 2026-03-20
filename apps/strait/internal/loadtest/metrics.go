//go:build loadtest

package loadtest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const defaultMaxFileSize int64 = 100 * 1024 * 1024 // 100MB

// MetricsCollector gathers system metrics at regular intervals
// and writes them to JSONL files for later analysis.
type MetricsCollector struct {
	pool      *pgxpool.Pool
	redis     *redis.Client
	outputDir string
	interval  time.Duration

	mu        sync.Mutex
	snapshots []MetricsSnapshot

	fileMu       sync.Mutex
	file         *os.File
	writer       *bufio.Writer
	maxFileSize  int64
	bytesWritten int64
	fileIndex    int
	filePrefix   string

	cancel context.CancelFunc
	done   chan struct{}
}

// MetricsSnapshot is a single point-in-time measurement.
type MetricsSnapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	Go        GoMetrics     `json:"go"`
	Postgres  *PGMetrics    `json:"postgres,omitempty"`
	Redis     *RedisMetrics `json:"redis,omitempty"`
	App       *AppMetrics   `json:"app,omitempty"`
}

// GoMetrics captures Go runtime stats.
type GoMetrics struct {
	Goroutines int    `json:"goroutines"`
	HeapAlloc  uint64 `json:"heap_alloc_bytes"`
	HeapSys    uint64 `json:"heap_sys_bytes"`
	HeapInuse  uint64 `json:"heap_inuse_bytes"`
	StackInuse uint64 `json:"stack_inuse_bytes"`
	GCPauseNs  uint64 `json:"gc_pause_ns"`
	NumGC      uint32 `json:"num_gc"`
	GCCPUFrac  float64 `json:"gc_cpu_fraction"`
}

// PGMetrics captures PostgreSQL connection pool and activity stats.
type PGMetrics struct {
	ActiveConns   int32 `json:"active_connections"`
	IdleConns     int32 `json:"idle_connections"`
	TotalConns    int32 `json:"total_connections"`
	MaxConns      int32 `json:"max_connections"`
	WaitCount     int64 `json:"wait_count"`
	WaitDurationMs int64 `json:"wait_duration_ms"`
}

// RedisMetrics captures Redis server stats.
type RedisMetrics struct {
	ConnectedClients int64  `json:"connected_clients"`
	UsedMemoryBytes  int64  `json:"used_memory_bytes"`
	OpsPerSec        int64  `json:"ops_per_sec"`
	KeyspaceHits     int64  `json:"keyspace_hits"`
	KeyspaceMisses   int64  `json:"keyspace_misses"`
	Error            string `json:"error,omitempty"`
}

// AppMetrics captures application-level metrics.
type AppMetrics struct {
	QueueDepth     int64   `json:"queue_depth"`
	DequeueRate    float64 `json:"dequeue_rate_per_sec"`
	ErrorRate      float64 `json:"error_rate_per_sec"`
	ActiveRuns     int64   `json:"active_runs"`
	CompletedRuns  int64   `json:"completed_runs"`
	FailedRuns     int64   `json:"failed_runs"`
}

// MetricsCollectorConfig configures the metrics collector.
type MetricsCollectorConfig struct {
	Pool      *pgxpool.Pool
	Redis     *redis.Client
	OutputDir string
	Interval  time.Duration
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(cfg MetricsCollectorConfig) (*MetricsCollector, error) {
	if cfg.Interval == 0 {
		cfg.Interval = 10 * time.Second
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "loadtest-results"
	}

	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	return &MetricsCollector{
		pool:        cfg.Pool,
		redis:       cfg.Redis,
		outputDir:   cfg.OutputDir,
		interval:    cfg.Interval,
		maxFileSize: defaultMaxFileSize,
		done:        make(chan struct{}),
	}, nil
}

// Start begins collecting metrics in a background goroutine.
func (mc *MetricsCollector) Start(ctx context.Context) error {
	mc.filePrefix = fmt.Sprintf("metrics_%s", time.Now().Format("2006-01-02T15-04-05"))
	mc.fileIndex = 0

	if err := mc.openNewFile(); err != nil {
		return err
	}

	ctx, mc.cancel = context.WithCancel(ctx)

	go mc.collectLoop(ctx)
	return nil
}

// Stop halts metrics collection and flushes remaining data.
func (mc *MetricsCollector) Stop() error {
	if mc.cancel != nil {
		mc.cancel()
	}
	<-mc.done

	mc.fileMu.Lock()
	defer mc.fileMu.Unlock()

	if mc.writer != nil {
		if err := mc.writer.Flush(); err != nil {
			return fmt.Errorf("flushing metrics writer: %w", err)
		}
	}
	if mc.file != nil {
		return mc.file.Close()
	}
	return nil
}

// Snapshots returns all collected snapshots.
func (mc *MetricsCollector) Snapshots() []MetricsSnapshot {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	result := make([]MetricsSnapshot, len(mc.snapshots))
	copy(result, mc.snapshots)
	return result
}

func (mc *MetricsCollector) openNewFile() error {
	filename := filepath.Join(mc.outputDir, fmt.Sprintf("%s_%d.jsonl", mc.filePrefix, mc.fileIndex))

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating metrics file: %w", err)
	}

	mc.file = f
	mc.writer = bufio.NewWriterSize(f, 64*1024) // 64KB buffer
	mc.bytesWritten = 0
	return nil
}

func (mc *MetricsCollector) rotateFile() error {
	if mc.writer != nil {
		if err := mc.writer.Flush(); err != nil {
			return fmt.Errorf("flushing metrics writer during rotation: %w", err)
		}
	}
	if mc.file != nil {
		if err := mc.file.Close(); err != nil {
			return fmt.Errorf("closing metrics file during rotation: %w", err)
		}
	}

	mc.fileIndex++
	return mc.openNewFile()
}

func (mc *MetricsCollector) collectLoop(ctx context.Context) {
	defer close(mc.done)

	ticker := time.NewTicker(mc.interval)
	defer ticker.Stop()

	// Collect initial snapshot
	mc.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			// Final collection
			mc.collect(context.Background())
			return
		case <-ticker.C:
			mc.collect(ctx)
		}
	}
}

func (mc *MetricsCollector) collect(ctx context.Context) {
	snap := MetricsSnapshot{
		Timestamp: time.Now(),
		Go:        collectGoMetrics(),
	}

	if mc.pool != nil {
		snap.Postgres = collectPGMetrics(mc.pool)
	}

	if mc.redis != nil {
		snap.Redis = collectRedisMetrics(ctx, mc.redis)
	}

	mc.mu.Lock()
	mc.snapshots = append(mc.snapshots, snap)
	mc.mu.Unlock()

	// Write to JSONL file under a separate mutex
	mc.fileMu.Lock()
	defer mc.fileMu.Unlock()

	if mc.writer != nil {
		data, err := json.Marshal(snap)
		if err == nil {
			n, _ := mc.writer.Write(data)
			mc.writer.Write([]byte("\n"))
			mc.bytesWritten += int64(n) + 1

			// Check if rotation is needed
			if mc.bytesWritten >= mc.maxFileSize {
				mc.rotateFile()
			}
		}
	}
}

func collectGoMetrics() GoMetrics {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var lastPause uint64
	if m.NumGC > 0 {
		lastPause = m.PauseNs[(m.NumGC+255)%256]
	}

	return GoMetrics{
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.HeapAlloc,
		HeapSys:    m.HeapSys,
		HeapInuse:  m.HeapInuse,
		StackInuse: m.StackInuse,
		GCPauseNs:  lastPause,
		NumGC:      m.NumGC,
		GCCPUFrac:  m.GCCPUFraction,
	}
}

func collectPGMetrics(pool *pgxpool.Pool) *PGMetrics {
	stat := pool.Stat()
	return &PGMetrics{
		ActiveConns:    stat.AcquiredConns(),
		IdleConns:      stat.IdleConns(),
		TotalConns:     stat.TotalConns(),
		MaxConns:       stat.MaxConns(),
		WaitCount:      stat.EmptyAcquireCount(),
		WaitDurationMs: stat.AcquireDuration().Milliseconds(),
	}
}

func collectRedisMetrics(ctx context.Context, client *redis.Client) *RedisMetrics {
	info, err := client.Info(ctx, "stats", "memory", "clients").Result()
	if err != nil {
		return &RedisMetrics{Error: err.Error()}
	}

	metrics := &RedisMetrics{}
	// Parse key metrics from INFO output
	parseRedisInfo(info, metrics)
	return metrics
}

func parseRedisInfo(info string, metrics *RedisMetrics) {
	lines := splitLines(info)
	for _, line := range lines {
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		key, val := splitKV(line)
		switch key {
		case "connected_clients":
			metrics.ConnectedClients = parseInt64(val)
		case "used_memory":
			metrics.UsedMemoryBytes = parseInt64(val)
		case "instantaneous_ops_per_sec":
			metrics.OpsPerSec = parseInt64(val)
		case "keyspace_hits":
			metrics.KeyspaceHits = parseInt64(val)
		case "keyspace_misses":
			metrics.KeyspaceMisses = parseInt64(val)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitKV(s string) (string, string) {
	for i := range len(s) {
		if s[i] == ':' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int64(c-'0')
		}
	}
	return n
}
