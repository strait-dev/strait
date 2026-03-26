//go:build cloud

package domain

// ParseEdition normalizes a string into a known Edition value.
// Unknown values default to EditionCommunity.
func ParseEdition(s string) Edition {
	if s == "cloud" {
		return EditionCloud
	}
	return EditionCommunity
}
