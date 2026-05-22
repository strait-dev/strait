package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

type mockBudgetTestStore struct {
	mockBillingStore
	budget    int64
	action    string
	budgetErr error
	spend     int64
	spendErr  error
	setBudget int64
	setAction string
}

func (m *mockBudgetTestStore) GetProjectBudget(_ context.Context, _ string) (int64, string, error) {
	return m.budget, m.action, m.budgetErr
}

func (m *mockBudgetTestStore) GetProjectPeriodSpend(_ context.Context, _ string, _ time.Time) (int64, error) {
	return m.spend, m.spendErr
}

func (m *mockBudgetTestStore) SetProjectBudget(_ context.Context, _ string, budget int64, action string) error {
	m.setBudget = budget
	m.setAction = action
	return nil
}

func TestSetProjectBudget_Valid_Stores(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	err := svc.SetProjectBudget(context.Background(), "proj-1", 50000000, "reject")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store.setBudget != 50000000 {
		t.Errorf("expected stored budget 50000000, got %d", store.setBudget)
	}
	if store.setAction != "reject" {
		t.Errorf("expected stored action reject, got %s", store.setAction)
	}
}

func TestSetProjectBudget_InvalidAction_Rejects(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	err := svc.SetProjectBudget(context.Background(), "proj-1", 50000000, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestSetProjectBudget_NegativeBudget_SetsUnlimited(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	err := svc.SetProjectBudget(context.Background(), "proj-1", -5, "notify")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store.setBudget != -1 {
		t.Errorf("expected stored budget -1 (unlimited), got %d", store.setBudget)
	}
}
