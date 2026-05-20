//go:build loadtest

package loadtest

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMetricsCollectorRecordsWriteErrors(t *testing.T) {
	mc, err := NewMetricsCollector(MetricsCollectorConfig{
		OutputDir: t.TempDir(),
		Interval:  time.Hour,
	})
	if err != nil {
		t.Fatalf("NewMetricsCollector: %v", err)
	}
	mc.filePrefix = "metrics-test"
	if err := mc.openNewFile(); err != nil {
		t.Fatalf("openNewFile: %v", err)
	}
	if err := mc.file.Close(); err != nil {
		t.Fatalf("close metrics file: %v", err)
	}
	mc.maxFileSize = 1

	mc.collect(context.Background())

	err = mc.collectionError()
	if err == nil {
		t.Fatal("expected metrics collection error")
	}
	if !strings.Contains(err.Error(), "rotating metrics file") {
		t.Fatalf("collection error = %v, want write error", err)
	}
}
