package authoring

import (
	"maps"
	"math"
)

// AgentRunContext extends RunContext with agent-specific fields.
type AgentRunContext struct {
	RunContext
	iteration               int
	accumulatedCostMicrousd int64
	maxCostMicrousd         int64
}

// Iteration returns the current iteration count.
func (a *AgentRunContext) Iteration() int { return a.iteration }

// AccumulatedCostMicrousd returns the total accumulated cost.
func (a *AgentRunContext) AccumulatedCostMicrousd() int64 { return a.accumulatedCostMicrousd }

// IsBudgetExceeded returns true if accumulated cost >= max cost.
func (a *AgentRunContext) IsBudgetExceeded() bool {
	return a.accumulatedCostMicrousd >= a.maxCostMicrousd
}

// AgentOptions configures an agent definition.
type AgentOptions struct {
	Name            string
	Slug            string
	EndpointURL     string
	ProjectID       string
	Description     string
	Tags            map[string]string
	MaxIterations   *int
	MaxCostMicrousd *int64
	AutoCheckpoint  *bool
	TimeoutSecs     *int
	MaxAttempts     *int
	RetryStrategy   string
	Run             func(payload any, ctx *AgentRunContext) (any, error)
	OnSuccess       func(payload any, result any, ctx RunContext) error
	OnFailure       func(payload any, err error, ctx RunContext) error
	OnStart         func(payload any, ctx RunContext) error
}

// DefineAgent creates a job definition with agent conventions.
func DefineAgent(opts AgentOptions) *JobDefinition[any] {
	var maxCost int64 = math.MaxInt64
	if opts.MaxCostMicrousd != nil {
		maxCost = *opts.MaxCostMicrousd
	}
	autoCheckpoint := true
	if opts.AutoCheckpoint != nil {
		autoCheckpoint = *opts.AutoCheckpoint
	}

	tags := make(map[string]string)
	maps.Copy(tags, opts.Tags)
	tags["strait.kind"] = "agent"

	timeoutSecs := 600
	if opts.TimeoutSecs != nil {
		timeoutSecs = *opts.TimeoutSecs
	}
	maxAttempts := 5
	if opts.MaxAttempts != nil {
		maxAttempts = *opts.MaxAttempts
	}
	retryStrategy := "exponential"
	if opts.RetryStrategy != "" {
		retryStrategy = opts.RetryStrategy
	}

	return DefineJob(JobOptions[any]{
		Name:          opts.Name,
		Slug:          opts.Slug,
		EndpointURL:   opts.EndpointURL,
		ProjectID:     opts.ProjectID,
		Description:   opts.Description,
		Tags:          tags,
		TimeoutSecs:   &timeoutSecs,
		MaxAttempts:   &maxAttempts,
		RetryStrategy: retryStrategy,
		OnSuccess:     opts.OnSuccess,
		OnFailure:     opts.OnFailure,
		OnStart:       opts.OnStart,
		Run: func(payload any, ctx RunContext) (any, error) {
			agentCtx := &AgentRunContext{
				RunContext:      ctx,
				maxCostMicrousd: maxCost,
			}

			// Wrap ReportUsage to track cost
			originalReportUsage := ctx.ReportUsage
			if originalReportUsage != nil {
				agentCtx.ReportUsage = func(usage UsageReport) error {
					if usage.CostMicrousd != nil {
						agentCtx.accumulatedCostMicrousd += int64(*usage.CostMicrousd)
					}
					return originalReportUsage(usage)
				}
			}

			// Wrap Checkpoint to track iterations
			originalCheckpoint := ctx.Checkpoint
			agentCtx.Checkpoint = func(state map[string]any) error {
				agentCtx.iteration++
				if autoCheckpoint && originalCheckpoint != nil {
					return originalCheckpoint(state)
				}
				return nil
			}

			if opts.Run != nil {
				return opts.Run(payload, agentCtx)
			}
			return nil, nil
		},
	})
}
