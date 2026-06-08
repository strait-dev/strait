//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
)

func TestCreateWebhookSubscription(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-create-ws-" + newID(),
		WebhookURL: "https://example.com/hook",
		EventTypes: []string{"run.completed", "run.failed"},
		Secret:     "whsec_test",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))
	require.NotEqual(t, "",

		sub.ID)
	require.False(t, sub.CreatedAt.
		IsZero())

	// Verify all fields round-trip.
	got, err := q.GetWebhookSubscription(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, sub.ID,

		got.ID)
	require.Equal(t, sub.ProjectID,

		got.
			ProjectID,
	)
	require.Equal(t, sub.WebhookURL,

		got.WebhookURL,
	)
	require.Len(t, got.EventTypes,

		len(sub.EventTypes))

	for i, et := range got.EventTypes {
		require.Equal(t, sub.EventTypes[i], et)

	}
	require.Equal(t, sub.Secret,

		got.
			Secret)
	require.Equal(t, sub.Active,

		got.
			Active)

}

func TestCreateWebhookSubscriptionWithOrgLimit_ConcurrentCreatesCannotExceedLimit(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-limit-" + newID()
	projectID := "proj-webhook-limit-" + newID()
	require.NoError(t, q.CreateProject(ctx, &domain.
		Project{ID: projectID,

		OrgID: orgID,
		Name:  "Webhook Limit",
	}))

	const maxEndpoints = 3
	const attempts = 16

	start := make(chan struct{})
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		{
			i := i
			concWG.Go(func() {
				defer wg.Done()
				<-start
				sub := &domain.WebhookSubscription{
					ID:         "sub-webhook-limit-" + newID(),
					ProjectID:  projectID,
					WebhookURL: "https://example.com/hook/" + newID(),
					EventTypes: []string{"run.completed"},
					Secret:     "whsec_test",
					Active:     true,
				}
				if i%2 == 0 {
					sub.EventTypes = []string{"run.failed"}
				}
				errs <- q.CreateWebhookSubscriptionWithOrgLimit(ctx, sub, orgID, maxEndpoints)
			})
		}
	}
	close(start)
	wg.Wait()
	close(errs)

	var created, limited int
	for err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, store.ErrWebhookEndpointLimitExceeded):
			limited++
		default:
			require.Failf(t, "test failure", "unexpected create error: %v", err)
		}
	}
	require.Equal(t, maxEndpoints,

		created,
	)
	require.Equal(t, attempts-
		maxEndpoints,
		limited,
	)

	count, err := q.CountWebhookSubscriptionsByOrg(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, maxEndpoints,

		count,
	)

}

func TestCountWebhookSubscriptionsByOrg_IncludesSiblingProjectsUnderProjectRLS(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-rls-count-" + newID()
	projectA := "proj-webhook-rls-count-a-" + newID()
	projectB := "proj-webhook-rls-count-b-" + newID()
	for _, projectID := range []string{projectA, projectB} {
		require.NoError(t, q.CreateProject(ctx, &domain.
			Project{ID: projectID,

			OrgID: orgID,
			Name:  projectID},
		),
		)

	}

	for _, projectID := range []string{projectA, projectB} {
		sub := &domain.WebhookSubscription{
			ID:         "sub-webhook-rls-count-" + newID(),
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook/" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "whsec_test",
			Active:     true,
		}
		require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	}

	var count int
	var err error
	runAsProject(t, ctx, projectA, false, func(txq *store.Queries) {
		count, err = txq.CountWebhookSubscriptionsByOrg(ctx, orgID)
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

}

func TestCreateWebhookSubscriptionWithOrgLimit_CountsSiblingProjectsUnderProjectRLS(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	orgID := "org-webhook-rls-limit-" + newID()
	projectA := "proj-webhook-rls-limit-a-" + newID()
	projectB := "proj-webhook-rls-limit-b-" + newID()
	for _, projectID := range []string{projectA, projectB} {
		require.NoError(t, q.CreateProject(ctx, &domain.
			Project{ID: projectID,

			OrgID: orgID,
			Name:  projectID},
		),
		)

	}

	existing := &domain.WebhookSubscription{
		ID:         "sub-webhook-rls-existing-" + newID(),
		ProjectID:  projectB,
		WebhookURL: "https://example.com/hook/existing",
		EventTypes: []string{"run.completed"},
		Secret:     "whsec_test",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, existing))

	candidate := &domain.WebhookSubscription{
		ID:         "sub-webhook-rls-candidate-" + newID(),
		ProjectID:  projectA,
		WebhookURL: "https://example.com/hook/candidate",
		EventTypes: []string{"run.failed"},
		Secret:     "whsec_test",
		Active:     true,
	}

	var err error
	runAsProject(t, ctx, projectA, false, func(txq *store.Queries) {
		err = txq.CreateWebhookSubscriptionWithOrgLimit(ctx, candidate, orgID, 1)
	})
	require.True(t, errors.Is(err, store.
		ErrWebhookEndpointLimitExceeded,
	))

}

func TestWebhookDeliverySubscriptionPayloadRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-wd-sub-" + newID(),
		WebhookURL: "https://example.com/hook",
		EventTypes: []string{"run.completed"},
		Secret:     "whsec_test",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	payload := json.RawMessage(`{"run_id":"run-1","status":"completed"}`)
	delivery := &domain.WebhookDelivery{
		SubscriptionID: sub.ID,
		WebhookURL:     sub.WebhookURL,
		Status:         domain.WebhookStatusPending,
		Attempts:       0,
		MaxAttempts:    3,
		Payload:        payload,
		LastError:      string(payload),
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx,
		delivery))

	got, err := q.GetWebhookDelivery(ctx, sub.ProjectID, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, sub.ID,

		got.SubscriptionID,
	)
	require.Equal(t, sub.ProjectID,

		got.
			ProjectID,
	)
	require.True(t, jsonPayloadEqual(
		got.Payload,
		payload))

	replay, err := q.ReplayWebhookDelivery(ctx, sub.ProjectID, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, sub.ID,

		replay.SubscriptionID,
	)
	require.True(t, jsonPayloadEqual(
		replay.Payload,
		payload))

}

func jsonPayloadEqual(a, b json.RawMessage) bool {
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func TestCreateWebhookSubscription_CustomID(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	customID := newID()
	sub := &domain.WebhookSubscription{
		ID:         customID,
		ProjectID:  "proj-ws-custom-id-" + newID(),
		WebhookURL: "https://example.com/custom",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))
	require.Equal(t, customID,

		sub.ID,
	)

}

func TestGetWebhookSubscription_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetWebhookSubscription(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrWebhookSubscriptionNotFound,
	))

}

func TestListWebhookSubscriptions(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-list-ws-" + newID()
	otherProjectID := "proj-list-ws-other-" + newID()

	// Create two active subscriptions.
	for range 2 {
		sub := &domain.WebhookSubscription{
			ProjectID:  projectID,
			WebhookURL: "https://example.com/hook-" + newID(),
			EventTypes: []string{"run.completed"},
			Secret:     "s",
			Active:     true,
		}
		require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	}

	// Create an inactive subscription (excluded from list).
	inactive := &domain.WebhookSubscription{
		ProjectID:  projectID,
		WebhookURL: "https://example.com/inactive",
		EventTypes: []string{"run.failed"},
		Secret:     "s",
		Active:     false,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, inactive))

	// Create one in another project.
	other := &domain.WebhookSubscription{
		ProjectID:  otherProjectID,
		WebhookURL: "https://example.com/other",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, other))

	subs, err := q.ListWebhookSubscriptions(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, subs, 2)

	for _, s := range subs {
		require.Equal(t, projectID,

			s.ProjectID,
		)
		require.True(t, s.Active)

	}

	// Empty project.
	empty, err := q.ListWebhookSubscriptions(ctx, "proj-ws-empty-"+newID())
	require.NoError(t, err)
	require.Len(t, empty, 0)

}

func TestDeleteWebhookSubscription(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-delete-ws-" + newID(),
		WebhookURL: "https://example.com/del",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))
	require.NoError(t, q.DeleteWebhookSubscription(ctx, sub.ID))

	// Should be gone.
	_, err := q.GetWebhookSubscription(ctx, sub.ID)
	require.True(t, errors.Is(err, store.
		ErrWebhookSubscriptionNotFound,
	))

}

func TestDeleteWebhookSubscription_WithDeliveriesDetachesHistory(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	sub := &domain.WebhookSubscription{
		ProjectID:  "proj-delete-ws-delivery-" + newID(),
		WebhookURL: "https://example.com/del-with-history",
		EventTypes: []string{"run.completed"},
		Secret:     "s",
		Active:     true,
	}
	require.NoError(t, q.CreateWebhookSubscription(ctx, sub))

	delivery := &domain.WebhookDelivery{
		SubscriptionID: sub.ID,
		WebhookURL:     sub.WebhookURL,
		Status:         domain.WebhookStatusDelivered,
		Attempts:       1,
		MaxAttempts:    3,
		Payload:        json.RawMessage(`{"status":"completed"}`),
	}
	require.NoError(t, q.CreateWebhookDelivery(ctx,
		delivery))
	require.NoError(t, q.DeleteWebhookSubscription(ctx, sub.ID))

	if _, err := q.GetWebhookSubscription(ctx, sub.ID); !errors.Is(err, store.ErrWebhookSubscriptionNotFound) {
		require.Failf(t, "test failure",

			"GetWebhookSubscription(deleted) error = %v, want ErrWebhookSubscriptionNotFound", err)
	}
	gotDelivery, err := q.GetWebhookDelivery(ctx, sub.ProjectID, delivery.ID)
	require.NoError(t, err)
	require.Equal(t, "", gotDelivery.
		SubscriptionID,
	)
	require.Equal(t, sub.ProjectID,

		gotDelivery.
			ProjectID)

}

func TestDeleteWebhookSubscription_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	err := q.DeleteWebhookSubscription(ctx, newID())
	require.True(t, errors.Is(err, store.
		ErrWebhookSubscriptionNotFound,
	))

}
