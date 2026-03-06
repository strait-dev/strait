package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/cli/client"

	"github.com/spf13/cobra"
)

func newTriggerCommand(state *appState) *cobra.Command {
	var payload string
	var payloadFile string
	var priority int

	cmd := &cobra.Command{
		Use:   "trigger <job-id-or-slug>",
		Short: "Shortcut for jobs trigger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			jobID, err := resolveJobIdentifier(cmd.Context(), cli, state, args[0])
			if err != nil {
				return err
			}

			req := client.TriggerJobRequest{Priority: priority}
			if payloadFile != "" {
				raw, err := os.ReadFile(payloadFile) //nolint:gosec // explicit user-selected local payload file
				if err != nil {
					return err
				}
				req.Payload = json.RawMessage(raw)
			} else if strings.TrimSpace(payload) != "" {
				req.Payload = json.RawMessage(payload)
			}
			if len(req.Payload) > 0 && !json.Valid(req.Payload) {
				return fmt.Errorf("payload must be valid JSON")
			}

			resp, err := cli.TriggerJob(cmd.Context(), jobID, req, "")
			if err != nil {
				return err
			}

			return printData(state, resp)
		},
	}

	cmd.Flags().StringVar(&payload, "payload", "", "inline JSON payload")
	cmd.Flags().StringVar(&payloadFile, "payload-file", "", "path to payload JSON file")
	cmd.Flags().IntVar(&priority, "priority", 0, "run priority")

	return cmd
}

func resolveJobIdentifier(ctx context.Context, cli *client.Client, state *appState, idOrSlug string) (string, error) {
	if _, err := cli.GetJob(ctx, idOrSlug); err == nil {
		return idOrSlug, nil
	}

	projectID := state.opts.projectID
	if projectID == "" {
		return "", fmt.Errorf("project is required to resolve slug %q", idOrSlug)
	}

	jobs, err := cli.ListJobs(ctx, projectID)
	if err != nil {
		return "", err
	}
	for _, job := range jobs {
		if job.Slug == idOrSlug {
			return job.ID, nil
		}
	}

	return "", fmt.Errorf("job %q not found", idOrSlug)
}
