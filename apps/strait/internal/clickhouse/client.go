// Package clickhouse provides optional ClickHouse integration for analytics and log export.
// ClickHouse is never required for operational correctness — the engine always uses Postgres.
package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // registers "clickhouse" database/sql driver
)

// Client wraps a ClickHouse connection pool with health checks and graceful shutdown.
// When disabled (nil), all operations are no-ops.
type Client struct {
	db     *sql.DB
	logger *slog.Logger
}

// Config holds ClickHouse connection settings.
type Config struct {
	URL          string // ClickHouse native protocol URL (e.g., clickhouse://localhost:9000)
	Database     string // Target database name
	Enabled      bool   // Feature gate
	MaxOpenConns int    // Max open connections (default 10)
	MaxIdleConns int    // Max idle connections (default 5)
}

// New creates a new ClickHouse client. Returns nil if disabled.
func New(cfg Config, logger *slog.Logger) (*Client, error) {
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil // nil client is a valid disabled state.
	}
	if cfg.URL == "" {
		return nil, fmt.Errorf("clickhouse: URL is required when enabled")
	}
	if logger == nil {
		logger = slog.Default()
	}

	connURL, err := buildConnURL(cfg.URL, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: %w", err)
	}

	db, err := sql.Open("clickhouse", connURL)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open connection: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 10
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(30 * time.Minute)

	return &Client{db: db, logger: logger}, nil
}

// Healthy returns true if the ClickHouse connection is alive.
func (c *Client) Healthy(ctx context.Context) bool {
	if c == nil || c.db == nil {
		return false
	}
	return c.db.PingContext(ctx) == nil
}

// Close gracefully shuts down the ClickHouse connection pool.
func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// DB returns the underlying *sql.DB for direct queries. May be nil.
func (c *Client) DB() *sql.DB {
	if c == nil {
		return nil
	}
	return c.db
}

// buildConnURL appends the database name as a query parameter if not already present.
func buildConnURL(rawURL, database string) (string, error) {
	if database == "" {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	q := u.Query()
	if q.Get("database") == "" {
		q.Set("database", database)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// Exec executes a query without returning rows.
func (c *Client) Exec(ctx context.Context, query string, args ...any) error {
	if c == nil || c.db == nil {
		return nil
	}
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

// Query executes a query that returns rows.
func (c *Client) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("clickhouse: client is nil")
	}
	return c.db.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that returns at most one row.
func (c *Client) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return c.db.QueryRowContext(ctx, query, args...)
}
