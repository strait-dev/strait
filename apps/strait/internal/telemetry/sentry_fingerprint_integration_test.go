//go:build integration

package telemetry

import (
	"errors"
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestIntegrationSentryFingerprintAndRelease(t *testing.T) {
	t.Parallel()

	event := &sentry.Event{
		Tags: map[string]string{
			"subsystem":   SubsystemWorkflow,
			"error_class": "server",
		},
		Breadcrumbs: []*sentry.Breadcrumb{{
			Category: "workflow.step",
			Data: map[string]any{
				"step_type": "job",
			},
		}},
	}
	got := BeforeSend(event, &sentry.EventHint{OriginalException: errors.New("workflow step failed")})
	if got == nil {
		t.Fatal("expected workflow event to be kept")
	}
	if want := []string{"workflow", "job", "server"}; !reflect.DeepEqual(got.Fingerprint, want) {
		t.Fatalf("fingerprint = %#v, want %#v", got.Fingerprint, want)
	}
	if release := BuildSentryRelease("v1.2.3", "abcdef1234567890"); release != "v1.2.3+abcdef123456" {
		t.Fatalf("release = %q, want v1.2.3+abcdef123456", release)
	}
}
