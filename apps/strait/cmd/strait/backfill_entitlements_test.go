package main

import "testing"

func TestNewBackfillEntitlementsCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newBackfillEntitlementsCommand()
	if cmd.Use != "backfill-entitlements" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "backfill-entitlements")
	}
	if cmd.Short == "" {
		t.Fatal("Short description is empty")
	}

	for _, name := range []string{"batch-size", "dry-run", "timeout", "org-id"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected --%s flag", name)
		}
	}

	if got := cmd.Flags().Lookup("batch-size").DefValue; got != "500" {
		t.Errorf("--batch-size default = %q, want %q", got, "500")
	}
	if got := cmd.Flags().Lookup("dry-run").DefValue; got != "false" {
		t.Errorf("--dry-run default = %q, want %q", got, "false")
	}
	if got := cmd.Flags().Lookup("timeout").DefValue; got != "1h0m0s" {
		t.Errorf("--timeout default = %q, want %q", got, "1h0m0s")
	}
}

func TestNewBackfillEntitlementsCommand_RegisteredOnRoot(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	if findSubcommand(root, "backfill-entitlements") == nil {
		t.Fatal("backfill-entitlements not registered on root command")
	}
}

func TestNewBackfillEntitlementsCommand_RejectsBadBatchSize(t *testing.T) {
	t.Parallel()

	cmd := newBackfillEntitlementsCommand()
	cmd.SetArgs([]string{"--batch-size", "0"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for --batch-size=0, got nil")
	}
}
