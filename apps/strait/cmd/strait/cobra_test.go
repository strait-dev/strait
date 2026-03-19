package main

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestRootCommand_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	expected := []string{
		"serve", "server", "dev", "init", "version", "completion",
		"context", "alias", "login", "logout", "auth",
		"jobs", "runs", "migrate", "trigger", "health",
		"workflows", "workflow-runs", "api-keys", "stats", "api",
		"wait", "docs", "logs", "events", "verify", "diagnose",
		"top", "tui", "validate", "apply", "diff", "export",
		"db", "run", "send", "secrets", "fixtures",
		"check", "cleanup", "extension", "listen", "drain",
		"trace", "upgrade", "backup", "profile",
		"deploy", "project", "build", "doctor", "open",
		"status", "debug", "create", "ci", "perf", "team", "audit",
	}

	subs := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subs[sub.Name()] = true
	}

	for _, name := range expected {
		if !subs[name] {
			t.Errorf("missing subcommand: %s", name)
		}
	}
}

func TestRootCommand_PersistentFlags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	flags := []string{
		"server", "api-key", "project", "format", "no-headers",
		"output-template", "output-jsonpath", "no-color", "quiet",
		"verbose", "context", "config", "timeout", "ci",
	}

	for _, name := range flags {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("missing persistent flag: --%s", name)
		}
	}
}

func TestRootCommand_FlagDefaults(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()

	tests := []struct {
		flag string
		want string
	}{
		{flag: "timeout", want: (30 * time.Second).String()},
		{flag: "ci", want: "false"},
		{flag: "quiet", want: "false"},
		{flag: "no-color", want: "false"},
		{flag: "verbose", want: "false"},
		{flag: "no-headers", want: "false"},
	}

	for _, tc := range tests {
		f := cmd.PersistentFlags().Lookup(tc.flag)
		if f == nil {
			t.Errorf("flag --%s not found", tc.flag)
			continue
		}
		if f.DefValue != tc.want {
			t.Errorf("flag --%s default: got %q, want %q", tc.flag, f.DefValue, tc.want)
		}
	}
}

func TestJobsCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	jobs := findSubcommand(t, cmd, "jobs")

	expected := []string{"list", "get", "create", "trigger", "trigger-bulk", "delete", "versions", "describe", "edit"}
	assertSubcommands(t, jobs, expected)
}

func TestJobsListCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	jobs := findSubcommand(t, cmd, "jobs")
	list := findSubcommand(t, jobs, "list")

	if list.Flags().Lookup("project") == nil {
		t.Error("jobs list missing --project flag")
	}
}

func TestRunsCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	runs := findSubcommand(t, cmd, "runs")

	expected := []string{"list", "get", "cancel", "logs", "watch", "replay", "last", "diff"}
	assertSubcommands(t, runs, expected)
}

func TestRunsListCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	runs := findSubcommand(t, cmd, "runs")
	list := findSubcommand(t, runs, "list")

	for _, name := range []string{"project", "status", "limit"} {
		if list.Flags().Lookup(name) == nil {
			t.Errorf("runs list missing --%s flag", name)
		}
	}

	limitFlag := list.Flags().Lookup("limit")
	if limitFlag.DefValue != "50" {
		t.Errorf("runs list --limit default: got %q, want 50", limitFlag.DefValue)
	}
}

func TestRunsCancelCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	runs := findSubcommand(t, cmd, "runs")
	cancel := findSubcommand(t, runs, "cancel")

	for _, name := range []string{"all", "yes"} {
		if cancel.Flags().Lookup(name) == nil {
			t.Errorf("runs cancel missing --%s flag", name)
		}
	}
}

func TestWorkflowsCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	wf := findSubcommand(t, cmd, "workflows")

	expected := []string{"list", "get", "create", "trigger", "delete", "visualize"}
	assertSubcommands(t, wf, expected)
}

func TestVersionCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	version := findSubcommand(t, cmd, "version")

	for _, name := range []string{"short", "json", "check-server", "check-update"} {
		if version.Flags().Lookup(name) == nil {
			t.Errorf("version missing --%s flag", name)
		}
	}
}

func TestCIMode_Flag(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	ci := cmd.PersistentFlags().Lookup("ci")
	if ci == nil {
		t.Fatal("missing --ci flag")
	}
	if ci.DefValue != "false" {
		t.Errorf("--ci default: got %q, want false", ci.DefValue)
	}
}

func TestDeployCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	deploy := findSubcommand(t, cmd, "deploy")

	expected := []string{"create", "finalize", "promote", "rollback", "list", "preview"}
	assertSubcommands(t, deploy, expected)
}

func TestBuildCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	build := findSubcommand(t, cmd, "build")

	for _, name := range []string{"config", "out-dir", "dry-run", "json"} {
		if build.Flags().Lookup(name) == nil {
			t.Errorf("build missing --%s flag", name)
		}
	}
}

func TestDoctorCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	doctor := findSubcommand(t, cmd, "doctor")

	for _, name := range []string{"verbose", "json", "fix", "check-endpoints", "check-manifests"} {
		if doctor.Flags().Lookup(name) == nil {
			t.Errorf("doctor missing --%s flag", name)
		}
	}
}

func TestCreateCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	create := findSubcommand(t, cmd, "create")

	expected := []string{"job", "workflow"}
	assertSubcommands(t, create, expected)
}

func TestSecretsCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	secrets := findSubcommand(t, cmd, "secrets")

	expected := []string{"list", "create", "delete", "local"}
	assertSubcommands(t, secrets, expected)
}

func TestTeamCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	team := findSubcommand(t, cmd, "team")

	expected := []string{"list", "add", "remove", "roles"}
	assertSubcommands(t, team, expected)
}

func TestCICommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	ci := findSubcommand(t, cmd, "ci")

	expected := []string{"setup", "check"}
	assertSubcommands(t, ci, expected)
}

func TestDevCommand_HasOnlyStatusSubcommand(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	dev := findSubcommand(t, cmd, "dev")

	expected := []string{"status"}
	assertSubcommands(t, dev, expected)

	for _, unexpected := range []string{"test", "tunnel"} {
		for _, sub := range dev.Commands() {
			if sub.Name() == unexpected {
				t.Fatalf("dev should not expose %q", unexpected)
			}
		}
	}
}

func TestDebugCommand_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	debug := findSubcommand(t, cmd, "debug")

	expected := []string{"bundle"}
	assertSubcommands(t, debug, expected)
}

func TestPerfCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	perf := findSubcommand(t, cmd, "perf")

	for _, name := range []string{"project", "period", "json"} {
		if perf.Flags().Lookup(name) == nil {
			t.Errorf("perf missing --%s flag", name)
		}
	}
	if err := perf.Args(perf, []string{"job-slug"}); err == nil {
		t.Fatal("perf should reject positional job arguments")
	}
}

func TestAuditCommand_Flags(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	audit := findSubcommand(t, cmd, "audit")

	for _, name := range []string{"project", "actor-id", "resource-type", "resource-id", "limit", "from", "to", "order"} {
		if audit.Flags().Lookup(name) == nil {
			t.Errorf("audit missing --%s flag", name)
		}
	}
}

func TestTeamAddCommand_UsesUserAndRoleIDs(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	team := findSubcommand(t, cmd, "team")
	add := findSubcommand(t, team, "add")

	if add.Use != "add <user-id>" {
		t.Fatalf("unexpected usage: %s", add.Use)
	}
	if add.Flags().Lookup("role-id") == nil {
		t.Fatal("team add missing --role-id flag")
	}
	if add.Flags().Lookup("role") != nil {
		t.Fatal("team add should not expose deprecated --role flag")
	}
}

// Test helpers.

func findSubcommand(t *testing.T, parent interface{ Commands() []*cobra.Command }, name string) *cobra.Command {
	t.Helper()
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("subcommand %q not found", name)
	return nil
}

func assertSubcommands(t *testing.T, parent interface{ Commands() []*cobra.Command }, expected []string) {
	t.Helper()
	subs := make(map[string]bool)
	for _, sub := range parent.Commands() {
		subs[sub.Name()] = true
	}
	for _, name := range expected {
		if !subs[name] {
			t.Errorf("missing subcommand: %s", name)
		}
	}
}
