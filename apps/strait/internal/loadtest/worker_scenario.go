//go:build loadtest

package loadtest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// WorkerConfig holds all tunables for the worker-mode load test scenario.
// Every field maps 1:1 to an environment variable understood by the scenario
// entrypoint so operator can drive the scenario without recompiling.
type WorkerConfig struct {
	// GRPCAddr is the host:port of the Strait gRPC endpoint.
	// Default: localhost:50051
	GRPCAddr string

	// APIKey is the Bearer token sent in gRPC metadata.
	APIKey string

	// QueueName is the queue each simulated worker registers with.
	// Default: "loadtest"
	QueueName string

	// WorkerCount is the number of concurrent simulated workers.
	// Default: 10
	WorkerCount int

	// SlotsPerWorker is the slots_total advertised by each worker.
	// Default: 16
	SlotsPerWorker int

	// FailRate is the fraction of dispatches that reply with Status="failed".
	// Range: 0.0–1.0.  Default: 0.05
	FailRate float64

	// HMACSecret is the shared secret used to verify the X-Strait-Signature
	// header on the HTTP dispatch receiver.  Empty disables verification.
	HMACSecret string

	// HeartbeatInterval controls how often each worker sends a Heartbeat.
	// Default: 15s
	HeartbeatInterval time.Duration

	// MaxBackoff caps exponential reconnect delays.
	// Default: 60s
	MaxBackoff time.Duration

	// SimWorkMaxMs caps the simulated work duration per task in milliseconds.
	// Zero means no cap (up to 10s).  Used by smoke tests to run quickly.
	SimWorkMaxMs int
}

// DefaultWorkerConfig returns a WorkerConfig with sensible defaults.
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		GRPCAddr:          "localhost:50051",
		QueueName:         "loadtest",
		WorkerCount:       10,
		SlotsPerWorker:    16,
		FailRate:          0.05,
		HeartbeatInterval: 15 * time.Second,
		MaxBackoff:        60 * time.Second,
	}
}

// WorkerScenarioResult captures aggregate metrics from a worker-mode run.
type WorkerScenarioResult struct {
	Dispatched    int64         `json:"dispatched"`
	Succeeded     int64         `json:"succeeded"`
	Failed        int64         `json:"failed"`
	Errors        int64         `json:"errors"`
	Duration      time.Duration `json:"duration"`
	ThroughputRPS float64       `json:"throughput_rps"`
}

// RunWorkerScenario is the exported entrypoint used by tests and the CLI
// entrypoint. It delegates to workerScenarioImpl.
func RunWorkerScenario(ctx context.Context, cfg WorkerConfig) (*WorkerScenarioResult, error) {
	return workerScenarioImpl(ctx, cfg)
}

// workerScenarioImpl replaces the TODO stub in scenarios.go and drives
// ModeWorker load tests via the bidirectional gRPC WorkerService.StreamTasks RPC.
//
// Each simulated worker:
//  1. Dials the gRPC endpoint with a per-worker worker_id and the configured API key.
//  2. Sends WorkerRegistration as the first message.
//  3. Heartbeats every HeartbeatInterval.
//  4. On TaskAssignment: simulates work for a random fraction of timeout_secs,
//     then sends TaskResult{Status: "success"} (or "failed" at FailRate).
//  5. Reconnects with capped exponential backoff on stream errors.
func workerScenarioImpl(ctx context.Context, cfg WorkerConfig) (*WorkerScenarioResult, error) {
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = DefaultWorkerConfig().GRPCAddr
	}
	if cfg.QueueName == "" {
		cfg.QueueName = DefaultWorkerConfig().QueueName
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = DefaultWorkerConfig().WorkerCount
	}
	if cfg.SlotsPerWorker <= 0 {
		cfg.SlotsPerWorker = DefaultWorkerConfig().SlotsPerWorker
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = DefaultWorkerConfig().HeartbeatInterval
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = DefaultWorkerConfig().MaxBackoff
	}

	var (
		dispatched atomic.Int64
		succeeded  atomic.Int64
		failed     atomic.Int64
		errors     atomic.Int64
	)

	start := time.Now()

	var wg sync.WaitGroup
	for i := range cfg.WorkerCount {
		workerID := fmt.Sprintf("loadtest-worker-%04d", i)
		wg.Add(1)
		go runWorkerAndDone(ctx, cfg, workerID, &dispatched, &succeeded, &failed, &errors, &wg)
	}

	wg.Wait()
	elapsed := time.Since(start)

	total := dispatched.Load()
	rps := 0.0
	if elapsed > 0 {
		rps = float64(total) / elapsed.Seconds()
	}

	return &WorkerScenarioResult{
		Dispatched:    total,
		Succeeded:     succeeded.Load(),
		Failed:        failed.Load(),
		Errors:        errors.Load(),
		Duration:      elapsed,
		ThroughputRPS: rps,
	}, nil
}

// runWorkerAndDone is a thin shim that calls runSimulatedWorker then decrements wg.
func runWorkerAndDone(
	ctx context.Context,
	cfg WorkerConfig,
	workerID string,
	dispatched, succeeded, failed, errors *atomic.Int64,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	runSimulatedWorker(ctx, cfg, workerID, dispatched, succeeded, failed, errors)
}

// runSimulatedWorker dials, registers, and processes tasks in a reconnect loop
// until ctx is cancelled. On a clean stream close (server-initiated EOF with no
// error), it exits without reconnecting so the caller can wait-group correctly.
func runSimulatedWorker(
	ctx context.Context,
	cfg WorkerConfig,
	workerID string,
	dispatched, succeeded, failed, errors *atomic.Int64,
) {
	backoff := 250 * time.Millisecond

	for {
		if ctx.Err() != nil {
			return
		}

		streamErr := connectAndStream(ctx, cfg, workerID, dispatched, succeeded, failed)
		switch {
		case streamErr == nil:
			// Clean disconnect — either ctx was cancelled inside connectAndStream
			// or the server closed the stream gracefully (EOF). Exit the loop.
			return
		case ctx.Err() != nil:
			// Our own cancellation raced with a stream error; treat as clean exit.
			return
		default:
			// Transient error — reconnect with exponential backoff.
			errors.Add(1)
			slog.Warn("worker stream error; reconnecting",
				"worker_id", workerID,
				"error", streamErr,
				"backoff", backoff,
			)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, cfg.MaxBackoff)
		}
	}
}

// connectAndStream establishes one gRPC dial, registers, and runs the recv/send
// loops until the stream closes or ctx is cancelled.
//
// Architecture: gRPC Recv() is blocking so it runs in a dedicated goroutine that
// delivers incoming ServerMessages over an inbound channel.  A single send loop
// drains three sources (heartbeats, task results, ctx) and writes to the stream.
// This avoids data races on stream.Send() and keeps flow clean.
func connectAndStream(
	ctx context.Context,
	cfg WorkerConfig,
	workerID string,
	dispatched, succeeded, failed *atomic.Int64,
) error {
	// gRPC dial — plaintext; TLS termination is handled upstream in production.
	conn, err := grpc.NewClient(
		cfg.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.GRPCAddr, err)
	}
	defer conn.Close() //nolint:errcheck

	client := workerv1.NewWorkerServiceClient(conn)

	// Attach API key as Bearer token in gRPC metadata.
	outCtx := ctx
	if cfg.APIKey != "" {
		md := metadata.Pairs("authorization", "Bearer "+cfg.APIKey)
		outCtx = metadata.NewOutgoingContext(ctx, md)
	}

	stream, err := client.StreamTasks(outCtx)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	// Send registration as the very first message.
	if err := stream.Send(&workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       workerID,
				Name:           "loadtest-worker",
				Queues:         []string{cfg.QueueName},
				SlotsTotal:     int32(cfg.SlotsPerWorker), //nolint:gosec
				SlotsAvailable: int32(cfg.SlotsPerWorker), //nolint:gosec
				SdkVersion:     "loadtest/1.0",
				SdkLanguage:    "go",
				Hostname:       "loadtest-host",
				Metadata: map[string]string{
					"mode": "loadtest",
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("send registration: %w", err)
	}

	// inbound receives ServerMessages from the blocking Recv loop.
	inbound := make(chan *workerv1.ServerMessage, 64)
	// outbound carries WorkerMessages produced by task-simulation goroutines.
	outbound := make(chan *workerv1.WorkerMessage, 256)
	// recvErr carries the first error from the recv goroutine.
	recvErr := make(chan error, 1)

	// Recv goroutine — must not call stream.Send().
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			select {
			case inbound <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	hbTicker := time.NewTicker(cfg.HeartbeatInterval)
	defer hbTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = stream.CloseSend()
			return nil

		case err := <-recvErr:
			if isStreamClosedErr(err) {
				return nil
			}
			return fmt.Errorf("recv: %w", err)

		case <-hbTicker.C:
			if err := stream.Send(&workerv1.WorkerMessage{
				Payload: &workerv1.WorkerMessage_Heartbeat{
					Heartbeat: &workerv1.Heartbeat{
						SlotsAvailable: int32(cfg.SlotsPerWorker), //nolint:gosec
						TimestampUnix:  time.Now().Unix(),
					},
				},
			}); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}

		case msg := <-outbound:
			if err := stream.Send(msg); err != nil {
				return fmt.Errorf("send task result: %w", err)
			}

		case msg := <-inbound:
			switch p := msg.Payload.(type) {
			case *workerv1.ServerMessage_TaskAssignment:
				ta := p.TaskAssignment
				dispatched.Add(1)

				go func() {
					result := simulateTask(ctx, cfg, ta)
					if result.GetTaskResult().GetStatus() == "success" {
						succeeded.Add(1)
					} else {
						failed.Add(1)
					}
					select {
					case outbound <- result:
					case <-ctx.Done():
					}
				}()

			case *workerv1.ServerMessage_CancelTask:
				// Simulated tasks honour context cancellation; nothing extra needed.
				_ = p

			case *workerv1.ServerMessage_Ack:
				// Registration ACK.
				_ = p
			}
		}
	}
}

// simulateTask simulates executing a dispatched task and returns the TaskResult message.
// It sleeps for a random fraction (0.1–0.9) of the announced timeout (capped by
// SimWorkMaxMs when set), then decides pass/fail based on FailRate.
func simulateTask(ctx context.Context, cfg WorkerConfig, ta *workerv1.TaskAssignment) *workerv1.WorkerMessage {
	timeoutSecs := ta.GetTimeoutSecs()
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}

	// Random work duration: 10%–90% of timeout, capped at 10s for load test sanity.
	fraction := 0.1 + rand.Float64()*0.8 //nolint:gosec
	workDuration := time.Duration(float64(timeoutSecs) * fraction * float64(time.Second))
	const defaultMaxWork = 10 * time.Second
	maxWork := defaultMaxWork
	if cfg.SimWorkMaxMs > 0 {
		maxWork = time.Duration(cfg.SimWorkMaxMs) * time.Millisecond
	}
	if workDuration > maxWork {
		workDuration = maxWork
	}

	start := time.Now()

	select {
	case <-ctx.Done():
		return taskResultMsg(ta.RunId, "failed", "context cancelled", time.Since(start))
	case <-time.After(workDuration):
	}

	taskStatus := "success"
	var errMsg string
	if rand.Float64() < cfg.FailRate { //nolint:gosec
		taskStatus = "failed"
		errMsg = "simulated failure (loadtest)"
	}

	return taskResultMsg(ta.RunId, taskStatus, errMsg, time.Since(start))
}

// taskResultMsg builds a WorkerMessage wrapping a TaskResult.
func taskResultMsg(runID, taskStatus, errMsg string, duration time.Duration) *workerv1.WorkerMessage {
	return &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_TaskResult{
			TaskResult: &workerv1.TaskResult{
				RunId:        runID,
				Status:       taskStatus,
				ErrorMessage: errMsg,
				DurationMs:   duration.Milliseconds(),
			},
		},
	}
}

// isStreamClosedErr returns true for gRPC errors that indicate the stream is
// done and the worker should reconnect (or exit when ctx is cancelled).
func isStreamClosedErr(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if !ok {
		// Non-gRPC error (e.g. io.EOF) — treat as closed.
		return true
	}
	switch s.Code() { //nolint:exhaustive // only reconnect-worthy codes matter; all others are non-retriable
	case codes.Canceled, codes.Unavailable, codes.DeadlineExceeded:
		return true
	}
	return false
}

// straitV1SignatureHeader is the header name the Strait server sets on HTTP-mode
// dispatch requests. Its format mirrors the Stripe V1 convention:
// "t=<unix_timestamp>,v1=<hmac_sha256_hex>".
const straitV1SignatureHeader = "X-Strait-Signature"

// replayWindow is the maximum age of a valid signature.
const replayWindow = 5 * time.Minute

// VerifyStraitDispatchSignature validates the X-Strait-Signature header on an
// inbound HTTP dispatch request using the same algorithm as validateStripeV1 in
// internal/webhook/signature.go but with the Strait-specific header name and
// a 5-minute replay window.
//
// Signed payload: "<timestamp>.<body>". Header format: "t=<unix>,v1=<hmac-sha256-hex>".
func VerifyStraitDispatchSignature(secret string, body []byte, r *http.Request) error {
	headerValue := r.Header.Get(straitV1SignatureHeader)
	if headerValue == "" {
		return fmt.Errorf("missing %s header", straitV1SignatureHeader)
	}

	var ts, sig string
	for part := range strings.SplitSeq(headerValue, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sig = kv[1]
		}
	}

	if ts == "" || sig == "" {
		return fmt.Errorf("invalid %s header: missing t or v1", straitV1SignatureHeader)
	}

	// Parse timestamp and enforce replay window.
	var tsInt int64
	if _, err := fmt.Sscanf(ts, "%d", &tsInt); err != nil {
		return fmt.Errorf("invalid %s timestamp: %w", straitV1SignatureHeader, err)
	}
	age := time.Since(time.Unix(tsInt, 0)).Abs()
	if age > replayWindow {
		return fmt.Errorf("%s timestamp too old: %s", straitV1SignatureHeader, age)
	}

	// Recompute HMAC-SHA256(secret, "<ts>.<body>").
	payload := append([]byte(ts+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	computed := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(computed)) {
		return fmt.Errorf("%s signature mismatch", straitV1SignatureHeader)
	}
	return nil
}

// SignStraitDispatch produces an X-Strait-Signature header value for the given
// body. Used by the HTTP-mode test harness to sign outgoing dispatch requests
// and by tests to produce valid signatures for VerifyStraitDispatchSignature.
func SignStraitDispatch(secret string, body []byte) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	payload := append([]byte(ts+"."), body...)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}
