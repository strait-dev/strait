//go:build !cloud

package billing

// The Stripe-backed SLA credit issuer is cloud-only. Community builds
// leave SLACalculator.issuer nil, which means the calculator persists
// credit rows and dispatches sla.credit_issued without writing to any
// Stripe-side artifact. See sla_credit.go: WithIssuer for the nil-path
// contract.
