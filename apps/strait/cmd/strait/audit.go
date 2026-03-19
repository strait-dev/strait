package main

import (
	"github.com/spf13/cobra"
)

func newAuditCommand(state *appState) *cobra.Command {
	var projectID string
	var actor string
	var action string
	var limit int

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View audit log events",
		Long:  "Lists recent audit events for a project with optional filters.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			events, err := cli.ListAuditEvents(cmd.Context(), projectID, actor, action, limit)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(events))
			for _, e := range events {
				rows = append(rows, map[string]any{
					"id":         e.ID,
					"actor":      e.Actor,
					"action":     e.Action,
					"resource":   e.Resource,
					"detail":     e.Detail,
					"created_at": e.CreatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&actor, "actor", "", "filter by actor email")
	cmd.Flags().StringVar(&action, "action", "", "filter by action type")
	cmd.Flags().IntVar(&limit, "limit", 50, "max events to return")

	return cmd
}
