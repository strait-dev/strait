package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
)

// DLQ cap enforcement.
//
// A project or a job can be configured with maximum DLQ depth. When a
// failure is about to move a run into dead_letter and the depth is at or
// above the cap, one of two overflow policies kicks in:
//
//   - drop_oldest: soft-delete the oldest visible dead_letter row for the
//     same (project, job) via MaskOldestDLQRow, then let the new failure
//     enter the DLQ normally.
//   - reject: the caller should not transition to dead_letter; the
//     enforcer reports ErrDLQOverflow and the executor moves the run to
//     system_failed with error_class="dlq_overflow".

// DLQOverflowPolicy selects the behavior when a DLQ cap is reached.
type DLQOverflowPolicy string

const (
	DLQOverflowDropOldest DLQOverflowPolicy = "drop_oldest"
	DLQOverflowReject     DLQOverflowPolicy = "reject"
)

// ErrDLQOverflow is returned by DLQCapEnforcer.EnforceBeforeTransition when
// the cap is full under the reject policy.
var ErrDLQOverflow = errors.New("dlq overflow: cap reached")

// DLQCapStore is the minimal store surface the enforcer needs. Satisfied
// by *store.Queries.
type DLQCapStore interface {
	DLQDepth(ctx context.Context, projectID, jobID string) (int, error)
	DLQDepthByProject(ctx context.Context, projectID string) (int, error)
	MaskOldestDLQRow(ctx context.Context, projectID, jobID string) (string, error)
}

// DLQCapConfig holds enforcement thresholds.
type DLQCapConfig struct {
	MaxPerProject int
	MaxPerJob     int
	Policy        DLQOverflowPolicy
}

// DLQCapEnforcer wraps the store with cap enforcement logic.
type DLQCapEnforcer struct {
	store         DLQCapStore
	config        DLQCapConfig
	logger        *slog.Logger
	overflowCount atomic.Int64
	droppedCount  atomic.Int64
}

// NewDLQCapEnforcer constructs an enforcer. Invalid policies default to
// drop_oldest.
func NewDLQCapEnforcer(s DLQCapStore, cfg DLQCapConfig, logger *slog.Logger) *DLQCapEnforcer {
	cfg.Policy = normalizeDLQOverflowPolicy(cfg.Policy)
	if logger == nil {
		logger = slog.Default()
	}
	return &DLQCapEnforcer{store: s, config: cfg, logger: logger}
}

func normalizeDLQOverflowPolicy(policy DLQOverflowPolicy) DLQOverflowPolicy {
	switch policy {
	case DLQOverflowDropOldest, DLQOverflowReject:
		return policy
	default:
		return DLQOverflowDropOldest
	}
}

// OverflowCount returns the number of overflow events observed. For tests.
func (e *DLQCapEnforcer) OverflowCount() int64 { return e.overflowCount.Load() }

// DroppedCount returns the number of drop_oldest rows masked by the
// enforcer. For tests.
func (e *DLQCapEnforcer) DroppedCount() int64 { return e.droppedCount.Load() }

// EnforceBeforeTransition consults the DLQ depth counters and takes action
// according to the configured policy. Returns (proceed, err):
//
//   - proceed=true, err=nil: the caller should transition the run to
//     dead_letter as planned.
//   - proceed=false, err=ErrDLQOverflow: the caller should transition the
//     run to a supported non-DLQ terminal status instead.
//   - err=<other>: unexpected store failure; the caller should fail open
//     (transition anyway) and log.
func (e *DLQCapEnforcer) EnforceBeforeTransition(ctx context.Context, projectID, jobID string) (bool, error) {
	if e == nil {
		return true, nil
	}
	if e.config.MaxPerJob <= 0 && e.config.MaxPerProject <= 0 {
		return true, nil
	}

	if e.config.MaxPerJob > 0 {
		jobDepth, err := e.store.DLQDepth(ctx, projectID, jobID)
		if err != nil {
			return true, err
		}
		if jobDepth >= e.config.MaxPerJob {
			return e.handleOverflow(ctx, projectID, jobID, "per_job", jobDepth)
		}
	}

	if e.config.MaxPerProject > 0 {
		projDepth, err := e.store.DLQDepthByProject(ctx, projectID)
		if err != nil {
			return true, err
		}
		if projDepth >= e.config.MaxPerProject {
			return e.handleOverflow(ctx, projectID, jobID, "per_project", projDepth)
		}
	}

	return true, nil
}

func (e *DLQCapEnforcer) handleOverflow(ctx context.Context, projectID, jobID, scope string, depth int) (bool, error) {
	switch e.config.Policy {
	case DLQOverflowDropOldest:
		masked, err := e.store.MaskOldestDLQRow(ctx, projectID, jobID)
		if err != nil {
			e.logger.Warn("dlq drop_oldest mask failed; failing open",
				"project_id", projectID, "job_id", jobID, "error", err,
			)
			return true, err
		}
		if masked != "" {
			e.droppedCount.Add(1)
			e.logger.Info("dlq overflow: dropped oldest row",
				"project_id", projectID, "job_id", jobID,
				"masked_id", masked, "scope", scope, "depth", depth,
			)
		}
		return true, nil
	case DLQOverflowReject:
		e.overflowCount.Add(1)
		e.logger.Warn("dlq overflow: rejecting",
			"project_id", projectID, "job_id", jobID,
			"scope", scope, "depth", depth,
		)
		return false, ErrDLQOverflow
	default:
		return true, nil
	}
}
