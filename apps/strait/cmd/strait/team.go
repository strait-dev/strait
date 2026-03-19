package main

import (
	"fmt"

	"strait/internal/cli/client"

	"github.com/spf13/cobra"
)

func newTeamCommand(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage project team members",
	}

	cmd.AddCommand(newTeamListCommand(state))
	cmd.AddCommand(newTeamAddCommand(state))
	cmd.AddCommand(newTeamRemoveCommand(state))
	cmd.AddCommand(newTeamRolesCommand(state))

	return cmd
}

func newTeamListCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List project members",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			members, err := cli.ListMembers(cmd.Context(), projectID)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(members))
			for _, m := range members {
				rows = append(rows, map[string]any{
					"id":         m.ID,
					"email":      m.Email,
					"role":       m.Role,
					"created_at": m.CreatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}

func newTeamAddCommand(state *appState) *cobra.Command {
	var projectID string
	var role string

	cmd := &cobra.Command{
		Use:   "add <email>",
		Short: "Add a member to the project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			if role == "" {
				return fmt.Errorf("--role is required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			member, err := cli.AddMember(cmd.Context(), client.AddMemberRequest{
				ProjectID: projectID,
				Email:     args[0],
				Role:      role,
			})
			if err != nil {
				return err
			}

			return printData(state, member)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")
	cmd.Flags().StringVar(&role, "role", "", "member role")

	return cmd
}

func newTeamRemoveCommand(state *appState) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "remove <member-id>",
		Short: "Remove a member from the project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireConfirmation(state, "Remove this member?", yes); err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			if err := cli.RemoveMember(cmd.Context(), args[0]); err != nil {
				return err
			}

			return printData(state, map[string]any{"id": args[0], "removed": true})
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal")

	return cmd
}

func newTeamRolesCommand(state *appState) *cobra.Command {
	var projectID string

	cmd := &cobra.Command{
		Use:   "roles",
		Short: "List available roles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID, err := requireProjectID(state, projectID)
			if err != nil {
				return err
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			roles, err := cli.ListRoles(cmd.Context(), projectID)
			if err != nil {
				return err
			}

			rows := make([]map[string]any, 0, len(roles))
			for _, r := range roles {
				rows = append(rows, map[string]any{
					"id":          r.ID,
					"name":        r.Name,
					"description": r.Description,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}
