package main

import (
	"fmt"
	"os"

	"strait/internal/cli/client"
	"strait/internal/cli/styles"

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
			var err error
			projectID, err = requireProjectID(state, projectID)
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
					"project_id": m.ProjectID,
					"user_id":    m.UserID,
					"role_id":    m.RoleID,
					"granted_by": m.GrantedBy,
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
	var roleID string

	cmd := &cobra.Command{
		Use:   "add <user-id>",
		Short: "Assign a role to a project member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if roleID == "" {
				return fmt.Errorf("--role-id is required")
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			member, err := cli.AddMember(cmd.Context(), client.AssignMemberRequest{
				UserID: args[0],
				RoleID: roleID,
			})
			if err != nil {
				return err
			}

			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Added member "+styles.Bold.Render(args[0])))
				return nil
			}
			return printData(state, member)
		},
	}

	cmd.Flags().StringVar(&roleID, "role-id", "", "role ID to assign")

	return cmd
}

func newTeamRemoveCommand(state *appState) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "remove <user-id>",
		Short: "Remove a member role assignment from the project",
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

			if isTTYRich(state) {
				fmt.Fprintln(os.Stderr, styles.Success("Removed member "+styles.Bold.Render(args[0])))
				return nil
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
			var err error
			projectID, err = requireProjectID(state, projectID)
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
					"id":             r.ID,
					"project_id":     r.ProjectID,
					"name":           r.Name,
					"description":    r.Description,
					"permissions":    r.Permissions,
					"parent_role_id": r.ParentRoleID,
					"is_system":      r.IsSystem,
					"created_at":     r.CreatedAt,
					"updated_at":     r.UpdatedAt,
				})
			}

			return printData(state, rows)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID")

	return cmd
}
