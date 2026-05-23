package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// First-class singleton / mutex execution (STR-542).
//
// A singleton-configured job or workflow allows at most one active run per
// resolved key. The lock table (singleton_locks) is the source of truth for
// "who holds key K" for a given owner; SingletonOnConflict decides what happens
// to a second arrival while the key is held.

// maxSingletonTemplateLen bounds a user-supplied singleton key template to keep
// resolution cheap and prevent pathological inputs.
const maxSingletonTemplateLen = 1024

// SingletonKind distinguishes lock owners so a job and a workflow sharing an id
// space can never collide on the same lock key.
type SingletonKind string

const (
	SingletonKindJob      SingletonKind = "job"
	SingletonKindWorkflow SingletonKind = "workflow"
)

// IsValid returns true if the kind is a known lock-owner kind.
func (k SingletonKind) IsValid() bool {
	switch k {
	case SingletonKindJob, SingletonKindWorkflow:
		return true
	default:
		return false
	}
}

// SingletonLock is one row of the singleton_locks table: the live record of
// which run currently holds a resolved key for a given owner.
type SingletonLock struct {
	ProjectID   string        `json:"project_id"`
	Kind        SingletonKind `json:"kind"`
	OwnerID     string        `json:"owner_id"`
	LockKey     string        `json:"lock_key"`
	HolderRunID string        `json:"holder_run_id"`
	AcquiredAt  time.Time     `json:"acquired_at"`
	// LeaseUntil is set for job-run holders (extended by the heartbeat batch) and
	// NULL for workflow-run holders, which are reclaimed only on terminal/missing.
	LeaseUntil *time.Time `json:"lease_until,omitempty"`
}

// SingletonOnConflict controls what happens when a singleton run is triggered
// while another run already holds the resolved key.
//
//   - queue:   park the newcomer behind the holder; promote it when the key frees.
//   - drop:    discard the newcomer; the holder keeps running (0 billable runs).
//   - replace: cancel the holder and run the newcomer in its place.
//
// A nil/empty value means the owner is not a singleton.
type SingletonOnConflict string

const (
	SingletonOnConflictQueue   SingletonOnConflict = "queue"
	SingletonOnConflictDrop    SingletonOnConflict = "drop"
	SingletonOnConflictReplace SingletonOnConflict = "replace"
)

// Valid returns true if the policy is a known on-conflict value.
func (p SingletonOnConflict) Valid() bool {
	switch p {
	case SingletonOnConflictQueue, SingletonOnConflictDrop, SingletonOnConflictReplace:
		return true
	default:
		return false
	}
}

// SingletonOutcome reports what happened to a trigger that resolved a singleton
// key. Returned additively on the trigger response when a singleton is
// configured; omitted otherwise.
type SingletonOutcome string

const (
	// SingletonOutcomeDispatched means the run acquired the key and was enqueued.
	SingletonOutcomeDispatched SingletonOutcome = "dispatched"
	// SingletonOutcomeQueuedBehind means the run was parked behind the holder.
	SingletonOutcomeQueuedBehind SingletonOutcome = "queued_behind"
	// SingletonOutcomeDropped means the run was discarded (drop policy, or queue
	// policy at/over its depth cap). No run is created.
	SingletonOutcomeDropped SingletonOutcome = "dropped"
	// SingletonOutcomeReplaced means the holder was canceled in favor of the
	// newcomer (replace policy).
	SingletonOutcomeReplaced SingletonOutcome = "replaced"
)

// IsValid returns true if the outcome is a known value.
func (o SingletonOutcome) IsValid() bool {
	switch o {
	case SingletonOutcomeDispatched, SingletonOutcomeQueuedBehind,
		SingletonOutcomeDropped, SingletonOutcomeReplaced:
		return true
	default:
		return false
	}
}

// SingletonKeyExpr is the JSONB envelope describing how a run's singleton key is
// resolved at trigger time. The template supports ${dot.path} interpolation
// against the trigger payload (resolved by ResolveSingletonKey). A template with
// no interpolation is a constant key (a global mutex for the owner).
//
// Modeled on eventfilter.FilterExpr: a small typed envelope decoded from the
// stored JSONB with unknown fields rejected.
type SingletonKeyExpr struct {
	Template string `json:"template"`
}

// ParseSingletonKeyExpr decodes and validates a singleton key expression
// envelope. An empty/null raw value yields the zero expression with no error so
// callers can treat "no expression" uniformly.
func ParseSingletonKeyExpr(raw json.RawMessage) (SingletonKeyExpr, error) {
	var expr SingletonKeyExpr
	if len(raw) == 0 || string(raw) == "null" {
		return expr, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expr); err != nil {
		return expr, fmt.Errorf("invalid singleton key expression: %w", err)
	}
	if err := expr.Validate(); err != nil {
		return expr, err
	}
	return expr, nil
}

// IsZero reports whether the expression carries no template (no singleton key).
func (e SingletonKeyExpr) IsZero() bool {
	return e.Template == ""
}

// Validate checks the envelope's invariants. A zero expression is invalid here;
// callers that allow "no expression" must short-circuit on IsZero / empty raw
// before validating.
func (e SingletonKeyExpr) Validate() error {
	if e.Template == "" {
		return fmt.Errorf("singleton key expression template must not be empty")
	}
	if len(e.Template) > maxSingletonTemplateLen {
		return fmt.Errorf("singleton key expression template exceeds %d bytes", maxSingletonTemplateLen)
	}
	return nil
}
