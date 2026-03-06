package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newBackupCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup and restore the orchestrator database",
		Long:  "Wraps pg_dump and pg_restore for orchestrator database backup and restore operations.",
	}

	cmd.AddCommand(newBackupCreateCommand(state))
	cmd.AddCommand(newBackupRestoreCommand(state))

	return cmd
}

func newBackupCreateCommand(state *appState) *cobra.Command {
	var (
		output      string
		databaseURL string
		format      string
		verbose     bool
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a database backup",
		Long: `Creates a backup of the orchestrator database using pg_dump.

Requires pg_dump to be installed and available in PATH.`,
		Example: `  orchestrator backup create
  orchestrator backup create --output backup.sql
  orchestrator backup create --database-url postgres://user:pass@host:5432/db
  orchestrator backup create --format custom --output backup.dump`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := exec.LookPath("pg_dump"); err != nil {
				return fmt.Errorf("pg_dump not found in PATH: install PostgreSQL client tools")
			}

			dsn := resolveDatabaseURL(databaseURL)
			if dsn == "" {
				return fmt.Errorf("database URL required: set --database-url or DATABASE_URL env var")
			}

			if output == "" {
				output = fmt.Sprintf("orchestrator-backup-%s.sql", time.Now().UTC().Format("20060102-150405"))
				if format == "custom" {
					output = fmt.Sprintf("orchestrator-backup-%s.dump", time.Now().UTC().Format("20060102-150405"))
				}
			}

			args := []string{"--dbname", dsn, "--file", output}

			switch format {
			case "plain", "":
				args = append(args, "--format=plain")
			case "custom":
				args = append(args, "--format=custom")
			case "directory":
				args = append(args, "--format=directory")
			case "tar":
				args = append(args, "--format=tar")
			default:
				return fmt.Errorf("unsupported format %q: use plain, custom, directory, or tar", format)
			}

			if verbose || state.opts.verbose {
				args = append(args, "--verbose")
			}

			pgDump := exec.Command("pg_dump", args...) //nolint:gosec // args built from validated flags, not user-controlled input
			pgDump.Stdout = os.Stdout
			pgDump.Stderr = os.Stderr

			if verbose || state.opts.verbose {
				fmt.Fprintf(os.Stderr, "running: pg_dump --dbname=*** --file=%s --format=%s\n", output, format)
			}

			if err := pgDump.Run(); err != nil {
				return fmt.Errorf("pg_dump failed: %w", err)
			}

			fmt.Fprintf(os.Stderr, "backup written to %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "output file path (default: timestamped filename)")
	cmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection string (default: DATABASE_URL env)")
	cmd.Flags().StringVar(&format, "format", "plain", "dump format: plain, custom, directory, tar")
	cmd.Flags().BoolVarP(&verbose, "verbose", "V", false, "pass --verbose to pg_dump")

	return cmd
}

func newBackupRestoreCommand(state *appState) *cobra.Command {
	var (
		input       string
		databaseURL string
		clean       bool
		verbose     bool
		yes         bool
	)

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore a database backup",
		Long: `Restores an orchestrator database from a backup file.

Uses pg_restore for custom/directory/tar formats, or psql for plain SQL dumps.
Requires pg_restore and/or psql to be installed and available in PATH.`,
		Example: `  orchestrator backup restore --input backup.sql
  orchestrator backup restore --input backup.dump --clean
  orchestrator backup restore --input backup.dump --database-url postgres://user:pass@host:5432/db`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if input == "" {
				return fmt.Errorf("--input is required")
			}

			if _, err := os.Stat(input); os.IsNotExist(err) {
				return fmt.Errorf("backup file not found: %s", input)
			}

			dsn := resolveDatabaseURL(databaseURL)
			if dsn == "" {
				return fmt.Errorf("database URL required: set --database-url or DATABASE_URL env var")
			}

			if err := requireConfirmation(state, "This will overwrite data in the target database. Continue?", yes); err != nil {
				return err
			}

			if isPlainSQL(input) {
				return restoreWithPsql(dsn, input, verbose || state.opts.verbose)
			}

			return restoreWithPgRestore(dsn, input, clean, verbose || state.opts.verbose)
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "backup file to restore (required)")
	cmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection string (default: DATABASE_URL env)")
	cmd.Flags().BoolVar(&clean, "clean", false, "drop database objects before restoring")
	cmd.Flags().BoolVarP(&verbose, "verbose", "V", false, "pass --verbose to pg_restore/psql")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompt")

	return cmd
}

func resolveDatabaseURL(flagValue string) string {
	dsn := flagValue
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn != "" && !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return "" // invalid scheme
	}
	return dsn
}

func isPlainSQL(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path is the backup file provided by --input flag
	if err != nil {
		return true // default to psql
	}
	defer f.Close()

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil || n < 5 {
		return true
	}

	// pg_dump custom format starts with "PGDMP"
	if string(buf) == "PGDMP" {
		return false
	}
	return true
}

func restoreWithPsql(dsn, input string, verbose bool) error {
	if _, err := exec.LookPath("psql"); err != nil {
		return fmt.Errorf("psql not found in PATH: install PostgreSQL client tools")
	}

	args := []string{"--dbname", dsn, "--file", input}
	if !verbose {
		args = append(args, "--quiet")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "running: psql --dbname=*** --file=%s\n", input)
	}

	psql := exec.Command("psql", args...) //nolint:gosec // args built from validated flags
	psql.Stdout = os.Stdout
	psql.Stderr = os.Stderr

	if err := psql.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %w", err)
	}

	fmt.Fprintln(os.Stderr, "restore complete")
	return nil
}

func restoreWithPgRestore(dsn, input string, clean, verbose bool) error {
	if _, err := exec.LookPath("pg_restore"); err != nil {
		return fmt.Errorf("pg_restore not found in PATH: install PostgreSQL client tools")
	}

	args := []string{"--dbname", dsn, input}
	if clean {
		args = append(args, "--clean")
	}
	if verbose {
		args = append(args, "--verbose")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "running: pg_restore --dbname=*** %s\n", input)
	}

	pgRestore := exec.Command("pg_restore", args...) //nolint:gosec // args built from validated flags
	pgRestore.Stdout = os.Stdout
	pgRestore.Stderr = os.Stderr

	if err := pgRestore.Run(); err != nil {
		return fmt.Errorf("pg_restore failed: %w", err)
	}

	fmt.Fprintln(os.Stderr, "restore complete")
	return nil
}
