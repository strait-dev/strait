//go:build loadtest

package loadtest

import (
	"time"
)

// Scenario defines a pre-configured load test.
type Scenario struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Tier         int                    `json:"tier"`
	Duration     time.Duration          `json:"duration"`
	RampConfig   *RampConfig            `json:"ramp_config,omitempty"`
	TenantConfig *TenantSimulatorConfig `json:"tenant_config,omitempty"`
}

// QuickValidation returns a 15-minute quick CI scenario.
func QuickValidation() Scenario {
	return Scenario{
		Name:        "quick_validation",
		Description: "Quick CI validation: reduced scale throughput and concurrency check",
		Tier:        0,
		Duration:    15 * time.Minute,
		RampConfig: &RampConfig{
			Mode:         RampThroughput,
			StartRate:    10,
			StepSize:     10,
			StepInterval: 30 * time.Second,
			StopCondition: StopCondition{
				MaxQueueDepth: 5000,
				MaxLatencyP99: 3 * time.Second,
				MaxErrorRate:  0.05,
				MaxDuration:   15 * time.Minute,
			},
		},
	}
}

// ThroughputCeiling returns the Tier 1 throughput ramp scenario.
func ThroughputCeiling() Scenario {
	return Scenario{
		Name:        "throughput_ceiling",
		Description: "Tier 1: Ramp throughput until system breaks. Start 10 jobs/sec, increase +10/sec every 60s.",
		Tier:        1,
		Duration:    2 * time.Hour,
		RampConfig: &RampConfig{
			Mode:         RampThroughput,
			StartRate:    10,
			StepSize:     10,
			StepInterval: 60 * time.Second,
			StopCondition: StopCondition{
				MaxQueueDepth: 10000,
				MaxLatencyP99: 5 * time.Second,
				MaxErrorRate:  0.01,
				MaxDuration:   2 * time.Hour,
			},
		},
	}
}

// ConcurrencyCeiling returns the Tier 2 concurrency ramp scenario.
func ConcurrencyCeiling() Scenario {
	return Scenario{
		Name:        "concurrency_ceiling",
		Description: "Tier 2: Ramp concurrent connections until errors exceed 5%. Start 50, increase +50 every 2 min.",
		Tier:        2,
		Duration:    1 * time.Hour,
		RampConfig: &RampConfig{
			Mode:         RampConcurrency,
			StartRate:    50,
			StepSize:     50,
			StepInterval: 2 * time.Minute,
			StopCondition: StopCondition{
				MaxLatencyP99: 10 * time.Second,
				MaxErrorRate:  0.05,
				MaxDuration:   1 * time.Hour,
			},
		},
	}
}

// ProductionSimulation returns the Tier 3 multi-tenant scenario.
func ProductionSimulation(tenantCount int, duration time.Duration) Scenario {
	tenants := GenerateTenants(tenantCount)
	return Scenario{
		Name:        "production_simulation",
		Description: "Tier 3: Multi-tenant production simulation with mixed traffic patterns.",
		Tier:        3,
		Duration:    duration,
		TenantConfig: &TenantSimulatorConfig{
			Tenants:        tenants,
			TimeOfDayCurve: true,
			Duration:       duration,
			PeakHourUTC:    14,
			TroughHourUTC:  4,
		},
	}
}

// BreakingPoint returns the Tier 3 breaking point scenario.
func BreakingPoint() Scenario {
	return Scenario{
		Name:        "breaking_point",
		Description: "Tier 3: Add 100 tenants every 30 min until P99 > 5s or error rate > 0.1%.",
		Tier:        3,
		Duration:    12 * time.Hour,
	}
}

// Endurance returns the Tier 4 endurance scenario.
func Endurance(duration time.Duration) Scenario {
	return Scenario{
		Name:        "endurance",
		Description: "Tier 4: Run at 70% of throughput ceiling for extended duration. Detect leaks and drift.",
		Tier:        4,
		Duration:    duration,
	}
}

// ChaosAll returns the Tier 5 chaos engineering scenario.
func ChaosAll() Scenario {
	return Scenario{
		Name:        "chaos_all",
		Description: "Tier 5: Run all chaos scenarios during ~800 runs/sec production load.",
		Tier:        5,
		Duration:    4 * time.Hour,
	}
}

// ErrorScenarios returns the error scenario test configuration.
func ErrorScenarios() Scenario {
	return Scenario{
		Name:        "error_scenarios",
		Description: "Run all 12 error scenarios concurrently during production load.",
		Tier:        3,
		Duration:    1 * time.Hour,
	}
}

// EnduranceWeekend returns a 72-hour weekend endurance scenario.
func EnduranceWeekend() Scenario {
	return Scenario{
		Name:        "endurance_weekend",
		Description: "Tier 4: 72-hour weekend run at 70% of throughput ceiling. Extended leak and drift detection.",
		Tier:        4,
		Duration:    72 * time.Hour,
	}
}

// ProductionValidation returns a production real-network validation scenario.
func ProductionValidation() Scenario {
	return Scenario{
		Name:        "production_validation",
		Description: "Production validation: real network latency, production infrastructure.",
		Tier:        5,
		Duration:    1 * time.Hour,
		RampConfig: &RampConfig{
			Mode:         RampThroughput,
			StartRate:    10,
			StepSize:     5,
			StepInterval: 60 * time.Second,
			StopCondition: StopCondition{
				MaxQueueDepth: 10000,
				MaxLatencyP99: 10 * time.Second,
				MaxErrorRate:  0.01,
				MaxDuration:   1 * time.Hour,
			},
		},
	}
}

// BackpressureCeiling ramps enqueue across N projects past the token
// bucket and measures the rejection rate plus steady-state dequeue
// throughput. It is the canonical scenario for validating that
// per-project rate limiting shields the queue from a noisy tenant.
func BackpressureCeiling() Scenario {
	return Scenario{
		Name:        "backpressure_ceiling",
		Description: "Ramp enqueue across N projects past token bucket; measure rejection rate and steady-state dequeue throughput.",
		Tier:        2,
		Duration:    45 * time.Minute,
		RampConfig: &RampConfig{
			Mode:         RampThroughput,
			StartRate:    100,
			StepSize:     100,
			StepInterval: 30 * time.Second,
			StopCondition: StopCondition{
				MaxQueueDepth: 50000,
				MaxLatencyP99: 10 * time.Second,
				MaxErrorRate:  0.5,
				MaxDuration:   45 * time.Minute,
			},
		},
	}
}

// CircuitBreakerChaos uses pgxslow to inject DB slowness and asserts
// that the circuit opens within threshold, dequeue short-circuits, and
// the system recovers once latency returns to baseline.
func CircuitBreakerChaos() Scenario {
	return Scenario{
		Name:        "circuit_breaker_chaos",
		Description: "Inject DB slowness via pgxslow; assert circuit opens, dequeue short-circuits, recovers after backoff.",
		Tier:        5,
		Duration:    30 * time.Minute,
	}
}

// OutboxBurst bulk-enqueues via outbox with the flusher paused,
// releases it, and measures flusher throughput plus the
// outbox_lag_seconds histogram.
func OutboxBurst() Scenario {
	return Scenario{
		Name:        "outbox_burst",
		Description: "Bulk-enqueue via outbox with flusher paused, release, measure flusher throughput and outbox_lag_seconds.",
		Tier:        2,
		Duration:    20 * time.Minute,
	}
}

// PartitionCycleMatrix varies the partitionCycle length (1, 4, 12) at a
// fixed enqueue rate and captures per-partition P99 dequeue latency via
// the partition_dequeue_lag_seconds histogram.
func PartitionCycleMatrix() Scenario {
	return Scenario{
		Name:        "partition_cycle_matrix",
		Description: "Vary partitionCycle (1, 4, 12) at fixed enqueue rate; measure per-partition P99 dequeue latency.",
		Tier:        2,
		Duration:    90 * time.Minute,
	}
}

// ArchiveStress enqueues 10k terminal runs, runs the archive loop for
// 60 seconds, and measures rows/sec throughput. Validates that the
// archive CTE (INSERT INTO history + DELETE FROM hot) scales linearly.
func ArchiveStress() Scenario {
	return Scenario{
		Name:        "archive_stress",
		Description: "Insert 10k terminal runs, archive for 60s, measure archive throughput and verify zero stranded rows.",
		Tier:        2,
		Duration:    5 * time.Minute,
	}
}

// AllScenarios returns every pre-defined scenario.
func AllScenarios() []Scenario {
	return []Scenario{
		QuickValidation(),
		ThroughputCeiling(),
		ConcurrencyCeiling(),
		ProductionSimulation(500, 4*time.Hour),
		BreakingPoint(),
		Endurance(24 * time.Hour),
		EnduranceWeekend(),
		ChaosAll(),
		ErrorScenarios(),
		ProductionValidation(),
		BackpressureCeiling(),
		CircuitBreakerChaos(),
		OutboxBurst(),
		PartitionCycleMatrix(),
		ArchiveStress(),
	}
}
