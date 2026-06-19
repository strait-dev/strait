package billing

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type fakeBillingTxBeginner struct {
	tx  *fakeBillingTx
	err error
}

func (f fakeBillingTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tx, nil
}

type fakeBillingTx struct {
	commitErr   error
	rollbackErr error
	execResults []fakeBillingExecResult
	commitCalls int
	rollbacks   int
	execCalls   int
}

type fakeBillingExecResult struct {
	rows int64
	err  error
}

func (f *fakeBillingTx) Begin(context.Context) (pgx.Tx, error) { return f, nil }

func (f *fakeBillingTx) Commit(context.Context) error {
	f.commitCalls++
	return f.commitErr
}

func (f *fakeBillingTx) Rollback(context.Context) error {
	f.rollbacks++
	return f.rollbackErr
}

func (f *fakeBillingTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (f *fakeBillingTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }

func (f *fakeBillingTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }

func (f *fakeBillingTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (f *fakeBillingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	f.execCalls++
	if f.execCalls > len(f.execResults) {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	result := f.execResults[f.execCalls-1]
	return pgconn.NewCommandTag("UPDATE " + strconv.FormatInt(result.rows, 10)), result.err
}

func (f *fakeBillingTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }

func (f *fakeBillingTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (f *fakeBillingTx) Conn() *pgx.Conn { return nil }

func TestWithBillingTx(t *testing.T) {
	t.Parallel()

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()

		err := withBillingTx(context.Background(), fakeBillingTxBeginner{err: errors.New("begin failed")}, func(pgx.Tx) error {
			require.Fail(t, "callback should not run")
			return nil
		})
		require.ErrorContains(t, err, "begin billing tx")
	})

	t.Run("callback error rolls back", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{}
		err := withBillingTx(context.Background(), fakeBillingTxBeginner{tx: tx}, func(pgx.Tx) error {
			return errors.New("callback failed")
		})
		require.ErrorContains(t, err, "callback failed")
		require.Equal(t, 1, tx.rollbacks)
		require.Zero(t, tx.commitCalls)
	})

	t.Run("commit error rolls back", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{commitErr: errors.New("commit failed")}
		err := withBillingTx(context.Background(), fakeBillingTxBeginner{tx: tx}, func(pgx.Tx) error {
			return nil
		})
		require.ErrorContains(t, err, "commit billing tx")
		require.Equal(t, 1, tx.commitCalls)
		require.Equal(t, 1, tx.rollbacks)
	})

	t.Run("success commits without rollback", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{}
		require.NoError(t, withBillingTx(context.Background(), fakeBillingTxBeginner{tx: tx}, func(pgx.Tx) error {
			return nil
		}))
		require.Equal(t, 1, tx.commitCalls)
		require.Zero(t, tx.rollbacks)
	})
}

func TestRestrictOrgInTx(t *testing.T) {
	t.Parallel()

	t.Run("success updates payment plan and entitlements", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{execResults: []fakeBillingExecResult{
			{rows: 1},
			{rows: 1},
			{rows: 1},
		}}
		require.NoError(t, restrictOrgInTx(context.Background(), tx, "org-1", nil))
		require.Equal(t, 3, tx.execCalls)
	})

	t.Run("payment status update error", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{execResults: []fakeBillingExecResult{{err: errors.New("payment failed")}}}
		err := restrictOrgInTx(context.Background(), tx, "org-1", nil)
		require.ErrorContains(t, err, "restricting org payment status")
		require.Equal(t, 1, tx.execCalls)
	})

	t.Run("payment status missing row", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{execResults: []fakeBillingExecResult{{rows: 0}}}
		require.ErrorIs(t, restrictOrgInTx(context.Background(), tx, "org-1", nil), ErrSubscriptionNotFound)
		require.Equal(t, 1, tx.execCalls)
	})

	t.Run("plan downgrade error", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{execResults: []fakeBillingExecResult{
			{rows: 1},
			{err: errors.New("downgrade failed")},
		}}
		err := restrictOrgInTx(context.Background(), tx, "org-1", nil)
		require.ErrorContains(t, err, "downgrading org to free")
		require.Equal(t, 2, tx.execCalls)
	})

	t.Run("plan downgrade missing row", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{execResults: []fakeBillingExecResult{
			{rows: 1},
			{rows: 0},
		}}
		require.ErrorIs(t, restrictOrgInTx(context.Background(), tx, "org-1", nil), ErrSubscriptionNotFound)
		require.Equal(t, 2, tx.execCalls)
	})

	t.Run("entitlement update error", func(t *testing.T) {
		t.Parallel()

		tx := &fakeBillingTx{execResults: []fakeBillingExecResult{
			{rows: 1},
			{rows: 1},
			{err: errors.New("entitlements failed")},
		}}
		err := restrictOrgInTx(context.Background(), tx, "org-1", nil)
		require.ErrorContains(t, err, "resetting entitlements to free")
		require.Equal(t, 3, tx.execCalls)
	})
}
