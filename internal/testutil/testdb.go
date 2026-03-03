//go:build integration

package testutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	// Required for golang-migrate postgres driver registration.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// Required for golang-migrate file source driver registration.
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB struct {
	Pool      *pgxpool.Pool
	Container *postgres.PostgresContainer
	ConnStr   string
}

func SetupTestDB(ctx context.Context, migrationsPath string) (*TestDB, error) {
	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	m, err := migrate.New("file://"+migrationsPath, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return &TestDB{
		Pool:      pool,
		Container: pgContainer,
		ConnStr:   connStr,
	}, nil
}

func (tdb *TestDB) CleanTables(ctx context.Context) error {
	if tdb == nil || tdb.Pool == nil {
		return nil
	}

	_, err := tdb.Pool.Exec(ctx, "TRUNCATE TABLE run_events, job_runs, jobs CASCADE")
	if err != nil {
		return fmt.Errorf("clean tables: %w", err)
	}

	return nil
}

func (tdb *TestDB) Cleanup(ctx context.Context) {
	if tdb == nil {
		return
	}

	if tdb.Pool != nil {
		tdb.Pool.Close()
	}

	if tdb.Container != nil {
		_ = tdb.Container.Terminate(ctx)
	}
}
