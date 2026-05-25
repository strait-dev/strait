package cdc

import (
	"context"
	"encoding/json"
	"testing"

	straitcache "strait/internal/cache"
	"strait/internal/pubsub"
)

type cacheInvalidationPublisher struct {
	calls []publishCall
}

func (p *cacheInvalidationPublisher) Publish(_ context.Context, channel string, data []byte) error {
	p.calls = append(p.calls, publishCall{channel: channel, data: append([]byte(nil), data...)})
	return nil
}

func (p *cacheInvalidationPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := p.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func (p *cacheInvalidationPublisher) Subscribe(context.Context, string) (*pubsub.Subscription, error) {
	ch := make(chan []byte)
	close(ch)
	return pubsub.NewSubscription(ch, func() {}), nil
}

func (p *cacheInvalidationPublisher) Close() error { return nil }

func TestCacheInvalidationHandler_PublishesTargetedInvalidations(t *testing.T) {
	t.Parallel()

	publisher := &cacheInvalidationPublisher{}
	bus := straitcache.NewBus(publisher, straitcache.BusConfig{Origin: "cdc-test"})
	handlers := NewCacheInvalidationHandlers(bus, nil)
	byTable := make(map[string]Handler, len(handlers))
	for _, h := range handlers {
		byTable[h.Table()] = h
	}

	cases := []struct {
		table     string
		record    string
		namespace string
		key       string
	}{
		{table: "api_keys", record: `{"key_hash":"hash-1","cache_version":8}`, namespace: cacheNamespaceAPIKeyAuth, key: "hash-1"},
		{table: "project_roles", record: `{"project_id":"proj-1","cache_version":9}`, namespace: cacheNamespacePermissionProj, key: "proj-1"},
		{table: "project_member_roles", record: `{"project_id":"proj-1","user_id":"user-1","cache_version":10}`, namespace: cacheNamespacePermission, key: permissionCacheKey("proj-1", "user-1")},
		{table: "project_quotas", record: `{"project_id":"proj-1","cache_version":11}`, namespace: cacheNamespaceQuota, key: "proj-1"},
		{table: "organization_subscriptions", record: `{"org_id":"org-1","cache_version":12}`, namespace: cacheNamespaceBillingOrg, key: "org-1"},
		{table: "jobs", record: `{"id":"job-1","cache_version":13}`, namespace: cacheNamespaceWorkerJob, key: "job-1"},
		{table: "job_dependencies", record: `{"job_id":"job-1","cache_version":14}`, namespace: cacheNamespaceJobDependency, key: jobDependencyCacheKey("job-1", defaultJobDependencyListSize)},
	}

	for _, tc := range cases {
		h := byTable[tc.table]
		if h == nil {
			t.Fatalf("handler for %s missing", tc.table)
		}
		if err := h.Handle(context.Background(), Message{Action: ActionUpdate, Record: []byte(tc.record), Metadata: Metadata{TableName: tc.table}}); err != nil {
			t.Fatalf("%s Handle() error = %v", tc.table, err)
		}
		var msg straitcache.BusMessage
		if err := json.Unmarshal(publisher.calls[len(publisher.calls)-1].data, &msg); err != nil {
			t.Fatalf("%s bus message decode: %v", tc.table, err)
		}
		if msg.Action != straitcache.BusActionInvalidate || msg.Namespace != tc.namespace || msg.Key != tc.key {
			t.Fatalf("%s message = %+v, want namespace %q key %q", tc.table, msg, tc.namespace, tc.key)
		}
	}
}

func TestCacheInvalidationHandler_SkipsRowsWithoutAddressableKey(t *testing.T) {
	t.Parallel()

	publisher := &cacheInvalidationPublisher{}
	bus := straitcache.NewBus(publisher, straitcache.BusConfig{Origin: "cdc-test"})
	h := newCacheInvalidationHandler("api_keys", bus, nil, invalidateAPIKeyCache)

	if err := h.Handle(context.Background(), Message{Action: ActionUpdate, Record: []byte(`{"id":"key-1"}`)}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if len(publisher.calls) != 0 {
		t.Fatalf("published %d messages, want 0", len(publisher.calls))
	}
}
