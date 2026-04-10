// Package environments is the platform-level service for environment CRUD.
//
// Environments are product-neutral: Jobs bind to environments via
// Job.EnvironmentID, Agent deployments bind via AgentDeployment.EnvironmentID,
// and any future product consuming deploy-time buckets (dev/staging/prod)
// will do the same. This service owns the limit-checked creation flow and
// the read helpers so every product shares one canonical path.
package environments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
)

// Store is the subset of store operations the environments service needs.
type Store interface {
	CreateEnvironment(ctx context.Context, env *domain.Environment) error
	GetEnvironment(ctx context.Context, envID string) (*domain.Environment, error)
	ListEnvironments(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error)
	UpdateEnvironment(ctx context.Context, env *domain.Environment) error
	DeleteEnvironment(ctx context.Context, envID string) error
	GetResolvedEnvironmentVariables(ctx context.Context, envID string) (map[string]string, error)
	CountEnvironmentsByOrg(ctx context.Context, orgID string) (int, error)
}

// LimitChecker resolves the max-environments limit for a given project's org.
// Implementations typically read the org's plan limits via the billing
// enforcer. Returning zero means "unlimited / do not enforce".
type LimitChecker interface {
	MaxEnvironmentsForProject(ctx context.Context, projectID string) (limit int, orgID string, err error)
}

// Service is the platform-level environments service. Zero value is not
// usable; construct via NewService.
type Service struct {
	store  Store
	limits LimitChecker
}

// NewService constructs an environments service. limits is optional — if
// nil, environment creation is not rate-limited by plan tier.
func NewService(s Store, limits LimitChecker) *Service {
	return &Service{store: s, limits: limits}
}

// EnvironmentLimitReachedError is returned when creating an environment
// would exceed the org's plan-based MaxEnvironments. The HTTP layer should
// translate this to a 400 or a structured upgrade-required response.
type EnvironmentLimitReachedError struct {
	Limit    int
	Existing int
}

func (e *EnvironmentLimitReachedError) Error() string {
	return fmt.Sprintf("environments: limit reached (%d of %d)", e.Existing, e.Limit)
}

// Create validates environment creation against the org's plan limits and
// inserts the environment. Fails open on limit-lookup errors — billing
// unavailability should not block environment creation.
func (s *Service) Create(ctx context.Context, env *domain.Environment) error {
	if env == nil || env.ProjectID == "" {
		return errors.New("environments: project ID is required")
	}

	if s.limits != nil {
		limit, orgID, err := s.limits.MaxEnvironmentsForProject(ctx, env.ProjectID)
		if err == nil && limit > 0 && orgID != "" {
			count, countErr := s.store.CountEnvironmentsByOrg(ctx, orgID)
			if countErr == nil && count >= limit {
				return &EnvironmentLimitReachedError{Limit: limit, Existing: count}
			}
		}
	}

	return s.store.CreateEnvironment(ctx, env)
}

// Get returns a single environment by ID. Returns store.ErrEnvironmentNotFound
// if the environment does not exist.
func (s *Service) Get(ctx context.Context, envID string) (*domain.Environment, error) {
	return s.store.GetEnvironment(ctx, envID)
}

// List returns environments for a project with pagination.
func (s *Service) List(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Environment, error) {
	return s.store.ListEnvironments(ctx, projectID, limit, cursor)
}

// Update persists changes to an existing environment.
func (s *Service) Update(ctx context.Context, env *domain.Environment) error {
	return s.store.UpdateEnvironment(ctx, env)
}

// Delete removes an environment. Store errors (not-found, standard env
// protection) flow back unwrapped for the HTTP layer to translate.
func (s *Service) Delete(ctx context.Context, envID string) error {
	return s.store.DeleteEnvironment(ctx, envID)
}

// ResolveVariables returns the merged variable set for an environment,
// walking parent chains.
func (s *Service) ResolveVariables(ctx context.Context, envID string) (map[string]string, error) {
	return s.store.GetResolvedEnvironmentVariables(ctx, envID)
}
