package cache

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegistryRegisteredNamespacesReturnsSortedCopy(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(RegistryConfig{})
	registry.Register("quota", NamespaceHandlerFuncs{Invalidate: func(context.Context, string, int64) {}})
	registry.Register("authn_keys", NamespaceHandlerFuncs{Update: func(context.Context, string, int64, json.RawMessage) {}})

	namespaces := registry.RegisteredNamespaces()
	if got, want := len(namespaces), 2; got != want {
		t.Fatalf("len(RegisteredNamespaces()) = %d, want %d", got, want)
	}
	if namespaces[0] != "authn_keys" || namespaces[1] != "quota" {
		t.Fatalf("RegisteredNamespaces() = %v, want sorted authn_keys/quota", namespaces)
	}

	namespaces[0] = "mutated"
	if registry.RegisteredNamespaces()[0] != "authn_keys" {
		t.Fatal("RegisteredNamespaces() returned internal storage instead of a copy")
	}
}

func TestRegistryRegisteredNamespacesNilSafe(t *testing.T) {
	t.Parallel()

	var registry *Registry
	if namespaces := registry.RegisteredNamespaces(); namespaces != nil {
		t.Fatalf("nil registry RegisteredNamespaces() = %v, want nil", namespaces)
	}
}
