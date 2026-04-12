package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"strait/internal/config"
	"strait/internal/domain"
)

// countByReason returns a map of reason-label -> summed counter value
// for the named instrument. Unlabeled data points appear under the
// empty-string key.
func countByReason(t *testing.T, reader *sdkmetric.ManualReader, name string) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, inst := range sm.Metrics {
			if inst.Name != name {
				continue
			}
			if sum, ok := inst.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range sum.DataPoints {
					reason, _ := dp.Attributes.Value("reason")
					out[reason.AsString()] += dp.Value
				}
			}
		}
	}
	return out
}

func newAuditVerifyMetricsServer(t *testing.T, ms *APIStoreMock, h *auditMetricsHarness) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   ms,
		Metrics: h.metrics,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)
	return srv
}

// TestHandleVerifyAuditChain_CountsValid asserts the API handler
// increments chain_verify_total on every attempt and leaves
// chain_verify_failed_total at zero when the chain passes.
func TestHandleVerifyAuditChain_CountsValid(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{
				ProjectID:     projectID,
				Valid:         true,
				EventsChecked: 10,
			}, nil
		},
	}

	h := newAuditMetricsHarness(t)
	srv := newAuditVerifyMetricsServer(t, ms, h)

	handler := srv.requirePermission(domain.ScopeRBACManage)(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-ok")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	totals := countByReason(t, h.reader, "strait.audit.chain_verify_total")
	var sum int64
	for _, v := range totals {
		sum += v
	}
	if sum != 1 {
		t.Errorf("chain_verify_total = %d, want 1", sum)
	}
	failed := countByReason(t, h.reader, "strait.audit.chain_verify_failed_total")
	var failedSum int64
	for _, v := range failed {
		failedSum += v
	}
	if failedSum != 0 {
		t.Errorf("chain_verify_failed_total = %d, want 0 on valid chain", failedSum)
	}
}

// TestHandleVerifyAuditChain_CountsBroken asserts a non-Valid result
// increments the failed counter with reason=chain_broken.
func TestHandleVerifyAuditChain_CountsBroken(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{
				ProjectID:     projectID,
				Valid:         false,
				EventsChecked: 3,
				BrokenAtID:    "ev-3",
				Error:         "signature mismatch",
			}, nil
		},
	}

	h := newAuditMetricsHarness(t)
	srv := newAuditVerifyMetricsServer(t, ms, h)

	handler := srv.requirePermission(domain.ScopeRBACManage)(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-broken")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	failed := countByReason(t, h.reader, "strait.audit.chain_verify_failed_total")
	if got := failed["chain_broken"]; got != 1 {
		t.Errorf("chain_verify_failed_total{reason=chain_broken} = %d, want 1; full map: %+v", got, failed)
	}
}

// TestHandleVerifyAuditChain_CountsVerifierError asserts a verifier
// error from the store increments failed with reason=verifier_error.
// This distinction (verifier infra broken vs. chain broken) is
// load-bearing for the runbook: a verifier_error page means
// "investigate the verifier and DB"; chain_broken means "investigate
// evidence tampering".
func TestHandleVerifyAuditChain_CountsVerifierError(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		VerifyAuditChainFunc: func(_ context.Context, _ string) (*domain.AuditChainVerification, error) {
			return nil, errors.New("db unreachable")
		},
	}
	h := newAuditMetricsHarness(t)
	srv := newAuditVerifyMetricsServer(t, ms, h)

	handler := srv.requirePermission(domain.ScopeRBACManage)(
		TypedHandler(srv, http.StatusOK, srv.handleVerifyAuditChain),
	)
	req := httptest.NewRequest(http.MethodGet, "/v1/audit-events/verify", nil)
	ctx := context.WithValue(req.Context(), ctxProjectIDKey, "proj-err")
	ctx = context.WithValue(ctx, ctxScopesKey, []string{domain.ScopeRBACManage})
	ctx = context.WithValue(ctx, ctxActorTypeKey, "api_key")
	ctx = context.WithValue(ctx, ctxActorIDKey, "apikey:test")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	failed := countByReason(t, h.reader, "strait.audit.chain_verify_failed_total")
	if got := failed["verifier_error"]; got != 1 {
		t.Errorf("chain_verify_failed_total{reason=verifier_error} = %d, want 1; map=%+v", got, failed)
	}

	totals := countByReason(t, h.reader, "strait.audit.chain_verify_total")
	var sum int64
	for _, v := range totals {
		sum += v
	}
	if sum != 1 {
		t.Errorf("chain_verify_total = %d, want 1 (every attempt counted, pass or fail)", sum)
	}
}

