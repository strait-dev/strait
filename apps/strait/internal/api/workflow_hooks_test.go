package api

import (
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestWorkflowWebhookEventTypeMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		candidate string
		target    string
		want      bool
	}{
		{
			name:      "exact match",
			candidate: domain.WebhookEventWorkflowCompleted,
			target:    domain.WebhookEventWorkflowCompleted,
			want:      true,
		},
		{
			name:      "wildcard match",
			candidate: "*",
			target:    domain.WebhookEventWorkflowCompleted,
			want:      true,
		},
		{
			name:      "different event",
			candidate: domain.WebhookEventWorkflowFailed,
			target:    domain.WebhookEventWorkflowCompleted,
			want:      false,
		},
		{
			name:      "empty candidate",
			candidate: "",
			target:    domain.WebhookEventWorkflowCompleted,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, workflowWebhookEventTypeMatches(tt.candidate, tt.target))
		})
	}
}

func TestWorkflowWebhookEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status domain.WorkflowRunStatus
		event  string
		ok     bool
	}{
		{
			name:   "completed",
			status: domain.WfStatusCompleted,
			event:  domain.WebhookEventWorkflowCompleted,
			ok:     true,
		},
		{
			name:   "failed",
			status: domain.WfStatusFailed,
			event:  domain.WebhookEventWorkflowFailed,
			ok:     true,
		},
		{
			name:   "timed out",
			status: domain.WfStatusTimedOut,
			event:  domain.WebhookEventWorkflowFailed,
			ok:     true,
		},
		{
			name:   "running has no hook",
			status: domain.WfStatusRunning,
			event:  "",
			ok:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			event, ok := workflowWebhookEventType(tt.status)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.event, event)
		})
	}
}
