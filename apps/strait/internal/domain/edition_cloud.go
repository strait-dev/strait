//go:build cloud

package domain

// ParseEdition always returns EditionCloud in the cloud build.
// The edition is determined at compile time by the build tag, not by
// configuration. This prevents anyone from running cloud features on
// a community build or downgrading a cloud build via env vars.
func ParseEdition(_ string) Edition {
	return EditionCloud
}
