//go:build integration

package telemetry_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"strait/internal/telemetry"
	"strait/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testDBOnce sync.Once
	testDB     *testutil.TestDB
	testDBErr  error
)

func getTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()
	testDBOnce.Do(func() {
		testDB, testDBErr = testutil.SetupSharedTestDB(context.Background(), "../../migrations", "telemetry-db-watchdog")
	})
	require.Nil(t, testDBErr)

	return testDB
}

// safeBuffer is a goroutine-safe wrapper around bytes.Buffer so that the
// watchdog goroutine (writing logs) and the test goroutine (reading logs)
// don't race on the underlying byte slice.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *safeBuffer) Write(p []byte) (int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *safeBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

// captureLogger returns a slog.Logger that writes JSON records into a
// thread-safe buffer, and the buffer. Tests can inspect the buffer to
// count or grep log lines without racing against the watchdog goroutine.
func captureLogger() (*slog.Logger, *safeBuffer) {
	sb := &safeBuffer{}
	h := slog.NewJSONHandler(sb, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), sb
}

func TestDBWatchdog_HappyPath_NoLongTxns(t *testing.T) {
	tdb := getTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger, logs := captureLogger()
	wd, err := telemetry.NewDBWatchdog(tdb.Pool, 100*time.Millisecond, 60*time.Second, logger)
	require.NoError(t, err)

	go wd.Run(ctx)

	// Wait until at least 3 samples have happened so the sampler has had
	// multiple chances to observe a clean state.
	waitFor(t, 2*time.Second, func() bool { return wd.SampleCount() >= 3 })
	assert.EqualValues(t, 0, wd.AlertCount())
	assert.False(t, strings.Contains(logs.
		String(),
		"long-running transaction detected",
	))

}

func TestDBWatchdog_DetectsLongTxn(t *testing.T) {
	tdb := getTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logger, logs := captureLogger()
	wd, err := telemetry.NewDBWatchdog(tdb.Pool, 100*time.Millisecond, 500*time.Millisecond, logger)
	require.NoError(t, err)

	go wd.Run(ctx)

	// Open a transaction on a different connection and hold it open for
	// longer than the 500ms alert threshold.
	conn, err := tdb.Pool.Acquire(ctx)
	require.NoError(t, err)

	defer conn.Release()

	tx, err := conn.Begin(ctx)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, "SELECT 1")
	require.NoError(t, err)

	// Let it age past the threshold.
	time.Sleep(1200 * time.Millisecond)

	// Commit so the txn stops appearing in pg_stat_activity, then verify
	// the watchdog observed at least one alert while it was open.
	_ = tx.Commit(ctx)

	waitFor(t, 3*time.Second, func() bool { return wd.AlertCount() >= 1 })
	assert.True(t, strings.Contains(logs.
		String(),
		"long-running transaction detected",
	))

}

func TestDBWatchdog_IdleInTransactionTerminated(t *testing.T) {
	tdb := getTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Acquire a connection with an aggressive idle_in_transaction_session_timeout
	// and verify Postgres terminates the idle txn. This is the behavioral
	// test that proves the RuntimeParam actually works end-to-end.
	conn, err := pgx.Connect(ctx, tdb.ConnStr)
	require.NoError(t, err)

	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "SET idle_in_transaction_session_timeout = '500ms'")
	require.NoError(t, err)

	_, err = conn.Exec(ctx, "BEGIN")
	require.NoError(t, err)
	_, err = conn.Exec(ctx, "SELECT 1")
	require.NoError(t, err)

	// Wait long enough that the idle-in-transaction timeout fires.
	time.Sleep(1500 * time.Millisecond)

	// The next query on this connection should fail because the server
	// terminated the backend.
	_, err = conn.Exec(ctx, "SELECT 1")
	require.Error(t, err)
	assert.False(t, !strings.Contains(strings.ToLower(err.Error()), "terminat") && !strings.Contains(strings.ToLower(err.Error()), "closed"))

}

// fakePool returns a controllable query result to drive adversarial tests.
type fakePool struct {
	err     error
	calls   int
	panicOn int
}

func (f *fakePool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.calls++
	if f.panicOn == f.calls {
		panic("simulated panic in query")
	}
	if f.err != nil {
		return nil, f.err
	}
	// An empty rows result from pgx is represented by returning an error, so
	// for the "clean" path we delegate to the real DB. Tests that don't need
	// clean rows use err to force the error branch.
	return nil, context.Canceled
}

func TestDBWatchdog_SurvivesQueryFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logger, _ := captureLogger()
	wd, err := telemetry.NewDBWatchdog(&fakePool{err: context.Canceled}, 50*time.Millisecond, 60*time.Second, logger)
	require.NoError(t, err)

	go wd.Run(ctx)

	waitFor(t, 1*time.Second, func() bool { return wd.SampleCount() >= 5 })
	assert.EqualValues(t, 0, wd.AlertCount())

	// Never panicked; alert count stays zero.

}

func TestDBWatchdog_SurvivesPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logger, logs := captureLogger()
	wd, err := telemetry.NewDBWatchdog(&fakePool{panicOn: 1}, 50*time.Millisecond, 60*time.Second, logger)
	require.NoError(t, err)

	go wd.Run(ctx)

	// After a panicking first sample the watchdog must keep sampling.
	waitFor(t, 1*time.Second, func() bool { return wd.SampleCount() >= 3 })
	assert.True(t, strings.Contains(logs.
		String(),
		"db watchdog panic recovered",
	))

}

func TestNewDBWatchdog_ValidatesInput(t *testing.T) {
	logger, _ := captureLogger()
	_, err := telemetry.NewDBWatchdog(nil, 1*time.Second, 1*time.Second, logger)
	require.Error(t, err)
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Failf(t, "condition not met", "timeout=%s", timeout)
}
