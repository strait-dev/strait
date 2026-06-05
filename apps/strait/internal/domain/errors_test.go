package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransitionError_Error(t *testing.T) {
	t.Parallel()
	err := &TransitionError{From: StatusQueued, To: StatusCompleted}
	got := err.Error()
	assert.Contains(t,
		got, "queued")
	assert.Contains(t,
		got, "completed")
	assert.Contains(t,
		got, "invalid transition")
}

func TestTransitionError_ImplementsError(t *testing.T) {
	t.Parallel()
	_, ok := any(&TransitionError{From: StatusQueued, To: StatusCompleted}).(error)
	require.True(t,
		ok)
}

func TestUnknownStatusError_Error(t *testing.T) {
	t.Parallel()
	err := &UnknownStatusError{Status: RunStatus("bogus")}
	got := err.Error()
	assert.Contains(t,
		got, "bogus")
	assert.Contains(t,
		got, "unknown status")
}

func TestEndpointError_Error(t *testing.T) {
	t.Parallel()
	err := &EndpointError{StatusCode: 503, Body: "service unavailable with tenant secret"}
	got := err.Error()
	assert.Contains(t,
		got, "503")
	assert.False(t,
		strings.Contains(got,
			"tenant secret",
		) || strings.Contains(got, "service unavailable"))
}

func TestEndpointError_EmptyBody(t *testing.T) {
	t.Parallel()
	err := &EndpointError{StatusCode: 500, Body: ""}
	got := err.Error()
	assert.Contains(t,
		got, "500")
}

func TestFieldError_Error(t *testing.T) {
	t.Parallel()
	err := &FieldError{Field: "nonexistent_field"}
	got := err.Error()
	assert.Contains(t,
		got, "nonexistent_field")
	assert.Contains(t,
		got, "unsupported update field")
}

func TestConfigError_Error(t *testing.T) {
	t.Parallel()
	err := &ConfigError{Field: "DATABASE_URL", Message: "is required"}
	got := err.Error()
	assert.Contains(t,
		got, "DATABASE_URL")
	assert.Contains(t,
		got, "is required")
}

func TestErrJobDisabled(t *testing.T) {
	t.Parallel()
	require.Error(
		t, ErrJobDisabled,
	)
	assert.Equal(t,
		"job is disabled",
		ErrJobDisabled.
			Error())
}

func TestErrJobDisabled_IsComparable(t *testing.T) {
	t.Parallel()
	_ = ErrJobDisabled
}

func TestValidateTransition_ReturnsTransitionError(t *testing.T) {
	t.Parallel()
	err := ValidateTransition(StatusQueued, StatusCompleted)
	require.Error(t,
		err)

	var te *TransitionError
	require.ErrorAs(t,
		err, &te)
	assert.Equal(t,
		StatusQueued,
		te.From,
	)
	assert.Equal(t,
		StatusCompleted,
		te.To,
	)
}

func TestValidateTransition_ReturnsUnknownStatusError(t *testing.T) {
	t.Parallel()
	err := ValidateTransition(RunStatus("invalid"), StatusQueued)
	require.Error(t,
		err)

	var ue *UnknownStatusError
	require.ErrorAs(t,
		err, &ue)
	assert.Equal(t,
		RunStatus(
			"invalid"),
		ue.
			Status)
}

func TestTransition_ValidReturnsNil(t *testing.T) {
	t.Parallel()
	require.NoError(t, Transition(StatusQueued,

		StatusDequeued))
}

func TestTransition_InvalidReturnsError(t *testing.T) {
	t.Parallel()
	err := Transition(StatusCompleted, StatusExecuting)
	require.Error(t,
		err)
}

func TestTransition_ErrorContainsStatus(t *testing.T) {
	t.Parallel()
	err := Transition(StatusCompleted, StatusQueued)
	require.Error(t,
		err)
	assert.Contains(t,
		err.Error(), "completed")
}
