package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"
)

func newDBCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database utility commands",
	}

	cmd.AddCommand(newDBShellCommand())
	cmd.AddCommand(newDBStatsCommand())

	return cmd
}

func newDBShellCommand() *cobra.Command {
	var query string

	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Open psql shell using DATABASE_URL",
		RunE: func(_ *cobra.Command, _ []string) error {
			databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
			if databaseURL == "" {
				return fmt.Errorf("DATABASE_URL is required")
			}

			if query != "" {
				c := exec.Command("psql", databaseURL, "-c", query) //nolint:gosec // databaseURL is from env var, query is from CLI flag
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}

			c := exec.Command("psql", databaseURL) //nolint:gosec // databaseURL is from env var, interactive psql session
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}

	cmd.Flags().StringVar(&query, "query", "", "run one SQL query and exit")

	return cmd
}

func newDBStatsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show database table and connection statistics",
		RunE: func(_ *cobra.Command, _ []string) error {
			databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
			if databaseURL == "" {
				return fmt.Errorf("DATABASE_URL is required")
			}

			db, err := sql.Open("pgx", databaseURL)
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := db.Query(`
				SELECT relname::text AS table_name, n_live_tup::bigint AS live_rows, n_dead_tup::bigint AS dead_rows
				FROM pg_stat_user_tables
				ORDER BY n_live_tup DESC
				LIMIT 20`)
			if err != nil {
				return err
			}
			defer rows.Close()

			statsRows := make([]map[string]any, 0)
			for rows.Next() {
				var table string
				var liveRows int64
				var deadRows int64
				if err := rows.Scan(&table, &liveRows, &deadRows); err != nil {
					return err
				}
				statsRows = append(statsRows, map[string]any{
					"table":     table,
					"live_rows": liveRows,
					"dead_rows": deadRows,
				})
			}
			if err := rows.Err(); err != nil {
				return err
			}

			return printData(&appState{opts: &rootOptions{}}, statsRows)
		},
	}

	return cmd
}
