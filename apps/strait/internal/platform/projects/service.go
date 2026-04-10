// Package projects is the platform-level service for project CRUD.
//
// Projects are product-neutral: they scope every Strait product (Jobs,
// Agents, future). This service owns the transactional project-creation
// flow — advisory lock per org, billing limit check, subscription ensure,
// project insert, and system-role seeding — so the HTTP layer stays thin
// and every caller that needs to create a project goes through one path.
package projects

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"

	"strait/internal/domain"
	"strait/internal/store"
)

// Store is the subset of store operations the projects service needs
// outside of transactions. Inside WithTx callbacks the service uses
// *store.Queries directly.
type Store interface {
	CreateProject(ctx context.Context, project *domain.Project) error
	GetProject(ctx context.Context, projectID string) (*domain.Project, error)
	ListProjectsByOrg(ctx context.Context, orgID string) ([]domain.Project, error)
	DeleteProject(ctx context.Context, projectID string) error
	SeedProjectSystemRoles(ctx context.Context, projectID string) error
}

// BillingEnforcer is the subset of billing operations the projects service
// needs. It is intentionally narrow so this package doesn't pull in the
// full api.BillingEnforcer interface.
type BillingEnforcer interface {
	CheckProjectLimit(ctx context.Context, orgID string) error
	EnsureOrgSubscription(ctx context.Context, orgID string) error
}

// Service is the platform-level projects service. Zero value is not usable;
// construct via NewService.
type Service struct {
	store   Store
	txPool  store.TxBeginner
	billing BillingEnforcer
	logger  *slog.Logger
}

// NewService constructs a projects service. txPool and billing are optional:
// if txPool is nil the service skips the advisory lock; if billing is nil the
// service skips the limit/subscription checks.
func NewService(s Store, txPool store.TxBeginner, b BillingEnforcer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: s, txPool: txPool, billing: b, logger: logger}
}

// ErrInvalidProject is returned when required fields are missing.
var ErrInvalidProject = errors.New("projects: project ID and org ID are required")

// Create runs the transactional project-creation flow: advisory lock per
// org, billing limit check, subscription ensure, project insert, and
// system-role seeding. If the tx pool is unavailable, falls back to a
// non-transactional flow without the advisory lock. Any error from the
// billing check (e.g. *billing.LimitError) is returned unwrapped so callers
// can translate it to an HTTP response.
func (s *Service) Create(ctx context.Context, project *domain.Project) error {
	if project == nil || project.ID == "" || project.OrgID == "" {
		return ErrInvalidProject
	}

	if s.txPool != nil && s.billing != nil {
		return store.WithTx(ctx, s.txPool, func(q *store.Queries) error {
			if err := q.AdvisoryXactLock(ctx, orgAdvisoryLockID(project.OrgID)); err != nil {
				return fmt.Errorf("advisory lock: %w", err)
			}

			if err := s.billing.CheckProjectLimit(ctx, project.OrgID); err != nil {
				return err
			}

			if subErr := s.billing.EnsureOrgSubscription(ctx, project.OrgID); subErr != nil {
				s.logger.Warn("failed to ensure org subscription", "org_id", project.OrgID, "error", subErr)
			}

			if err := q.CreateProject(ctx, project); err != nil {
				return fmt.Errorf("create project: %w", err)
			}

			if err := q.SeedProjectSystemRoles(ctx, project.ID); err != nil {
				s.logger.Error("failed to seed system roles for project", "project_id", project.ID, "error", err)
			}

			return nil
		})
	}

	// Fallback: no tx pool, run without advisory lock.
	if s.billing != nil {
		if err := s.billing.CheckProjectLimit(ctx, project.OrgID); err != nil {
			return err
		}
		if err := s.billing.EnsureOrgSubscription(ctx, project.OrgID); err != nil {
			s.logger.Warn("failed to ensure org subscription", "org_id", project.OrgID, "error", err)
		}
	}

	if err := s.store.CreateProject(ctx, project); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	if err := s.store.SeedProjectSystemRoles(ctx, project.ID); err != nil {
		s.logger.Error("failed to seed system roles for project", "project_id", project.ID, "error", err)
	}
	return nil
}

// Get returns a single project by ID. Returns store.ErrProjectNotFound if
// the project does not exist.
func (s *Service) Get(ctx context.Context, projectID string) (*domain.Project, error) {
	return s.store.GetProject(ctx, projectID)
}

// ListByOrg returns all projects for an org.
func (s *Service) ListByOrg(ctx context.Context, orgID string) ([]domain.Project, error) {
	return s.store.ListProjectsByOrg(ctx, orgID)
}

// Delete soft-deletes a project.
func (s *Service) Delete(ctx context.Context, projectID string) error {
	return s.store.DeleteProject(ctx, projectID)
}

// orgAdvisoryLockID returns a deterministic int64 hash of the org ID for
// use as a pg_advisory_xact_lock key, serializing per-org project creation.
func orgAdvisoryLockID(orgID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(orgID))
	return int64(h.Sum64()) //nolint:gosec // advisory lock IDs can wrap
}
