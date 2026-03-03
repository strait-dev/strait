package store

import (
	"context"
	"time"

	"orchestrator/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type JobStore interface {
	CreateJob(ctx context.Context, job *domain.Job) error
	GetJob(ctx context.Context, id string) (*domain.Job, error)
	GetJobBySlug(ctx context.Context, projectID, slug string) (*domain.Job, error)
	ListJobs(ctx context.Context, projectID string) ([]domain.Job, error)
	UpdateJob(ctx context.Context, job *domain.Job) error
	DeleteJob(ctx context.Context, id string) error
	ListCronJobs(ctx context.Context) ([]domain.Job, error)
}

type RunStore interface {
	CreateRun(ctx context.Context, run *domain.JobRun) error
	GetRun(ctx context.Context, id string) (*domain.JobRun, error)
	ListRunsByJob(ctx context.Context, jobID string, limit, offset int) ([]domain.JobRun, error)
	ListRunsByProject(ctx context.Context, projectID string, status *domain.RunStatus, limit int, cursor *time.Time) ([]domain.JobRun, error)
	UpdateRunStatus(ctx context.Context, id string, from, to domain.RunStatus, fields map[string]any) error
	UpdateHeartbeat(ctx context.Context, id string) error
	ListStaleRuns(ctx context.Context, threshold time.Duration) ([]domain.JobRun, error)
	ListDueRuns(ctx context.Context) ([]domain.JobRun, error)
	ListExpiredRuns(ctx context.Context) ([]domain.JobRun, error)
}

type EventStore interface {
	InsertEvent(ctx context.Context, event *domain.RunEvent) error
	ListEvents(ctx context.Context, runID string) ([]domain.RunEvent, error)
}

type Store interface {
	JobStore
	RunStore
	EventStore
}

type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries {
	return &Queries{db: db}
}
