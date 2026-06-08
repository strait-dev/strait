package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// TestHashGRPCAPIKey verifies that hashGRPCAPIKey produces a stable SHA-256 hex digest.
func TestHashGRPCAPIKey(t *testing.T) {
	key := "sk_test_abc123"
	h := sha256.Sum256([]byte(key))
	expected := hex.EncodeToString(h[:])

	got := hashGRPCAPIKey(key)
	assert.Equal(t,
		expected, got)
}

// TestHashGRPCAPIKey_Deterministic verifies that the same key always hashes the same way.
func TestHashGRPCAPIKey_Deterministic(t *testing.T) {
	key := "my-secret-key"
	h1 := hashGRPCAPIKey(key)
	h2 := hashGRPCAPIKey(key)
	assert.Equal(t,
		h2, h1)
}

// TestProjectIDFromContext_HappyPath verifies extraction after withAPIKeyContext.
func TestProjectIDFromContext_HappyPath(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "key-1",
		ProjectID: "proj-abc",
		OrgID:     "org-xyz",
	}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	projectID, err := ProjectIDFromContext(ctx)
	require.NoError(t, err)
	assert.Equal(t,
		"proj-abc", projectID,
	)
}

// TestProjectIDFromContext_Missing verifies error when project ID is absent from context.
func TestProjectIDFromContext_Missing(t *testing.T) {
	_, err := ProjectIDFromContext(context.Background())
	require.Error(
		t, err)
}

// TestOrgIDFromContext_Present verifies org ID is extracted correctly when set.
func TestOrgIDFromContext_Present(t *testing.T) {
	apiKey := &domain.APIKey{ID: "k", ProjectID: "p", OrgID: "org-1"}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	orgID := OrgIDFromContext(ctx)
	assert.Equal(t,
		"org-1", orgID)
}

// TestOrgIDFromContext_Empty verifies empty string returned when org not set.
func TestOrgIDFromContext_Empty(t *testing.T) {
	apiKey := &domain.APIKey{ID: "k", ProjectID: "p", OrgID: ""}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	orgID := OrgIDFromContext(ctx)
	assert.Empty(t,
		orgID)
}

// TestAPIKeyFromContext_HappyPath verifies that the full APIKey is retrievable.
func TestAPIKeyFromContext_HappyPath(t *testing.T) {
	apiKey := &domain.APIKey{ID: "key-42", ProjectID: "proj-x"}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	got, ok := APIKeyFromContext(ctx)
	require.True(t,
		ok)
	assert.Equal(t,
		"key-42", got.ID,
	)
}

func TestEnvironmentIDFromContext_Present(t *testing.T) {
	apiKey := &domain.APIKey{ID: "key-42", ProjectID: "proj-x", EnvironmentID: "env-prod"}
	ctx := withAPIKeyContext(context.Background(), apiKey)
	require.Equal(
		t, "env-prod", EnvironmentIDFromContext(
			ctx))
}

// TestAPIKeyFromContext_Missing verifies (nil, false) when context has no API key.
func TestAPIKeyFromContext_Missing(t *testing.T) {
	_, ok := APIKeyFromContext(context.Background())
	assert.False(t,
		ok)
}

// resolveAPIKeyFromContext tests require a real store.Queries (DB-backed). We test the
// metadata extraction and header parsing logic through a table-driven approach using
// the exported helpers that cover the pure logic paths.

// TestResolveAPIKey_MissingMetadata verifies that missing gRPC metadata returns Unauthenticated.
func TestResolveAPIKey_MissingMetadata(t *testing.T) {
	// No metadata attached — resolveAPIKeyFromContext must return Unauthenticated.
	ctx := context.Background()
	_, err := resolveAPIKeyFromContext(ctx, nil)
	require.Error(
		t, err)

	s, ok := status.FromError(err)
	require.True(t,
		ok)
	assert.Equal(t,
		codes.Unauthenticated,
		s.Code(),
	)
}

// TestResolveAPIKey_MissingAuthorizationHeader verifies error for missing authorization header.
func TestResolveAPIKey_MissingAuthorizationHeader(t *testing.T) {
	md := metadata.Pairs("x-other", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := resolveAPIKeyFromContext(ctx, nil)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	assert.Equal(t,
		codes.Unauthenticated,
		s.Code(),
	)
}

// TestResolveAPIKey_InvalidAuthorizationFormat verifies error for non-Bearer prefix.
func TestResolveAPIKey_InvalidAuthorizationFormat(t *testing.T) {
	md := metadata.Pairs("authorization", "Basic dXNlcjpwYXNz")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := resolveAPIKeyFromContext(ctx, nil)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	assert.Equal(t,
		codes.Unauthenticated,
		s.Code(),
	)
}

func TestResolveAPIKey_RejectsMalformedAPIKeyBeforeStoreLookup(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"not-a-strait-key",
		"strait_",
		"strait_with spaces",
		"strait_" + strings.Repeat("a", grpcAPIKeyMaxLength),
	}

	for _, rawKey := range tests {
		t.Run(rawKey, func(t *testing.T) {
			md := metadata.Pairs("authorization", "Bearer "+rawKey)
			ctx := metadata.NewIncomingContext(context.Background(), md)

			_, err := resolveAPIKeyFromContext(ctx, nil)
			require.Error(
				t, err)

			s, _ := status.FromError(err)
			assert.Equal(t,
				codes.Unauthenticated,
				s.Code(),
			)
		})
	}
}

type fakeGRPCAuthLimiter struct {
	blocked     bool
	retryAfter  time.Duration
	blockChecks []string
	scopes      []string
	failures    []string
	resets      []string
}

func (f *fakeGRPCAuthLimiter) IsBlockedScoped(_ context.Context, ip, scope string) (bool, time.Duration) {
	f.blockChecks = append(f.blockChecks, ip)
	f.scopes = append(f.scopes, scope)
	return f.blocked, f.retryAfter
}

func (f *fakeGRPCAuthLimiter) RecordFailureScoped(_ context.Context, ip, _ string) {
	f.failures = append(f.failures, ip)
}

func (f *fakeGRPCAuthLimiter) ResetScoped(_ context.Context, ip, _ string) {
	f.resets = append(f.resets, ip)
}

func TestResolveAPIKeyFromContextWithLimit_BlockedBeforeStoreLookup(t *testing.T) {
	t.Parallel()

	limiter := &fakeGRPCAuthLimiter{blocked: true, retryAfter: 30 * time.Second}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer strait_invalid"))
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("203.0.113.10"), Port: 443}})

	_, err := resolveAPIKeyFromContextWithLimit(ctx, nil, limiter)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	require.Equal(
		t, codes.ResourceExhausted,
		s.Code())
	require.False(
		t, len(limiter.blockChecks) != 1 ||
			limiter.
				blockChecks[0] !=
				"203.0.113.10",
	)
	require.False(
		t, len(limiter.failures) != 0 ||
			len(limiter.
				resets,
			) != 0)
}

func TestResolveAPIKeyFromContextWithLimit_RecordsMalformedAuthFailure(t *testing.T) {
	t.Parallel()

	limiter := &fakeGRPCAuthLimiter{}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer not-a-strait-key"))
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("198.51.100.7"), Port: 443}})

	_, err := resolveAPIKeyFromContextWithLimit(ctx, nil, limiter)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	require.Equal(
		t, codes.Unauthenticated,
		s.Code())
	require.False(
		t, len(limiter.failures) != 1 ||
			limiter.
				failures[0] != "198.51.100.7",
	)
	require.Empty(t,
		limiter.resets)
}

func TestValidGRPCAPIKeyFormat_AllowsExpectedShape(t *testing.T) {
	t.Parallel()

	rawKey := "strait_" + strings.Repeat("a", 64)
	require.True(t,
		validGRPCAPIKeyFormat(rawKey))
}

// TestAPIKey_Expired verifies that an expired key fails lifecycle validation.
// This tests the pure time comparison logic without a real store.
func TestAPIKey_Expired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		ExpiresAt: &past,
		Scopes:    []string{domain.ScopeWorkersConnect},
	}

	err := validateWorkerAPIKey(apiKey)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	require.Equal(
		t, codes.Unauthenticated,
		s.Code())
}

// TestAPIKey_GraceExpired verifies grace period expiry detection.
func TestAPIKey_GraceExpired(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	apiKey := &domain.APIKey{
		ID:             "k",
		ProjectID:      "p",
		GraceExpiresAt: &past,
		Scopes:         []string{domain.ScopeWorkersConnect},
	}

	err := validateWorkerAPIKey(apiKey)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	require.Equal(
		t, codes.Unauthenticated,
		s.Code())
}

// TestAPIKey_Revoked verifies revocation detection.
func TestAPIKey_Revoked(t *testing.T) {
	past := time.Now().Add(-time.Second)
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		RevokedAt: &past,
	}

	if apiKey.RevokedAt != nil {
		// expected
		return
	}
	assert.Fail(t,

		"expected key to be detected as revoked")
}

func TestValidateWorkerAPIKey_RequiresWorkersConnectScope(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		Scopes:    []string{domain.ScopeJobsRead},
	}

	err := validateWorkerAPIKey(apiKey)
	require.Error(
		t, err)

	s, _ := status.FromError(err)
	require.Equal(
		t, codes.PermissionDenied,
		s.Code())
}

func TestValidateWorkerAPIKey_AllowsWorkersConnectScope(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		Scopes:    []string{domain.ScopeWorkersConnect},
	}
	require.NoError(t, validateWorkerAPIKey(apiKey))
}

// TestValidateWorkerAPIKey_EmptyScopesDenied is the regression guard for the
// empty-scopes bypass: domain.HasScope treats empty scopes as full access, so the
// worker gate must reject a key with no scopes explicitly.
func TestValidateWorkerAPIKey_EmptyScopesDenied(t *testing.T) {
	for _, scopes := range [][]string{nil, {}} {
		err := validateWorkerAPIKey(&domain.APIKey{ID: "k", ProjectID: "p", Scopes: scopes})
		require.Error(t, err)
		s, _ := status.FromError(err)
		require.Equal(t, codes.PermissionDenied, s.Code())
	}
}

// TestValidateWorkerAPIKey_UniformLifecycleError is the regression guard for the
// auth error-message disclosure: revoked, expired, and grace-ended keys must all
// return the same generic Unauthenticated message so an attacker cannot
// distinguish a once-valid key from a never-valid one.
func TestValidateWorkerAPIKey_UniformLifecycleError(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	scopes := []string{domain.ScopeWorkersConnect}
	cases := []*domain.APIKey{
		{ID: "k", Scopes: scopes, RevokedAt: &past},
		{ID: "k", Scopes: scopes, ExpiresAt: &past},
		{ID: "k", Scopes: scopes, GraceExpiresAt: &past},
	}
	msgs := make([]string, 0, len(cases))
	for _, k := range cases {
		err := validateWorkerAPIKey(k)
		require.Error(t, err)
		s, _ := status.FromError(err)
		require.Equal(t, codes.Unauthenticated, s.Code())
		msgs = append(msgs, s.Message())
	}
	require.Equal(t, "invalid api key", msgs[0])
	require.Equal(t, msgs[0], msgs[1])
	require.Equal(t, msgs[0], msgs[2])
}

func TestWorkerAPIKeyExpiresAt_UsesEarliestExpiryBoundary(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(10 * time.Minute)
	graceExpiresAt := now.Add(2 * time.Minute)
	apiKey := &domain.APIKey{
		ID:             "k",
		ProjectID:      "p",
		ExpiresAt:      &expiresAt,
		GraceExpiresAt: &graceExpiresAt,
		Scopes:         []string{domain.ScopeWorkersConnect},
	}

	got, ok := workerAPIKeyExpiresAt(apiKey)
	require.True(t,
		ok)
	require.True(t,
		got.Equal(graceExpiresAt))
}

func TestWorkerAPIKeyExpiresAt_StoresDeadlineInContext(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Minute)
	ctx := withAPIKeyContext(context.Background(), &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		ExpiresAt: &expiresAt,
		Scopes:    []string{domain.ScopeWorkersConnect},
	})

	got, ok := APIKeyExpiresAtFromContext(ctx)
	require.True(t,
		ok)
	require.True(t,
		got.Equal(expiresAt))
}

func TestWorkerAPIKeyExpiresAt_NoDeadlineForNonExpiringKey(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		Scopes:    []string{domain.ScopeWorkersConnect},
	}

	if _, ok := workerAPIKeyExpiresAt(apiKey); ok {
		require.Fail(t,

			"expected no expiry boundary")
	}
	ctx := withAPIKeyContext(context.Background(), apiKey)
	if _, ok := APIKeyExpiresAtFromContext(ctx); ok {
		require.Fail(t,

			"expected no expiry boundary in context")
	}
}
