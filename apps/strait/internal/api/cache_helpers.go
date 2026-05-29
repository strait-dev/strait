package api

import (
	"context"

	straitcache "strait/internal/cache"
)

// Otter eviction callbacks do not carry request context, but metric records
// still need a stable context for OpenTelemetry.
var cacheMetricsContext = context.Background()

func strongCachePolicy(namespace string) straitcache.StrongNamespacePolicy {
	return straitcache.StrongNamespacePolicy{Namespace: namespace}
}

func cacheVersionBarrier(version int64) straitcache.VersionBarrier {
	return straitcache.VersionBarrier{Version: version}
}
