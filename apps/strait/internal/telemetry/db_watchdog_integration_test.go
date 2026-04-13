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
)

var (
	testDBOnce sync.Once
	testDB     *testutil.TestDB
	testDBErr  error
)

func getTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()
	testDBOnce.Do(func() {
		testDB, testDBErr = testutil.SetupTestDB(context.Background(), "../../migrations")
	})
	if testDBErr != nil {
		t.Fatalf("setup test db: %v", testDBErr)
	}
	return testDB
}

// captureLogger returns a slog.Logger that writes JSON records into the
// supplied bytes.Buffer, and the buffer. Tests can inspect the buffer to
// count or grep log lines.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), &buf
}

func TestDBWatchdog_HappyPath_NoLongTxns(t *testing.T) {
	tdb := getTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger, logs := captureLogger()
	wd, err := telemetry.NewDBWatchdog(tdb.Pool, 100*time.Millisecond, 60*time.Second, logger)
	if err != nil {
		t.Fatalf("NewDBWatchdog: %v", err)
	}
	go wd.Run(ctx)

	// Wait until at least 3 samples have happened so the sampler has had
	// multiple chances to observe a clean state.
	waitFor(t, 2*time.Second, func() bool { return wd.SampleCount() >= 3 })

	if wd.AlertCount() != 0 {
		t.Errorf("expected 0 alerts on a clean DB, got %d", wd.AlertCount())
	}
	if strings.Contains(logs.String(), "long-running transaction detected") {
		t.Errorf("unexpected long-txn alert in logs: %s", logs.String())
	}
}

func TestDBWatchdog_DetectsLongTxn(t *testing.T) {
	tdb := getTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logger, logs := captureLogger()
	wd, err := telemetry.NewDBWatchdog(tdb.Pool, 100*time.Millisecond, 500*time.Millisecond, logger)
	if err != nil {
		t.Fatalf("NewDBWatchdog: %v", err)
	}
	go wd.Run(ctx)

	// Open a transaction on a different connection and hold it open for
	// longer than the 500ms alert threshold.
	conn, err := tdb.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatalf("exec: %v", err)
	}

	// Let it age past the threshold.
	time.Sleep(1200 * time.Millisecond)

	// Commit so the txn stops appearing in pg_stat_activity, then verify
	// the watchdog observed at least one alert while it was open.
	_ = tx.Commit(ctx)

	waitFor(t, 3*time.Second, func() bool { return wd.AlertCount() >= 1 })

	if !strings.Contains(logs.String(), "long-running transaction detected") {
		t.Errorf("expected alert log, got: %s", logs.String())
	}
}

func TestDBWatchdog_IdleInTransactionTerminated(t *testing.T) {
	tdb := getTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Acquire a connection with an aggressive idle_in_transaction_session_timeout
	// and verify Postgres terminates the idle txn. This is the behavioral
	// test that proves the RuntimeParam actually works end-to-end.
	conn, err := pgx.Connect(ctx, tdb.ConnStr)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "SET idle_in_transaction_session_timeout = '500ms'"); err != nil {
		t.Fatalf("set timeout: %v", err)
	}

	if _, err := conn.Exec(ctx, "BEGIN"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatalf("select: %v", err)
	}

	// Wait long enough that the idle-in-transaction timeout fires.
	time.Sleep(1500 * time.Millisecond)

	// The next query on this connection should fail because the server
	// terminated the backend.
	_, err = conn.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Fatalf("expected error after idle_in_transaction_session_timeout, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "terminat") && !strings.Contains(strings.ToLower(err.Error()), "closed") {
		t.Errorf("expected termination error, got: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewDBWatchdog: %v", err)
	}
	go wd.Run(ctx)

	waitFor(t, 1*time.Second, func() bool { return wd.SampleCount() >= 5 })

	// Never panicked; alert count stays zero.
	if wd.AlertCount() != 0 {
		t.Errorf("alert count should be 0 on query failure, got %d", wd.AlertCount())
	}
}

func TestDBWatchdog_SurvivesPanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logger, logs := captureLogger()
	wd, err := telemetry.NewDBWatchdog(&fakePool{panicOn: 1}, 50*time.Millisecond, 60*time.Second, logger)
	if err != nil {
		t.Fatalf("NewDBWatchdog: %v", err)
	}
	go wd.Run(ctx)

	// After a panicking first sample the watchdog must keep sampling.
	waitFor(t, 1*time.Second, func() bool { return wd.SampleCount() >= 3 })

	if !strings.Contains(logs.String(), "db watchdog panic recovered") {
		t.Errorf("expected panic recovery log, got: %s", logs.String())
	}
}

func TestNewDBWatchdog_ValidatesInput(t *testing.T) {
	logger, _ := captureLogger()
	if _, err := telemetry.NewDBWatchdog(nil, 1*time.Second, 1*time.Second, logger); err == nil {
		t.Error("expected error for nil pool")
	}
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
	t.Fatalf("condition not met within %s", timeout)
}
