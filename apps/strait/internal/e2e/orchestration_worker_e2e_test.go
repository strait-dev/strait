//go:build integration

// Package e2e_test contains end-to-end tests for the Strait orchestration platform.
//
// This file documents the planned orchestration-only e2e test suite covering the
// full gRPC worker lifecycle. The actual implementation requires additional test
// infrastructure (in-process gRPC worker, synthetic job executor) that is deferred
// until the test harness can run a worker in-process.
package e2e_test

import (
	"testing"
)

// TestOrchestrationWorkerE2E is the skeleton for the orchestration-only end-to-end
// test suite. It will be expanded as the gRPC test infrastructure matures.
//
// Planned coverage:
//
//  1. Server startup — spin up Strait in "all" mode with both HTTP (:8080) and
//     gRPC (:50051) listeners active.
//
//  2. Worker connect — dial the gRPC WorkerService endpoint in-process, send
//     RegisterWorker with {queue: "default", concurrency_slots: 5}, assert the
//     server acknowledges with a worker_id and routes the stream to the registry.
//
//  3. Bulk throughput — create a worker-mode job, enqueue 100 runs, drive the
//     in-process worker to claim + report success for each, assert all 100
//     terminal states are recorded within a deadline (e.g. 30s), assert zero
//     runs are stranded (status != terminal after deadline).
//
//  4. Cost accounting — after bulk run, assert that job_costs rows exist for
//     each run (or project aggregate updated), verify compute_cost_cents > 0
//     where billing enforcement is enabled.
//
//  5. Goroutine leak guard — capture runtime.NumGoroutine() before and after
//     bulk run; assert the delta is within an acceptable tolerance (e.g. < 5).
//
//  6. Force-disconnect and reconnect — call DELETE /internal/workers/{id} via
//     REST to forcibly evict the worker; assert the gRPC stream receives a
//     server-side DISCONNECT message; assert the worker can immediately
//     re-register and resume claiming.
//
//  7. API key revocation — revoke the API key the in-process worker used;
//     assert the server closes the gRPC stream with status UNAUTHENTICATED
//     within WORKER_HEARTBEAT_TIMEOUT; assert a new stream opened with a
//     valid key succeeds.
//
//  8. Monthly cap enforcement — set project quota monthly_run_cap=10, enqueue
//     15 runs; assert exactly 10 complete, runs 11-15 are rejected with the
//     cap-exceeded error code; assert a CRON schedule for that project is
//     paused by the quota enforcer; assert a webhook event
//     "project.quota.exceeded" is emitted.
//
//  9. HTTP serve mode — symmetric scenario using httptest.NewServer for a
//     job that exposes an HTTP endpoint; harness POSTs dispatch payloads with
//     HMAC-SHA256 signatures (X-Strait-Signature header); assert the handler
//     verifies the signature and returns 200; assert runs reach "completed"
//     state; assert that requests with a bad or missing signature are rejected
//     with 401.
func TestOrchestrationWorkerE2E(t *testing.T) {
	t.Skip("e2e skeleton - implement when in-process gRPC worker harness lands")

	// Pseudocode outline — will be replaced with real implementation.
	//
	// Step 1: server startup
	//   cfg := testutil.DefaultConfig(t)
	//   cfg.GRPCEnabled = true
	//   cfg.GRPCPort = freePort(t)
	//   srv := testutil.StartServer(t, cfg)
	//   t.Cleanup(srv.Stop)
	//
	// Step 2: worker connect
	//   conn, err := grpc.Dial(srv.GRPCAddr(), grpc.WithInsecure())
	//   require.NoError(t, err)
	//   wc := workerv1.NewWorkerServiceClient(conn)
	//   stream, err := wc.Connect(ctx)
	//   require.NoError(t, err)
	//   require.NoError(t, stream.Send(&workerv1.ConnectRequest{...}))
	//   resp, err := stream.Recv()
	//   require.NoError(t, err)
	//   require.NotEmpty(t, resp.WorkerId)
	//
	// Step 3: bulk throughput
	//   job := testutil.CreateWorkerJob(t, srv, "default")
	//   testutil.EnqueueRuns(t, srv, job.ID, 100)
	//   inProcessWorker := testutil.NewInProcessWorker(stream, job.ID)
	//   inProcessWorker.DrainUntilIdle(t, 30*time.Second)
	//   testutil.AssertAllRunsTerminal(t, srv, job.ID, 100)
	//
	// Step 4: cost accounting
	//   testutil.AssertCostsRecorded(t, srv, job.ID, 100)
	//
	// Step 5: goroutine leak guard
	//   before := runtime.NumGoroutine()
	//   // ... run bulk throughput ...
	//   after := runtime.NumGoroutine()
	//   require.Less(t, after-before, 5)
	//
	// Step 6: force-disconnect and reconnect
	//   testutil.ForceDisconnectWorker(t, srv, resp.WorkerId)
	//   _, err = stream.Recv() // expect DISCONNECT or stream close
	//   require.Error(t, err)
	//   stream2, _ := wc.Connect(ctx)
	//   // re-register succeeds
	//
	// Step 7: API key revocation
	//   testutil.RevokeAPIKey(t, srv, apiKeyID)
	//   // within WORKER_HEARTBEAT_TIMEOUT the stream should close
	//   testutil.AssertStreamClosesWithin(t, stream, cfg.WorkerHeartbeatTimeout)
	//
	// Step 8: monthly cap
	//   testutil.SetProjectQuota(t, srv, projectID, MonthlyRunCap, 10)
	//   testutil.EnqueueRuns(t, srv, job.ID, 15)
	//   testutil.AssertRunCount(t, srv, job.ID, domain.StatusCompleted, 10)
	//   testutil.AssertRunCount(t, srv, job.ID, domain.StatusCapExceeded, 5)
	//   testutil.AssertCronPaused(t, srv, projectID)
	//   testutil.AssertWebhookEvent(t, srv, "project.quota.exceeded")
	//
	// Step 9: HTTP serve mode with HMAC
	//   httpJob := testutil.CreateHTTPJob(t, srv, handler.URL)
	//   testutil.EnqueueRuns(t, srv, httpJob.ID, 10)
	//   // handler verifies X-Strait-Signature; bad-sig requests return 401
	//   testutil.AssertAllRunsTerminal(t, srv, httpJob.ID, 10)
}
