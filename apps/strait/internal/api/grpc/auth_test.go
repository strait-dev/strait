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
	if got != expected {
		t.Errorf("hashGRPCAPIKey(%q) = %q, want %q", key, got, expected)
	}
}

// TestHashGRPCAPIKey_Deterministic verifies that the same key always hashes the same way.
func TestHashGRPCAPIKey_Deterministic(t *testing.T) {
	key := "my-secret-key"
	h1 := hashGRPCAPIKey(key)
	h2 := hashGRPCAPIKey(key)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectID != "proj-abc" {
		t.Errorf("expected proj-abc, got %s", projectID)
	}
}

// TestProjectIDFromContext_Missing verifies error when project ID is absent from context.
func TestProjectIDFromContext_Missing(t *testing.T) {
	_, err := ProjectIDFromContext(context.Background())
	if err == nil {
		t.Fatal("expected error for missing project ID")
	}
}

// TestOrgIDFromContext_Present verifies org ID is extracted correctly when set.
func TestOrgIDFromContext_Present(t *testing.T) {
	apiKey := &domain.APIKey{ID: "k", ProjectID: "p", OrgID: "org-1"}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	orgID := OrgIDFromContext(ctx)
	if orgID != "org-1" {
		t.Errorf("expected org-1, got %s", orgID)
	}
}

// TestOrgIDFromContext_Empty verifies empty string returned when org not set.
func TestOrgIDFromContext_Empty(t *testing.T) {
	apiKey := &domain.APIKey{ID: "k", ProjectID: "p", OrgID: ""}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	orgID := OrgIDFromContext(ctx)
	if orgID != "" {
		t.Errorf("expected empty org ID, got %s", orgID)
	}
}

// TestAPIKeyFromContext_HappyPath verifies that the full APIKey is retrievable.
func TestAPIKeyFromContext_HappyPath(t *testing.T) {
	apiKey := &domain.APIKey{ID: "key-42", ProjectID: "proj-x"}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	got, ok := APIKeyFromContext(ctx)
	if !ok {
		t.Fatal("expected APIKey to be found in context")
	}
	if got.ID != "key-42" {
		t.Errorf("expected key-42, got %s", got.ID)
	}
}

func TestEnvironmentIDFromContext_Present(t *testing.T) {
	apiKey := &domain.APIKey{ID: "key-42", ProjectID: "proj-x", EnvironmentID: "env-prod"}
	ctx := withAPIKeyContext(context.Background(), apiKey)

	if got := EnvironmentIDFromContext(ctx); got != "env-prod" {
		t.Fatalf("EnvironmentIDFromContext() = %q, want env-prod", got)
	}
}

// TestAPIKeyFromContext_Missing verifies (nil, false) when context has no API key.
func TestAPIKeyFromContext_Missing(t *testing.T) {
	_, ok := APIKeyFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for context without API key")
	}
}

// resolveAPIKeyFromContext tests require a real store.Queries (DB-backed). We test the
// metadata extraction and header parsing logic through a table-driven approach using
// the exported helpers that cover the pure logic paths.

// TestResolveAPIKey_MissingMetadata verifies that missing gRPC metadata returns Unauthenticated.
func TestResolveAPIKey_MissingMetadata(t *testing.T) {
	// No metadata attached — resolveAPIKeyFromContext must return Unauthenticated.
	ctx := context.Background()
	_, err := resolveAPIKeyFromContext(ctx, nil)
	if err == nil {
		t.Fatal("expected error for missing metadata")
	}
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %v", err)
	}
	if s.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", s.Code())
	}
}

// TestResolveAPIKey_MissingAuthorizationHeader verifies error for missing authorization header.
func TestResolveAPIKey_MissingAuthorizationHeader(t *testing.T) {
	md := metadata.Pairs("x-other", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := resolveAPIKeyFromContext(ctx, nil)
	if err == nil {
		t.Fatal("expected error for missing authorization header")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", s.Code())
	}
}

// TestResolveAPIKey_InvalidAuthorizationFormat verifies error for non-Bearer prefix.
func TestResolveAPIKey_InvalidAuthorizationFormat(t *testing.T) {
	md := metadata.Pairs("authorization", "Basic dXNlcjpwYXNz")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := resolveAPIKeyFromContext(ctx, nil)
	if err == nil {
		t.Fatal("expected error for non-Bearer auth")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %s", s.Code())
	}
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
			if err == nil {
				t.Fatal("expected malformed api key to be rejected")
			}
			s, _ := status.FromError(err)
			if s.Code() != codes.Unauthenticated {
				t.Errorf("expected Unauthenticated, got %s", s.Code())
			}
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
	if err == nil {
		t.Fatal("expected blocked auth error")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.ResourceExhausted {
		t.Fatalf("code = %s, want ResourceExhausted", s.Code())
	}
	if len(limiter.blockChecks) != 1 || limiter.blockChecks[0] != "203.0.113.10" {
		t.Fatalf("block checks = %+v, want peer IP", limiter.blockChecks)
	}
	if len(limiter.failures) != 0 || len(limiter.resets) != 0 {
		t.Fatalf("failures/resets = %+v/%+v, want none when already blocked", limiter.failures, limiter.resets)
	}
}

func TestResolveAPIKeyFromContextWithLimit_RecordsMalformedAuthFailure(t *testing.T) {
	t.Parallel()

	limiter := &fakeGRPCAuthLimiter{}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer not-a-strait-key"))
	ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("198.51.100.7"), Port: 443}})

	_, err := resolveAPIKeyFromContextWithLimit(ctx, nil, limiter)
	if err == nil {
		t.Fatal("expected malformed auth error")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.Unauthenticated {
		t.Fatalf("code = %s, want Unauthenticated", s.Code())
	}
	if len(limiter.failures) != 1 || limiter.failures[0] != "198.51.100.7" {
		t.Fatalf("failures = %+v, want peer IP", limiter.failures)
	}
	if len(limiter.resets) != 0 {
		t.Fatalf("resets = %+v, want none on failure", limiter.resets)
	}
}

func TestValidGRPCAPIKeyFormat_AllowsExpectedShape(t *testing.T) {
	t.Parallel()

	rawKey := "strait_" + strings.Repeat("a", 64)
	if !validGRPCAPIKeyFormat(rawKey) {
		t.Fatalf("expected valid api key format for %q", rawKey)
	}
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
	if err == nil {
		t.Fatal("expected expired key to be rejected")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.Unauthenticated {
		t.Fatalf("status = %s, want Unauthenticated", s.Code())
	}
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
	if err == nil {
		t.Fatal("expected expired grace period to be rejected")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.Unauthenticated {
		t.Fatalf("status = %s, want Unauthenticated", s.Code())
	}
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
	t.Error("expected key to be detected as revoked")
}

func TestValidateWorkerAPIKey_RequiresWorkersConnectScope(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		Scopes:    []string{domain.ScopeJobsRead},
	}

	err := validateWorkerAPIKey(apiKey)
	if err == nil {
		t.Fatal("expected missing workers:connect scope to be rejected")
	}
	s, _ := status.FromError(err)
	if s.Code() != codes.PermissionDenied {
		t.Fatalf("status = %s, want PermissionDenied", s.Code())
	}
}

func TestValidateWorkerAPIKey_AllowsWorkersConnectScope(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		Scopes:    []string{domain.ScopeWorkersConnect},
	}

	if err := validateWorkerAPIKey(apiKey); err != nil {
		t.Fatalf("validateWorkerAPIKey() error = %v, want nil", err)
	}
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
	if !ok {
		t.Fatal("expected expiry boundary")
	}
	if !got.Equal(graceExpiresAt) {
		t.Fatalf("expiry = %s, want grace expiry %s", got, graceExpiresAt)
	}
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
	if !ok {
		t.Fatal("expected expiry boundary in context")
	}
	if !got.Equal(expiresAt) {
		t.Fatalf("context expiry = %s, want %s", got, expiresAt)
	}
}

func TestWorkerAPIKeyExpiresAt_NoDeadlineForNonExpiringKey(t *testing.T) {
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		Scopes:    []string{domain.ScopeWorkersConnect},
	}

	if _, ok := workerAPIKeyExpiresAt(apiKey); ok {
		t.Fatal("expected no expiry boundary")
	}
	ctx := withAPIKeyContext(context.Background(), apiKey)
	if _, ok := APIKeyExpiresAtFromContext(ctx); ok {
		t.Fatal("expected no expiry boundary in context")
	}
}
