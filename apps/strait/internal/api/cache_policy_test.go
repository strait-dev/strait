package api

import (
	"context"
	"testing"
	"time"

	straitcache "strait/internal/cache"
	"strait/internal/store"
)

func TestAPIStrongCacheConstructorsRegisterRuntimeNamespaces(t *testing.T) {
	t.Parallel()

	registry := straitcache.NewRegistry(straitcache.RegistryConfig{Origin: "api-test"})
	deps, cleanup := newTestRedisCacheDeps(t, registry)
	t.Cleanup(cleanup)

	_ = newAPIKeyCache(time.Minute, deps)
	_ = newPermissionCache(time.Minute, deps)
	_ = newQuotaCache(time.Minute, func(context.Context, string) (*store.ProjectQuota, error) {
		return nil, nil
	}, deps)
	_ = newJobDependencyCache(time.Minute, deps)

	assertRegisteredNamespaces(t, registry, []string{
		apiKeyAuthCacheNamespace,
		permissionCacheNamespace,
		permissionProjectCacheNamespace,
		quotaCacheNamespace,
		jobDependencyCacheNamespace,
	})
}

func assertRegisteredNamespaces(t *testing.T, registry *straitcache.Registry, expected []string) {
	t.Helper()

	registered := make(map[string]struct{}, len(registry.RegisteredNamespaces()))
	for _, namespace := range registry.RegisteredNamespaces() {
		registered[namespace] = struct{}{}
	}
	for _, namespace := range expected {
		if _, ok := registered[namespace]; !ok {
			t.Fatalf("cache namespace %s was not registered; registered namespaces: %v", namespace, registry.RegisteredNamespaces())
		}
	}
}
