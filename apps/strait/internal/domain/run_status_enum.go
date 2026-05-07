package domain

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// Typed enum plumbing for RunStatus.
//
// RunStatus is already `type RunStatus string` in types.go, but it lacked
// Scan/Value integration with database/sql and a few grouping helpers used
// by the queue hot path. Adding them here keeps types.go lean and makes
// the enum interchange-safe in all directions (struct, DB, JSON).

// Scan implements database/sql.Scanner so RunStatus fields can be scanned
// directly from pgx without an intermediate string variable. Pointer
// receiver is required by the Scanner interface.
func (s *RunStatus) Scan(src any) error {
	if src == nil {
		*s = ""
		return nil
	}
	switch v := src.(type) {
	case string:
		*s = RunStatus(v)
	case []byte:
		*s = RunStatus(v)
	case RunStatus:
		*s = v
	default:
		return fmt.Errorf("RunStatus.Scan: unsupported type %T", src)
	}
	if *s != "" && !s.IsValid() {
		return fmt.Errorf("RunStatus.Scan: unknown status %q", string(*s))
	}
	return nil
}

// Value implements database/sql/driver.Valuer so RunStatus can be used as
// a positional parameter without an explicit string conversion.
func (s RunStatus) Value() (driver.Value, error) {
	if s == "" {
		return nil, nil
	}
	if !s.IsValid() {
		return nil, fmt.Errorf("RunStatus.Value: invalid status %q", string(s))
	}
	return string(s), nil
}

// IsActive returns true when the run is consuming a concurrency slot.
// Symmetric with the job_active_counts trigger predicate in migration 186.
func (s RunStatus) IsActive() bool {
	switch s {
	case StatusDequeued, StatusExecuting:
		return true
	default:
		return false
	}
}

// IsClaimable returns true when a dequeue could move the run into the
// active set. This is the predicate the dequeue CTE filters on.
func (s RunStatus) IsClaimable() bool {
	return s == StatusQueued
}

// IsDeadLetter returns true for the dead_letter status. Dead-letter runs
// are also terminal (see IsTerminal); use this predicate when callers need
// to distinguish a permanently-failed run from a normally-completed one
// without a direct string compare.
func (s RunStatus) IsDeadLetter() bool {
	return s == StatusDeadLetter
}

// IsFailure returns true for any terminal status that represents a
// failure mode (excluding canceled/expired which are user-initiated).
func (s RunStatus) IsFailure() bool {
	switch s {
	case StatusFailed, StatusTimedOut, StatusCrashed, StatusSystemFailed, StatusDeadLetter:
		return true
	default:
		return false
	}
}

// ParseRunStatus validates and returns a RunStatus. Returns an error for
// unknown values so callers at trust boundaries (HTTP handlers, CDC
// consumers) can reject junk without a second pass.
func ParseRunStatus(raw string) (RunStatus, error) {
	s := RunStatus(raw)
	if !s.IsValid() {
		return "", fmt.Errorf("%w: %q", ErrUnknownRunStatus, raw)
	}
	return s, nil
}

// ErrUnknownRunStatus is returned by ParseRunStatus for values outside
// the closed constant set.
var ErrUnknownRunStatus = errors.New("unknown run status")
