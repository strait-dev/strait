//go:build !cloud

package domain

// ParseEdition always returns EditionCommunity in the community build.
// Cloud features require the cloud-tagged binary deployed on strait.dev.
func ParseEdition(_ string) Edition {
	return EditionCommunity
}
