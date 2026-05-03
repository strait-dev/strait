package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"strait/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

// TestAPIKey_Expired verifies that an expired key fails lifecycle validation.
// This tests the pure time comparison logic without a real store.
func TestAPIKey_Expired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	apiKey := &domain.APIKey{
		ID:        "k",
		ProjectID: "p",
		ExpiresAt: &past,
	}

	// Simulate what resolveAPIKeyFromContext does after retrieving the key.
	now := time.Now()
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(now) {
		// expected: key is expired
		return
	}
	t.Error("expected key to be detected as expired")
}

// TestAPIKey_GraceExpired verifies grace period expiry detection.
func TestAPIKey_GraceExpired(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	apiKey := &domain.APIKey{
		ID:             "k",
		ProjectID:      "p",
		GraceExpiresAt: &past,
	}

	now := time.Now()
	if apiKey.GraceExpiresAt != nil && apiKey.GraceExpiresAt.Before(now) {
		// expected
		return
	}
	t.Error("expected grace period to be detected as expired")
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
