package webhook

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"testing"

	"strait/internal/domain"
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

	if err := d.DispatchBillingEvent(context.Background(), "org-empty", domain.WebhookEventBillingCapWarning, []byte(`{"x":1}`)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := len(ms.getDeliveries()); got != 0 {
		t.Fatalf("expected 0 deliveries, got %d", got)
	}
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
	if err := d.DispatchBillingEvent(context.Background(), orgID, domain.WebhookEventBillingCapWarning, payload); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	got := ms.getDeliveries()
	if len(got) != 2 {
		var ids []string
		for _, dl := range got {
			ids = append(ids, dl.SubscriptionID)
		}
		t.Fatalf("expected 2 deliveries (s-a1, s-b1 wildcard), got %d: %v", len(got), ids)
	}

	subIDs := []string{got[0].SubscriptionID, got[1].SubscriptionID}
	sort.Strings(subIDs)
	if subIDs[0] != "s-a1" || subIDs[1] != "s-b1" {
		t.Fatalf("expected s-a1 and s-b1, got %v", subIDs)
	}

	for _, dl := range got {
		if string(dl.Payload) != string(payload) {
			t.Fatalf("delivery %s payload = %s, want %s", dl.SubscriptionID, dl.Payload, payload)
		}
		if dl.Status != domain.WebhookStatusPending {
			t.Fatalf("delivery %s status = %s, want pending", dl.SubscriptionID, dl.Status)
		}
		if dl.NextRetryAt == nil {
			t.Fatalf("delivery %s next_retry_at is nil; poll loop would never claim it", dl.SubscriptionID)
		}
		if dl.RetryPolicy == "" {
			t.Fatalf("delivery %s retry_policy is empty", dl.SubscriptionID)
		}
		if dl.WebhookURL == "" {
			t.Fatalf("delivery %s webhook_url is empty", dl.SubscriptionID)
		}
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

	if err := d.DispatchBillingEvent(context.Background(), orgID, domain.WebhookEventBillingCapWarning, []byte(`{}`)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if got := len(ms.getDeliveries()); got != 0 {
		t.Fatalf("literal 'billing.*' should not match 'billing.cap_warning'; got %d deliveries", got)
	}
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

	if err := d.DispatchBillingEvent(context.Background(), orgID, domain.WebhookEventBillingCapReached, []byte(`{}`)); err != nil {
		t.Fatalf("dispatch returned err despite per-project failure being skippable: %v", err)
	}
	got := ms.getDeliveries()
	if len(got) != 1 || got[0].SubscriptionID != "s-g" {
		t.Fatalf("expected delivery to proj-good's sub, got %+v", got)
	}
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
	if err == nil {
		t.Fatal("expected error when project lookup fails")
	}
}

func TestBillingDispatcher_RejectsEmptyArgs(t *testing.T) {
	t.Parallel()

	d, _ := newDispatcherFixture(t, map[string][]string{}, nil)

	if err := d.DispatchBillingEvent(context.Background(), "", "billing.cap_warning", []byte(`{}`)); err == nil {
		t.Fatal("expected error for empty orgID")
	}
	if err := d.DispatchBillingEvent(context.Background(), "org-1", "", []byte(`{}`)); err == nil {
		t.Fatal("expected error for empty eventType")
	}
}
