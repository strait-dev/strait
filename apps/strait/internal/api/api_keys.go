package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"strait/internal/domain"
	"strait/internal/httputil"

	"github.com/danielgtaylor/huma/v2"
)

// maxAPIKeyDurationDays caps expires_in_days and rotation_interval_days at
// 100 years. time.Duration(days)*24*time.Hour overflows once days exceeds
// ~106,750, and even an "expires never" intent is not a good reason to
// open the API to user-controlled int64 overflow.
const maxAPIKeyDurationDays = domain.MaxAPIKeyDurationDays

type CreateAPIKeyRequest struct {
	ProjectID            string   `json:"project_id" validate:"required"`
	OrgID                string   `json:"org_id,omitempty"`
	Name                 string   `json:"name" validate:"required,max=255"`
	Scopes               []string `json:"scopes,omitempty"`
	ExpiresIn            *int     `json:"expires_in_days,omitempty" validate:"omitempty,min=1,max=36500"`
	EnvironmentID        string   `json:"environment_id,omitempty"`
	RotationIntervalDays *int     `json:"rotation_interval_days,omitempty" validate:"omitempty,min=1,max=36500"`
	RotationWebhookURL   string   `json:"rotation_webhook_url,omitempty" validate:"omitempty,url,max=2048"`
}
type CreateAPIKeyResponse struct {
	ID                    string     `json:"id"`
	ProjectID             string     `json:"project_id"`
	Name                  string     `json:"name"`
	Key                   string     `json:"key"`
	KeyPrefix             string     `json:"key_prefix"`
	Scopes                []string   `json:"scopes"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	RotationWebhookSecret string     `json:"rotation_webhook_secret,omitempty"`
}
type RotateAPIKeyRequest struct {
	GracePeriodMinutes int `json:"grace_period_minutes,omitempty"`
}

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "strait_" + hex.EncodeToString(b), nil
}
func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	var out [sha256.Size * 2]byte
	hex.Encode(out[:], sum[:])
	return string(out[:])
}

type CreateAPIKeyInput struct{ Body CreateAPIKeyRequest }
type CreateAPIKeyOutput struct{ Body CreateAPIKeyResponse }

func (s *Server) handleCreateAPIKey(ctx context.Context, input *CreateAPIKeyInput) (*CreateAPIKeyOutput, error) {
	req := input.Body
	if err := s.validate.Struct(&req); err != nil {
		return nil, newValidationError(err)
	}
	if err := requireProjectMatch(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("project_id does not match authenticated project")
	}
	if err := requireAPIKeyCreationScope(ctx, req); err != nil {
		return nil, err
	}
	rawKey, err := generateAPIKey()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate api key")
	}
	if len(req.Scopes) == 0 {
		return nil, huma.Error400BadRequest("scopes must contain at least one explicit permission")
	}
	if err := domain.ValidateScopes(req.Scopes); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	// Prevent scope escalation: the caller cannot create a key with
	// scopes broader than their own effective permissions.
	if err := s.validateCallerCanGrantPermissions(ctx, req.Scopes); err != nil {
		return nil, err
	}
	if req.ExpiresIn != nil && *req.ExpiresIn <= 0 {
		return nil, huma.Error400BadRequest("expires_in_days must be greater than 0")
	}
	if req.RotationWebhookURL != "" {
		if err := validateAPIKeyRotationWebhookURL(req.RotationWebhookURL, s.config != nil && s.config.AllowPrivateEndpoints); err != nil {
			slog.Warn("rotation webhook URL rejected", "url", httputil.RedactURLForLog(req.RotationWebhookURL), "error", err)
			return nil, huma.Error400BadRequest("invalid rotation_webhook_url")
		}
	}
	if req.RotationIntervalDays != nil && *req.RotationIntervalDays > 0 && req.RotationWebhookURL == "" {
		return nil, huma.Error400BadRequest("rotation_webhook_url is required when rotation_interval_days is set")
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		t := time.Now().Add(time.Duration(*req.ExpiresIn) * 24 * time.Hour)
		expiresAt = &t
	}

	expiresAt, err = s.apiKeyExpiryFromProjectPolicy(ctx, req.ProjectID, expiresAt)
	if err != nil {
		return nil, err
	}

	key := &domain.APIKey{ProjectID: req.ProjectID, OrgID: req.OrgID, Name: req.Name, KeyHash: hashAPIKey(rawKey), KeyPrefix: rawKey[:domain.APIKeyPrefixLen], Scopes: req.Scopes, ExpiresAt: expiresAt, EnvironmentID: req.EnvironmentID, RotationIntervalDays: req.RotationIntervalDays, RotationWebhookURL: req.RotationWebhookURL}
	if req.RotationIntervalDays != nil && *req.RotationIntervalDays > 0 {
		nr := time.Now().Add(time.Duration(*req.RotationIntervalDays) * 24 * time.Hour)
		key.NextRotationAt = &nr
	}
	var rotationWebhookSecretPlaintext string
	if req.RotationWebhookURL != "" {
		if s.encryptor == nil {
			return nil, huma.Error500InternalServerError("server not configured for rotation webhook signing: ENCRYPTION_KEY required")
		}
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, huma.Error500InternalServerError("failed to generate rotation webhook secret")
		}
		rotationWebhookSecretPlaintext = "whsec_" + hex.EncodeToString(secretBytes)
		ciphertext, err := s.encryptor.Encrypt([]byte(rotationWebhookSecretPlaintext))
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to encrypt rotation webhook secret")
		}
		key.RotationWebhookSecret = ciphertext
	}
	if err := s.store.CreateAPIKey(ctx, key); err != nil {
		return nil, huma.Error500InternalServerError("failed to create api key")
	}
	s.apiKeyCache.Set(ctx, key)
	s.emitAuditEvent(ctx, domain.AuditActionAPIKeyCreated, "api_key", key.ID, map[string]any{
		"name":                      key.Name,
		"key_prefix":                key.KeyPrefix,
		"scopes":                    key.Scopes,
		"expires_at":                key.ExpiresAt,
		"environment_id":            key.EnvironmentID,
		"rotation_interval_days":    req.RotationIntervalDays,
		"rotation_webhook_url_host": urlHost(key.RotationWebhookURL),
	})
	return &CreateAPIKeyOutput{Body: CreateAPIKeyResponse{ID: key.ID, ProjectID: key.ProjectID, Name: key.Name, Key: rawKey, KeyPrefix: key.KeyPrefix, Scopes: key.Scopes, ExpiresAt: key.ExpiresAt, CreatedAt: key.CreatedAt, RotationWebhookSecret: rotationWebhookSecretPlaintext}}, nil
}

func (s *Server) apiKeyExpiryFromProjectPolicy(ctx context.Context, projectID string, requested *time.Time) (*time.Time, error) {
	quota, err := s.store.GetProjectQuota(ctx, projectID)
	if err != nil {
		slog.Warn("failed to load project quota while creating api key",
			"project_id", projectID, "error", err)
		return nil, huma.Error500InternalServerError("failed to load project quota")
	}
	maxLifetimeDays := 0
	if quota != nil {
		maxLifetimeDays = quota.MaxKeyLifetimeDays
	}
	expiresAt, err := domain.ApplyAPIKeyLifetimePolicy(time.Now(), requested, maxLifetimeDays)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	if requested == nil && maxLifetimeDays > 0 {
		slog.Info("api key expiry auto-capped by project max_key_lifetime_days",
			"project_id", projectID, "max_days", min(maxLifetimeDays, maxAPIKeyDurationDays))
	}
	return expiresAt, nil
}

func validateAPIKeyRotationWebhookURL(rawURL string, allowPrivateEndpoints bool) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse rotation webhook url: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("rotation webhook must use https")
	}
	if err := httputil.ValidateExternalURL(rawURL); err != nil && !allowPrivateEndpoints {
		return fmt.Errorf("ssrf guard rejected rotation webhook url: %w", err)
	}
	return nil
}

type ListAPIKeysInput struct {
	OrgID  string `query:"org_id"`
	Limit  string `query:"limit"`
	Cursor string `query:"cursor"`
}
type ListAPIKeysOutput struct{ Body PaginatedResponse }

func (s *Server) handleListAPIKeys(ctx context.Context, input *ListAPIKeysInput) (*ListAPIKeysOutput, error) {
	limit, cursor, err := parsePaginationFromStrings(input.Limit, input.Cursor)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	var keys []domain.APIKey
	if input.OrgID != "" {
		if err := requireOrgAPIKeyListAccess(ctx, input.OrgID); err != nil {
			return nil, err
		}
		restoreRLS, rlsErr := s.useOrgWideAPIKeyListContext(ctx)
		if rlsErr != nil {
			return nil, huma.Error500InternalServerError("failed to initialize org api key list context")
		}
		defer restoreRLS()
		keys, err = s.store.ListAPIKeysByOrg(ctx, input.OrgID, limit+1, cursor)
	} else {
		projectID := projectIDFromContext(ctx)
		if projectID == "" {
			return nil, huma.Error400BadRequest("project_id is required")
		}
		keys, err = s.store.ListAPIKeysByProject(ctx, projectID, limit+1, cursor)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list api keys")
	}
	keys = filterAPIKeysForEnvironment(ctx, keys)
	s.emitAuditEvent(ctx, domain.AuditActionAPIKeyListRead, "api_key", "", map[string]any{
		"count":  len(keys),
		"org_id": input.OrgID,
	})
	return &ListAPIKeysOutput{Body: paginatedResult(keys, limit, func(k domain.APIKey) string { return k.CreatedAt.Format(time.RFC3339Nano) })}, nil
}

type ExpiringKeyInfo struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyPrefix string     `json:"key_prefix"`
	ExpiresAt *time.Time `json:"expires_at"`
	DaysLeft  *int       `json:"days_left"`
	NoExpiry  bool       `json:"no_expiry"`
	CreatedAt time.Time  `json:"created_at"`
}

type ListExpiringKeysInput struct {
	WithinDays int `query:"within_days"`
}
type ListExpiringKeysOutput struct{ Body []ExpiringKeyInfo }

func (s *Server) handleListExpiringKeys(ctx context.Context, input *ListExpiringKeysInput) (*ListExpiringKeysOutput, error) {
	projectID := projectIDFromContext(ctx)
	if projectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}

	withinDays := input.WithinDays
	if withinDays <= 0 {
		withinDays = 30
	}
	if withinDays > 365 {
		withinDays = 365
	}

	keys, err := s.store.ListAPIKeysExpiringSoon(ctx, projectID, withinDays)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list expiring keys")
	}
	keys = filterAPIKeysForEnvironment(ctx, keys)

	now := time.Now()
	result := make([]ExpiringKeyInfo, 0, len(keys))
	for _, k := range keys {
		info := ExpiringKeyInfo{
			ID:        k.ID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			ExpiresAt: k.ExpiresAt,
			NoExpiry:  k.ExpiresAt == nil,
			CreatedAt: k.CreatedAt,
		}
		if k.ExpiresAt != nil {
			days := max(int(k.ExpiresAt.Sub(now).Hours()/24), 0)
			info.DaysLeft = &days
		}
		result = append(result, info)
	}

	return &ListExpiringKeysOutput{Body: result}, nil
}

type RevokeAPIKeyInput struct {
	KeyID string `path:"keyID"`
}
type RevokeAPIKeyOutput struct{ Body map[string]string }

func apiKeyRevokedChannel(apiKeyID string) string {
	return "apikey:revoked:" + apiKeyID
}

func apiKeyExpiresChannel(apiKeyID string) string {
	return "apikey:expires:" + apiKeyID
}

func (s *Server) handleRevokeAPIKey(ctx context.Context, input *RevokeAPIKeyInput) (*RevokeAPIKeyOutput, error) {
	key, err := s.store.GetAPIKeyByID(ctx, input.KeyID)
	if err != nil || key == nil {
		return nil, huma.Error404NotFound("api key not found")
	}
	if err := requireProjectMatch(ctx, key.ProjectID); err != nil {
		return nil, huma.Error404NotFound("api key not found")
	}
	if err := requireEnvironmentMatch(ctx, key.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("api key not found")
	}
	if s.config != nil && s.config.GRPCEnabled && s.pubsub == nil {
		return nil, huma.Error503ServiceUnavailable("api key revocation unavailable: pubsub not configured")
	}
	alreadyRevoked := key.RevokedAt != nil
	if !alreadyRevoked {
		if err := s.store.RevokeAPIKey(ctx, input.KeyID); err != nil {
			return nil, huma.Error404NotFound("api key not found or already revoked")
		}
		version := key.CacheVersion + 1
		if revoked, getErr := s.store.GetAPIKeyByID(ctx, input.KeyID); getErr == nil && revoked != nil && revoked.CacheVersion > 0 {
			version = revoked.CacheVersion
		}
		s.apiKeyCache.InvalidateWithVersion(ctx, key.KeyHash, version)
		slog.Info("api key revoked", "key_id", input.KeyID, "actor", actorFromContext(ctx), "project_id", projectIDFromContext(ctx))
		s.emitAuditEvent(ctx, domain.AuditActionAPIKeyRevoked, "api_key", input.KeyID, nil)
	}

	// Broadcast revocation to all gRPC replicas so any worker streams authenticated
	// with this key are closed immediately.
	if s.pubsub != nil {
		revokeChannel := apiKeyRevokedChannel(input.KeyID)
		if pubErr := s.pubsub.Publish(ctx, revokeChannel, []byte(input.KeyID)); pubErr != nil {
			slog.Error("api key revoke: broadcast publish failed",
				"key_id", input.KeyID,
				"error", pubErr,
			)
			if s.config != nil && s.config.GRPCEnabled {
				return nil, huma.Error503ServiceUnavailable("api key revoked but active worker stream broadcast failed")
			}
		}
	}

	return &RevokeAPIKeyOutput{Body: map[string]string{"status": "revoked"}}, nil
}

type RotateAPIKeyInput struct {
	KeyID string `path:"keyID"`
	Body  RotateAPIKeyRequest
}
type RotateAPIKeyOutput struct{ Body any }

func (s *Server) handleRotateAPIKey(ctx context.Context, input *RotateAPIKeyInput) (*RotateAPIKeyOutput, error) {
	req := input.Body
	if req.GracePeriodMinutes <= 0 {
		req.GracePeriodMinutes = 60
	}
	if req.GracePeriodMinutes > 7*24*60 {
		return nil, huma.Error400BadRequest("grace_period_minutes must be <= 10080")
	}
	oldKey, err := s.store.GetAPIKeyByID(ctx, input.KeyID)
	if err != nil || oldKey == nil {
		return nil, huma.Error404NotFound("api key not found")
	}
	if err := requireProjectMatch(ctx, oldKey.ProjectID); err != nil {
		return nil, huma.Error404NotFound("api key not found")
	}
	if err := requireEnvironmentMatch(ctx, oldKey.EnvironmentID); err != nil {
		return nil, huma.Error404NotFound("api key not found")
	}
	if err := s.validateCallerCanGrantPermissions(ctx, apiKeyScopesForGrant(oldKey.Scopes)); err != nil {
		return nil, err
	}
	if oldKey.RevokedAt != nil {
		return nil, huma.Error409Conflict("api key is already revoked")
	}
	if s.config != nil && s.config.GRPCEnabled && s.pubsub == nil {
		return nil, huma.Error503ServiceUnavailable("api key rotation unavailable: pubsub not configured")
	}
	rawKey, err := generateAPIKey()
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to generate api key")
	}
	expiresAt, err := s.apiKeyExpiryFromProjectPolicy(ctx, oldKey.ProjectID, oldKey.ExpiresAt)
	if err != nil {
		return nil, err
	}
	newKey := &domain.APIKey{ProjectID: oldKey.ProjectID, OrgID: oldKey.OrgID, Name: oldKey.Name + " (rotated)", KeyHash: hashAPIKey(rawKey), KeyPrefix: rawKey[:domain.APIKeyPrefixLen], Scopes: oldKey.Scopes, ExpiresAt: expiresAt, EnvironmentID: oldKey.EnvironmentID, RotationIntervalDays: oldKey.RotationIntervalDays, RotationWebhookURL: oldKey.RotationWebhookURL, RotationWebhookSecret: oldKey.RotationWebhookSecret}
	if oldKey.RotationIntervalDays != nil && *oldKey.RotationIntervalDays > 0 {
		nr := time.Now().Add(time.Duration(*oldKey.RotationIntervalDays) * 24 * time.Hour)
		newKey.NextRotationAt = &nr
	}
	if err := s.store.CreateAPIKey(ctx, newKey); err != nil {
		return nil, huma.Error500InternalServerError("failed to create rotated api key")
	}
	graceExpiresAt := time.Now().Add(time.Duration(req.GracePeriodMinutes) * time.Minute)
	if err := s.store.MarkAPIKeyRotated(ctx, oldKey.ID, newKey.ID, graceExpiresAt); err != nil {
		if newKey.ID != "" {
			if revokeErr := s.store.RevokeAPIKey(ctx, newKey.ID); revokeErr != nil {
				slog.Error("api key rotation cleanup failed",
					"old_key_id", oldKey.ID,
					"new_key_id", newKey.ID,
					"error", revokeErr,
				)
			}
		}
		s.apiKeyCache.Invalidate(ctx, newKey.KeyHash)
		return nil, huma.Error500InternalServerError("failed to mark old key as rotated")
	}
	s.apiKeyCache.Set(ctx, newKey)
	oldKeyVersion := oldKey.CacheVersion + 1
	if rotatedOldKey, getErr := s.store.GetAPIKeyByID(ctx, oldKey.ID); getErr == nil && rotatedOldKey != nil && rotatedOldKey.CacheVersion > 0 {
		oldKeyVersion = rotatedOldKey.CacheVersion
	}
	s.apiKeyCache.InvalidateWithVersion(ctx, oldKey.KeyHash, oldKeyVersion)
	if s.pubsub != nil && oldKey.ID != "" {
		expireChannel := apiKeyExpiresChannel(oldKey.ID)
		if pubErr := s.pubsub.Publish(ctx, expireChannel, []byte(graceExpiresAt.UTC().Format(time.RFC3339Nano))); pubErr != nil {
			slog.Error("api key rotation expiry broadcast failed",
				"key_id", oldKey.ID,
				"error", pubErr,
			)
			if s.config != nil && s.config.GRPCEnabled {
				return nil, huma.Error503ServiceUnavailable("api key rotated but active worker stream expiry broadcast failed")
			}
		}
	}
	s.emitAuditEvent(ctx, domain.AuditActionAPIKeyRotated, "api_key", input.KeyID, map[string]any{"new_key_id": newKey.ID, "grace_expires_at": graceExpiresAt, "grace_period_minute": req.GracePeriodMinutes})
	return &RotateAPIKeyOutput{Body: map[string]any{"old_key_id": oldKey.ID, "new_key_id": newKey.ID, "project_id": newKey.ProjectID, "name": newKey.Name, "key": rawKey, "key_prefix": newKey.KeyPrefix, "scopes": newKey.Scopes, "expires_at": newKey.ExpiresAt, "created_at": newKey.CreatedAt, "grace_expires_at": graceExpiresAt}}, nil
}

func requireAPIKeyCreationScope(ctx context.Context, req CreateAPIKeyRequest) error {
	if callerEnv := environmentIDFromContext(ctx); callerEnv != "" && req.EnvironmentID != callerEnv {
		return huma.Error404NotFound("environment not found")
	}
	if req.OrgID == "" || isInternalCaller(ctx) {
		return nil
	}
	if callerOrg := orgIDFromContext(ctx); callerOrg == "" || callerOrg != req.OrgID {
		return huma.Error403Forbidden("org_id does not match authenticated organization")
	}
	return nil
}

func requireOrgAPIKeyListAccess(ctx context.Context, orgID string) error {
	if orgID == "" {
		return nil
	}
	if isInternalCaller(ctx) {
		return nil
	}
	if callerOrg := orgIDFromContext(ctx); callerOrg == "" || callerOrg != orgID {
		return huma.Error403Forbidden("org_id does not match authenticated organization")
	}
	return nil
}

func (s *Server) useOrgWideAPIKeyListContext(ctx context.Context) (func(), error) {
	setter, ok := s.store.(ProjectContextSetter)
	if !ok {
		return func() {}, nil
	}
	projectID := projectIDFromContext(ctx)
	if err := setter.ClearProjectContext(ctx); err != nil {
		return nil, err
	}
	return func() {
		if projectID != "" {
			if err := setter.SetProjectContext(ctx, projectID); err != nil {
				slog.Warn("failed to restore project RLS context after org api key list", "project_id", projectID, "error", err)
			}
		}
	}, nil
}

func filterAPIKeysForEnvironment(ctx context.Context, keys []domain.APIKey) []domain.APIKey {
	callerEnv := environmentIDFromContext(ctx)
	if callerEnv == "" {
		return keys
	}
	filtered := keys[:0]
	for _, key := range keys {
		if key.EnvironmentID == callerEnv {
			filtered = append(filtered, key)
		}
	}
	return filtered
}

func apiKeyScopesForGrant(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{domain.ScopeAll}
	}
	return scopes
}
