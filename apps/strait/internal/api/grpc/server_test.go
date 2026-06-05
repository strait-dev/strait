package grpc

import (
	"strings"
	"testing"
	"time"

	"strait/internal/config"

	"github.com/stretchr/testify/require"
)

// TestNewServer_NilPubRejected pins the precondition that NewServer must
// refuse a nil publisher. The worker stream relies on pubsub for cross-
// replica force-disconnect, API-key revocation broadcast, and run-result
// fan-out — booting without it would panic on the first connect, far away
// from any signal a startup-time misconfiguration is the cause.
func TestNewServer_NilPubRejected(t *testing.T) {
	t.Parallel()
	srv, err := NewServer(testConfig(), nil, nil)
	require.Error(
		t, err)
	require.Nil(t, srv)
	require.True(t,
		strings.Contains(
			err.
				Error(), "pubsub"))

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
	require.NoError(t, err)
	require.NotNil(t, gs)

	gs.Stop()
}

func TestGRPCListenAddress_DefaultsToLoopback(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCPort = 50051
	require.Equal(
		t, "127.0.0.1:50051",

		grpcListenAddress(cfg))

}

func TestValidateGRPCPlaintextExposure_BlocksPublicBind(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"
	require.Error(
		t, validateGRPCPlaintextExposure(cfg))

}

func TestValidateGRPCPlaintextExposure_AllowsPublicBindWithExplicitOverride(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"
	cfg.GRPCAllowPlaintext = true
	require.NoError(t, validateGRPCPlaintextExposure(cfg))

}

func TestValidateGRPCPlaintextExposure_AllowsLoopback(t *testing.T) {
	t.Parallel()

	tests := []string{"127.0.0.1", "::1", "localhost"}
	for _, bind := range tests {
		t.Run(bind, func(t *testing.T) {
			cfg := testConfig()
			cfg.GRPCBindAddr = bind
			require.NoError(t, validateGRPCPlaintextExposure(cfg))

		})
	}
}

func TestValidateGRPCPlaintextExposure_AllowsTLSOnPublicBind(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"
	cfg.GRPCTLSCertPath = "/tmp/cert.pem"
	cfg.GRPCTLSKeyPath = "/tmp/key.pem"
	require.NoError(t, validateGRPCPlaintextExposure(cfg))

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
	require.Error(
		t, err)

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
	require.Error(
		t, err)

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
	require.Error(
		t, err)

}

// TestNewServer_Plaintext verifies NewServer succeeds with plaintext config.
func TestNewServer_Plaintext(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)
	require.NotNil(t, srv)

	srv.GracefulStop()
}

// TestServer_Registry verifies Registry() returns the internal registry.
func TestServer_Registry(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)

	defer srv.GracefulStop()

	reg := srv.Registry()
	require.NotNil(t, reg)

}

// TestServer_WorkerDispatcher verifies WorkerDispatcher() returns a non-nil dispatcher.
func TestServer_WorkerDispatcher(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)

	defer srv.GracefulStop()

	d := srv.WorkerDispatcher("test-jwt-key")
	require.NotNil(t, d)

}
