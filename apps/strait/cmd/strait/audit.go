package main

import (
	"strings"
	"time"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"

	"github.com/spf13/cobra"
)

func newAuditCommand(state *appState) *cobra.Command {
	var projectID string
	var actorID string
	var resourceType string
	var resourceID string
	var limit int
	var from string
	var to string
	var order string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View audit log events",
		Long:  "Lists recent audit events for a project with optional filters.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			projectID, err = requireProjectID(state, projectID)
			if err != nil {
				return err
			}
			var fromTime *time.Time
			if from != "" {
				parsedFrom, parseErr := parseAuditTime(from)
				if parseErr != nil {
					return parseErr
				}
				fromTime = &parsedFrom
			}
			var toTime *time.Time
			if to != "" {
				parsedTo, parseErr := parseAuditTime(to)
				if parseErr != nil {
					return parseErr
				}
				toTime = &parsedTo
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			events, err := cli.ListAuditEvents(cmd.Context(), client.ListAuditEventsParams{
				ProjectID:    projectID,
				ActorID:      actorID,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Limit:        limit,
				From:         fromTime,
				To:           toTime,
				Order:        order,
			})
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(events))
			for _, e := range events {
				action := e.Action
				if isTTYRich(state) {
					action = colorAuditAction(e.Action)
				}
				rows = append(rows, map[string]any{
					"id":            e.ID,
					"actor_id":      e.ActorID,
					"actor_type":    e.ActorType,
					"action":        action,
					"resource_type": e.ResourceType,
					"resource_id":   e.ResourceID,
					"details":       e.Details,
					"created_at":    e.CreatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "filter by actor ID")
	cmd.Flags().StringVar(&resourceType, "resource-type", "", "filter by resource type")
	cmd.Flags().StringVar(&resourceID, "resource-id", "", "filter by resource ID")
	cmd.Flags().IntVar(&limit, "limit", 50, "max events to return")
	cmd.Flags().StringVar(&from, "from", "", "filter events created after this RFC3339 timestamp")
	cmd.Flags().StringVar(&to, "to", "", "filter events created before this RFC3339 timestamp")
	cmd.Flags().StringVar(&order, "order", "desc", "sort order (asc or desc)")

	return cmd
}

func colorAuditAction(action string) string {
	lower := strings.ToLower(action)
	switch {
	case strings.HasPrefix(lower, "create"):
		return styles.Green.Render(action)
	case strings.HasPrefix(lower, "delete"), strings.HasPrefix(lower, "revoke"):
		return styles.Red.Render(action)
	case strings.HasPrefix(lower, "update"), strings.HasPrefix(lower, "rotate"):
		return styles.Yellow.Render(action)
	default:
		return action
	}
}

func parseAuditTime(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
