package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"

	"strait/internal/domain"
)

// OrchestratorStore is the subset of store operations required by the build
// orchestrator to pick up, update, and finalise code deployments.
type OrchestratorStore interface {
	ListBuildingDeployments(ctx context.Context, limit int) ([]domain.CodeDeployment, error)
	UpdateCodeDeploymentStatus(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error
	SetActiveDeployment(ctx context.Context, jobID, deploymentID, projectID string) error
}

// Orchestrator polls for code deployments in "building" status, calls Builder
// to execute the container image build, then updates the deployment record with
// the result and activates the new image on the job.
//
// Concurrency is bounded by concurrency; 0 defaults to 2.
type Orchestrator struct {
	store        OrchestratorStore
	builder      *Builder
	pollInterval time.Duration
	concurrency  int
	logger       *slog.Logger
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

// NewOrchestrator creates a new build orchestrator.
func NewOrchestrator(store OrchestratorStore, builder *Builder, opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		store:        store,
		builder:      builder,
		pollInterval: 5 * time.Second,
		concurrency:  2,
		logger:       slog.Default(),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Run polls for and dispatches builds until ctx is cancelled.
// It blocks until the context is done.
func (o *Orchestrator) Run(ctx context.Context) {
	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	sem := make(chan struct{}, o.concurrency)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.dispatch(ctx, sem)
		}
	}
}

// dispatch fetches pending builds and launches goroutines up to the concurrency limit.
func (o *Orchestrator) dispatch(ctx context.Context, sem chan struct{}) {
	ctx, span := otel.Tracer("strait").Start(ctx, "build.Orchestrator.dispatch")
	defer span.End()

	available := cap(sem) - len(sem)
	if available <= 0 {
		return
	}

	deployments, err := o.store.ListBuildingDeployments(ctx, available)
	if err != nil {
		o.logger.Error("list building deployments failed", "error", err)
		return
	}

	for i := range deployments {
		d := deployments[i]

		// Try to acquire a slot.
		select {
		case sem <- struct{}{}:
		default:
			return // All slots taken.
		}

		go func() {
			defer func() { <-sem }()
			o.runBuild(ctx, &d)
		}()
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

// handleBuildFailure marks the deployment as failed and persists the error.
func (o *Orchestrator) handleBuildFailure(ctx context.Context, d *domain.CodeDeployment, buildErr error, log *slog.Logger) {
	log.Error("build failed", "error", buildErr)

	var tarErr *TarballError
	errMsg := buildErr.Error()
	if errors.As(buildErr, &tarErr) {
		// Tarball security failure — surfaced directly to the user.
		errMsg = fmt.Sprintf("source validation failed: %s", tarErr.Reason)
	}

	fields := map[string]any{
		"error_message": truncateString(errMsg, 4096),
	}
	if updateErr := o.store.UpdateCodeDeploymentStatus(ctx, d.ID, domain.DeploymentStatusFailed, fields); updateErr != nil {
		log.Error("failed to mark deployment as failed", "update_error", updateErr)
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
