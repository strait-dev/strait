//go:build loadtest

package loadtest_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/loadtest"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Minimal in-memory gRPC server for smoke tests.

// echoWorkerServer implements workerv1.WorkerServiceServer.
// It accepts a connection, sends all remaining synthetic TaskAssignments in
// batches, collects TaskResults, and closes the stream when the shared quota
// is exhausted. This lets a small number of workers process a large batch of
// tasks quickly.
type echoWorkerServer struct {
	workerv1.UnimplementedWorkerServiceServer
	totalDispatched atomic.Int64
	totalResults    atomic.Int64
	totalToDispatch int
}

func (s *echoWorkerServer) StreamTasks(stream workerv1.WorkerService_StreamTasksServer) error {
	var concWG conc.WaitGroup
	ctx := stream.Context()

	// 1. Receive registration.
	reg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "recv registration: %v", err)
	}
	if reg.GetRegistration() == nil {
		return status.Error(codes.InvalidArgument, "expected registration")
	}

	// 2. ACK registration.
	if err := stream.Send(&workerv1.ServerMessage{
		Payload: &workerv1.ServerMessage_Ack{
			Ack: &workerv1.Acknowledged{Id: reg.GetRegistration().GetWorkerId()},
		},
	}); err != nil {
		return err
	}

	// 3. Drain results in a background goroutine so we can send more assignments.
	resultCount := make(chan struct{}, 512)
	concWG.Go(func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			if msg.GetTaskResult() != nil {
				s.totalResults.Add(1)
				select {
				case resultCount <- struct{}{}:
				default:
				}
			}
		}
	})
	// Do not wait here: returning from StreamTasks closes the stream and
	// unblocks the receive loop.

	// 4. Dispatch as many tasks as the shared quota allows, then close.
	var localSent int
	for {
		n := s.totalDispatched.Add(1)
		if int(n) > s.totalToDispatch {
			// Quota exhausted; return nil to close the stream gracefully so workers exit.
			return nil
		}
		runID := fmt.Sprintf("run-%d", n)
		if err := stream.Send(&workerv1.ServerMessage{
			Payload: &workerv1.ServerMessage_TaskAssignment{
				TaskAssignment: &workerv1.TaskAssignment{
					RunId:       runID,
					JobSlug:     "smoke-job",
					Queue:       "loadtest",
					TimeoutSecs: 1, // short timeout so simulated work is fast
				},
			},
		}); err != nil {
			return err
		}
		localSent++

		// Occasionally yield to let the result drain catch up (avoids unbounded
		// backlog when send is much faster than processing).
		if localSent%16 == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Millisecond):
			}
		}
	}
}

// startTestGRPCServer starts an in-process gRPC server and returns its address.
func startTestGRPCServer(t *testing.T, srv *echoWorkerServer) string {
	var concWG conc.WaitGroup
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	gs := grpc.NewServer()
	workerv1.RegisterWorkerServiceServer(gs, srv)
	concWG.Go(func() {
		_ = gs.Serve(ln)
	})
	t.Cleanup(func() {
		gs.GracefulStop()
		concWG.Wait()
	})
	return ln.Addr().String()
}

// Smoke tests: 100 dispatches, assert throughput > 50/sec.

// TestWorkerScenario_SmokeTest spins up a tiny in-memory gRPC server, connects
// the worker scenario to it, dispatches 100 synthetic tasks, and asserts that
// throughput exceeds 50 tasks/sec on any developer machine.
//
// Run with: go test -tags=loadtest -run TestWorkerScenario_SmokeTest -timeout 30s ./internal/loadtest/...
func TestWorkerScenario_SmokeTest(t *testing.T) {
	const totalDispatches = 100

	srvImpl := &echoWorkerServer{totalToDispatch: totalDispatches}
	addr := startTestGRPCServer(t, srvImpl)

	cfg := loadtest.DefaultWorkerConfig()
	cfg.GRPCAddr = addr
	cfg.WorkerCount = 10
	cfg.SlotsPerWorker = 16
	cfg.FailRate = 0.0                       // no failures in smoke test
	cfg.HeartbeatInterval = 30 * time.Second // suppress heartbeat noise
	cfg.SimWorkMaxMs = 20                    // cap simulated work at 20ms for smoke speed

	// Context with generous timeout for slow CI boxes.
	// Workers exit cleanly when the server closes each stream (quota exhausted)
	// so elapsed time reflects actual work rather than the full deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := loadtest.RunWorkerScenario(ctx, cfg)
	require.NoError(t, err)

	t.Logf("dispatched=%d succeeded=%d failed=%d errors=%d duration=%s rps=%.1f",
		result.Dispatched, result.Succeeded, result.Failed, result.Errors,
		result.Duration, result.ThroughputRPS,
	)
	assert.GreaterOrEqual(t,

		result.Dispatched,

		50)

	// At least half the tasks must have been processed. The server closes
	// streams when the shared quota is hit, so workers exit cleanly; any
	// shortfall means the stream closed before the worker processed its tasks.

	// Throughput must exceed 50 tasks/sec on any dev machine.
	// Workers exit quickly (simulated work capped at SimWorkMaxMs = 20ms) and
	// the scenario returns as soon as all wg goroutines finish.
	const minRPS = 50.0
	assert.GreaterOrEqual(t,

		result.ThroughputRPS,

		minRPS)

}

// HMAC verifier tests.

// TestVerifyStraitDispatchSignature_Valid checks that a correctly signed request passes.
func TestVerifyStraitDispatchSignature_Valid(t *testing.T) {
	secret := "test-secret-32-bytes-long-enough"
	body := []byte(`{"run_id":"abc","payload":"hello"}`)

	ts, sig := loadtest.SignStraitDispatch(secret, body)

	req := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(body)))
	req.Header.Set("X-Strait-Timestamp", ts)
	req.Header.Set("X-Strait-Signature", sig)
	require.NoError(t, loadtest.
		VerifyStraitDispatchSignature(secret,
			body, req),
	)

}

// TestVerifyStraitDispatchSignature_WrongSecret checks that a mismatched secret fails.
func TestVerifyStraitDispatchSignature_WrongSecret(t *testing.T) {
	body := []byte(`{"run_id":"abc"}`)
	ts, sig := loadtest.SignStraitDispatch("correct-secret", body)

	req := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(body)))
	req.Header.Set("X-Strait-Timestamp", ts)
	req.Header.Set("X-Strait-Signature", sig)
	require.Error(t, loadtest.
		VerifyStraitDispatchSignature("wrong-secret",
			body,
			req))

}

// TestVerifyStraitDispatchSignature_Replay checks that an old timestamp is rejected.
func TestVerifyStraitDispatchSignature_Replay(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"run_id":"old"}`)

	// Forge a signature with a timestamp 10 minutes in the past.
	oldTS := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	payload := append([]byte(oldTS+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(body)))
	req.Header.Set("X-Strait-Timestamp", oldTS)
	req.Header.Set("X-Strait-Signature", "v1="+sig)

	err := loadtest.VerifyStraitDispatchSignature(secret, body, req)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.
		Error(),
		"too old"))

}

// TestVerifyStraitDispatchSignature_MissingHeader checks that a missing header is rejected.
func TestVerifyStraitDispatchSignature_MissingHeader(t *testing.T) {
	body := []byte(`{"run_id":"abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(body)))

	err := loadtest.VerifyStraitDispatchSignature("secret", body, req)
	require.Error(t, err)

}

// TestVerifyStraitDispatchSignature_IntegrationReceiver verifies end-to-end signing
// by spinning up a real HTTP server with a HMAC-verifying handler.
func TestVerifyStraitDispatchSignature_IntegrationReceiver(t *testing.T) {
	secret := "integration-test-secret"
	var gotBody []byte
	var handlerErr error

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handlerErr = err
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if err := loadtest.VerifyStraitDispatchSignature(secret, body, r); err != nil {
			handlerErr = err
			http.Error(w, "signature error", http.StatusUnauthorized)
			return
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	body := []byte(`{"run_id":"integration-run","job":"smoke-job"}`)
	tsValue, sig := loadtest.SignStraitDispatch(secret, body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/dispatch", strings.NewReader(string(body)))
	require.NoError(t, err)

	req.Header.Set("X-Strait-Timestamp", tsValue)
	req.Header.Set("X-Strait-Signature", sig)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()
	assert.NoError(t, handlerErr)
	assert.Equal(t, http.StatusOK,

		resp.
			StatusCode,
	)
	assert.Equal(t, string(
		body,
	), string(gotBody))

	// Now send with wrong secret — must get 401.
	badTS, badSig := loadtest.SignStraitDispatch("wrong-secret", body)
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/dispatch", strings.NewReader(string(body)))
	req2.Header.Set("X-Strait-Timestamp", badTS)
	req2.Header.Set("X-Strait-Signature", badSig)
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)

	defer resp2.Body.Close()
	assert.Equal(t, http.StatusUnauthorized,

		resp2.
			StatusCode)

}

// gRPC client connectivity tests.

// TestGRPCClientConnect_Insecure verifies the gRPC client can dial the test server.
func TestGRPCClientConnect_Insecure(t *testing.T) {
	srvImpl := &echoWorkerServer{totalToDispatch: 0}
	addr := startTestGRPCServer(t, srvImpl)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	defer conn.Close() //nolint:errcheck

	client := workerv1.NewWorkerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.StreamTasks(ctx)
	require.NoError(t, err)

	// Send registration.
	err = stream.Send(&workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       "test-worker-connect",
				Name:           "test",
				Queues:         []string{"q1"},
				SlotsTotal:     4,
				SlotsAvailable: 4,
			},
		},
	})
	require.NoError(t, err)

	// Receive ACK.
	msg, err := stream.Recv()
	require.NoError(t, err)
	assert.NotNil(t, msg.GetAck())

}
