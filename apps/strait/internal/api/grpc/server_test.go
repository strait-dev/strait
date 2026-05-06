package grpc

import (
	"strings"
	"testing"
	"time"

	"strait/internal/config"
)

// TestNewServer_NilPubRejected pins the precondition that NewServer must
// refuse a nil publisher. The worker stream relies on pubsub for cross-
// replica force-disconnect, API-key revocation broadcast, and run-result
// fan-out — booting without it would panic on the first connect, far away
// from any signal a startup-time misconfiguration is the cause.
func TestNewServer_NilPubRejected(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(testConfig(), nil, nil)
	if err == nil {
		t.Fatal("expected error when pub is nil")
	}
	if srv != nil {
		t.Fatal("expected nil server when pub is nil")
	}
	if !strings.Contains(err.Error(), "pubsub") {
		t.Fatalf("error message should mention pubsub, got: %v", err)
	}
}

// testConfig returns a minimal config suitable for unit tests.
func testConfig() *config.Config {
	return &config.Config{
		GRPCEnabled:          true,
		GRPCPort:             0,
		GRPCKeepaliveTime:    30 * time.Second,
		GRPCKeepaliveTimeout: 10 * time.Second,
	}
}

// TestBuildServer_Plaintext verifies that an empty TLS config produces a plaintext server.
func TestBuildServer_Plaintext(t *testing.T) {
	s := &Server{
		cfg:            testConfig(),
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	gs, err := s.buildServer()
	if err != nil {
		t.Fatalf("expected no error for plaintext config, got: %v", err)
	}
	if gs == nil {
		t.Fatal("expected non-nil gRPC server")
	}
	gs.Stop()
}

func TestGRPCListenAddress_DefaultsToLoopback(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCPort = 50051

	if got := grpcListenAddress(cfg); got != "127.0.0.1:50051" {
		t.Fatalf("grpcListenAddress = %q, want 127.0.0.1:50051", got)
	}
}

func TestValidateGRPCPlaintextExposure_BlocksPublicBind(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"

	if err := validateGRPCPlaintextExposure(cfg); err == nil {
		t.Fatal("expected public plaintext bind to be rejected")
	}
}

func TestValidateGRPCPlaintextExposure_AllowsPublicBindWithExplicitOverride(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"
	cfg.GRPCAllowPlaintext = true

	if err := validateGRPCPlaintextExposure(cfg); err != nil {
		t.Fatalf("expected explicit plaintext override to pass, got %v", err)
	}
}

func TestValidateGRPCPlaintextExposure_AllowsLoopback(t *testing.T) {
	t.Parallel()

	tests := []string{"127.0.0.1", "::1", "localhost"}
	for _, bind := range tests {
		t.Run(bind, func(t *testing.T) {
			cfg := testConfig()
			cfg.GRPCBindAddr = bind
			if err := validateGRPCPlaintextExposure(cfg); err != nil {
				t.Fatalf("loopback bind %q rejected: %v", bind, err)
			}
		})
	}
}

func TestValidateGRPCPlaintextExposure_AllowsTLSOnPublicBind(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"
	cfg.GRPCTLSCertPath = "/tmp/cert.pem"
	cfg.GRPCTLSKeyPath = "/tmp/key.pem"

	if err := validateGRPCPlaintextExposure(cfg); err != nil {
		t.Fatalf("expected TLS-configured public bind to pass, got %v", err)
	}
}

// TestBuildServer_OnlyCertPath_Error verifies that setting only cert path (not key) returns error.
func TestBuildServer_OnlyCertPath_Error(t *testing.T) {
	cfg := testConfig()
	cfg.GRPCTLSCertPath = "/tmp/cert.pem"
	cfg.GRPCTLSKeyPath = ""

	s := &Server{
		cfg:            cfg,
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	_, err := s.buildServer()
	if err == nil {
		t.Fatal("expected error when only cert path is set, got nil")
	}
}

// TestBuildServer_OnlyKeyPath_Error verifies that setting only key path (not cert) returns error.
func TestBuildServer_OnlyKeyPath_Error(t *testing.T) {
	cfg := testConfig()
	cfg.GRPCTLSCertPath = ""
	cfg.GRPCTLSKeyPath = "/tmp/key.pem"

	s := &Server{
		cfg:            cfg,
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	_, err := s.buildServer()
	if err == nil {
		t.Fatal("expected error when only key path is set, got nil")
	}
}

// TestBuildServer_BothPathsSet_BadCert verifies that invalid cert/key files return an error.
func TestBuildServer_BothPathsSet_BadCert(t *testing.T) {
	cfg := testConfig()
	cfg.GRPCTLSCertPath = "/tmp/does-not-exist-cert.pem"
	cfg.GRPCTLSKeyPath = "/tmp/does-not-exist-key.pem"

	s := &Server{
		cfg:            cfg,
		registry:       NewConnectionRegistry(),
		resultChannels: NewResultChannelRegistry(),
	}

	_, err := s.buildServer()
	if err == nil {
		t.Fatal("expected error for nonexistent TLS cert/key files")
	}
}

// TestNewServer_Plaintext verifies NewServer succeeds with plaintext config.
func TestNewServer_Plaintext(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	srv.GracefulStop()
}

// TestServer_Registry verifies Registry() returns the internal registry.
func TestServer_Registry(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer srv.GracefulStop()

	reg := srv.Registry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

// TestServer_WorkerDispatcher verifies WorkerDispatcher() returns a non-nil dispatcher.
func TestServer_WorkerDispatcher(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer srv.GracefulStop()

	d := srv.WorkerDispatcher("test-jwt-key")
	if d == nil {
		t.Fatal("expected non-nil WorkerDispatcher")
	}
}
