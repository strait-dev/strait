package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/ratelimit"
	"strait/internal/store"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type grpcContextKey string

const (
	grpcAPIKeyMaxLength = 128

	grpcCtxProjectIDKey     grpcContextKey = "grpc_project_id"
	grpcCtxOrgIDKey         grpcContextKey = "grpc_org_id"
	grpcCtxAPIKeyIDKey      grpcContextKey = "grpc_api_key_id" //nolint:gosec // not a credential; context-key name
	grpcCtxAPIKeyKey        grpcContextKey = "grpc_api_key"    //nolint:gosec // not a credential; context-key name
	grpcCtxAPIKeyExpiresKey grpcContextKey = "grpc_api_key_expires_at"
	grpcCtxEnvironmentIDKey grpcContextKey = "grpc_environment_id"
)

var grpcAPIKeyPattern = regexp.MustCompile(`^strait_[A-Za-z0-9]+$`)

type grpcAuthLimiter interface {
	IsBlockedScoped(ctx context.Context, ip, scope string) (bool, time.Duration)
	RecordFailureScoped(ctx context.Context, ip, scope string)
	ResetScoped(ctx context.Context, ip, scope string)
}

func resolveAPIKeyFromContextWithLimit(ctx context.Context, q *store.Queries, limiter grpcAuthLimiter) (*domain.APIKey, error) {
	if limiter == nil {
		return resolveAPIKeyFromContext(ctx, q)
	}

	ip := grpcPeerIP(ctx)
	if blocked, retryAfter := limiter.IsBlockedScoped(ctx, ip, ratelimit.AuthScopeGRPCWorker); blocked {
		return nil, status.Errorf(codes.ResourceExhausted, "too many failed authentication attempts; retry after %s", retryAfter.Truncate(time.Second))
	}

	apiKey, err := resolveAPIKeyFromContext(ctx, q)
	if err != nil {
		limiter.RecordFailureScoped(ctx, ip, ratelimit.AuthScopeGRPCWorker)
		return nil, err
	}
	limiter.ResetScoped(ctx, ip, ratelimit.AuthScopeGRPCWorker)
	return apiKey, nil
}

func grpcPeerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p == nil || p.Addr == nil {
		return "unknown"
	}
	addr := p.Addr.String()
	host, _, err := net.SplitHostPort(addr)
	if err == nil && host != "" {
		return host
	}
	if addr != "" {
		return addr
	}
	return "unknown"
}

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
	if !validGRPCAPIKeyFormat(rawKey) {
		return nil, status.Error(codes.Unauthenticated, "invalid api key")
	}
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

func validGRPCAPIKeyFormat(rawKey string) bool {
	if rawKey == "" || len(rawKey) > grpcAPIKeyMaxLength {
		return false
	}
	return grpcAPIKeyPattern.MatchString(rawKey)
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
	if expiresAt, ok := workerAPIKeyExpiresAt(apiKey); ok {
		ctx = context.WithValue(ctx, grpcCtxAPIKeyExpiresKey, expiresAt)
	}
	if apiKey.OrgID != "" {
		ctx = context.WithValue(ctx, grpcCtxOrgIDKey, apiKey.OrgID)
	}
	if apiKey.EnvironmentID != "" {
		ctx = context.WithValue(ctx, grpcCtxEnvironmentIDKey, apiKey.EnvironmentID)
	}
	configureGRPCSentryAPIKeyScope(ctx)
	return ctx
}

func workerAPIKeyExpiresAt(apiKey *domain.APIKey) (time.Time, bool) {
	if apiKey == nil {
		return time.Time{}, false
	}
	var expiresAt time.Time
	var ok bool
	if apiKey.ExpiresAt != nil {
		expiresAt = *apiKey.ExpiresAt
		ok = true
	}
	if apiKey.GraceExpiresAt != nil && (!ok || apiKey.GraceExpiresAt.Before(expiresAt)) {
		expiresAt = *apiKey.GraceExpiresAt
		ok = true
	}
	return expiresAt, ok
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

func APIKeyExpiresAtFromContext(ctx context.Context) (time.Time, bool) {
	v, ok := ctx.Value(grpcCtxAPIKeyExpiresKey).(time.Time)
	return v, ok
}
