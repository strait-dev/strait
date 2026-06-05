package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackfillEntitlementsCommand_Structure(t *testing.T) {
	t.Parallel()

	cmd := newBackfillEntitlementsCommand()
	assert.Equal(t, "backfill-entitlements", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	for _, name := range []string{"batch-size", "dry-run", "timeout", "org-id"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "expected --%s flag", name)
	}

	require.NotNil(t, cmd.Flags().Lookup("batch-size"))
	require.NotNil(t, cmd.Flags().Lookup("dry-run"))
	require.NotNil(t, cmd.Flags().Lookup("timeout"))
	assert.Equal(t, "500", cmd.Flags().Lookup("batch-size").DefValue)
	assert.Equal(t, "false", cmd.Flags().Lookup("dry-run").DefValue)
	assert.Equal(t, "1h0m0s", cmd.Flags().Lookup("timeout").DefValue)
}

func TestNewBackfillEntitlementsCommand_RegisteredOnRoot(t *testing.T) {
	t.Parallel()

	root := newRootCommand()
	assert.NotNil(t, findSubcommand(root, "backfill-entitlements"))
}

func TestNewBackfillEntitlementsCommand_RejectsBadBatchSize(t *testing.T) {
	t.Parallel()

	cmd := newBackfillEntitlementsCommand()
	cmd.SetArgs([]string{"--batch-size", "0"})
	assert.Error(t, cmd.Execute())
}
