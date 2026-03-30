package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"strait/internal/agents"
	"strait/internal/domain"
)

const (
	playgroundSlugPrefix = "playground-"
	playgroundMaxAge     = 1 * time.Hour
)

// PlaygroundCleanupStore defines the store methods needed for cleanup.
type PlaygroundCleanupStore interface {
	ListAgents(ctx context.Context, projectID string, limit int, cursor *time.Time) ([]domain.Agent, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
}

// PlaygroundCleanup deletes temporary playground agents older than the max age.
type PlaygroundCleanup struct {
	store    PlaygroundCleanupStore
	agentSvc agents.Service
	interval time.Duration
}

// NewPlaygroundCleanup creates a new playground cleanup routine.
func NewPlaygroundCleanup(store PlaygroundCleanupStore, agentSvc agents.Service, interval time.Duration) *PlaygroundCleanup {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &PlaygroundCleanup{
		store:    store,
		agentSvc: agentSvc,
		interval: interval,
	}
}

// Run executes one cleanup pass across all projects.
func (c *PlaygroundCleanup) Run(ctx context.Context) {
	if c == nil || c.agentSvc == nil || c.store == nil {
		return
	}

	projects, err := c.store.ListProjects(ctx)
	if err != nil {
		slog.Error("playground cleanup: list projects", "error", err)
		return
	}

	cutoff := time.Now().Add(-playgroundMaxAge)
	var deleted int

	for _, project := range projects {
		agentList, listErr := c.store.ListAgents(ctx, project.ID, 100, nil)
		if listErr != nil {
			continue
		}

		for _, agent := range agentList {
			if !strings.HasPrefix(agent.Slug, playgroundSlugPrefix) {
				continue
			}
			if agent.CreatedAt.After(cutoff) {
				continue
			}
			if deleteErr := c.agentSvc.DeleteAgent(ctx, project.ID, agent.ID); deleteErr != nil {
				slog.Warn("playground cleanup: delete agent",
					"agent_id", agent.ID,
					"slug", agent.Slug,
					"error", deleteErr,
				)
				continue
			}
			deleted++
		}
	}

	if deleted > 0 {
		slog.Info("playground cleanup completed", "deleted", deleted)
	}
}

// Interval returns the cleanup interval.
func (c *PlaygroundCleanup) Interval() time.Duration {
	if c == nil {
		return 10 * time.Minute
	}
	return c.interval
}
