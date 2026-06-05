package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutionMode_IsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode ExecutionMode
		want bool
	}{
		{ExecutionModeHTTP, true},
		{ExecutionModeWorker, true},
		{"http", true},
		{"worker", true},
		{"", false},
		{"managed", false},
		{"docker", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.mode.IsValid())
	}
}

func TestExecutionMode_Constants(t *testing.T) {
	t.Parallel()
	assert.Equal(
		t,
		ExecutionModeHTTP, ExecutionMode("http"))
	assert.Equal(
		t,
		ExecutionModeWorker, ExecutionMode("worker"))
}
