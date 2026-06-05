package webhook

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

// fakeProjectLister returns a fixed slice of project IDs for the given org.
// Any other org returns the empty slice.
type fakeProjectLister struct {
	byOrg map[string][]string
	err   error
}

func (f *fakeProjectLister) ListProjectsByOrg(_ context.Context, orgID string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byOrg[orgID], nil
}

// fakeSubscriptionLister returns a fixed slice of subscriptions for the given
// project. An err per project lets us simulate the "one project errors,
// others succeed" continuation behaviour.
type fakeSubscriptionLister struct {
	byProject map[string][]domain.WebhookSubscription
	errProj   map[string]error
}

func (f *fakeSubscriptionLister) ListWebhookSubscriptions(_ context.Context, projectID string) ([]domain.WebhookSubscription, error) {
	if err, ok := f.errProj[projectID]; ok {
		return nil, err
	}
	return f.byProject[projectID], nil
}

func newDispatcherFixture(t *testing.T, projects map[string][]string, subs map[string][]domain.WebhookSubscription) (*BillingDispatcher, *mockDeliveryStore) {
	t.Helper()
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())
	d := NewBillingDispatcher(
		worker,
		&fakeProjectLister{byOrg: projects},
		&fakeSubscriptionLister{byProject: subs},
		slog.Default(),
	)
	return d, ms
}

func TestBillingDispatcher_NoProjects_NoDeliveries(t *testing.T) {
	t.Parallel()

	d, ms := newDispatcherFixture(t, map[string][]string{}, nil)
	require.NoError(t,
		d.DispatchBillingEvent(context.
			Background(), "org-empty", domain.WebhookEventBillingCapWarning,

			[]byte(`{"x":1}`)))
	require.EqualValues(t, 0, len(ms.
		getDeliveries()))

}

func TestBillingDispatcher_FansOutToMatchingProjectsAndSubs(t *testing.T) {
	t.Parallel()

	orgID := "org-1"
	subs := map[string][]domain.WebhookSubscription{
		"proj-a": {
			{ID: "s-a1", ProjectID: "proj-a", WebhookURL: "https://a1.example.com",
				Active: true, EventTypes: []string{domain.WebhookEventBillingCapWarning}},
			{ID: "s-a2", ProjectID: "proj-a", WebhookURL: "https://a2.example.com",
				Active: true, EventTypes: []string{"run.completed"}},
			{ID: "s-a3", ProjectID: "proj-a", WebhookURL: "https://a3.example.com",
				Active: false, EventTypes: []string{domain.WebhookEventBillingCapWarning}},
		},
		"proj-b": {
			{ID: "s-b1", ProjectID: "proj-b", WebhookURL: "https://b1.example.com",
				Active: true, EventTypes: []string{"*"}},
		},
	}
	d, ms := newDispatcherFixture(t, map[string][]string{orgID: {"proj-a", "proj-b"}}, subs)

	payload := []byte(`{"detail":"warn"}`)
	require.NoError(t,
		d.DispatchBillingEvent(context.
			Background(), orgID, domain.WebhookEventBillingCapWarning,

			payload))

	got := ms.getDeliveries()
	if len(got) != 2 {
		ids := make([]string, 0, len(got))
		for _, dl := range got {
			ids = append(ids, dl.SubscriptionID)
		}
		require.Failf(t, "test failure", "expected 2 deliveries (s-a1, s-b1 wildcard), got %d: %v", len(got), ids)
	}

	subIDs := []string{got[0].SubscriptionID, got[1].SubscriptionID}
	sort.Strings(subIDs)
	require.False(t,
		subIDs[0] !=
			"s-a1" ||
			subIDs[1] != "s-b1",
	)

	for _, dl := range got {
		require.Equal(t,
			string(payload), string(dl.Payload))
		require.Equal(t,
			domain.WebhookStatusPending,

			dl.
				Status)
		require.NotNil(t,
			dl.NextRetryAt,
		)
		require.NotEqual(
			t, "", dl.
				RetryPolicy)
		require.NotEqual(
			t, "", dl.
				WebhookURL)

	}
}

// matchesEventType is currently exact-match-or-star. A literal "billing.*" in
// event_types should NOT silently match every billing event — that would be
// an overbroad wildcard expansion and would break customer intent. This test
// is the regression guard for that behaviour.
func TestBillingDispatcher_LiteralGlobIsNotInterpreted(t *testing.T) {
	t.Parallel()

	orgID := "org-glob"
	subs := map[string][]domain.WebhookSubscription{
		"proj-a": {
			{ID: "s-glob", ProjectID: "proj-a", WebhookURL: "https://g.example.com",
				Active: true, EventTypes: []string{"billing.*"}},
		},
	}
	d, ms := newDispatcherFixture(t, map[string][]string{orgID: {"proj-a"}}, subs)
	require.NoError(t,
		d.DispatchBillingEvent(context.
			Background(), orgID, domain.WebhookEventBillingCapWarning,

			[]byte(`{}`)))
	require.EqualValues(t, 0, len(ms.
		getDeliveries()))

}

func TestBillingDispatcher_PerProjectErrorDoesNotAbortFanout(t *testing.T) {
	t.Parallel()

	orgID := "org-mixed"
	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())
	d := NewBillingDispatcher(
		worker,
		&fakeProjectLister{byOrg: map[string][]string{orgID: {"proj-bad", "proj-good"}}},
		&fakeSubscriptionLister{
			byProject: map[string][]domain.WebhookSubscription{
				"proj-good": {{ID: "s-g", ProjectID: "proj-good", WebhookURL: "https://g.example.com",
					Active: true, EventTypes: []string{domain.WebhookEventBillingCapReached}}},
			},
			errProj: map[string]error{"proj-bad": errors.New("simulated db error")},
		},
		slog.Default(),
	)
	require.NoError(t,
		d.DispatchBillingEvent(context.
			Background(), orgID, domain.WebhookEventBillingCapReached,

			[]byte(`{}`)))

	got := ms.getDeliveries()
	require.False(t,
		len(got) !=
			1 || got[0].SubscriptionID !=

			"s-g")

}

func TestBillingDispatcher_ProjectsLookupError_PropagatesAsError(t *testing.T) {
	t.Parallel()

	ms := &mockDeliveryStore{}
	worker := NewDeliveryWorker(ms, slog.Default())
	d := NewBillingDispatcher(
		worker,
		&fakeProjectLister{err: errors.New("db down")},
		&fakeSubscriptionLister{},
		slog.Default(),
	)

	err := d.DispatchBillingEvent(context.Background(), "org-x", domain.WebhookEventBillingSuspended, []byte(`{}`))
	require.Error(t,
		err)

}

func TestBillingDispatcher_RejectsEmptyArgs(t *testing.T) {
	t.Parallel()

	d, _ := newDispatcherFixture(t, map[string][]string{}, nil)
	require.Error(t,
		d.DispatchBillingEvent(context.
			Background(), "", "billing.cap_warning",

			[]byte(`{}`)))
	require.Error(t,
		d.DispatchBillingEvent(context.
			Background(), "org-1", "", []byte(`{}`),
		),
	)

}
