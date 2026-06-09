package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildWorkerTaskResult(t *testing.T) {
	t.Parallel()

	require.Nil(t, buildWorkerTaskResult(nil, nil, nil, nil, nil))

	status := "success"
	output := []byte(`{"ok":true}`)
	errText := "none"
	durationMS := int64(42)
	receivedAt := time.Now().UTC()

	result := buildWorkerTaskResult(&status, output, &errText, &durationMS, &receivedAt)
	require.NotNil(t, result)
	require.Equal(t, status, result.Status)
	require.JSONEq(t, string(output), string(result.Output))
	require.Equal(t, errText, result.Error)
	require.Equal(t, durationMS, result.DurationMS)
	require.Same(t, &receivedAt, result.ReceivedAt)

	output[0] = '['
	require.JSONEq(t, `{"ok":true}`, string(result.Output))
}
