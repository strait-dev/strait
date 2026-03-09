// Package sandbox provides a gRPC client for the Forge sandbox execution service.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	sandboxv1 "strait/internal/sandbox/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client connects to the Forge sandbox service via gRPC.
type Client struct {
	conn   *grpc.ClientConn
	addr   string
	logger *slog.Logger
	mu     sync.RWMutex
}

// NewClient creates a new sandbox client. Call Connect() before use.
func NewClient(addr string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		addr:   addr,
		logger: logger,
	}
}

// Connect establishes the gRPC connection to Forge.
func (c *Client) Connect(ctx context.Context) error {
	conn, err := grpc.NewClient(
		c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect to forge at %s: %w", c.addr, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.logger.Info("connected to forge", "addr", c.addr)
	return nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// EventHandler is called for each streamed event during sandbox execution.
type EventHandler func(event *sandboxv1.ExecutionEvent) error

// ExecuteRequest contains the parameters for a sandbox execution.
type ExecuteRequest struct {
	RunID    string
	Language string
	Code     string
	Payload  json.RawMessage
	Env      map[string]string
	Timeout  time.Duration
	MemoryMB int64
}

// ExecuteResult is the outcome of a sandbox execution.
type ExecuteResult struct {
	Success    bool
	Result     json.RawMessage
	Error      string
	DurationMs int64
	Events     []sandboxv1.ExecutionEvent
}

// Execute runs code in the Forge sandbox and collects all streamed events.
// It returns the final result after the execution completes.
// The provided context controls cancellation — canceling it will terminate
// the sandbox execution in Forge.
func (c *Client) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	var events []sandboxv1.ExecutionEvent
	var finalResult *sandboxv1.ExecutionResult

	err := c.ExecuteStream(ctx, req, func(event *sandboxv1.ExecutionEvent) error {
		events = append(events, *event)
		if event.Result != nil {
			finalResult = event.Result
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if finalResult == nil {
		return nil, fmt.Errorf("sandbox execution completed without a result event")
	}

	execResult := &ExecuteResult{
		Success:    finalResult.Success,
		DurationMs: finalResult.DurationMs,
		Events:     events,
	}
	if len(finalResult.Result) > 0 {
		execResult.Result = json.RawMessage(finalResult.Result)
	}
	if finalResult.Error != "" {
		execResult.Error = finalResult.Error
	}

	return execResult, nil
}

// ExecuteStream runs code in the Forge sandbox and calls handler for each
// streamed event. This is the low-level API for when you need to process
// events as they arrive (e.g., writing to the run events store).
func (c *Client) ExecuteStream(ctx context.Context, req *ExecuteRequest, handler EventHandler) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("sandbox client not connected")
	}

	// Build the gRPC request
	grpcReq := &sandboxv1.ExecuteRequest{
		RunID:    req.RunID,
		Language: req.Language,
		Code:     req.Code,
		Payload:  req.Payload,
		Env:      req.Env,
		Limits: &sandboxv1.ResourceLimits{
			TimeoutSecs: int32(req.Timeout.Seconds()),
			MemoryBytes: req.MemoryMB * 1024 * 1024,
		},
	}

	// Use the JSON-over-gRPC approach: we serialize the request, send it,
	// and receive streamed JSON responses. This avoids depending on protobuf
	// codegen while maintaining the same wire format.
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "Execute",
		ServerStreams:  true,
	}, "/sandbox.v1.SandboxExecutor/Execute")
	if err != nil {
		return fmt.Errorf("open sandbox stream: %w", err)
	}

	reqBytes, err := json.Marshal(grpcReq)
	if err != nil {
		return fmt.Errorf("marshal execute request: %w", err)
	}

	if err := stream.SendMsg(&rawMessage{data: reqBytes}); err != nil {
		return fmt.Errorf("send execute request: %w", err)
	}

	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("close send: %w", err)
	}

	for {
		var eventBytes rawMessage
		err := stream.RecvMsg(&eventBytes)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive sandbox event: %w", err)
		}

		var event sandboxv1.ExecutionEvent
		if err := json.Unmarshal(eventBytes.data, &event); err != nil {
			c.logger.Warn("failed to unmarshal sandbox event", "error", err)
			continue
		}

		if err := handler(&event); err != nil {
			return fmt.Errorf("event handler: %w", err)
		}
	}
}

// rawMessage implements the grpc codec message interface for raw bytes.
type rawMessage struct {
	data []byte
}

func (r *rawMessage) Marshal() ([]byte, error) {
	return r.data, nil
}

func (r *rawMessage) Unmarshal(b []byte) error {
	r.data = make([]byte, len(b))
	copy(r.data, b)
	return nil
}

func (r *rawMessage) ProtoMessage()             {}
func (r *rawMessage) Reset()                    { r.data = nil }
func (r *rawMessage) String() string            { return string(r.data) }
