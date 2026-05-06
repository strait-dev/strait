//go:build loadtest

package loadtest

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGRPCTransportCredentials_DefaultsTLSForRemote(t *testing.T) {
	t.Parallel()

	creds := grpcTransportCredentials(WorkerConfig{GRPCAddr: "workers.example.com:50051"})
	if got := creds.Info().SecurityProtocol; got != "tls" {
		t.Fatalf("SecurityProtocol = %q, want tls", got)
	}
}

func TestGRPCTransportCredentials_AllowsPlaintextOnlyForLoopbackOrOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  WorkerConfig
		want string
	}{
		{name: "localhost", cfg: WorkerConfig{GRPCAddr: "localhost:50051"}, want: "insecure"},
		{name: "ipv4 loopback", cfg: WorkerConfig{GRPCAddr: "127.0.0.1:50051"}, want: "insecure"},
		{name: "ipv6 loopback", cfg: WorkerConfig{GRPCAddr: "[::1]:50051"}, want: "insecure"},
		{name: "explicit override", cfg: WorkerConfig{GRPCAddr: "10.0.0.10:50051", GRPCPlaintext: true}, want: "insecure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := grpcTransportCredentials(tt.cfg)
			if got := creds.Info().SecurityProtocol; got != tt.want {
				t.Fatalf("SecurityProtocol = %q, want %s", got, tt.want)
			}
		})
	}
}

func TestTestServer_BindsLoopbackAndRequiresSignature(t *testing.T) {
	t.Parallel()

	const secret = "loadtest-secret-32-bytes-long"
	srv := NewTestServer(0, WithTestServerHMACSecret(secret))
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer srv.Close()

	if strings.HasPrefix(srv.Addr(), ":") || strings.HasPrefix(srv.Addr(), "0.0.0.0") || strings.HasPrefix(srv.Addr(), "[::]") {
		t.Fatalf("server addr = %q, want loopback bind", srv.Addr())
	}

	client := &http.Client{Timeout: 5 * time.Second}
	unsignedReq, err := http.NewRequest(http.MethodPost, srv.URL("/fast-echo"), strings.NewReader(`{"unsigned":true}`))
	if err != nil {
		t.Fatalf("unsigned request: %v", err)
	}
	unsignedReq.Header.Set("Content-Type", "application/json")
	unsignedResp, err := client.Do(unsignedReq)
	if err != nil {
		t.Fatalf("unsigned request failed: %v", err)
	}
	_ = unsignedResp.Body.Close()
	if unsignedResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned status = %d, want 401", unsignedResp.StatusCode)
	}

	body := []byte(`{"signed":true}`)
	ts, sig := SignStraitDispatch(secret, body)
	signedReq, err := http.NewRequest(http.MethodPost, srv.URL("/fast-echo"), strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("signed request: %v", err)
	}
	signedReq.Header.Set("Content-Type", "application/json")
	signedReq.Header.Set("X-Strait-Timestamp", ts)
	signedReq.Header.Set("X-Strait-Signature", sig)
	signedResp, err := client.Do(signedReq)
	if err != nil {
		t.Fatalf("signed request failed: %v", err)
	}
	_ = signedResp.Body.Close()
	if signedResp.StatusCode != http.StatusOK {
		t.Fatalf("signed status = %d, want 200", signedResp.StatusCode)
	}
}

func TestValidateLoadTestEndpointURLRejectsWildcardHosts(t *testing.T) {
	t.Parallel()

	tests := []string{
		"http://0.0.0.0:8080/fast-echo",
		"http://[::]:8080/fast-echo",
	}
	for _, endpointURL := range tests {
		t.Run(endpointURL, func(t *testing.T) {
			if err := validateLoadTestEndpointURL(endpointURL); err == nil {
				t.Fatal("expected wildcard endpoint URL to be rejected")
			}
		})
	}
}

func TestValidateLoadTestEndpointURLAllowsLoopbackAndRemoteHosts(t *testing.T) {
	t.Parallel()

	tests := []string{
		"http://127.0.0.1:8080/fast-echo",
		"http://localhost:8080/fast-echo",
		"https://loadtest-target.example.com/fast-echo",
	}
	for _, endpointURL := range tests {
		t.Run(endpointURL, func(t *testing.T) {
			if err := validateLoadTestEndpointURL(endpointURL); err != nil {
				t.Fatalf("expected endpoint URL to be allowed: %v", err)
			}
		})
	}
}
