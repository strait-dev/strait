package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

type fakeServerSecretDecryptor struct{}

func (fakeServerSecretDecryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}

type fakeServerPlanEnforcer struct{}

func (*fakeServerPlanEnforcer) CheckWorkerConnectionLimit(_ context.Context, _ string, _ int) error {
	return nil
}

func (*fakeServerPlanEnforcer) GetActiveProjectOrgID(_ context.Context, projectID string) (string, error) {
	return "org-" + projectID, nil
}

type fakeServerReadyRunEnqueuer struct{}

func (*fakeServerReadyRunEnqueuer) EnqueueExisting(_ context.Context, _ *domain.JobRun) error {
	return nil
}

type fakeServerRunFinalizer struct{}

func (*fakeServerRunFinalizer) FinalizeWorkerRunResult(_ context.Context, _, _, _ string, _ json.RawMessage) (domain.WorkerTaskStatus, error) {
	return domain.WorkerTaskStatusCompleted, nil
}

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
	require.Contains(t,
		err.
			Error(), "pubsub")
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
	require.Contains(t, err.Error(), "both GRPC_TLS_CERT_PATH and GRPC_TLS_KEY_PATH")
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
	require.Contains(t, err.Error(), "both GRPC_TLS_CERT_PATH and GRPC_TLS_KEY_PATH")
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
	require.Contains(t, err.Error(), "grpc tls: load cert")
}

// TestNewServer_Plaintext verifies NewServer succeeds with plaintext config.
func TestNewServer_Plaintext(t *testing.T) {
	cfg := testConfig()
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)
	require.NotNil(t, srv)

	srv.GracefulStop()
}

func TestNewServer_AppliesOptionsAndSkipsNilOption(t *testing.T) {
	cfg := testConfig()
	limiter := &fakeGRPCAuthLimiter{}
	decryptor := fakeServerSecretDecryptor{}
	enforcer := &fakeServerPlanEnforcer{}
	enqueuer := &fakeServerReadyRunEnqueuer{}

	srv, err := NewServer(
		cfg,
		nil,
		noopPub{},
		nil,
		WithAuthLimiter(limiter),
		WithVersion("test-version"),
		WithSecretDecryptor(decryptor),
		WithBillingEnforcer(enforcer),
		WithReadyRunEnqueuer(enqueuer),
	)
	require.NoError(t, err)
	require.NotNil(t, srv)
	defer srv.GracefulStop()

	require.Same(t, limiter, srv.authLimiter)
	require.Equal(t, "test-version", srv.version)
	require.Equal(t, decryptor, srv.secretDecryptor)
	require.Same(t, enforcer, srv.billingEnforcer)
	require.Same(t, enqueuer, srv.readyRunQueue)
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

func TestServer_RunResultFinalizer(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	require.Nil(t, srv.runResultFinalizer())

	srv.SetRunResultFinalizer(nil)
	require.Nil(t, srv.runResultFinalizer())

	finalizer := &fakeServerRunFinalizer{}
	srv.SetRunResultFinalizer(finalizer)
	require.Same(t, finalizer, srv.runResultFinalizer())
}

func TestServer_SentryMetadata(t *testing.T) {
	t.Parallel()

	t.Run("without config", func(t *testing.T) {
		t.Parallel()

		meta := (&Server{version: "test-version"}).sentryMetadata()
		require.Equal(t, string(domain.BuildEdition()), meta.edition)
		require.Equal(t, "test-version", meta.version)
		require.Empty(t, meta.mode)
		require.Empty(t, meta.region)
	})

	t.Run("with config", func(t *testing.T) {
		t.Parallel()

		cfg := testConfig()
		cfg.Mode = "all"
		cfg.DefaultRegion = "test-region"

		meta := (&Server{cfg: cfg, version: "test-version"}).sentryMetadata()
		require.Equal(t, string(domain.BuildEdition()), meta.edition)
		require.Equal(t, "test-version", meta.version)
		require.Equal(t, "all", meta.mode)
		require.Equal(t, "test-region", meta.region)
	})
}

func TestServer_ServeDisabledWaitsForContext(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCEnabled = false
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)
	defer srv.GracefulStop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, srv.Serve(ctx))
}

func TestServer_ServeRejectsPublicPlaintext(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "0.0.0.0"
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)
	defer srv.GracefulStop()

	err = srv.Serve(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "plaintext listener refused")
}

func TestServer_ServeReportsListenError(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCBindAddr = "256.256.256.256"
	cfg.GRPCAllowPlaintext = true
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)
	defer srv.GracefulStop()

	err = srv.Serve(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "grpc listen")
}

func TestServer_ServeReturnsNilAfterGracefulStop(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.GRPCAllowPlaintext = true
	cfg.WorkerDBSyncInterval = time.Hour
	cfg.WorkerDisconnectSweepInterval = time.Hour
	srv, err := NewServer(cfg, nil, noopPub{})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ctx)
	}()

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		srv.GracefulStop()
		t.Fatal("Serve did not return after context cancellation")
	}
}
