package scheduler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stubSecretDecryptor struct {
	plaintext []byte
	err       error
}

func (s stubSecretDecryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.plaintext, nil
}

func TestNotifyRotationWebhook_SignsWithHMACWhenSecretPresent(t *testing.T) {
	t.Parallel()

	plaintext := []byte("rotation-secret")
	signingSecret := []byte("whsec_" + hex.EncodeToString(plaintext))
	var (
		mu          sync.Mutex
		gotBody     []byte
		gotSig      string
		gotTS       string
		gotDelivery string
		gotSig256   string
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody = body
		gotSig = r.Header.Get("X-Strait-Signature")
		gotTS = r.Header.Get("X-Strait-Timestamp")
		gotDelivery = r.Header.Get("X-Strait-Delivery-ID")
		gotSig256 = r.Header.Get("X-Strait-Signature-256")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).
		WithAllowPrivateEndpoints(true).
		WithRotationSecretDecryptor(stubSecretDecryptor{plaintext: plaintext})
	r.rotationWebhookClient = server.Client()
	require.NoError(t,
		r.notifyRotationWebhook(
			context.Background(), server.URL,
			[]byte("ciphertext"), "old-key",
			"new-key", "strait_secret",

			"strait_secre",
			"proj-1",
		))

	mu.Lock()
	defer mu.Unlock()
	require.False(t,
		gotTS == "" ||
			gotDelivery ==
				"")
	require.NotEmpty(
		t, gotSig,
	)
	require.True(t, strings.HasPrefix(gotSig, "v1="))

	// X-Strait-Signature is v1=<hex> over timestamp + "." + body, the shared
	// scheme across all signing paths (delivery id is a header, not in the HMAC).
	mac := hmac.New(sha256.New, signingSecret)
	mac.Write([]byte(gotTS))
	mac.Write([]byte("."))
	mac.Write(gotBody)
	wantSig := "v1=" + hex.EncodeToString(mac.Sum(nil))
	require.Equal(t,
		wantSig,
		gotSig)

	// X-Strait-Signature-256 is the GitHub-style body-only HMAC.
	bodyMac := hmac.New(sha256.New, signingSecret)
	bodyMac.Write(gotBody)
	require.Equal(t,
		"sha256="+hex.EncodeToString(bodyMac.Sum(nil)), gotSig256,
	)
	require.True(t, bytes.
		Contains(gotBody, []byte(`"event":"api_key.auto_rotated"`)))
}

func TestNotifyRotationWebhook_DoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var (
		mu             sync.Mutex
		redirectCalled bool
		targetCalled   bool
	)
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		targetCalled = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		redirectCalled = true
		mu.Unlock()
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).
		WithAllowPrivateEndpoints(true).
		WithRotationSecretDecryptor(stubSecretDecryptor{plaintext: []byte("rotation-secret")})
	r.rotationWebhookClient = redirector.Client()

	err := r.notifyRotationWebhook(context.Background(), redirector.URL, []byte("ciphertext"), "old-key", "new-key", "strait_secret", "strait_secre", "proj-1")
	require.Error(t,
		err)

	mu.Lock()
	defer mu.Unlock()
	require.True(t, redirectCalled)
	require.False(t,
		targetCalled,
	)
}

func TestNotifyRotationWebhook_MissingSecretFailsClosed(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).
		WithAllowPrivateEndpoints(true).
		WithRotationSecretDecryptor(stubSecretDecryptor{plaintext: []byte("unused")})
	r.rotationWebhookClient = server.Client()
	require.Error(t,
		r.notifyRotationWebhook(context.
			Background(), server.URL, nil,
			"old-key",
			"new-key",
			"strait_secret",
			"strait_secre",
			"proj-1",
		),
	)
	require.False(t,
		called.Load())
}

func TestNotifyRotationWebhook_DecryptFailureFailsClosed(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).
		WithAllowPrivateEndpoints(true).
		WithRotationSecretDecryptor(stubSecretDecryptor{err: errors.New("kms unavailable")})
	r.rotationWebhookClient = server.Client()
	require.Error(t,
		r.notifyRotationWebhook(context.
			Background(), server.URL, []byte("ciphertext"), "old-key",
			"new-key", "strait_secret",

			"strait_secre",
			"proj-1",
		))
	require.False(t,
		called.Load())
}

func TestNotifyRotationWebhook_NoDecryptorFailsClosed(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).
		WithAllowPrivateEndpoints(true)
	r.rotationWebhookClient = server.Client()
	require.Error(t,
		r.notifyRotationWebhook(context.
			Background(), server.URL, []byte("ciphertext"), "old-key",
			"new-key", "strait_secret",

			"strait_secre",
			"proj-1",
		))
	require.False(t,
		called.Load())
}

func TestRotationWebhookSigningSecretRejectsMissingInputs(t *testing.T) {
	t.Parallel()

	t.Run("missing encrypted secret", func(t *testing.T) {
		t.Parallel()

		r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil).
			WithRotationSecretDecryptor(stubSecretDecryptor{plaintext: []byte("unused")})

		secret, err := r.rotationWebhookSigningSecret(nil, "key-1", "project-1")

		require.Nil(t, secret)
		require.ErrorContains(t, err, "has no rotation webhook signing secret")
	})

	t.Run("missing decryptor", func(t *testing.T) {
		t.Parallel()

		r := NewReaper(&mockReaperStore{}, time.Second, 30*time.Second, 0, 0, false, nil)

		secret, err := r.rotationWebhookSigningSecret([]byte("ciphertext"), "key-1", "project-1")

		require.Nil(t, secret)
		require.ErrorContains(t, err, "decryptor is not configured")
	})
}
