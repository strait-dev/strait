package cache

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryRegisteredNamespacesReturnsSortedCopy(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(RegistryConfig{})
	registry.Register("quota", NamespaceHandlerFuncs{Invalidate: func(context.Context, string, int64) {}})
	registry.Register("authn_keys", NamespaceHandlerFuncs{Update: func(context.Context, string, int64, json.RawMessage) {}})

	namespaces := registry.RegisteredNamespaces()
	require.Len(t, namespaces, 2)
	assert.Equal(t, []string{"authn_keys", "quota"}, namespaces)

	namespaces[0] = "mutated"
	require.Equal(t, "authn_keys", registry.RegisteredNamespaces()[0])
}

func TestRegistryRegisterSkipsInvalidEntries(t *testing.T) {
	t.Parallel()

	handler := NamespaceHandlerFuncs{Invalidate: func(context.Context, string, int64) {}}
	registry := NewRegistry(RegistryConfig{})

	var nilRegistry *Registry
	nilRegistry.Register("jobs", handler)

	registry.Register("", handler)
	registry.Register("jobs", nil)
	require.Empty(t, registry.RegisteredNamespaces())

	registry.Register("jobs", handler)
	require.Equal(t, []string{"jobs"}, registry.RegisteredNamespaces())
}

func TestRegistryRegisteredNamespacesNilSafe(t *testing.T) {
	t.Parallel()

	var registry *Registry
	require.Nil(t, registry.RegisteredNamespaces())
}
