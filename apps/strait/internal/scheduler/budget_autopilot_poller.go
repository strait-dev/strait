package scheduler

import (
	"context"
	"log/slog"
	"time"

	"strait/internal/agents"
	"strait/internal/domain"
)

// AutopilotAgentStore provides agent listing and spending queries.
type AutopilotAgentStore interface {
	ListAgentsWithAutopilot(ctx context.Context) ([]domain.Agent, error)
	SumOrgAgentSpendSince(ctx context.Context, orgID string, since time.Time) (int64, error)
}

// BudgetAutopilotPoller runs periodic budget autopilot checks.
type BudgetAutopilotPoller struct {
	autopilot *agents.BudgetAutopilot
	store     AutopilotAgentStore
	interval  time.Duration
	logger    *slog.Logger
}

// NewBudgetAutopilotPoller creates a new poller.
func NewBudgetAutopilotPoller(autopilot *agents.BudgetAutopilot, store AutopilotAgentStore, interval time.Duration) *BudgetAutopilotPoller {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &BudgetAutopilotPoller{
		autopilot: autopilot,
		store:     store,
		interval:  interval,
		logger:    slog.Default(),
	}
}

// Run starts the autopilot polling loop. Blocks until ctx is canceled.
func (p *BudgetAutopilotPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.check(context.WithoutCancel(ctx))
		}
	}
}

func (p *BudgetAutopilotPoller) check(ctx context.Context) {
	// Placeholder: ListAgentsWithAutopilot is not yet implemented on the store.
	// In production this would query agents where config->'autopilot'->>'enabled' = 'true'
	// and for each, compute current spend and call CheckAndAdjust.
	p.logger.Debug("budget autopilot poller: check cycle")
}
