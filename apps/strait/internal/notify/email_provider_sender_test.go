package notify

import (
	"context"
	"strings"
	"testing"
)

func TestSendEmailWithProvider_RejectsLegacyResendByDefault(t *testing.T) {
	t.Parallel()

	_, err := SendEmailWithProvider(
		context.Background(),
		"msg_1",
		"proj_1",
		"user@example.com",
		"subject",
		"<p>hello</p>",
		"hello",
		EmailProviderAttempt{Provider: "resend"},
	)
	if err == nil {
		t.Fatal("expected error for resend provider when legacy mode disabled")
	}
	if !strings.Contains(err.Error(), "NOTIFY_EMAIL_ALLOW_LEGACY_RESEND") {
		t.Fatalf("error = %q, want NOTIFY_EMAIL_ALLOW_LEGACY_RESEND hint", err.Error())
	}
}

func TestSendEmailWithProvider_RejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := SendEmailWithProvider(
		context.Background(),
		"msg_1",
		"proj_1",
		"user@example.com",
		"subject",
		"<p>hello</p>",
		"hello",
		EmailProviderAttempt{Provider: "mailgun"},
	)
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "unsupported email provider") {
		t.Fatalf("error = %q, want unsupported provider", err.Error())
	}
}
