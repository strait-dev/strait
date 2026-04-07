package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
	"go.opentelemetry.io/otel"

	"strait/internal/domain"
)

// OrchestratorStore is the subset of store operations required by the build
// orchestrator to pick up, update, and finalise code deployments.
type OrchestratorStore interface {
	// ClaimBuildingDeployment atomically selects and claims the oldest unclaimed
	// building deployment for the given workerID. Returns nil, nil when there is
	// nothing to claim.
	ClaimBuildingDeployment(ctx context.Context, workerID string) (*domain.CodeDeployment, error)
	// ReleaseStaleClaimedDeployments resets the claim on building deployments
	// whose build_node_claimed_at is older than olderThan.
	ReleaseStaleClaimedDeployments(ctx context.Context, olderThan time.Duration) (int64, error)
	// ListBuildingDeployments is retained for testing and diagnostic purposes.
	ListBuildingDeployments(ctx context.Context, limit int) ([]domain.CodeDeployment, error)
	UpdateCodeDeploymentStatus(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error
	SetActiveDeployment(ctx context.Context, jobID, deploymentID, projectID string) error
}

// Orchestrator polls for code deployments in "building" status, claims them
// with FOR UPDATE SKIP LOCKED so multiple replicas cannot double-dispatch,
// then calls Builder to execute the container image build and persists the
// outcome.
//
// Concurrency is bounded by concurrency; 0 defaults to 2.
type Orchestrator struct {
	store         OrchestratorStore
	builder       *Builder
	workerID      string
	pollInterval  time.Duration
	staleInterval time.Duration
	concurrency   int
	logger        *slog.Logger
}

// OrchestratorOption configures an Orchestrator.
type OrchestratorOption func(*Orchestrator)

// WithPollInterval sets how often the orchestrator polls for new builds.
func WithPollInterval(d time.Duration) OrchestratorOption {
	return func(o *Orchestrator) { o.pollInterval = d }
}

// WithConcurrency sets the maximum number of concurrent builds.
func WithConcurrency(n int) OrchestratorOption {
	return func(o *Orchestrator) { o.concurrency = n }
}

// WithOrchestratorLogger sets the structured logger.
func WithOrchestratorLogger(l *slog.Logger) OrchestratorOption {
	return func(o *Orchestrator) { o.logger = l }
}

// NewOrchestrator creates a new build orchestrator. Each instance gets a unique
// workerID so stale-claim recovery can identify which node owned an abandoned build.
func NewOrchestrator(store OrchestratorStore, builder *Builder, opts ...OrchestratorOption) *Orchestrator {
	id, _ := uuid.NewV7()
	o := &Orchestrator{
		store:         store,
		builder:       builder,
		workerID:      fmt.Sprintf("orchestrator-%s", id.String()),
		pollInterval:  5 * time.Second,
		staleInterval: 1 * time.Minute,
		concurrency:   2,
		logger:        slog.Default(),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Run polls for and dispatches builds until ctx is cancelled.
// It also runs a periodic recovery ticker that releases stale claims from
// crashed workers. Blocks until the context is done.
func (o *Orchestrator) Run(ctx context.Context) {
	pollTicker := time.NewTicker(o.pollInterval)
	staleTicker := time.NewTicker(o.staleInterval)
	defer pollTicker.Stop()
	defer staleTicker.Stop()

	// sem tracks in-flight builds and bounds concurrency.
	sem := make(chan struct{}, o.concurrency)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			o.dispatch(ctx, sem)
		case <-staleTicker.C:
			o.releaseStale(ctx)
		}
	}
}

// dispatch attempts to claim one unclaimed building deployment per available
// concurrency slot. Each claimed deployment is executed in a goroutine managed
// by conc.WaitGroup so panics are recovered and propagated safely.
func (o *Orchestrator) dispatch(ctx context.Context, sem chan struct{}) {
	ctx, span := otel.Tracer("strait").Start(ctx, "build.Orchestrator.dispatch")
	defer span.End()

	var wg conc.WaitGroup

dispatch:
	for {
		// Try to acquire a concurrency slot without blocking.
		select {
		case sem <- struct{}{}:
		default:
			break dispatch // All slots taken.
		}

		d, err := o.store.ClaimBuildingDeployment(ctx, o.workerID)
		if err != nil {
			<-sem // release slot
			o.logger.Error("claim building deployment failed", "error", err)
			break dispatch
		}
		if d == nil {
			<-sem // release slot — queue is empty
			break dispatch
		}

		dep := d
		wg.Go(func() {
			defer func() { <-sem }()
			o.runBuild(ctx, dep)
		})
	}

	wg.Wait()
}

// releaseStale releases claims on building deployments held longer than
// builder.timeout*2 so that work orphaned by a crashed orchestrator can be
// reclaimed by another replica. Defaults to 30 minutes when no builder is set.
func (o *Orchestrator) releaseStale(ctx context.Context) {
	staleCutoff := 30 * time.Minute
	if o.builder != nil && o.builder.timeout > 0 {
		staleCutoff = o.builder.timeout * 2
	}
	released, err := o.store.ReleaseStaleClaimedDeployments(ctx, staleCutoff)
	if err != nil {
		o.logger.Error("release stale claimed deployments failed", "error", err)
		return
	}
	if released > 0 {
		o.logger.Warn("released stale build claims", "count", released, "stale_cutoff", staleCutoff)
	}
}

// runBuild executes the full build pipeline for a single deployment and
// persists the outcome (ready + activate, or failed with error message).
func (o *Orchestrator) runBuild(ctx context.Context, d *domain.CodeDeployment) {
	ctx, span := otel.Tracer("strait").Start(ctx, "build.Orchestrator.runBuild")
	defer span.End()

	log := o.logger.With("deployment_id", d.ID, "job_id", d.JobID, "runtime", d.Runtime)
	log.Info("starting build")

	if o.builder == nil {
		o.handleBuildFailure(ctx, d, errors.New("build orchestrator has no builder configured"), log)
		return
	}

	result, err := o.builder.Build(ctx, d)
	if err != nil {
		o.handleBuildFailure(ctx, d, err, log)
		return
	}

	// Persist success: mark ready with image URI + digest.
	fields := map[string]any{
		"built_image_uri":    result.ImageURI,
		"built_image_digest": result.Digest,
		"build_logs":         truncateLogs(result.BuildLogs),
	}
	if updateErr := o.store.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusReady, fields); updateErr != nil {
		log.Error("failed to mark deployment ready", "error", updateErr)
		return
	}

	// Activate: make this deployment the live version on the job.
	if activateErr := o.store.SetActiveDeployment(ctx, d.JobID, d.ID, d.ProjectID); activateErr != nil {
		// Log but don't fail the build — the image was pushed successfully.
		// The operator can manually activate via the rollback API.
		log.Error("build succeeded but failed to set active deployment",
			"error", activateErr,
			"image", result.ImageURI,
		)
		return
	}

	log.Info("build succeeded and deployment activated",
		"image", result.ImageURI,
		"digest", result.Digest,
	)
}

// handleBuildFailure marks the deployment as failed (or timed_out) and
// persists the error message. Context cancellation / deadline exceeded
// produces a timed_out status so the CLI can surface a clear message.
func (o *Orchestrator) handleBuildFailure(ctx context.Context, d *domain.CodeDeployment, buildErr error, log *slog.Logger) {
	log.Error("build failed", "error", buildErr)

	status := domain.DeploymentStatusFailed
	if errors.Is(buildErr, context.DeadlineExceeded) || errors.Is(buildErr, context.Canceled) {
		status = domain.DeploymentStatusTimedOut
	}

	var tarErr *TarballError
	errMsg := buildErr.Error()
	if errors.As(buildErr, &tarErr) {
		// Tarball security failure — surfaced directly to the user.
		errMsg = fmt.Sprintf("source validation failed: %s", tarErr.Reason)
	}

	fields := map[string]any{
		"error_message": truncateString(errMsg, 4096),
	}
	if updateErr := o.store.UpdateCodeDeploymentStatus(ctx, d.ID, status, fields); updateErr != nil {
		log.Error("failed to mark deployment as failed", "update_error", updateErr, "status", status)
	}
}

// truncateLogs caps build log output at 1 MB to avoid bloating the DB.
const maxBuildLogsBytes = 1 * 1024 * 1024

func truncateLogs(s string) string {
	return truncateString(s, maxBuildLogsBytes)
}

func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes]
}
