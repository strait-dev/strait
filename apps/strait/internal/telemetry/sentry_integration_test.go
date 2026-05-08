//go:build integration

package telemetry

import (
	"errors"
	"net/http"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestIntegrationSentryBeforeSendDropsExpectedNoiseAndKeepsServerErrors(t *testing.T) {
	t.Parallel()

	dropped := BeforeSend(
		&sentry.Event{Request: &sentry.Request{URL: "https://api.example.test/v1/jobs"}},
		&sentry.EventHint{OriginalException: testStatusError{status: http.StatusNotFound}},
	)
	if dropped != nil {
		t.Fatal("expected request 4xx error to be dropped")
	}

	kept := BeforeSend(
		&sentry.Event{Message: "dispatch failed"},
		&sentry.EventHint{OriginalException: errors.New("dispatch panic")},
	)
	if kept == nil {
		t.Fatal("expected dispatch panic to be kept")
	}
}
