package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type MaintenanceLoop struct {
	name     string
	interval time.Duration
	task     func(context.Context)
	logger   *slog.Logger
}

func NewMaintenanceLoop(name string, interval time.Duration, logger *slog.Logger, task func(context.Context)) *MaintenanceLoop {
	if interval <= 0 {
		interval = time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &MaintenanceLoop{
		name:     name,
		interval: interval,
		task:     task,
		logger:   logger,
	}
}

func (m *MaintenanceLoop) Run(ctx context.Context) {
	m.logger.Info("maintenance loop started", "name", m.name, "interval", m.interval)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("maintenance loop stopping", "name", m.name)
			return
		case scheduledAt := <-ticker.C:
			if m.task != nil {
				startedAt := time.Now()
				runSchedulerCycleCheckIn(ctx, m.interval, func() {
					m.task(ctx)
				})
				recordSchedulerLoop(ctx, m.name, scheduledAt, startedAt, m.interval, "tick", 1)
			}
		}
	}
}
