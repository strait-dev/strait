//go:build loadtest

package loadtest

import (
	"time"
)

// Scenario defines a pre-configured load test.
type Scenario struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Tier        int           `json:"tier"`
	Duration    time.Duration `json:"duration"`
	RampConfig  *RampConfig   `json:"ramp_config,omitempty"`
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

// AllScenarios returns every pre-defined scenario.
func AllScenarios() []Scenario {
	return []Scenario{
		QuickValidation(),
		ThroughputCeiling(),
		ConcurrencyCeiling(),
		ProductionSimulation(500, 4*time.Hour),
		BreakingPoint(),
		Endurance(24 * time.Hour),
		ChaosAll(),
		ErrorScenarios(),
	}
}
