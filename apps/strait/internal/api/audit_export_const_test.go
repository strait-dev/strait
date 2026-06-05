package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultMaxExportRows_IsOneMillionDefault(t *testing.T) {
	t.Parallel()
	require.EqualValues(t, 1_000_000,

		defaultMaxExportRows)
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
	require.Equal(t, defaultMaxExportRows,

		cap)
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
	require.EqualValues(t, 500, cap)
}
