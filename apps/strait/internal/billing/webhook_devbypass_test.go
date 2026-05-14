package billing

// WithDevBypassSignatureCheck allows skipping Stripe signature verification.
// It lives in a _test.go file by design: only test binaries can link it, so
// production builds cannot accidentally bypass signature verification even if
// the option name is referenced elsewhere by mistake.
func WithDevBypassSignatureCheck() WebhookOption {
	return func(h *WebhookHandler) {
		h.devBypassSigCheck = true
		h.allowTestMetadata = true
	}
}
