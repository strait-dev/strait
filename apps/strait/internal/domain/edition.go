package domain

// Edition represents which edition of Strait is running.
type Edition string

const (
	EditionCommunity Edition = "community"
	EditionCloud     Edition = "cloud"
)

// ParseEdition is defined in edition_community.go (default) or
// edition_cloud.go (when built with -tags cloud).

// AllowsManagedExecution returns true when managed container execution is available.
func (e Edition) AllowsManagedExecution() bool { return e == EditionCloud }

// AllowsMultiRegion returns true when multi-region execution is available.
func (e Edition) AllowsMultiRegion() bool { return e == EditionCloud }

// AllowsAdvancedAnalytics returns true when advanced ClickHouse-backed analytics are available.
func (e Edition) AllowsAdvancedAnalytics() bool { return e == EditionCloud }

// AllowsWarmPool returns true when warm machine pool management is available.
func (e Edition) AllowsWarmPool() bool { return e == EditionCloud }

// RequiresHTTPModeGating returns true when HTTP execution mode should be gated by plan.
// On cloud, HTTP mode is restricted to Pro+. On community (self-hosted), HTTP mode is unrestricted.
func (e Edition) RequiresHTTPModeGating() bool { return e == EditionCloud }
