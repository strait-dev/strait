//go:build !integration

package clickhouse

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
)

// Client construction edge cases

func TestNew_InvalidURL(t *testing.T) {
	t.Parallel()

	// url.Parse is very permissive; we need a URL with a control character
	// AND a non-empty Database to trigger the buildConnURL parse path.
	_, err := New(Config{
		Enabled:  true,
		URL:      "clickhouse://host:9000" + string([]byte{0x7f}),
		Database: "testdb",
	}, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "clickhouse") {
		t.Errorf("expected error to mention clickhouse, got: %v", err)
	}
}

func TestNew_DefaultPoolSizes(t *testing.T) {
	t.Parallel()

	// MaxOpenConns <= 0 and MaxIdleConns <= 0 should use defaults.
	// We cannot easily inspect the sql.DB settings, but we verify no panic.
	cfg := Config{
		Enabled:      true,
		URL:          "clickhouse://localhost:9000",
		MaxOpenConns: -1,
		MaxIdleConns: 0,
	}
	// sql.Open with the clickhouse driver succeeds even for unreachable hosts.
	c, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	defer c.Close()
}

func TestNew_NilLogger(t *testing.T) {
	t.Parallel()

	c, err := New(Config{Enabled: true, URL: "clickhouse://localhost:9000"}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
		return
	}
	if c.logger == nil {
		t.Error("expected default logger when nil is passed")
	}
	defer c.Close()
}

func TestNew_WithDatabase(t *testing.T) {
	t.Parallel()

	c, err := New(Config{
		Enabled:  true,
		URL:      "clickhouse://localhost:9000",
		Database: "testdb",
	}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	defer c.Close()
}

// buildConnURL adversarial inputs

func TestBuildConnURL_MalformedURL(t *testing.T) {
	t.Parallel()

	_, err := buildConnURL("://broken", "db")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
}

func TestBuildConnURL_URLWithExistingParams(t *testing.T) {
	t.Parallel()

	got, err := buildConnURL("clickhouse://host:9000?timeout=5s", "mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain both existing and new params.
	if !strings.Contains(got, "timeout=5s") {
		t.Errorf("expected existing param to be preserved, got %q", got)
	}
	if !strings.Contains(got, "database=mydb") {
		t.Errorf("expected database param to be appended, got %q", got)
	}
}

func TestBuildConnURL_EmptyURL(t *testing.T) {
	t.Parallel()

	// Empty URL with a database should still work (url.Parse handles empty string).
	got, err := buildConnURL("", "analytics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "database=analytics") {
		t.Errorf("expected database in URL, got %q", got)
	}
}

// Client nil-safety and error paths

func TestClient_HealthyWithNilDB(t *testing.T) {
	t.Parallel()

	c := &Client{db: nil, logger: slog.Default()}
	if c.Healthy(context.Background()) {
		t.Error("client with nil db should not be healthy")
	}
}

func TestClient_CloseWithNilDB(t *testing.T) {
	t.Parallel()

	c := &Client{db: nil, logger: slog.Default()}
	if err := c.Close(); err != nil {
		t.Errorf("Close with nil db should return nil, got %v", err)
	}
}

func TestClient_DBReturnsUnderlyingDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	c := &Client{db: db}
	if c.DB() != db {
		t.Error("DB() should return the underlying *sql.DB")
	}
}

func TestClient_Query_NilClient(t *testing.T) {
	t.Parallel()

	var c *Client
	rows, err := c.Query(context.Background(), "SELECT 1")
	if err == nil {
		if rows != nil {
			defer rows.Close()
			_ = rows.Err()
		}
		t.Fatal("expected error from nil client Query")
	}
	if rows != nil {
		defer rows.Close()
		_ = rows.Err()
		t.Error("expected nil rows from nil client")
	}
}

func TestClient_Query_NilDB(t *testing.T) {
	t.Parallel()

	c := &Client{db: nil}
	rows, err := c.Query(context.Background(), "SELECT 1")
	if err == nil {
		if rows != nil {
			defer rows.Close()
			_ = rows.Err()
		}
		t.Fatal("expected error from nil-db client Query")
	}
	if rows != nil {
		defer rows.Close()
		_ = rows.Err()
		t.Error("expected nil rows from nil-db client")
	}
}

func TestClient_Exec_ClosedDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	c := &Client{db: db, logger: slog.Default()}
	err = c.Exec(context.Background(), "SELECT 1")
	if err == nil {
		t.Error("expected error from exec on closed db")
	}
}

func TestClient_Query_ClosedDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	c := &Client{db: db, logger: slog.Default()}
	rows, qErr := c.Query(context.Background(), "SELECT 1")
	if rows != nil {
		defer rows.Close()
		_ = rows.Err()
	}
	if qErr == nil {
		t.Error("expected error from query on closed db")
	}
}

func TestClient_Exec_CanceledContext(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := &Client{db: db, logger: slog.Default()}
	err = c.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Error("expected error from exec with canceled context")
	}
}

func TestClient_Healthy_ClosedDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	c := &Client{db: db, logger: slog.Default()}
	if c.Healthy(context.Background()) {
		t.Error("client with closed db should not be healthy")
	}
}
