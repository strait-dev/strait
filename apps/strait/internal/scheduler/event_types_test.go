package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		types  []string
		target string
		want   bool
	}{
		{
			name:   "exact match",
			types:  []string{"slo.budget_warning"},
			target: "slo.budget_warning",
			want:   true,
		},
		{
			name:   "wildcard match",
			types:  []string{"*"},
			target: "slo.budget_warning",
			want:   true,
		},
		{
			name:   "missing match",
			types:  []string{"run.failed"},
			target: "slo.budget_warning",
			want:   false,
		},
		{
			name:   "empty types",
			types:  []string{},
			target: "slo.budget_warning",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, containsEventType(tt.types, tt.target))
		})
	}
}

func TestEventTypeMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		candidate string
		target    string
		want      bool
	}{
		{
			name:      "exact match",
			candidate: "slo.budget_warning",
			target:    "slo.budget_warning",
			want:      true,
		},
		{
			name:      "wildcard match",
			candidate: "*",
			target:    "slo.budget_warning",
			want:      true,
		},
		{
			name:      "different event",
			candidate: "run.failed",
			target:    "slo.budget_warning",
			want:      false,
		},
		{
			name:      "empty candidate",
			candidate: "",
			target:    "slo.budget_warning",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, eventTypeMatches(tt.candidate, tt.target))
		})
	}
}
