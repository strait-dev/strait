package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzValidateScopes(f *testing.F) {
	f.Add("jobs:read")
	f.Add("*")
	f.Add("")
	f.Add("unknown:scope")
	f.Add("jobs:read,runs:write")
	f.Add("api-keys:manage")

	f.Fuzz(func(t *testing.T, s string) {
		// ValidateScopes should never panic regardless of input.
		_ = ValidateScopes([]string{s})
	})
}

func FuzzParseEdition(f *testing.F) {
	f.Add("community")
	f.Add("cloud")
	f.Add("")
	f.Add("enterprise")
	f.Add("CLOUD")
	f.Add("Community")

	f.Fuzz(func(t *testing.T, s string) {
		// ParseEdition should never panic regardless of input.
		edition := ParseEdition(s)
		require.False(
			t, edition != EditionCommunity && edition !=
				EditionCloud)

		// Result must be one of the known values.

	})
}
