package cdc

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	straitcache "strait/internal/cache"
)

const (
	cacheNamespaceAPIKeyAuth     = "authn_keys" // #nosec G101 -- cache namespace, not a credential.
	cacheNamespacePermission     = "permission"
	cacheNamespacePermissionProj = "permission_project"
	cacheNamespaceQuota          = "quota"
	cacheNamespaceBillingOrg     = "billing_org_limits"
	cacheNamespaceWorkerJob      = "worker_job"
	cacheNamespaceJobDependency  = "api_job_dependencies"
	defaultJobDependencyListSize = 1000
)

func NewCacheInvalidationHandlers(bus *straitcache.Bus, logger *slog.Logger) []Handler {
	if bus == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return []Handler{
		newCacheInvalidationHandler("api_keys", bus, logger, invalidateAPIKeyCache),
		newCacheInvalidationHandler("project_roles", bus, logger, invalidatePermissionProjectCache),
		newCacheInvalidationHandler("project_member_roles", bus, logger, invalidatePermissionCache),
		newCacheInvalidationHandler("resource_policies", bus, logger, invalidatePermissionCache),
		newCacheInvalidationHandler("tag_policies", bus, logger, invalidatePermissionCache),
		newCacheInvalidationHandler("project_quotas", bus, logger, invalidateQuotaCache),
		newCacheInvalidationHandler("organization_subscriptions", bus, logger, invalidateBillingOrgCache),
		newCacheInvalidationHandler("jobs", bus, logger, invalidateWorkerJobCache),
		newCacheInvalidationHandler("job_dependencies", bus, logger, invalidateJobDependencyCache),
	}
}

type cacheInvalidationHandler struct {
	table  string
	bus    *straitcache.Bus
	logger *slog.Logger
	fn     cacheInvalidationFunc
}

type cacheInvalidationFunc func(context.Context, *straitcache.Bus, map[string]any, int64) error

func newCacheInvalidationHandler(
	table string,
	bus *straitcache.Bus,
	logger *slog.Logger,
	fn cacheInvalidationFunc,
) Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &cacheInvalidationHandler{table: table, bus: bus, logger: logger, fn: fn}
}

func (h *cacheInvalidationHandler) Table() string { return h.table }

func (h *cacheInvalidationHandler) Handle(ctx context.Context, msg Message) error {
	if !cacheInvalidationHandlerCanProcess(h, msg) {
		return nil
	}
	var record map[string]any
	if err := json.Unmarshal(msg.Record, &record); err != nil {
		h.logger.Warn("cdc cache invalidation ignored malformed record", "table", h.table, "error", err)
		return nil
	}
	version := cacheVersionFromRecord(record)
	if version <= 0 {
		version = time.Now().UnixNano()
	}
	if err := h.fn(ctx, h.bus, record, version); err != nil {
		h.logger.Warn("cdc cache invalidation skipped", "table", h.table, "error", err)
	}
	return nil
}

func cacheInvalidationHandlerCanProcess(h *cacheInvalidationHandler, msg Message) bool {
	if h == nil {
		return false
	}

	hasBus := h.bus != nil
	hasInvalidationFunc := h.fn != nil
	hasRecord := len(msg.Record) > 0
	return hasBus && hasInvalidationFunc && hasRecord
}

func invalidatePermissionProjectCache(
	ctx context.Context,
	bus *straitcache.Bus,
	record map[string]any,
	version int64,
) error {
	projectID := stringField(record, "project_id")
	if projectID == "" {
		return nil
	}
	return bus.PublishInvalidate(ctx, cacheNamespacePermissionProj, projectID, version)
}

func invalidateAPIKeyCache(ctx context.Context, bus *straitcache.Bus, record map[string]any, version int64) error {
	keyHash := stringField(record, "key_hash")
	if keyHash == "" {
		return nil
	}
	return bus.PublishInvalidate(ctx, cacheNamespaceAPIKeyAuth, keyHash, version)
}

func invalidatePermissionCache(ctx context.Context, bus *straitcache.Bus, record map[string]any, version int64) error {
	projectID := stringField(record, "project_id")
	userID := stringField(record, "user_id")
	if !permissionCacheRecordAddressable(projectID, userID) {
		return nil
	}
	return bus.PublishInvalidate(ctx, cacheNamespacePermission, permissionCacheKey(projectID, userID), version)
}

func permissionCacheRecordAddressable(projectID, userID string) bool {
	return projectID != "" && userID != ""
}

func invalidateQuotaCache(ctx context.Context, bus *straitcache.Bus, record map[string]any, version int64) error {
	projectID := stringField(record, "project_id")
	if projectID == "" {
		return nil
	}
	return bus.PublishInvalidate(ctx, cacheNamespaceQuota, projectID, version)
}

func invalidateBillingOrgCache(ctx context.Context, bus *straitcache.Bus, record map[string]any, version int64) error {
	orgID := stringField(record, "org_id")
	if orgID == "" {
		return nil
	}
	return bus.PublishInvalidate(ctx, cacheNamespaceBillingOrg, orgID, version)
}

func invalidateWorkerJobCache(ctx context.Context, bus *straitcache.Bus, record map[string]any, version int64) error {
	jobID := stringField(record, "id")
	if jobID == "" {
		return nil
	}
	return bus.PublishInvalidate(ctx, cacheNamespaceWorkerJob, jobID, version)
}

func invalidateJobDependencyCache(
	ctx context.Context,
	bus *straitcache.Bus,
	record map[string]any,
	version int64,
) error {
	jobID := stringField(record, "job_id")
	if jobID == "" {
		return nil
	}
	return bus.PublishInvalidate(
		ctx,
		cacheNamespaceJobDependency,
		jobDependencyCacheKey(jobID, defaultJobDependencyListSize),
		version,
	)
}

func permissionCacheKey(projectID, userID string) string {
	return projectID + "\x00" + userID
}

func jobDependencyCacheKey(jobID string, limit int) string {
	const maxIntDigits = 20
	const sepCount = 2
	size := len(jobID) + sepCount + maxIntDigits
	if size <= 96 {
		var buf [96]byte
		out := append(buf[:0], jobID...)
		out = append(out, 0)
		out = strconv.AppendInt(out, int64(limit), 10)
		out = append(out, 0)
		return string(out)
	}
	out := make([]byte, 0, size)
	out = append(out, jobID...)
	out = append(out, 0)
	out = strconv.AppendInt(out, int64(limit), 10)
	out = append(out, 0)
	return string(out)
}

func cacheVersionFromRecord(record map[string]any) int64 {
	switch v := record["cache_version"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case json.Number:
		got, _ := v.Int64()
		return got
	case string:
		got, _ := strconv.ParseInt(v, 10, 64)
		return got
	default:
		return 0
	}
}

func stringField(record map[string]any, name string) string {
	if record == nil {
		return ""
	}
	switch v := record[name].(type) {
	case string:
		return v
	default:
		return ""
	}
}
