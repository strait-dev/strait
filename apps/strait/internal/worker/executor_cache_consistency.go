package worker

import (
	straitcache "strait/internal/cache"
	"strait/internal/domain"
)

func workerCachePolicy(namespace string) straitcache.StrongNamespacePolicy {
	return straitcache.StrongNamespacePolicy{Namespace: namespace}
}

func workerCacheBarrier(version int64) straitcache.VersionBarrier {
	return straitcache.VersionBarrier{Version: version}
}

func jobCacheVersion(job *domain.Job) int64 {
	if job == nil {
		return 0
	}
	if job.CacheVersion > 0 {
		return job.CacheVersion
	}
	if !job.UpdatedAt.IsZero() {
		return job.UpdatedAt.UnixNano()
	}
	if job.Version > 0 {
		return int64(job.Version)
	}
	return 1
}
