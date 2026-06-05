package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validChannelBody() string {
	return `{
		"channel_type": "slack",
		"name": "alerts",
		"config": {"webhook_url": "https://hooks.slack.com/services/T/B/X"}
	}`
}

// TestCreateNotificationChannel_FreeTier_RejectsZeroCap proves that Free
// (cap=0) rejects channel creation outright before any store call.
func TestCreateNotificationChannel_FreeTier_RejectsZeroCap(t *testing.T) {
	t.Parallel()

	createCalls := atomic.Int64{}
	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, _ *domain.NotificationChannel) error {
			createCalls.Add(1)
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: freeLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)
	assert.True(t,
		strings.Contains(
			w.Body.String(), "not available",
		))
	assert.EqualValues(t, 0, createCalls.
		Load())

}

// TestCreateNotificationChannel_ProTier_BlocksAtCap verifies that on Pro
// (cap=5) the 6th channel is rejected with a message naming the cap and
// current count.
func TestCreateNotificationChannel_ProTier_BlocksAtCap(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountNotificationChannelsByProjectFunc: func(_ context.Context, _ string) (int, error) {
			return 5, nil
		},
		CreateNotificationChannelFunc: func(_ context.Context, _ *domain.NotificationChannel) error {
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
	require.Equal(t, http.
		StatusBadRequest,
		w.Code)

	body := w.Body.String()
	assert.False(
		t, !strings.Contains(body, "5 notification channels") || !strings.Contains(body,

			"have 5"))

}

// TestCreateNotificationChannel_ProTier_BelowCap_Succeeds verifies that 4
// channels under the Pro cap of 5 still allows creation of the 5th.
func TestCreateNotificationChannel_ProTier_BelowCap_Succeeds(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountNotificationChannelsByProjectFunc: func(_ context.Context, _ string) (int, error) {
			return 4, nil
		},
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
	require.Equal(t, http.
		StatusCreated,
		w.Code)

}

// TestCreateNotificationChannel_EnterpriseUnlimited_NoCountLookup confirms
// that the unlimited tier short-circuits without querying the count.
func TestCreateNotificationChannel_EnterpriseUnlimited_NoCountLookup(t *testing.T) {
	t.Parallel()

	countCalls := atomic.Int64{}
	ms := &APIStoreMock{
		CountNotificationChannelsByProjectFunc: func(_ context.Context, _ string) (int, error) {
			countCalls.Add(1)
			return 9999, nil
		},
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: enterpriseLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
	require.Equal(t, http.
		StatusCreated,
		w.Code)
	assert.EqualValues(t, 0, countCalls.
		Load())

}

// TestCreateNotificationChannel_NilEnforcer_FailsOpen confirms that the
// community edition (no billing enforcer) does not block channel creation.
func TestCreateNotificationChannel_NilEnforcer_FailsOpen(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.edition = domain.EditionCommunity

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
	require.Equal(t, http.
		StatusCreated,
		w.Code)

}

// TestCreateNotificationChannel_CountQueryFails_FailsClosed ensures a transient
// store failure does not bypass the paid notification-channel cap.
func TestCreateNotificationChannel_CountQueryFails_FailsClosed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CountNotificationChannelsByProjectFunc: func(_ context.Context, _ string) (int, error) {
			return 0, fmt.Errorf("transient db failure")
		},
		CreateNotificationChannelFunc: func(_ context.Context, ch *domain.NotificationChannel) error {
			ch.ID = "ch-1"
			return nil
		},
	}
	enforcer := &tunableLimitsEnforcer{limits: proLimits()}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/notification-channels", validChannelBody(), "proj-1"))
	require.Equal(t, http.
		StatusServiceUnavailable,
		w.Code,
	)

}
