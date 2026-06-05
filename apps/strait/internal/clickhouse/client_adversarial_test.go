//go:build !integration

package clickhouse

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Error(t, err)
	assert.True(t, strings.Contains(err.
		Error(),
		"clickhouse"))

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
	require.NoError(t, err)
	require.NotNil(t, c)

	defer c.Close()
}

func TestNew_NilLogger(t *testing.T) {
	t.Parallel()

	c, err := New(Config{Enabled: true, URL: "clickhouse://localhost:9000"}, nil)
	require.NoError(t, err)

	require.NotNil(t, c)
	assert.NotNil(t, c.logger)

	defer c.Close()
}

func TestNew_WithDatabase(t *testing.T) {
	t.Parallel()

	c, err := New(Config{
		Enabled:  true,
		URL:      "clickhouse://localhost:9000",
		Database: "testdb",
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, c)

	defer c.Close()
}

// buildConnURL adversarial inputs

func TestBuildConnURL_MalformedURL(t *testing.T) {
	t.Parallel()

	_, err := buildConnURL("://broken", "db")
	require.Error(t, err)

}

func TestBuildConnURL_URLWithExistingParams(t *testing.T) {
	t.Parallel()

	got, err := buildConnURL("clickhouse://host:9000?timeout=5s", "mydb")
	require.NoError(t, err)
	assert.True(t, strings.Contains(got,
		"timeout=5s",
	))
	assert.True(t, strings.Contains(got,
		"database=mydb",
	))

	// Should contain both existing and new params.

}

func TestBuildConnURL_EmptyURL(t *testing.T) {
	t.Parallel()

	// Empty URL with a database should still work (url.Parse handles empty string).
	got, err := buildConnURL("", "analytics")
	require.NoError(t, err)
	assert.True(t, strings.Contains(got,
		"database=analytics",
	))

}

// Client nil-safety and error paths

func TestClient_HealthyWithNilDB(t *testing.T) {
	t.Parallel()

	c := &Client{db: nil, logger: slog.Default()}
	assert.False(t, c.Healthy(context.
		Background()))

}

func TestClient_CloseWithNilDB(t *testing.T) {
	t.Parallel()

	c := &Client{db: nil, logger: slog.Default()}
	assert.NoError(t, c.Close())

}

func TestClient_DBReturnsUnderlyingDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	require.NoError(t, err)

	defer db.Close()

	c := &Client{db: db}
	assert.Equal(t, db, c.DB())

}

func TestClient_Query_NilClient(t *testing.T) {
	t.Parallel()

	var c *Client
	rows, err := c.Query(context.Background(), "SELECT 1")
	require.Error(t, err)
	if rows != nil {
		defer rows.Close()
		_ = rows.Err()
	}
	assert.Nil(t, rows)
}

func TestClient_Query_NilDB(t *testing.T) {
	t.Parallel()

	c := &Client{db: nil}
	rows, err := c.Query(context.Background(), "SELECT 1")
	require.Error(t, err)
	if rows != nil {
		defer rows.Close()
		_ = rows.Err()
	}
	assert.Nil(t, rows)
}

func TestClient_Exec_ClosedDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	require.NoError(t, err)

	db.Close()

	c := &Client{db: db, logger: slog.Default()}
	err = c.Exec(context.Background(), "SELECT 1")
	assert.Error(t, err)

}

func TestClient_Query_ClosedDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	require.NoError(t, err)

	db.Close()

	c := &Client{db: db, logger: slog.Default()}
	rows, qErr := c.Query(context.Background(), "SELECT 1")
	if rows != nil {
		defer rows.Close()
		_ = rows.Err()
	}
	assert.NotNil(t, qErr)

}

func TestClient_Exec_CanceledContext(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	require.NoError(t, err)

	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := &Client{db: db, logger: slog.Default()}
	err = c.Exec(ctx, "SELECT 1")
	assert.Error(t, err)

}

func TestClient_Healthy_ClosedDB(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("clickhouse", "clickhouse://localhost:0")
	require.NoError(t, err)

	db.Close()

	c := &Client{db: db, logger: slog.Default()}
	assert.False(t, c.Healthy(context.
		Background()))

}
