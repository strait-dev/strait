//go:build cloud

package main

import (
	"log/slog"

	"strait/internal/billing"
	"strait/internal/config"
)

// newSLAIssuer constructs the cloud-only Stripe-backed SLA credit
// issuer. A missing Stripe secret leaves the issuer unset, which causes
// SLACalculator to log + persist + dispatch the credit but skip the
// Stripe-side artifact — matching the operator escape-hatch behavior
// already documented on billing.SLACalculator.WithIssuer.
func newSLAIssuer(cfg *config.Config, store billing.CustomerLookupStore, logger *slog.Logger) billing.SLACreditIssuer {
	if cfg.StripeSecretKey == "" {
		logger.Warn("sla_credit_issuer_disabled_no_stripe_key")
		return nil
	}
	return billing.NewStripeSLAIssuer(cfg.StripeSecretKey, store, logger)
}
