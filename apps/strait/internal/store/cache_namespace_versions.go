package store

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

func (q *Queries) BumpCacheNamespaceVersion(ctx context.Context, namespace, cacheKey string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.BumpCacheNamespaceVersion")
	defer span.End()

	var version int64
	err := q.db.QueryRow(ctx, `
		INSERT INTO cache_namespace_versions (namespace, cache_key, version)
		VALUES ($1, $2, 1)
		ON CONFLICT (namespace, cache_key)
		DO UPDATE SET version = cache_namespace_versions.version + 1, updated_at = NOW()
		RETURNING version`, namespace, cacheKey).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("bump cache namespace version: %w", err)
	}
	return version, nil
}

func (q *Queries) GetCacheNamespaceVersion(ctx context.Context, namespace, cacheKey string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.GetCacheNamespaceVersion")
	defer span.End()

	var version int64
	err := q.db.QueryRow(ctx, `
		SELECT version
		FROM cache_namespace_versions
		WHERE namespace = $1 AND cache_key = $2`, namespace, cacheKey).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get cache namespace version: %w", err)
	}
	return version, nil
}

func (q *Queries) EnsureCacheNamespaceVersion(ctx context.Context, namespace, cacheKey string) (int64, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.EnsureCacheNamespaceVersion")
	defer span.End()

	var version int64
	err := q.db.QueryRow(ctx, `
		INSERT INTO cache_namespace_versions (namespace, cache_key, version)
		VALUES ($1, $2, 1)
		ON CONFLICT (namespace, cache_key)
		DO UPDATE SET updated_at = cache_namespace_versions.updated_at
		RETURNING version`, namespace, cacheKey).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("ensure cache namespace version: %w", err)
	}
	return version, nil
}
