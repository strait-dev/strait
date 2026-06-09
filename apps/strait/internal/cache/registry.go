package cache

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type NamespaceHandler interface {
	InvalidateCacheKey(ctx context.Context, key string, version int64)
	ApplyCacheUpdate(ctx context.Context, key string, version int64, payload json.RawMessage)
}

type NamespaceHandlerFuncs struct {
	Invalidate func(context.Context, string, int64)
	Update     func(context.Context, string, int64, json.RawMessage)
}

func (h NamespaceHandlerFuncs) InvalidateCacheKey(ctx context.Context, key string, version int64) {
	if h.Invalidate != nil {
		h.Invalidate(ctx, key, version)
	}
}

func (h NamespaceHandlerFuncs) ApplyCacheUpdate(ctx context.Context, key string, version int64, payload json.RawMessage) {
	if h.Update != nil {
		h.Update(ctx, key, version, payload)
	}
}

type RegistryConfig struct {
	Origin       string
	Logger       *slog.Logger
	OnInvalid    func(reason string)
	OnSuppressed func()
	OnUnknown    func(namespace string)
	OnInvalidate func(namespace string)
	OnUpdate     func(namespace string)
	OnMessage    func(BusMessage)
}

type Registry struct {
	mu       sync.RWMutex
	origin   string
	logger   *slog.Logger
	handlers map[string]NamespaceHandler
	cfg      RegistryConfig
}

func NewRegistry(cfg RegistryConfig) *Registry {
	origin := cfg.Origin
	if origin == "" {
		origin = newOriginID()
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		origin:   origin,
		logger:   logger,
		handlers: make(map[string]NamespaceHandler),
		cfg:      cfg,
	}
}

func (r *Registry) Origin() string {
	if r == nil {
		return ""
	}
	return r.origin
}

func (r *Registry) Register(namespace string, handler NamespaceHandler) {
	if !r.canRegister(namespace, handler) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[namespace] = handler
}

func (r *Registry) canRegister(namespace string, handler NamespaceHandler) bool {
	if r == nil {
		return false
	}
	if namespace == "" {
		return false
	}
	return handler != nil
}

func (r *Registry) RegisteredNamespaces() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	namespaces := make([]string, 0, len(r.handlers))
	for namespace := range r.handlers {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces
}

func (r *Registry) Unregister(namespace string) {
	if r == nil || namespace == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, namespace)
}

func (r *Registry) Handle(ctx context.Context, data []byte) {
	if r == nil {
		return
	}
	msg, ok := r.decodeBusMessage(data)
	if !ok {
		return
	}
	r.dispatchBusMessage(ctx, msg)
}

func (r *Registry) decodeBusMessage(data []byte) (BusMessage, bool) {
	var msg BusMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		r.invalid("malformed_json")
		return BusMessage{}, false
	}
	if msg.Namespace == "" || msg.Key == "" {
		r.invalid("missing_namespace_or_key")
		return BusMessage{}, false
	}
	return msg, true
}

func (r *Registry) dispatchBusMessage(ctx context.Context, msg BusMessage) {
	if msg.Origin != "" && msg.Origin == r.origin {
		if r.cfg.OnSuppressed != nil {
			r.cfg.OnSuppressed()
		}
		return
	}

	r.mu.RLock()
	handler := r.handlers[msg.Namespace]
	r.mu.RUnlock()
	if handler == nil {
		if r.cfg.OnUnknown != nil {
			r.cfg.OnUnknown(msg.Namespace)
		}
		return
	}
	if r.cfg.OnMessage != nil {
		r.cfg.OnMessage(msg)
	}

	switch msg.Action {
	case BusActionInvalidate:
		handler.InvalidateCacheKey(ctx, msg.Key, msg.Version)
		if r.cfg.OnInvalidate != nil {
			r.cfg.OnInvalidate(msg.Namespace)
		}
	case BusActionUpdate:
		handler.ApplyCacheUpdate(ctx, msg.Key, msg.Version, msg.Payload)
		if r.cfg.OnUpdate != nil {
			r.cfg.OnUpdate(msg.Namespace)
		}
	default:
		r.invalid("unknown_action")
	}
}

func (r *Registry) invalid(reason string) {
	if r.cfg.OnInvalid != nil {
		r.cfg.OnInvalid(reason)
	}
}

type StringTierHandler[V any] struct {
	Tier *Tier[string, V]
}

func (h StringTierHandler[V]) InvalidateCacheKey(ctx context.Context, key string, _ int64) {
	if h.Tier != nil {
		h.Tier.Invalidate(ctx, key)
	}
}

func (h StringTierHandler[V]) ApplyCacheUpdate(_ context.Context, _ string, _ int64, _ json.RawMessage) {
}

type UpdatingStringTierHandler[V any] struct {
	Tier *Tier[string, V]
}

func (h UpdatingStringTierHandler[V]) InvalidateCacheKey(ctx context.Context, key string, version int64) {
	if h.Tier != nil {
		h.Tier.applyBarrier(ctx, key, version)
	}
}

func (h UpdatingStringTierHandler[V]) ApplyCacheUpdate(ctx context.Context, key string, _ int64, payload json.RawMessage) {
	if h.Tier == nil || len(payload) == 0 {
		return
	}
	var entry cacheEntry[V]
	if err := json.Unmarshal(payload, &entry); err != nil {
		recordCacheFailOpen(ctx, h.Tier.name, "cachebus_update_decode")
		return
	}
	h.Tier.applyUpdate(ctx, key, entry)
}

type TierHandler[K comparable, V any] struct {
	Tier  *Tier[K, V]
	Parse func(string) (K, bool)
}

func (h TierHandler[K, V]) InvalidateCacheKey(ctx context.Context, key string, version int64) {
	if h.Tier == nil || h.Parse == nil {
		return
	}
	parsed, ok := h.Parse(key)
	if ok {
		h.Tier.applyBarrier(ctx, parsed, version)
	}
}

func (h TierHandler[K, V]) ApplyCacheUpdate(ctx context.Context, key string, _ int64, payload json.RawMessage) {
	if !h.canApplyUpdate(payload) {
		return
	}
	parsed, ok := h.Parse(key)
	if !ok {
		return
	}
	var entry cacheEntry[V]
	if err := json.Unmarshal(payload, &entry); err != nil {
		recordCacheFailOpen(ctx, h.Tier.name, "cachebus_update_decode")
		return
	}
	h.Tier.applyUpdate(ctx, parsed, entry)
}

func (h TierHandler[K, V]) canApplyUpdate(payload json.RawMessage) bool {
	if h.Tier == nil {
		return false
	}
	if h.Parse == nil {
		return false
	}
	return len(payload) > 0
}

func newOriginID() string {
	return uuid.NewString()
}
