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
	cfg      *config.Config
	queries  *store.Queries
	pub      pubsub.Publisher
	registry *ConnectionRegistry
	gs       *grpc.Server
}

// NewServer creates a new gRPC Server. It does not start listening.
func NewServer(cfg *config.Config, queries *store.Queries, pub pubsub.Publisher) *Server {
	s := &Server{
		cfg:      cfg,
		queries:  queries,
		pub:      pub,
		registry: NewConnectionRegistry(),
	}
	s.gs = s.buildServer()
	return s
}

// Registry returns the connection registry for external use (e.g. dispatcher).
func (s *Server) Registry() *ConnectionRegistry {
	return s.registry
}

func (s *Server) buildServer() *grpc.Server {
	kpParams := keepalive.ServerParameters{
		Time:    s.cfg.GRPCKeepaliveTime,
		Timeout: s.cfg.GRPCKeepaliveTimeout,
	}
	kpPolicy := keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}

	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(kpParams),
		grpc.KeepaliveEnforcementPolicy(kpPolicy),
		grpc.ChainUnaryInterceptor(unaryInterceptorChain()...),
		grpc.ChainStreamInterceptor(streamInterceptorChain()...),
	}

	if s.cfg.GRPCTLSCertPath != "" && s.cfg.GRPCTLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(s.cfg.GRPCTLSCertPath, s.cfg.GRPCTLSKeyPath)
		if err != nil {
			slog.Warn("grpc tls cert load failed; falling back to plaintext", "error", err)
		} else {
			tlsCfg := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
			opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
			slog.Info("grpc TLS enabled", "cert", s.cfg.GRPCTLSCertPath)
		}
	}

	gs := grpc.NewServer(opts...)

	// Register worker service.
	svc := &workerService{
		queries:  s.queries,
		pub:      s.pub,
		registry: s.registry,
		cfg:      s.cfg,
	}
	workerv1.RegisterWorkerServiceServer(gs, svc)

	// Register standard health service.
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("strait.worker.v1.WorkerService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(gs, hs)

	return gs
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
