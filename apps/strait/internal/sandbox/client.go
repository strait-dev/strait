// Package sandbox provides a gRPC client for the Forge sandbox execution service.
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	sandboxv1 "strait/internal/sandbox/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

var (
	// ErrNotConnected is returned when Execute is called before Connect.
	ErrNotConnected = fmt.Errorf("sandbox client not connected")

	// ErrNoResult is returned when the sandbox stream ends without a result event.
	ErrNoResult = fmt.Errorf("sandbox execution completed without a result event")
)

// Default keepalive parameters for the gRPC connection.
const (
	defaultKeepaliveTime    = 10 * time.Second // ping every 10s when idle
	defaultKeepaliveTimeout = 3 * time.Second  // wait 3s for ping ack
)

// ClientOption configures optional client behaviour.
type ClientOption func(*clientConfig)

type clientConfig struct {
	keepaliveTime    time.Duration
	keepaliveTimeout time.Duration
	extraDialOpts    []grpc.DialOption
}

// WithKeepaliveInterval sets how often the client pings the server when idle.
// Default: 10 seconds.
func WithKeepaliveInterval(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.keepaliveTime = d }
}

// WithKeepaliveTimeout sets how long the client waits for a keepalive ping
// acknowledgment before considering the connection dead. Default: 3 seconds.
func WithKeepaliveTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.keepaliveTimeout = d }
}

// WithDialOptions appends additional gRPC dial options. These are passed
// directly to grpc.NewClient when Connect is called.
func WithDialOptions(opts ...grpc.DialOption) ClientOption {
	return func(c *clientConfig) { c.extraDialOpts = append(c.extraDialOpts, opts...) }
}

// Client connects to the Forge sandbox service via gRPC.
type Client struct {
	conn   *grpc.ClientConn
	rpc    sandboxv1.SandboxExecutorClient
	addr   string
	logger *slog.Logger
	cfg    clientConfig
	mu     sync.RWMutex
}

// NewClient creates a new sandbox client. Call Connect() before use.
func NewClient(addr string, logger *slog.Logger, opts ...ClientOption) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := clientConfig{
		keepaliveTime:    defaultKeepaliveTime,
		keepaliveTimeout: defaultKeepaliveTimeout,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &Client{
		addr:   addr,
		logger: logger,
		cfg:    cfg,
	}
}

// Connect establishes the gRPC connection to Forge. If a previous connection
// exists, it is closed first to prevent resource leaks.
//
// The connection uses keepalive pings so that dead connections (e.g. Forge
// restarts) are detected quickly rather than waiting for TCP timeouts.
func (c *Client) Connect(_ context.Context) error {
	dialOpts := make([]grpc.DialOption, 0, 2+len(c.cfg.extraDialOpts))
	dialOpts = append(dialOpts,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                c.cfg.keepaliveTime,
			Timeout:             c.cfg.keepaliveTimeout,
			PermitWithoutStream: true, // ping even with no active RPCs
		}),
	)
	dialOpts = append(dialOpts, c.cfg.extraDialOpts...)

	conn, err := grpc.NewClient(c.addr, dialOpts...)
	if err != nil {
		return fmt.Errorf("connect to forge at %s: %w", c.addr, err)
	}

	c.mu.Lock()
	old := c.conn
	c.conn = conn
	c.rpc = sandboxv1.NewSandboxExecutorClient(conn)
	c.mu.Unlock()

	if old != nil {
		if closeErr := old.Close(); closeErr != nil {
			c.logger.Warn("failed to close previous connection", "error", closeErr)
		}
	}

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

// IsConnected reports whether the gRPC connection is in a usable state
// (Ready or Idle). Returns false if Connect has not been called or if the
// underlying transport has failed.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return false
	}
	state := conn.GetState()
	return state == connectivity.Ready || state == connectivity.Idle
}

// WaitForReady blocks until the gRPC connection reaches the Ready state or
// the context expires. This is useful at startup to ensure Forge is reachable
// before accepting work.
func (c *Client) WaitForReady(ctx context.Context) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return ErrNotConnected
	}

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return fmt.Errorf("connection shut down")
		}
		if !conn.WaitForStateChange(ctx, state) {
			return ctx.Err()
		}
	}
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
	Events     []*sandboxv1.ExecutionEvent
}

// Execute runs code in the Forge sandbox and collects all streamed events.
// It returns the final result after the execution completes.
// The provided context controls cancellation — canceling it will terminate
// the sandbox execution in Forge.
func (c *Client) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error) {
	var events []*sandboxv1.ExecutionEvent
	var finalResult *sandboxv1.ExecutionResult

	err := c.ExecuteStream(ctx, req, func(event *sandboxv1.ExecutionEvent) error {
		events = append(events, event)
		if r := event.GetResult(); r != nil {
			finalResult = r
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	if finalResult == nil {
		return nil, ErrNoResult
	}

	execResult := &ExecuteResult{
		Success:    finalResult.GetSuccess(),
		DurationMs: finalResult.GetDurationMs(),
		Events:     events,
	}
	if len(finalResult.GetResult()) > 0 {
		execResult.Result = json.RawMessage(finalResult.GetResult())
	}
	if finalResult.GetError() != "" {
		execResult.Error = finalResult.GetError()
	}

	return execResult, nil
}

// ExecuteStream runs code in the Forge sandbox and calls handler for each
// streamed event. This is the low-level API for when you need to process
// events as they arrive (e.g., writing to the run events store).
func (c *Client) ExecuteStream(ctx context.Context, req *ExecuteRequest, handler EventHandler) error {
	c.mu.RLock()
	rpc := c.rpc
	c.mu.RUnlock()

	if rpc == nil {
		return ErrNotConnected
	}

	// Build the protobuf request
	grpcReq := &sandboxv1.ExecuteRequest{
		RunId:    req.RunID,
		Language: req.Language,
		Code:     req.Code,
		Payload:  req.Payload,
		Env:      req.Env,
		Limits: &sandboxv1.ResourceLimits{
			TimeoutSecs:    int32(req.Timeout.Seconds()),
			MemoryBytes:    req.MemoryMB * 1024 * 1024,
			NetworkEnabled: false,
		},
	}

	stream, err := rpc.Execute(ctx, grpcReq)
	if err != nil {
		return fmt.Errorf("open sandbox stream: %w", err)
	}

	for {
		event, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("receive sandbox event: %w", err)
		}

		if err := handler(event); err != nil {
			return fmt.Errorf("event handler: %w", err)
		}
	}
}
