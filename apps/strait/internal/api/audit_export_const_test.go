package api

import (
	"context"
	"testing"
)

func TestDefaultMaxExportRows_IsOneMillionDefault(t *testing.T) {
	t.Parallel()
	if defaultMaxExportRows != 1_000_000 {
		t.Fatalf("expected 1_000_000, got %d", defaultMaxExportRows)
	}
}

func TestResolveExportRowCap_FallsToDefault(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAuditExportRowCapFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	cap := srv.resolveExportRowCap(context.Background(), "proj-1")
	if cap != defaultMaxExportRows {
		t.Fatalf("expected %d, got %d", defaultMaxExportRows, cap)
	}
}

func TestResolveExportRowCap_ConfigOverride(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetAuditExportRowCapFunc: func(_ context.Context, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)
	srv.config.AuditExportRowCapDefault = 500
	cap := srv.resolveExportRowCap(context.Background(), "proj-1")
	if cap != 500 {
		t.Fatalf("expected 500, got %d", cap)
	}
}
