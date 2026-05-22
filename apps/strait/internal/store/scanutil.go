package store

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier abstracts pgxpool.Pool and pgx.Tx for scan utilities.
type Querier interface {
	pgxscan.Querier
}

// Ensure pool and tx satisfy Querier at compile time.
var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)

// hashString returns a deterministic int64 hash suitable for advisory lock keys.
func hashString(s string) int64 {
	var h int64
	for _, c := range s {
		h = h*31 + int64(c)
	}
	return h
}

// ScanOne executes a query and scans a single row into a struct of type T.
// Returns pgx.ErrNoRows if no rows match.
func ScanOne[T any](ctx context.Context, q Querier, query string, args ...any) (T, error) {
	var result T
	if err := pgxscan.Get(ctx, q, &result, query, args...); err != nil {
		return result, fmt.Errorf("scan one: %w", err)
	}
	return result, nil
}

// ScanAll executes a query and scans all matching rows into a slice of type T.
// Returns an empty (non-nil) slice if no rows match.
func ScanAll[T any](ctx context.Context, q Querier, query string, args ...any) ([]T, error) {
	var results []T
	if err := pgxscan.Select(ctx, q, &results, query, args...); err != nil {
		return nil, fmt.Errorf("scan all: %w", err)
	}
	if results == nil {
		results = []T{}
	}
	return results, nil
}
