// Package grpc provides the gRPC server for the Worker mode streaming API.
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	workerv1 "strait/internal/api/grpc/proto/workerv1"
	"strait/internal/config"
	"strait/internal/pubsub"
	"strait/internal/store"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

// Server wraps the gRPC server and all its dependencies.
type Server struct {
	cfg            *config.Config
	queries        *store.Queries
	pub            pubsub.Publisher
	registry       *ConnectionRegistry
	resultChannels *ResultChannelRegistry
	gs             *grpc.Server
}

// NewServer creates a new gRPC Server. It does not start listening.
// Returns an error if TLS is configured but cert/key cannot be loaded —
// silent fallback to plaintext would leak API keys in transit.
func NewServer(cfg *config.Config, queries *store.Queries, pub pubsub.Publisher) (*Server, error) {
	s := &Server{
		cfg:            cfg,
		queries:        queries,
		pub:            pub,
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
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
	return NewWorkerDispatcher(s.registry, s.queries, jwtSigningKey, s.resultChannels)
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
		grpc.ChainUnaryInterceptor(unaryInterceptorChain()...),
		grpc.ChainStreamInterceptor(streamInterceptorChain()...),
	}

	// TLS is mutually exclusive: either both paths are set and both must load,
	// or both are empty and the server runs plaintext (cloud terminates at the
	// LB). Partial config or load failure is a hard error so an operator who
	// configured TLS never silently runs cleartext.
	switch {
	case s.cfg.GRPCTLSCertPath != "" && s.cfg.GRPCTLSKeyPath != "":
		cert, err := tls.LoadX509KeyPair(s.cfg.GRPCTLSCertPath, s.cfg.GRPCTLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("grpc tls: load cert %s: %w", s.cfg.GRPCTLSCertPath, err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
		slog.Info("grpc TLS enabled", "cert", s.cfg.GRPCTLSCertPath)
	case s.cfg.GRPCTLSCertPath != "" || s.cfg.GRPCTLSKeyPath != "":
		return nil, fmt.Errorf("grpc tls: both GRPC_TLS_CERT_PATH and GRPC_TLS_KEY_PATH must be set together")
	}

	gs := grpc.NewServer(opts...)

	// Register worker service.
	svc := &workerService{
		queries:        s.queries,
		pub:            s.pub,
		registry:       s.registry,
		cfg:            s.cfg,
		resultChannels: s.resultChannels,
	}
	workerv1.RegisterWorkerServiceServer(gs, svc)

	// Register standard health service.
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("strait.worker.v1.WorkerService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(gs, hs)

	return gs, nil
}

// Serve starts the gRPC listener and blocks until ctx is cancelled or an error occurs.
// It also starts the DB sync and sweep background loops.
func (s *Server) Serve(ctx context.Context) error {
	if !s.cfg.GRPCEnabled {
		slog.Info("grpc server disabled via GRPC_ENABLED=false")
		<-ctx.Done()
		return nil
	}

	addr := fmt.Sprintf(":%d", s.cfg.GRPCPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	slog.Info("grpc server listening", "addr", addr)

	// Start background maintenance loops.
	go runDBSync(ctx, s.registry, s.queries, s.cfg.WorkerDBSyncInterval)
	go runSweep(ctx, s.registry, s.queries, s.cfg.WorkerHeartbeatTimeout, s.cfg.WorkerDisconnectSweepInterval)

	// Shutdown goroutine.
	go func() {
		<-ctx.Done()
		slog.Info("grpc server shutting down")
		stopped := make(chan struct{})
		go func() {
			s.gs.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
			slog.Info("grpc server stopped gracefully")
		case <-time.After(30 * time.Second):
			slog.Warn("grpc graceful stop timed out; forcing stop")
			s.gs.Stop()
		}
	}()

	if err := s.gs.Serve(ln); err != nil {
		return fmt.Errorf("grpc serve: %w", err)
	}
	return nil
}

// GracefulStop stops the gRPC server gracefully.
func (s *Server) GracefulStop() {
	s.gs.GracefulStop()
}
