package cdc

import (
	"context"
	"encoding/json"
	"testing"

	straitcache "strait/internal/cache"
	"strait/internal/pubsub"

	"github.com/stretchr/testify/require"
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
		{
			table:     "api_keys",
			record:    `{"key_hash":"hash-1","cache_version":8}`,
			namespace: cacheNamespaceAPIKeyAuth,
			key:       "hash-1",
		},
		{
			table:     "project_roles",
			record:    `{"project_id":"proj-1","cache_version":9}`,
			namespace: cacheNamespacePermissionProj,
			key:       "proj-1",
		},
		{
			table:     "project_member_roles",
			record:    `{"project_id":"proj-1","user_id":"user-1","cache_version":10}`,
			namespace: cacheNamespacePermission,
			key:       permissionCacheKey("proj-1", "user-1"),
		},
		{
			table:     "project_quotas",
			record:    `{"project_id":"proj-1","cache_version":11}`,
			namespace: cacheNamespaceQuota,
			key:       "proj-1",
		},
		{
			table:     "organization_subscriptions",
			record:    `{"org_id":"org-1","cache_version":12}`,
			namespace: cacheNamespaceBillingOrg,
			key:       "org-1",
		},
		{
			table:     "jobs",
			record:    `{"id":"job-1","cache_version":13}`,
			namespace: cacheNamespaceWorkerJob,
			key:       "job-1",
		},
		{
			table:     "job_dependencies",
			record:    `{"job_id":"job-1","cache_version":14}`,
			namespace: cacheNamespaceJobDependency,
			key:       jobDependencyCacheKey("job-1", defaultJobDependencyListSize),
		},
	}

	for _, tc := range cases {
		h := byTable[tc.table]
		require.NotNil(t, h)

		request := Message{
			Action:   ActionUpdate,
			Record:   []byte(tc.record),
			Metadata: Metadata{TableName: tc.table},
		}
		require.NoError(t, h.Handle(t.
			Context(),
			request,
		))

		var busMsg straitcache.BusMessage
		require.NoError(t, json.Unmarshal(publisher.
			calls[len(publisher.calls)-1].data,
			&busMsg))
		require.Equal(t, straitcache.BusActionInvalidate, busMsg.Action)
		require.Equal(t, tc.namespace, busMsg.Namespace)
		require.Equal(t, tc.key, busMsg.Key)
	}
}

func TestCacheInvalidationHandler_SkipsRowsWithoutAddressableKey(t *testing.T) {
	t.Parallel()

	publisher := &cacheInvalidationPublisher{}
	bus := straitcache.NewBus(publisher, straitcache.BusConfig{Origin: "cdc-test"})
	h := newCacheInvalidationHandler("api_keys", bus, nil, invalidateAPIKeyCache)
	require.NoError(t, h.Handle(t.
		Context(),
		Message{Action: ActionUpdate, Record: []byte(`{"id":"key-1"}`)}),
	)
	require.Empty(t,
		publisher.calls)
}

func TestCacheInvalidationHandlerCanProcess(t *testing.T) {
	t.Parallel()

	publisher := &cacheInvalidationPublisher{}
	bus := straitcache.NewBus(publisher, straitcache.BusConfig{Origin: "cdc-test"})

	tests := []struct {
		name    string
		handler *cacheInvalidationHandler
		msg     Message
		want    bool
	}{
		{
			name:    "nil handler",
			handler: nil,
			msg:     Message{Record: []byte(`{"key_hash":"hash-1"}`)},
			want:    false,
		},
		{
			name: "missing bus",
			handler: &cacheInvalidationHandler{
				fn: invalidateAPIKeyCache,
			},
			msg:  Message{Record: []byte(`{"key_hash":"hash-1"}`)},
			want: false,
		},
		{
			name: "missing invalidation function",
			handler: &cacheInvalidationHandler{
				bus: bus,
			},
			msg:  Message{Record: []byte(`{"key_hash":"hash-1"}`)},
			want: false,
		},
		{
			name: "empty record",
			handler: &cacheInvalidationHandler{
				bus: bus,
				fn:  invalidateAPIKeyCache,
			},
			msg:  Message{},
			want: false,
		},
		{
			name: "ready",
			handler: &cacheInvalidationHandler{
				bus: bus,
				fn:  invalidateAPIKeyCache,
			},
			msg:  Message{Record: []byte(`{"key_hash":"hash-1"}`)},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, cacheInvalidationHandlerCanProcess(tt.handler, tt.msg))
		})
	}
}

func TestPermissionCacheRecordAddressable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		projectID string
		userID    string
		want      bool
	}{
		{
			name:      "project and user",
			projectID: "proj-1",
			userID:    "user-1",
			want:      true,
		},
		{
			name:      "missing project",
			projectID: "",
			userID:    "user-1",
			want:      false,
		},
		{
			name:      "missing user",
			projectID: "proj-1",
			userID:    "",
			want:      false,
		},
		{
			name:      "missing both",
			projectID: "",
			userID:    "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, permissionCacheRecordAddressable(tt.projectID, tt.userID))
		})
	}
}

func TestCacheInvalidationHandler_DeletePublishesVersionedBarrier(t *testing.T) {
	t.Parallel()

	publisher := &cacheInvalidationPublisher{}
	bus := straitcache.NewBus(publisher, straitcache.BusConfig{Origin: "cdc-test"})
	h := newCacheInvalidationHandler("jobs", bus, nil, invalidateWorkerJobCache)

	request := Message{
		Action: ActionDelete,
		Record: []byte(`{"id":"job-1","cache_version":19}`),
	}
	require.NoError(t, h.Handle(t.
		Context(),
		request,
	))
	require.Len(t,
		publisher.calls,
		1)

	var busMsg straitcache.BusMessage
	require.NoError(t, json.Unmarshal(publisher.
		calls[0].data, &busMsg))

	gotWorkerJobBarrier := busMsg.Action == straitcache.BusActionInvalidate &&
		busMsg.Namespace == cacheNamespaceWorkerJob &&
		busMsg.Key == "job-1" &&
		busMsg.Version == 19
	require.True(
		t, gotWorkerJobBarrier,
	)
}

func TestCacheInvalidationHandler_BadPayloadIsIgnored(t *testing.T) {
	t.Parallel()

	publisher := &cacheInvalidationPublisher{}
	bus := straitcache.NewBus(publisher, straitcache.BusConfig{Origin: "cdc-test"})
	h := newCacheInvalidationHandler("api_keys", bus, nil, invalidateAPIKeyCache)
	require.NoError(t, h.Handle(t.
		Context(),
		Message{Action: ActionUpdate, Record: []byte(`{"key_hash":`)}))
	require.Empty(t,
		publisher.calls)
}
