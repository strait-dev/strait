package cdc

import (
	"testing"

	straitcache "strait/internal/cache"

	"github.com/stretchr/testify/require"
)

func TestCacheInvalidationHandlersCoverStrongPolicyTables(t *testing.T) {
	t.Parallel()

	handlers := NewCacheInvalidationHandlers(straitcache.NewBus(&cacheInvalidationPublisher{}, straitcache.BusConfig{}), nil)
	registered := make(map[string]struct{}, len(handlers))
	for _, handler := range handlers {
		registered[handler.Table()] = struct{}{}
	}

	for _, policy := range straitcache.StrongNamespacePolicies {
		for _, table := range policy.CDCTables {
			if _, ok := registered[table]; !ok {
				require.Failf(t, "test failure",

					"policy %s declares CDC table %s without a registered handler", policy.Namespace, table)
			}
		}
	}
}
