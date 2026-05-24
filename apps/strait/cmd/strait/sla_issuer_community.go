//go:build !cloud

package main

import (
	"log/slog"

	"strait/internal/billing"
	"strait/internal/config"
)

// newSLAIssuer is a no-op in community builds. The Stripe SDK only
// links into the cloud edition, so there is no concrete issuer to wire
// here; returning nil keeps SLACalculator on the "persist + dispatch
// without Stripe write" path.
func newSLAIssuer(_ *config.Config, _ billing.CustomerLookupStore, _ *slog.Logger) billing.SLACreditIssuer {
	return nil
}
