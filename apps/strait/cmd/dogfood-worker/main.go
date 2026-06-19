package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"

	"github.com/sourcegraph/conc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type config struct {
	GRPCAddr          string
	APIKey            string
	WorkerID          string
	QueueName         string
	JobSlugs          []string
	Slots             int32
	HeartbeatInterval time.Duration
	WorkDelay         time.Duration
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	cfg := config{
		GRPCAddr:          envString("DOGFOOD_GRPC_ADDR", "localhost:15053"),
		APIKey:            os.Getenv("DOGFOOD_WORKER_API_KEY"),
		WorkerID:          envString("DOGFOOD_WORKER_ID", "dogfood-worker"),
		QueueName:         envString("DOGFOOD_WORKER_QUEUE", "dogfood"),
		Slots:             int32(envInt("DOGFOOD_WORKER_SLOTS", 1)), //nolint:gosec // local dogfood knob.
		HeartbeatInterval: envDuration("DOGFOOD_WORKER_HEARTBEAT", 2*time.Second),
		WorkDelay:         envDuration("DOGFOOD_WORKER_DELAY", 100*time.Millisecond),
	}
	if cfg.APIKey == "" {
		return cfg, fmt.Errorf("DOGFOOD_WORKER_API_KEY is required")
	}
	if cfg.Slots <= 0 {
		return cfg, fmt.Errorf("DOGFOOD_WORKER_SLOTS must be greater than zero")
	}
	if raw := strings.TrimSpace(os.Getenv("DOGFOOD_WORKER_JOB_SLUGS")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if slug := strings.TrimSpace(part); slug != "" {
				cfg.JobSlugs = append(cfg.JobSlugs, slug)
			}
		}
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config) error {
	conn, err := grpc.NewClient(
		cfg.GRPCAddr,
		grpc.WithTransportCredentials(transportCredentials(cfg.GRPCAddr)),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.GRPCAddr, err)
	}
	defer conn.Close() //nolint:errcheck

	client := workerv1.NewWorkerServiceClient(conn)
	streamCtx, streamCancel := context.WithCancel(ctx)
	outCtx := metadata.NewOutgoingContext(streamCtx, metadata.Pairs("authorization", "Bearer "+cfg.APIKey))
	stream, err := client.StreamTasks(outCtx)
	if err != nil {
		streamCancel()
		return fmt.Errorf("open stream: %w", err)
	}

	if err := stream.Send(registrationMessage(cfg)); err != nil {
		return fmt.Errorf("send registration: %w", err)
	}
	slog.Info("dogfood worker registered", "worker_id", cfg.WorkerID, "queue", cfg.QueueName)

	var wg conc.WaitGroup
	defer func() {
		streamCancel()
		wg.Wait()
	}()

	inbound := make(chan *workerv1.ServerMessage, 16)
	outbound := make(chan *workerv1.WorkerMessage, 16)
	recvErr := make(chan error, 1)

	wg.Go(func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				select {
				case recvErr <- err:
				case <-streamCtx.Done():
				}
				return
			}
			select {
			case inbound <- msg:
			case <-streamCtx.Done():
				return
			}
		}
	})

	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-streamCtx.Done():
			_ = stream.CloseSend()
			return streamCtx.Err()
		case err := <-recvErr:
			if isCleanStreamClose(err) {
				return nil
			}
			return fmt.Errorf("recv: %w", err)
		case <-ticker.C:
			if err := stream.Send(heartbeatMessage(cfg)); err != nil {
				return fmt.Errorf("send heartbeat: %w", err)
			}
		case msg := <-outbound:
			if err := stream.Send(msg); err != nil {
				return fmt.Errorf("send task result: %w", err)
			}
		case msg := <-inbound:
			switch payload := msg.Payload.(type) {
			case *workerv1.ServerMessage_TaskAssignment:
				assignment := payload.TaskAssignment
				wg.Go(func() {
					result := executeAssignment(streamCtx, cfg, assignment)
					select {
					case outbound <- result:
					case <-streamCtx.Done():
					}
				})
			case *workerv1.ServerMessage_Ack:
				slog.Info("dogfood worker ack", "id", payload.Ack.GetId())
			case *workerv1.ServerMessage_CancelTask:
				slog.Info("dogfood worker cancel", "run_id", payload.CancelTask.GetRunId(), "reason", payload.CancelTask.GetReason())
			}
		}
	}
}

func registrationMessage(cfg config) *workerv1.WorkerMessage {
	return &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Registration{
			Registration: &workerv1.WorkerRegistration{
				WorkerId:       cfg.WorkerID,
				Name:           "dogfood-worker",
				Queues:         []string{cfg.QueueName},
				JobSlugs:       cfg.JobSlugs,
				SdkVersion:     "dogfood/1.0",
				SdkLanguage:    "go",
				Hostname:       "local-dogfood",
				SlotsTotal:     cfg.Slots,
				SlotsAvailable: cfg.Slots,
				Metadata: map[string]string{
					"mode": "dogfood",
				},
			},
		},
	}
}

func heartbeatMessage(cfg config) *workerv1.WorkerMessage {
	return &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_Heartbeat{
			Heartbeat: &workerv1.Heartbeat{
				SlotsAvailable: cfg.Slots,
				TimestampUnix:  time.Now().Unix(),
			},
		},
	}
}

func executeAssignment(ctx context.Context, cfg config, assignment *workerv1.TaskAssignment) *workerv1.WorkerMessage {
	start := time.Now()
	select {
	case <-ctx.Done():
		return taskResult(assignment, "failed", "dogfood worker stopped", nil, time.Since(start))
	case <-time.After(cfg.WorkDelay):
	}

	output, err := json.Marshal(map[string]any{
		"ok":        true,
		"worker_id": cfg.WorkerID,
		"job_slug":  assignment.GetJobSlug(),
		"queue":     assignment.GetQueue(),
	})
	if err != nil {
		return taskResult(assignment, "failed", err.Error(), nil, time.Since(start))
	}
	return taskResult(assignment, "success", "", output, time.Since(start))
}

func taskResult(assignment *workerv1.TaskAssignment, state, errMsg string, output []byte, duration time.Duration) *workerv1.WorkerMessage {
	return &workerv1.WorkerMessage{
		Payload: &workerv1.WorkerMessage_TaskResult{
			TaskResult: &workerv1.TaskResult{
				RunId:        assignment.GetRunId(),
				Status:       state,
				OutputJson:   output,
				ErrorMessage: errMsg,
				DurationMs:   duration.Milliseconds(),
				AssignmentId: assignment.GetAssignmentId(),
				Attempt:      assignment.GetAttempt(),
			},
		},
	}
}

func isCleanStreamClose(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.OK, codes.Canceled:
		return true
	default:
		return false
	}
}

func transportCredentials(addr string) credentials.TransportCredentials {
	host, _, err := net.SplitHostPort(addr)
	if os.Getenv("DOGFOOD_GRPC_PLAINTEXT") == "true" || (err == nil && isLoopback(host)) {
		return insecure.NewCredentials()
	}
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
