package scheduler

import (
	"context"
	"time"
)

type schedulerComponent struct {
	name string
	run  func(context.Context)
}

func component(name string, run func(context.Context)) schedulerComponent {
	return schedulerComponent{name: name, run: run}
}

func (c schedulerComponent) valid() bool {
	return c.name != "" && c.run != nil
}

func (s *Scheduler) trackComponents(ctx context.Context, components []schedulerComponent) {
	for _, component := range components {
		if !component.valid() {
			continue
		}
		s.tracker.track(ctx, &s.wg, component.name, component.run)
	}
}

func (s *Scheduler) components() []schedulerComponent {
	var components []schedulerComponent
	if s.cronReloadInterval > 0 {
		components = append(components, schedulerComponent{
			name: "cron_reloader",
			run:  s.runCronReloader,
		})
	}

	components = append(components,
		component("poller", s.poller.Run),
		component("reaper", s.reaper.Run),
		component("index_maintainer", s.indexMaintainer.Run),
		component("debounce_poller", s.debouncePoller.Run),
		component("batch_flusher", s.batchFlusher.Run),
		component("stats_aggregator", s.statsAggregator.Run),
		component("budget_monitor", s.budgetMonitor.Run),
		component("memory_cleanup", s.memoryCleanup.Run),
	)

	if s.usageFlusher != nil {
		components = append(components, schedulerComponent{
			name: "usage_flusher",
			run:  s.usageFlusher.Run,
		})
	}
	if s.sloEvaluator != nil {
		components = append(components, schedulerComponent{
			name: "slo_evaluator",
			run: func(ctx context.Context) {
				s.sloEvaluator.Run(ctx, 5*time.Minute)
			},
		})
	}
	if s.concurrentReconciler != nil {
		components = append(components, schedulerComponent{
			name: "concurrent_reconciler",
			run:  s.concurrentReconciler.Run,
		})
	}
	if s.downgradeApplier != nil {
		components = append(components, schedulerComponent{
			name: "downgrade_applier",
			run:  s.downgradeApplier.Run,
		})
	}
	if s.anomalyMonitor != nil {
		components = append(components, schedulerComponent{
			name: "anomaly_monitor",
			run:  s.anomalyMonitor.Run,
		})
	}
	if s.dunner != nil {
		components = append(components, schedulerComponent{
			name: "dunner",
			run:  s.dunner.Run,
		})
	}
	if s.slaCalculator != nil {
		components = append(components, schedulerComponent{
			name: "sla_calculator",
			run:  s.slaCalculator.Run,
		})
	}
	if s.gracePeriodEnforcer != nil {
		components = append(components, schedulerComponent{
			name: "grace_period_enforcer",
			run:  s.gracePeriodEnforcer.Run,
		})
	}
	if s.quotaResumeEnforcer != nil {
		components = append(components, schedulerComponent{
			name: "quota_resume_enforcer",
			run:  s.quotaResumeEnforcer.Run,
		})
	}
	if s.staleSubscriptionChecker != nil {
		components = append(components, schedulerComponent{
			name: "stale_subscription_checker",
			run:  s.staleSubscriptionChecker.Run,
		})
	}
	if s.webhookMessageCleanup != nil {
		components = append(components, schedulerComponent{
			name: "webhook_message_cleanup",
			run:  s.webhookMessageCleanup.Run,
		})
	}
	if s.contractExpiryChecker != nil {
		components = append(components, schedulerComponent{
			name: "contract_expiry_checker",
			run:  s.contractExpiryChecker.Run,
		})
	}
	if s.usageReportEmailer != nil {
		components = append(components, schedulerComponent{
			name: "usage_report_emailer",
			run:  s.usageReportEmailer.Run,
		})
	}
	if s.priorityPromoter != nil {
		components = append(components, schedulerComponent{
			name: "priority_promoter",
			run:  s.priorityPromoter.Run,
		})
	}
	if s.counterReconciler != nil {
		components = append(components, schedulerComponent{
			name: "counter_reconciler",
			run:  s.counterReconciler.Run,
		})
	}
	if s.readyRunReconciler != nil {
		components = append(components, schedulerComponent{
			name: "ready_run_reconciler",
			run:  s.readyRunReconciler.Run,
		})
	}
	if s.partitionEnsurer != nil {
		components = append(components, schedulerComponent{
			name: "partition_ensurer",
			run:  s.partitionEnsurer.Run,
		})
	}
	if s.partitionTuner != nil {
		components = append(components, schedulerComponent{
			name: "partition_tuner",
			run:  s.partitionTuner.Run,
		})
	}
	if s.partitionReclaimer != nil {
		components = append(components, schedulerComponent{
			name: "partition_reclaimer",
			run:  s.partitionReclaimer.Run,
		})
	}
	if s.dlqAgeOut != nil {
		components = append(components, schedulerComponent{
			name: "dlq_age_out",
			run:  s.dlqAgeOut.Run,
		})
	}
	if s.outboxFlusher != nil {
		components = append(components, schedulerComponent{
			name: "outbox_flusher",
			run:  s.outboxFlusher.Run,
		})
	}
	if s.outboxArchiver != nil {
		components = append(components, schedulerComponent{
			name: "outbox_archiver",
			run:  s.outboxArchiver.Run,
		})
	}
	if s.planDriftMonitor != nil {
		components = append(components, schedulerComponent{
			name: "plan_drift_monitor",
			run:  s.planDriftMonitor.Run,
		})
	}
	if s.backpressureSampler != nil {
		components = append(components, schedulerComponent{
			name: "backpressure_sampler",
			run:  s.backpressureSampler.Run,
		})
	}
	if s.idempotencyGC != nil {
		components = append(components, schedulerComponent{
			name: "idempotency_gc",
			run:  s.idempotencyGC.Run,
		})
	}
	if s.heartbeatGC != nil {
		components = append(components, schedulerComponent{
			name: "heartbeat_gc",
			run:  s.heartbeatGC.Run,
		})
	}
	return components
}
