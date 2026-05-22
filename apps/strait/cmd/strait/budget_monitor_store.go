package main

import (
	"context"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
	"strait/internal/store"
)

type budgetMonitorStore struct {
	billing *billing.PgStore
	queries *store.Queries
}

func newBudgetMonitorStore(billingStore *billing.PgStore, queries *store.Queries) *budgetMonitorStore {
	return &budgetMonitorStore{billing: billingStore, queries: queries}
}

func (s *budgetMonitorStore) ListAllSubscribedOrgIDs(ctx context.Context) ([]string, error) {
	return s.billing.ListAllSubscribedOrgIDs(ctx)
}

func (s *budgetMonitorStore) GetOrgSubscription(ctx context.Context, orgID string) (*billing.OrgSubscription, error) {
	return s.billing.GetOrgSubscription(ctx, orgID)
}

func (s *budgetMonitorStore) SumOrgPeriodSpend(ctx context.Context, orgID string, from time.Time) (int64, error) {
	return s.billing.SumOrgPeriodSpend(ctx, orgID, from)
}

func (s *budgetMonitorStore) ListProjectsByOrg(ctx context.Context, orgID string) ([]string, error) {
	return s.billing.ListProjectsByOrg(ctx, orgID)
}

func (s *budgetMonitorStore) ListEnabledNotificationChannels(ctx context.Context, projectID string) ([]domain.NotificationChannel, error) {
	return s.queries.ListEnabledNotificationChannels(ctx, projectID)
}

func (s *budgetMonitorStore) ListEnabledNotificationChannelsByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]domain.NotificationChannel, error) {
	return s.queries.ListEnabledNotificationChannelsByProjectIDs(ctx, projectIDs)
}

func (s *budgetMonitorStore) CreateNotificationDelivery(ctx context.Context, d *domain.NotificationDelivery) error {
	return s.queries.CreateNotificationDelivery(ctx, d)
}
