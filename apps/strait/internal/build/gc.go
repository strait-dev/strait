package build

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"

	"strait/internal/telemetry"
)

// GCStore is the subset of store operations required by the deployment GC worker.
type GCStore interface {
	DeleteExpiredDeployments(ctx context.Context, pendingBefore, failedBefore time.Time) (int64, error)
}

// DeploymentGC periodically deletes stale deployments that are no longer actionable:
//   - pending deployments whose presigned-upload TTL has expired (never uploaded)
//   - failed / timed_out deployments that are older than the configured retention window
//
// It runs as a background goroutine and exits when its context is cancelled.
type DeploymentGC struct {
	store      GCStore
	interval   time.Duration
	pendingTTL time.Duration
	failedAge  time.Duration
	logger     *slog.Logger
	metrics    *telemetry.Metrics
}

// GCOption configures a DeploymentGC.
type GCOption func(*DeploymentGC)

// WithGCInterval sets how often the GC ticker fires.
func WithGCInterval(d time.Duration) GCOption {
	return func(g *DeploymentGC) { g.interval = d }
}

// WithGCPendingTTL sets the age at which a pending deployment is considered expired.
func WithGCPendingTTL(d time.Duration) GCOption {
	return func(g *DeploymentGC) { g.pendingTTL = d }
}

// WithGCFailedAge sets the retention window for failed and timed_out deployments.
func WithGCFailedAge(d time.Duration) GCOption {
	return func(g *DeploymentGC) { g.failedAge = d }
}

// WithGCLogger sets the structured logger.
func WithGCLogger(l *slog.Logger) GCOption {
	return func(g *DeploymentGC) { g.logger = l }
}

// WithGCMetrics wires Prometheus metrics into the GC worker so the number of
// deployments collected per sweep is recorded.
func WithGCMetrics(m *telemetry.Metrics) GCOption {
	return func(g *DeploymentGC) { g.metrics = m }
}

// NewDeploymentGC creates a deployment GC worker with the given options.
func NewDeploymentGC(store GCStore, opts ...GCOption) *DeploymentGC {
	g := &DeploymentGC{
		store:      store,
		interval:   1 * time.Hour,
		pendingTTL: 15 * time.Minute,
		failedAge:  7 * 24 * time.Hour,
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Run starts the GC ticker loop. It blocks until ctx is cancelled.
func (g *DeploymentGC) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.collect(ctx)
		}
	}
}

// collect performs one GC pass and logs the outcome.
func (g *DeploymentGC) collect(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "build.DeploymentGC.collect")
	defer span.End()

	now := time.Now()
	pendingBefore := now.Add(-g.pendingTTL)
	failedBefore := now.Add(-g.failedAge)

	deleted, err := g.store.DeleteExpiredDeployments(ctx, pendingBefore, failedBefore)
	if err != nil {
		g.logger.Error("deployment GC failed", "error", err)
		return
	}
	if deleted > 0 {
		g.logger.Info("deployment GC collected expired deployments",
			"count", deleted,
			"pending_before", pendingBefore,
			"failed_before", failedBefore,
		)
		if g.metrics != nil {
			g.metrics.CodeDeployGCCollected.Add(ctx, deleted)
		}
	}
}
