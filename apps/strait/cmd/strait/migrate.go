package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"strait/migrations"

	"github.com/golang-migrate/migrate/v4"
	pgmigrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
)

func newMigrateCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage database migrations",
	}

	cmd.AddCommand(newMigrateUpCommand())
	cmd.AddCommand(newMigrateDownCommand(state))
	cmd.AddCommand(newMigrateStatusCommand())
	cmd.AddCommand(newMigrateCreateCommand())

	return cmd
}

func newMigrateCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new up/down SQL migration pair",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := sanitizeMigrationName(args[0])
			if name == "" {
				return fmt.Errorf("migration name must contain alphanumeric characters")
			}

			next, err := nextMigrationVersion("migrations")
			if err != nil {
				return err
			}

			upPath := filepath.Join("migrations", fmt.Sprintf("%06d_%s.up.sql", next, name))
			downPath := filepath.Join("migrations", fmt.Sprintf("%06d_%s.down.sql", next, name))

			if err := os.WriteFile(upPath, []byte("-- Write your UP migration here\n"), 0o600); err != nil {
				return err
			}
			if err := os.WriteFile(downPath, []byte("-- Write your DOWN migration here\n"), 0o600); err != nil {
				return err
			}

			fmt.Printf("created %s\n", upPath)
			fmt.Printf("created %s\n", downPath)
			return nil
		},
	}

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

func newMigrateDownCommand(state *appState) *cobra.Command {
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

			if err := requireConfirmation(state, fmt.Sprintf("Roll back %d migration(s)?", count), yes); err != nil {
				return err
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

func nextMigrationVersion(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	re := regexp.MustCompile(`^(\d{6})_.*\.(up|down)\.sql$`)
	maxVersion := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}
		v, convErr := strconv.Atoi(matches[1])
		if convErr != nil {
			continue
		}
		if v > maxVersion {
			maxVersion = v
		}
	}

	return maxVersion + 1, nil
}

func sanitizeMigrationName(raw string) string {
	clean := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(raw, "_")
	clean = regexp.MustCompile(`_+`).ReplaceAllString(clean, "_")
	clean = regexp.MustCompile(`^_|_$`).ReplaceAllString(clean, "")
	return clean
}
