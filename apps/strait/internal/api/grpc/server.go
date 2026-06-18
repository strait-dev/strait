// Package grpc provides the gRPC server for the Worker mode streaming API.
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/store"

	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

// Server wraps the gRPC server and all its dependencies.
type Server struct {
	cfg             *config.Config
	queries         *store.Queries
	pub             pubsub.Publisher
	registry        *ConnectionRegistry
	resultChannels  *ResultChannelRegistry
	runFinalizer    atomic.Value
	authLimiter     grpcAuthLimiter
	apiKeyResolver  apiKeyResolver
	secretDecryptor SecretDecryptor
	billingEnforcer planLimitEnforcer
	edition         domain.Edition
	readyRunQueue   ReadyRunEnqueuer
	gs              *grpc.Server
	version         string
}

// planLimitEnforcer is the subset of *billing.Enforcer the gRPC stream uses to
// gate connect-time worker registration. Defined here rather than imported as
// the concrete *billing.Enforcer so the gRPC package stays free of a
// circular dep on billing (billing already imports nothing from grpc, but the
// interface seam keeps tests from needing a full Enforcer).
type planLimitEnforcer interface {
	CheckWorkerConnectionLimit(ctx context.Context, orgID string, currentActive int) error
	GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error)
}

type workerConnectionReservationEnforcer interface {
	ReserveWorkerConnection(ctx context.Context, orgID, reservationID string, lease time.Duration) (func(), error)
	RenewWorkerConnection(ctx context.Context, orgID, reservationID string, lease time.Duration) error
}

// ReadyRunEnqueuer is implemented by queue backends that can emit a ready
// event for an existing run after store-owned recovery moves it back to queued.
type ReadyRunEnqueuer interface {
	EnqueueExisting(ctx context.Context, run *domain.JobRun) error
}

type ServerOption func(*Server)

func WithAuthLimiter(limiter grpcAuthLimiter) ServerOption {
	return func(s *Server) {
		s.authLimiter = limiter
	}
}

func WithAPIKeyCache(client redis.Cmdable, ttl time.Duration) ServerOption {
	return func(s *Server) {
		s.apiKeyResolver = newCachedAPIKeyResolver(client, ttl, queryAPIKeyResolver(s.queries))
	}
}

func WithVersion(version string) ServerOption {
	return func(s *Server) {
		s.version = version
	}
}

func WithSecretDecryptor(dec SecretDecryptor) ServerOption {
	return func(s *Server) {
		s.secretDecryptor = dec
	}
}

// WithBillingEnforcer attaches a plan-limit enforcer used to gate worker
// registration by tier. Cloud registrations fail closed when this dependency
// is absent; community/self-hosted builds skip the billing gate.
func WithBillingEnforcer(enforcer planLimitEnforcer) ServerOption {
	return func(s *Server) {
		s.billingEnforcer = enforcer
	}
}

func WithReadyRunEnqueuer(enqueuer ReadyRunEnqueuer) ServerOption {
	return func(s *Server) {
		s.readyRunQueue = enqueuer
	}
}

// NewServer creates a new gRPC Server. It does not start listening.
//
// Preconditions:
//   - pub MUST be non-nil. The worker stream depends on the publisher for
//     cross-replica force-disconnect, API-key revocation broadcast, and
//     run-result / log-line fan-out. A nil publisher would panic the first
//     time a worker connects (Subscribe / Publish dereference). We refuse
//     to boot rather than serve a half-functional stream.
//   - Returns an error if TLS is configured but cert/key cannot be loaded —
//     silent fallback to plaintext would leak API keys in transit.
func NewServer(cfg *config.Config, queries *store.Queries, pub pubsub.Publisher, opts ...ServerOption) (*Server, error) {
	if pub == nil {
		return nil, fmt.Errorf("grpc server: pubsub publisher is required (set REDIS_URL or disable GRPC_ENABLED)")
	}
	s := &Server{
		cfg:            cfg,
		queries:        queries,
		pub:            pub,
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
		apiKeyResolver: queryAPIKeyResolver(queries),
		edition:        domain.ParseEdition(""),
		version:        "unknown",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.authLimiter == nil {
		// No brute-force protection on the gRPC worker-auth path. The production
		// wiring always passes WithAuthLimiter; warn loudly so a manual/community
		// deployment that omits it does not silently run without throttling.
		slog.Warn("grpc server: no auth limiter configured; gRPC worker authentication is NOT rate limited")
	}
	gs, err := s.buildServer()
	if err != nil {
		return nil, err
	}
	s.gs = gs
	return s, nil
}

// Registry returns the connection registry for external use (e.g. dispatcher).
func (s *Server) Registry() *ConnectionRegistry {
	return s.registry
}

// WorkerDispatcher returns a WorkerDispatcher wired to this server's registry
// and result channel registry. Used by the executor to dispatch worker-mode runs.
func (s *Server) WorkerDispatcher(jwtSigningKey string) *WorkerDispatcher {
	return NewWorkerDispatcher(s.registry, s.queries, jwtSigningKey, s.resultChannels).
		WithSecretDecryptor(s.secretDecryptor)
}

// SetRunResultFinalizer wires the executor-owned completion path for worker
// results that arrive after the normal dispatch goroutine is gone.
func (s *Server) SetRunResultFinalizer(finalizer WorkerRunResultFinalizer) {
	if finalizer != nil {
		s.runFinalizer.Store(finalizer)
	}
}

func (s *Server) buildServer() (*grpc.Server, error) {
	kpParams := keepalive.ServerParameters{
		Time:    s.cfg.GRPCKeepaliveTime,
		Timeout: s.cfg.GRPCKeepaliveTimeout,
	}
	kpPolicy := keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}

	// Cap inbound message size so a malicious worker cannot send 4 GB protos.
	const maxRecvBytes = 1 * 1024 * 1024 // 1 MiB
	const maxSendBytes = 4 * 1024 * 1024 // 4 MiB

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(kpParams),
		grpc.KeepaliveEnforcementPolicy(kpPolicy),
		grpc.MaxRecvMsgSize(maxRecvBytes),
		grpc.MaxSendMsgSize(maxSendBytes),
		grpc.ChainUnaryInterceptor(unaryInterceptorChainWithMetadata(s.sentryMetadata())...),
		grpc.ChainStreamInterceptor(streamInterceptorChainWithMetadata(s.sentryMetadata())...),
	}

	// TLS is mutually exclusive: either both paths are set and both must load,
	// or both are empty and Serve enforces the plaintext exposure policy.
	// Partial config or load failure is a hard error so an operator who
	// configured TLS never silently runs cleartext.
	certPath := s.cfg.GRPCTLSCertPath
	keyPath := s.cfg.GRPCTLSKeyPath
	if certPath != "" {
		if keyPath == "" {
			return nil, fmt.Errorf("grpc tls: both GRPC_TLS_CERT_PATH and GRPC_TLS_KEY_PATH must be set together")
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("grpc tls: load cert %s: %w", certPath, err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("grpc TLS enabled", "cert", certPath)
	} else if keyPath != "" {
		return nil, fmt.Errorf("grpc tls: both GRPC_TLS_CERT_PATH and GRPC_TLS_KEY_PATH must be set together")
	}

	gs := grpc.NewServer(opts...)

	// Register worker service.
	svc := &workerService{
		queries:         s.queries,
		pub:             s.pub,
		registry:        s.registry,
		cfg:             s.cfg,
		resultChannels:  s.resultChannels,
		runFinalizer:    &s.runFinalizer,
		authLimiter:     s.authLimiter,
		apiKeyResolver:  s.apiKeyResolver,
		billingEnforcer: s.billingEnforcer,
		edition:         s.edition,
		readyRunQueue:   s.readyRunQueue,
	}
	workerv1.RegisterWorkerServiceServer(gs, svc)

	// Register standard health service.
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("strait.worker.v1.WorkerService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(gs, hs)

	return gs, nil
}

func (s *Server) sentryMetadata() grpcSentryMetadata {
	meta := grpcSentryMetadata{
		edition: string(domain.BuildEdition()),
		version: s.version,
	}
	if s.cfg != nil {
		meta.mode = s.cfg.Mode
		meta.region = s.cfg.DefaultRegion
	}
	return meta
}

// Serve starts the gRPC listener and blocks until ctx is cancelled or an error occurs.
// It also starts the DB sync and sweep background loops.
func (s *Server) Serve(ctx context.Context) error {
	if !s.cfg.GRPCEnabled {
		slog.Info("grpc server disabled via GRPC_ENABLED=false")
		<-ctx.Done()
		return nil
	}

	if err := validateGRPCPlaintextExposure(s.cfg); err != nil {
		return err
	}

	addr := grpcListenAddress(s.cfg)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	slog.Info("grpc server listening", "addr", addr)

	var bgWG conc.WaitGroup
	bgWG.Go(func() { runDBSync(ctx, s.registry, s.queries, s.cfg.WorkerDBSyncInterval) })
	bgWG.Go(func() {
		runSweep(ctx, s.registry, s.queries, s.cfg.WorkerHeartbeatTimeout, s.cfg.WorkerDisconnectSweepInterval, s.runResultFinalizer, s.readyRunQueue)
	})

	var shutdownWG conc.WaitGroup
	shutdownWG.Go(func() {
		<-ctx.Done()
		slog.Info("grpc server shutting down")
		stopped := make(chan struct{})
		var stopWG conc.WaitGroup
		stopWG.Go(func() {
			s.gs.GracefulStop()
			close(stopped)
		})
		select {
		case <-stopped:
			slog.Info("grpc server stopped gracefully")
		case <-time.After(30 * time.Second):
			slog.Warn("grpc graceful stop timed out; forcing stop")
			s.gs.Stop()
		}
	})

	if err := s.gs.Serve(ln); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

func (s *Server) runResultFinalizer() WorkerRunResultFinalizer {
	v := s.runFinalizer.Load()
	if v == nil {
		return nil
	}
	finalizer, _ := v.(WorkerRunResultFinalizer)
	return finalizer
}

// GracefulStop stops the gRPC server gracefully.
func (s *Server) GracefulStop() {
	s.gs.GracefulStop()
}

func grpcListenAddress(cfg *config.Config) string {
	host := grpcBindAddr(cfg)
	return net.JoinHostPort(host, strconv.Itoa(cfg.GRPCPort))
}

func grpcBindAddr(cfg *config.Config) string {
	if cfg == nil || cfg.GRPCBindAddr == "" {
		return "127.0.0.1"
	}
	return cfg.GRPCBindAddr
}

func validateGRPCPlaintextExposure(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("grpc config is required")
	}
	if cfg.GRPCTLSCertPath != "" && cfg.GRPCTLSKeyPath != "" {
		return nil
	}
	if cfg.GRPCAllowPlaintext {
		return nil
	}
	if isLoopbackBindAddr(grpcBindAddr(cfg)) {
		return nil
	}
	return fmt.Errorf("grpc plaintext listener refused on non-loopback bind address %q; configure GRPC_TLS_CERT_PATH/GRPC_TLS_KEY_PATH or set GRPC_ALLOW_PLAINTEXT=true", grpcBindAddr(cfg))
}

func isLoopbackBindAddr(addr string) bool {
	host := strings.Trim(strings.TrimSpace(addr), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
