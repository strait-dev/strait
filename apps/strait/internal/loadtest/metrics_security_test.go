//go:build loadtest

package loadtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetricsCollectorRecordsWriteErrors(t *testing.T) {
	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: t.TempDir(),
		Interval:  time.Hour,
	})
	require.NoError(t,

		err)

	mc.filePrefix = "metrics-test"
	require.NoError(t,

		mc.openNewFile())
	require.NoError(t,

		mc.file.Close())

	mc.maxFileSize = 1

	mc.collect(context.Background())

	err = mc.collectionError()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "rotating metrics file"))

}
