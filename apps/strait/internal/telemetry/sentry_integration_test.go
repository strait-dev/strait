//go:build integration

package telemetry

import (
	"errors"
	"net/http"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
)

func TestIntegrationSentryBeforeSendDropsExpectedNoiseAndKeepsServerErrors(t *testing.T) {
	t.Parallel()

	dropped := BeforeSend(
		&sentry.Event{Request: &sentry.Request{URL: "https://api.example.test/v1/jobs"}},
		&sentry.EventHint{OriginalException: testStatusError{status: http.StatusNotFound}},
	)
	require.Nil(t, dropped)

	kept := BeforeSend(
		&sentry.Event{Message: "dispatch failed"},
		&sentry.EventHint{OriginalException: errors.New("dispatch panic")},
	)
	require.NotNil(t, kept)

}
