package billing

import (
	"context"
	"errors"
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

func TestCheckProjectBudget_NoBudget_Passes(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: -1, action: "notify"}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected nil error for no-budget project, got %v", err)
	}
}

func TestCheckProjectBudget_UnderBudget_Passes(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: 100000, action: "reject", spend: 50000}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected nil error when under budget, got %v", err)
	}
}

func TestCheckProjectBudget_OverBudget_RejectAction_Rejects(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: 100000, action: "reject", spend: 100000}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error when at budget with reject action")
	}

	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "project_budget_reached" {
		t.Errorf("expected code project_budget_reached, got %s", le.Code)
	}
}

func TestCheckProjectBudget_OverBudget_NotifyAction_Passes(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: 100000, action: "notify", spend: 100000}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected nil error with notify action, got %v", err)
	}
}

func TestCheckProjectBudget_AtExactly80Pct_NoReject(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: 100000, action: "reject", spend: 80000}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected nil error at 80%% of budget, got %v", err)
	}
}

func TestCheckProjectBudget_ZeroBudget_RejectAction_Rejects(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: 0, action: "reject", spend: 500}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err == nil {
		t.Fatal("expected error when budget is zero with reject action")
	}

	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected LimitError, got %T: %v", err, err)
	}
	if le.Code != "project_budget_reached" {
		t.Errorf("expected code project_budget_reached, got %s", le.Code)
	}
}

func TestCheckProjectBudget_StoreError_FailOpen(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budgetErr: errors.New("db down")}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected nil error on store failure (fail open), got %v", err)
	}
}

func TestCheckProjectBudget_SpendError_FailOpen(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{budget: 100000, action: "reject", spendErr: errors.New("query failed")}
	e := NewEnforcer(store, nil, slog.Default())

	err := e.CheckProjectBudgetLimit(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("expected nil error on spend query failure (fail open), got %v", err)
	}
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
