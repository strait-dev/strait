package billing

import (
	"sync"

	"github.com/stripe/stripe-go/v82"
)

// stripeKeyInitOnce ensures the global stripe.Key is set exactly once,
// preventing data races when multiple Stripe-backed constructors run
// concurrently (e.g. in parallel tests, or when both the usage reporter
// and the SLA credit issuer are constructed in services.go).
var stripeKeyInitOnce sync.Once

// ensureStripeKey sets the package-global Stripe API key on first call
// and is a no-op on every subsequent call regardless of the value passed.
// stripe-go uses a process-wide key by design; rotating it from multiple
// goroutines would race without this guard.
func ensureStripeKey(secretKey string) {
	if secretKey == "" {
		return
	}
	stripeKeyInitOnce.Do(func() {
		stripe.Key = secretKey //nolint:reassign // stripe-go uses a global key by design
	})
}
