package queue

// Strait's pgque queue engine uses a vendored and modified SQL snapshot of
// PgQue, PgQ Universal Edition: https://github.com/NikolayS/pgque.
// PgQue is Apache-2.0 licensed and includes code derived from PgQ, originally
// developed at Skype Technologies OU by Marko Kreen under the ISC License.
// Permission to use, copy, modify, and distribute the PgQ-derived portions is
// granted with copyright and permission notices retained; those portions are
// provided "AS IS" without warranty.
// Strait uses PgQue as its PostgreSQL ready-event log; Strait owns run state,
// execution ownership, retries, workflows, workers, observability, and APIs.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

const (
	pgQueConsumerName               = "strait"
	pgQueReceiveAll                 = 2147483647
	pgQueMaxAttempts                = 3
	pgQueMaxCatchUpBatches          = 1024
	pgQueDefaultMaintenanceInterval = 30 * time.Second
	pgQueDefaultRotationPeriod      = 5 * time.Minute
)

type PgQueConfig struct {
	TickInterval        time.Duration
	MaintenanceInterval time.Duration
	RotationPeriod      time.Duration
	ConsumerName        string
	NackDelay           time.Duration
	ReceiveWindow       int
	Logger              *slog.Logger
}

func (c PgQueConfig) normalized() PgQueConfig {
	if c.TickInterval <= 0 {
		c.TickInterval = 50 * time.Millisecond
	}
	if c.MaintenanceInterval <= 0 {
		c.MaintenanceInterval = pgQueDefaultMaintenanceInterval
	}
	if c.RotationPeriod <= 0 {
		c.RotationPeriod = pgQueDefaultRotationPeriod
	}
	if c.ConsumerName == "" {
		c.ConsumerName = pgQueConsumerName
	}
	if c.NackDelay <= 0 {
		c.NackDelay = time.Second
	}
	if c.ReceiveWindow <= 0 {
		c.ReceiveWindow = 100
	}
	return c
}

type PgQueQueue struct {
	db            store.DBTX
	runWriter     *PostgresRunWriter
	cfg           PgQueConfig
	logger        *slog.Logger
	routeMu       sync.Mutex
	routeStates   map[string]*pgQueRouteState
	routeCache    map[string]pgQueRouteCacheEntry
	routeRefCache map[domain.WorkerQueueRef]pgQueRouteCacheEntry

	workerRouteCursor atomic.Uint64
}

type pgQueRouteState struct {
	mu            sync.Mutex
	configMu      sync.Mutex
	configured    atomic.Bool
	lastForceTick time.Time
	activeBatch   *pgQueActiveBatch
}

type pgQueRouteCacheEntry struct {
	routes    []string
	expiresAt time.Time
}

type pgQueReadyEvent struct {
	RunID      string `json:"run_id"`
	RouteKey   string `json:"route_key"`
	Generation int64  `json:"generation"`
	Priority   int    `json:"priority"`
}

type pgQueMessage struct {
	ID         int64
	BatchID    int64
	Type       string
	Payload    string
	RetryCount *int32
	CreatedAt  time.Time
	Extra1     *string
	Extra2     *string
	Extra3     *string
	Extra4     *string
}

func NewPgQueQueue(db store.DBTX, runWriter *PostgresRunWriter, cfg PgQueConfig) *PgQueQueue {
	if runWriter == nil {
		runWriter = NewPostgresRunWriter(db)
	}
	cfg = cfg.normalized()
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &PgQueQueue{
		db:            db,
		runWriter:     runWriter,
		cfg:           cfg,
		logger:        logger,
		routeStates:   make(map[string]*pgQueRouteState),
		routeCache:    make(map[string]pgQueRouteCacheEntry),
		routeRefCache: make(map[domain.WorkerQueueRef]pgQueRouteCacheEntry),
	}
}

func (q *PgQueQueue) markPgQueStorage(ctx context.Context, db store.DBTX) error {
	if _, err := db.Exec(ctx, `SET LOCAL strait.queue_backend = 'pgque'`); err != nil {
		return fmt.Errorf("pgque mark queue storage: %w", err)
	}
	return nil
}

var _ Queue = (*PgQueQueue)(nil)
var _ interface {
	EnqueueExisting(context.Context, *domain.JobRun) error
} = (*PgQueQueue)(nil)
var _ interface {
	RunTicker(context.Context)
	Maintain(context.Context) error
} = (*PgQueQueue)(nil)
