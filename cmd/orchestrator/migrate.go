package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"orchestrator/migrations"

	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
)

func newMigrateCommand(_ *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage database migrations",
	}

	cmd.AddCommand(newMigrateUpCommand())
	cmd.AddCommand(newMigrateDownCommand())
	cmd.AddCommand(newMigrateStatusCommand())

	return cmd
}

func newMigrateUpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up [n]",
		Short: "Apply all pending migrations or N migrations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, err := openMigratorFromEnv()
			if err != nil {
				return err
			}
			defer closeMigrator(m)

			if len(args) == 0 {
				if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
					return fmt.Errorf("apply migrations: %w", err)
				}
				fmt.Println("migrations up complete")
				return nil
			}

			count, err := parsePositiveInt(args[0])
			if err != nil {
				return err
			}
			if err := m.Steps(count); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				return fmt.Errorf("apply migrations: %w", err)
			}
			fmt.Printf("applied %d migration(s)\n", count)
			return nil
		},
	}

	return cmd
}

func newMigrateDownCommand() *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "down <n>",
		Short: "Rollback N migrations",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			count, err := parsePositiveInt(args[0])
			if err != nil {
				return err
			}

			if !yes {
				return fmt.Errorf("down is destructive; rerun with --yes to confirm")
			}

			m, err := openMigratorFromEnv()
			if err != nil {
				return err
			}
			defer closeMigrator(m)

			if err := m.Steps(-count); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				return fmt.Errorf("rollback migrations: %w", err)
			}
			fmt.Printf("rolled back %d migration(s)\n", count)
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm rollback")

	return cmd
}

func newMigrateStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current migration version",
		RunE: func(_ *cobra.Command, _ []string) error {
			m, err := openMigratorFromEnv()
			if err != nil {
				return err
			}
			defer closeMigrator(m)

			version, dirty, err := m.Version()
			if errors.Is(err, migrate.ErrNilVersion) {
				fmt.Println("version: none")
				fmt.Println("dirty: false")
				return nil
			}
			if err != nil {
				return fmt.Errorf("read migration status: %w", err)
			}

			fmt.Printf("version: %d\n", version)
			fmt.Printf("dirty: %t\n", dirty)
			return nil
		},
	}

	return cmd
}

func openMigratorFromEnv() (*migrate.Migrate, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open sql connection: %w", err)
	}

	driver, err := pgmigrate.WithInstance(db, &pgmigrate.Config{})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration driver: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}

	return m, nil
}

func closeMigrator(m *migrate.Migrate) {
	if m == nil {
		return
	}
	_, _ = m.Close()
}

func parsePositiveInt(raw string) (int, error) {
	var n int
	_, err := fmt.Sscanf(raw, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	if n <= 0 {
		return 0, fmt.Errorf("number must be positive")
	}
	return n, nil
}
