//go:build loadtest

package loadtest

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGRPCTransportCredentials_DefaultsTLSForRemote(t *testing.T) {
	t.Parallel()

	creds := grpcTransportCredentials(WorkerConfig{GRPCAddr: "workers.example.com:50051"})
	require.Equal(t, "tls", creds.Info().SecurityProtocol)
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
			require.Equal(t, tt.want, creds.Info().SecurityProtocol)
		})
	}
}

func TestTestServer_BindsLoopbackAndRequiresSignature(t *testing.T) {
	t.Parallel()

	const secret = "loadtest-secret-32-bytes-long"
	srv := NewTestServer(0, WithTestServerHMACSecret(secret))
	require.NoError(t,

		srv.Start())

	defer srv.Close()
	require.False(t, strings.HasPrefix(srv.
		Addr(), ":",
	) || strings.HasPrefix(srv.Addr(),
		"0.0.0.0") || strings.HasPrefix(srv.Addr(), "[::]"))

	client := &http.Client{Timeout: 5 * time.Second}
	unsignedReq, err := http.NewRequest(http.MethodPost, srv.URL("/fast-echo"), strings.NewReader(`{"unsigned":true}`))
	require.NoError(t,

		err)

	unsignedReq.Header.Set("Content-Type", "application/json")
	unsignedResp, err := client.Do(unsignedReq)
	require.NoError(t,

		err)

	_ = unsignedResp.Body.Close()
	require.Equal(t, http.
		StatusUnauthorized,

		unsignedResp.
			StatusCode,
	)

	body := []byte(`{"signed":true}`)
	ts, sig := SignStraitDispatch(secret, body)
	signedReq, err := http.NewRequest(http.MethodPost, srv.URL("/fast-echo"), strings.NewReader(string(body)))
	require.NoError(t,

		err)

	signedReq.Header.Set("Content-Type", "application/json")
	signedReq.Header.Set("X-Strait-Timestamp", ts)
	signedReq.Header.Set("X-Strait-Signature", sig)
	signedResp, err := client.Do(signedReq)
	require.NoError(t,

		err)

	_ = signedResp.Body.Close()
	require.Equal(t, http.
		StatusOK,
		signedResp.
			StatusCode,
	)

}

func TestGenerateLoadTestSecretPanicsWhenCryptoRandomFails(t *testing.T) {
	orig := loadtestRandRead
	loadtestRandRead = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	t.Cleanup(func() {
		loadtestRandRead = orig
	})

	defer func() {
		require.NotNil(t, recover())
	}()
	_ = generateLoadTestSecret()
}

func TestGenerateLoadTestSecretUsesCryptoRandomLength(t *testing.T) {
	secret := generateLoadTestSecret()
	require.True(t, strings.HasPrefix(secret,
		"loadtest_",
	))
	require.Len(t, secret,

		len("loadtest_")+64)

}

func TestValidateLoadTestEndpointURLRejectsWildcardHosts(t *testing.T) {
	t.Parallel()

	tests := []string{
		"http://0.0.0.0:8080/fast-echo",
		"http://[::]:8080/fast-echo",
	}
	for _, endpointURL := range tests {
		t.Run(endpointURL, func(t *testing.T) {
			require.Error(t, validateLoadTestEndpointURL(endpointURL))

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
			require.NoError(t,

				validateLoadTestEndpointURL(endpointURL))

		})
	}
}
