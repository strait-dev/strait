package domain

// Edition represents which edition of Strait is running.
type Edition string

const (
	EditionCommunity Edition = "community"
	EditionCloud     Edition = "cloud"
)

// ParseEdition normalizes a string into a known Edition value.
// Unknown values default to EditionCommunity.
func ParseEdition(s string) Edition {
	if s == "cloud" {
		return EditionCloud
	}
	return EditionCommunity
}

// AllowsManagedExecution returns true when managed container execution is available.
func (e Edition) AllowsManagedExecution() bool { return e == EditionCloud }

// AllowsMultiRegion returns true when multi-region execution is available.
func (e Edition) AllowsMultiRegion() bool { return e == EditionCloud }

// AllowsAdvancedAnalytics returns true when advanced ClickHouse-backed analytics are available.
func (e Edition) AllowsAdvancedAnalytics() bool { return e == EditionCloud }

// AllowsWarmPool returns true when warm machine pool management is available.
func (e Edition) AllowsWarmPool() bool { return e == EditionCloud }
