// Package clickhouse provides optional ClickHouse integration for analytics and log export.
// ClickHouse is never required for operational correctness — the engine always uses Postgres.
package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// Client wraps a ClickHouse connection pool with health checks and graceful shutdown.
// When disabled (nil), all operations are no-ops.
type Client struct {
	db     *sql.DB
	logger *slog.Logger
}

// Config holds ClickHouse connection settings.
type Config struct {
	URL      string // ClickHouse native protocol URL (e.g., clickhouse://localhost:9000)
	Database string // Target database name
	Enabled  bool   // Feature gate
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

	db, err := sql.Open("clickhouse", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: open connection: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
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

// Exec executes a query without returning rows.
func (c *Client) Exec(ctx context.Context, query string, args ...any) error {
	if c == nil || c.db == nil {
		return nil
	}
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}
