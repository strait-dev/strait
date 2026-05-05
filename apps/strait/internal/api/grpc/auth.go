package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcContextKey string

const (
	grpcCtxProjectIDKey     grpcContextKey = "grpc_project_id"
	grpcCtxOrgIDKey         grpcContextKey = "grpc_org_id"
	grpcCtxAPIKeyIDKey      grpcContextKey = "grpc_api_key_id" //nolint:gosec // not a credential; context-key name
	grpcCtxAPIKeyKey        grpcContextKey = "grpc_api_key"    //nolint:gosec // not a credential; context-key name
	grpcCtxEnvironmentIDKey grpcContextKey = "grpc_environment_id"
)

// resolveAPIKeyFromContext extracts the Bearer token from the gRPC metadata
// attached to ctx, resolves it against the API key store, validates its
// lifecycle (revoked, expired), and returns the key. On any failure it
// returns a gRPC status error suitable for returning directly from a handler.
func resolveAPIKeyFromContext(ctx context.Context, q *store.Queries) (*domain.APIKey, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := vals[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
	}

	rawKey := strings.TrimPrefix(authHeader, "Bearer ")
	keyHash := hashGRPCAPIKey(rawKey)

	apiKey, err := q.GetAPIKeyByHash(ctx, keyHash)
	if err != nil || apiKey == nil {
		return nil, status.Error(codes.Unauthenticated, "invalid api key")
	}

	if err := validateWorkerAPIKey(apiKey); err != nil {
		return nil, err
	}

	return apiKey, nil
}

func validateWorkerAPIKey(apiKey *domain.APIKey) error {
	if apiKey == nil {
		return status.Error(codes.Unauthenticated, "invalid api key")
	}
	if apiKey.RevokedAt != nil {
		return status.Error(codes.Unauthenticated, "api key has been revoked")
	}

	now := time.Now()
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(now) {
		return status.Error(codes.Unauthenticated, "api key has expired")
	}
	if apiKey.GraceExpiresAt != nil && apiKey.GraceExpiresAt.Before(now) {
		return status.Error(codes.Unauthenticated, "api key rotation grace period has ended")
	}
	if !domain.HasScope(apiKey.Scopes, domain.ScopeWorkersConnect) {
		return status.Error(codes.PermissionDenied, "api key does not allow worker connections")
	}
	return nil
}

// hashGRPCAPIKey returns the SHA-256 hex digest of the raw API key string.
// This must match the hashing used by the HTTP API layer (api.hashAPIKey).
func hashGRPCAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// withAPIKeyContext enriches ctx with the resolved API key's project and org IDs.
func withAPIKeyContext(ctx context.Context, apiKey *domain.APIKey) context.Context {
	ctx = context.WithValue(ctx, grpcCtxProjectIDKey, apiKey.ProjectID)
	ctx = context.WithValue(ctx, grpcCtxAPIKeyIDKey, apiKey.ID)
	ctx = context.WithValue(ctx, grpcCtxAPIKeyKey, apiKey)
	if apiKey.OrgID != "" {
		ctx = context.WithValue(ctx, grpcCtxOrgIDKey, apiKey.OrgID)
	}
	if apiKey.EnvironmentID != "" {
		ctx = context.WithValue(ctx, grpcCtxEnvironmentIDKey, apiKey.EnvironmentID)
	}
	return ctx
}

// ProjectIDFromContext extracts the project ID set by withAPIKeyContext.
func ProjectIDFromContext(ctx context.Context) (string, error) {
	v, ok := ctx.Value(grpcCtxProjectIDKey).(string)
	if !ok || v == "" {
		return "", fmt.Errorf("project_id not found in context")
	}
	return v, nil
}

// OrgIDFromContext extracts the org ID set by withAPIKeyContext (may be empty).
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(grpcCtxOrgIDKey).(string)
	return v
}

// EnvironmentIDFromContext extracts the environment ID set by withAPIKeyContext.
func EnvironmentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(grpcCtxEnvironmentIDKey).(string)
	return v
}

// APIKeyFromContext extracts the resolved APIKey set by withAPIKeyContext.
func APIKeyFromContext(ctx context.Context) (*domain.APIKey, bool) {
	v, ok := ctx.Value(grpcCtxAPIKeyKey).(*domain.APIKey)
	return v, ok
}
