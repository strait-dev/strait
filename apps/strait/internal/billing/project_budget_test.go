package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		err)
	assert.EqualValues(t, 50000000,
		store.
			setBudget,
	)
	assert.Equal(t, "reject",
		store.
			setAction,
	)
}

func TestSetProjectBudget_InvalidAction_Rejects(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	err := svc.SetProjectBudget(context.Background(), "proj-1", 50000000, "invalid")
	require.Error(t,
		err)
}

func TestSetProjectBudget_NegativeBudget_SetsUnlimited(t *testing.T) {
	t.Parallel()
	store := &mockBudgetTestStore{}
	e := NewEnforcer(store, nil, slog.Default())
	svc := NewUsageService(store, e)

	err := svc.SetProjectBudget(context.Background(), "proj-1", -5, "notify")
	require.NoError(t,
		err)
	assert.EqualValues(t, -1, store.setBudget)
}
