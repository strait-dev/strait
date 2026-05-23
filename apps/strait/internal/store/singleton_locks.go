package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
)

// ErrSingletonLockNotFound is returned when no lock row exists for the requested
// (project, kind, owner, key).
var ErrSingletonLockNotFound = errors.New("singleton lock not found")

// AcquireSingletonLock attempts to claim (project, kind, owner, lock_key) for
// lock.HolderRunID. The claim is a single atomic INSERT ... ON CONFLICT DO
// NOTHING, so under concurrent triggers exactly one caller wins.
//
// Returns (true, lock, nil) when the key was claimed (lock.AcquiredAt is filled
// in). Returns (false, holder, nil) when the key is already held; holder is the
// existing lock row. Callers run this inside the run-insert transaction so the
// claim and the run row commit together.
func (q *Queries) AcquireSingletonLock(ctx context.Context, lock domain.SingletonLock) (bool, *domain.SingletonLock, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.AcquireSingletonLock")
	defer span.End()

	if !lock.Kind.IsValid() {
		return false, nil, fmt.Errorf("acquire singleton lock: invalid kind %q", lock.Kind)
	}

	const insert = `
		INSERT INTO singleton_locks (project_id, kind, owner_id, lock_key, holder_run_id, lease_until)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, kind, owner_id, lock_key) DO NOTHING
		RETURNING acquired_at`

	var acquiredAt time.Time
	err := q.db.QueryRow(ctx, insert,
		lock.ProjectID, string(lock.Kind), lock.OwnerID, lock.LockKey, lock.HolderRunID, lock.LeaseUntil,
	).Scan(&acquiredAt)
	if err == nil {
		lock.AcquiredAt = acquiredAt
		return true, &lock, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, nil, fmt.Errorf("acquire singleton lock: %w", err)
	}

	// Conflict: the key is held. Read the current holder so the caller can apply
	// its on-conflict policy.
	holder, herr := q.GetSingletonHolder(ctx, lock.ProjectID, lock.Kind, lock.OwnerID, lock.LockKey)
	if herr != nil {
		return false, nil, fmt.Errorf("acquire singleton lock (read holder): %w", herr)
	}
	return false, holder, nil
}

// GetSingletonHolder returns the lock row for (project, kind, owner, lock_key),
// or ErrSingletonLockNotFound when the key is free.
func (q *Queries) GetSingletonHolder(ctx context.Context, projectID string, kind domain.SingletonKind, ownerID, lockKey string) (*domain.SingletonLock, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetSingletonHolder")
	defer span.End()

	const query = `
		SELECT project_id, kind, owner_id, lock_key, holder_run_id, acquired_at, lease_until
		FROM singleton_locks
		WHERE project_id = $1 AND kind = $2 AND owner_id = $3 AND lock_key = $4`

	lock, err := scanSingletonLock(q.db.QueryRow(ctx, query, projectID, string(kind), ownerID, lockKey))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSingletonLockNotFound
		}
		return nil, fmt.Errorf("get singleton holder: %w", err)
	}
	return lock, nil
}

// ReleaseSingletonLock removes the lock held by holderRunID, if any. It is
// idempotent: releasing a run that holds no lock returns (false, nil). Releasing
// by holder id (indexed) is safe to call from both the terminal-transition
// fast-path and the reaper without coordinating which lock key the run held.
func (q *Queries) ReleaseSingletonLock(ctx context.Context, holderRunID string) (bool, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ReleaseSingletonLock")
	defer span.End()

	const del = `DELETE FROM singleton_locks WHERE holder_run_id = $1`
	tag, err := q.db.Exec(ctx, del, holderRunID)
	if err != nil {
		return false, fmt.Errorf("release singleton lock: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListSingletonLocks returns all live lock rows for an owner (one per held key),
// ordered by acquisition time. Used by the inspection endpoints.
func (q *Queries) ListSingletonLocks(ctx context.Context, projectID string, kind domain.SingletonKind, ownerID string) ([]domain.SingletonLock, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListSingletonLocks")
	defer span.End()

	const query = `
		SELECT project_id, kind, owner_id, lock_key, holder_run_id, acquired_at, lease_until
		FROM singleton_locks
		WHERE project_id = $1 AND kind = $2 AND owner_id = $3
		ORDER BY acquired_at ASC, lock_key ASC`

	rows, err := q.db.Query(ctx, query, projectID, string(kind), ownerID)
	if err != nil {
		return nil, fmt.Errorf("list singleton locks: %w", err)
	}
	defer rows.Close()

	locks := make([]domain.SingletonLock, 0, 8)
	for rows.Next() {
		lock, scanErr := scanSingletonLock(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list singleton locks scan: %w", scanErr)
		}
		locks = append(locks, *lock)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list singleton locks rows: %w", err)
	}
	return locks, nil
}

// CountSingletonWaiters returns the number of runs parked behind the current
// holder of (kind, owner, lock_key). Job waiters are job_runs in 'waiting';
// workflow waiters are workflow_runs in 'queued'. Used to enforce the optional
// per-key queue depth cap.
func (q *Queries) CountSingletonWaiters(ctx context.Context, kind domain.SingletonKind, ownerID, lockKey string) (int, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.CountSingletonWaiters")
	defer span.End()

	var query string
	switch kind {
	case domain.SingletonKindJob:
		query = `SELECT COUNT(*) FROM job_runs WHERE job_id = $1 AND singleton_key = $2 AND status = 'waiting'`
	case domain.SingletonKindWorkflow:
		query = `SELECT COUNT(*) FROM workflow_runs WHERE workflow_id = $1 AND singleton_key = $2 AND status = 'queued'`
	default:
		return 0, fmt.Errorf("count singleton waiters: invalid kind %q", kind)
	}

	var n int
	if err := q.db.QueryRow(ctx, query, ownerID, lockKey).Scan(&n); err != nil {
		return 0, fmt.Errorf("count singleton waiters: %w", err)
	}
	return n, nil
}

func scanSingletonLock(scanner scanTarget) (*domain.SingletonLock, error) {
	var lock domain.SingletonLock
	var kind string
	var leaseUntil *time.Time
	if err := scanner.Scan(
		&lock.ProjectID,
		&kind,
		&lock.OwnerID,
		&lock.LockKey,
		&lock.HolderRunID,
		&lock.AcquiredAt,
		&leaseUntil,
	); err != nil {
		return nil, err
	}
	lock.Kind = domain.SingletonKind(kind)
	lock.LeaseUntil = leaseUntil
	return &lock, nil
}
