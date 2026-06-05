//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestCreateEventSource(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID:   "project-event-src-create",
		Name:        "webhook-events",
		Description: "Incoming webhooks",
		Schema:      json.RawMessage(`{"type":"object"}`),
		Enabled:     true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))
	require.NotEqual(t, "",

		src.ID)
	require.False(t, src.CreatedAt.
		IsZero())
	require.False(t, src.UpdatedAt.
		IsZero())

	got, err := q.GetEventSource(ctx, src.ID, src.ProjectID)
	require.NoError(t, err)
	require.Equal(t, src.ID,

		got.ID)
	require.Equal(t, src.ProjectID,

		got.
			ProjectID,
	)
	require.Equal(t, src.Name,

		got.Name,
	)
	require.Equal(t, src.Description,

		got.Description,
	)
	require.True(t, jsonEqual(got.Schema,
		src.Schema,
	))
	require.Equal(t, src.Enabled,

		got.
			Enabled)

}

func TestGetEventSource_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetEventSource(ctx, newID(), "project-not-found")
	require.True(t, errors.Is(err, store.
		ErrEventSourceNotFound,
	))

}

func TestGetEventSourceByName(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID:   "project-event-src-byname",
		Name:        "deploy-events",
		Description: "Deployment events",
		Enabled:     true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))

	got, err := q.GetEventSourceByName(ctx, src.ProjectID, src.Name)
	require.NoError(t, err)
	require.Equal(t, src.ID,

		got.ID)

}

func TestListEventSources(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	projectID := "project-event-src-list"

	src1 := &domain.EventSource{
		ProjectID: projectID,
		Name:      "source-alpha",
		Enabled:   true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src1))

	src2 := &domain.EventSource{
		ProjectID: projectID,
		Name:      "source-beta",
		Enabled:   true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src2))

	list, err := q.ListEventSources(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, src2.ID,

		list[0].
			ID)
	require.Equal(t, src1.ID,

		list[1].
			ID)

	// Ordered by created_at DESC: src2 first.

}

func TestUpdateEventSource(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID:   "project-event-src-update",
		Name:        "update-me",
		Description: "Original desc",
		Enabled:     true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))

	originalUpdatedAt := src.UpdatedAt
	require.NoError(t, q.UpdateEventSource(ctx,
		src.ID, src.ProjectID,
		map[string]any{
			"description": "Updated desc",
			"enabled":     false}))

	got, err := q.GetEventSource(ctx, src.ID, src.ProjectID)
	require.NoError(t, err)
	require.Equal(t, "Updated desc",

		got.Description,
	)
	require.Equal(t, false,

		got.Enabled,
	)
	require.True(t, got.UpdatedAt.
		After(originalUpdatedAt))

}

func TestDeleteEventSource(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID: "project-event-src-delete",
		Name:      "delete-me",
		Enabled:   true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))
	require.NoError(t, q.DeleteEventSource(ctx,
		src.ID, src.ProjectID,
	))

	_, err := q.GetEventSource(ctx, src.ID, src.ProjectID)
	require.True(t, errors.Is(err, store.
		ErrEventSourceNotFound,
	))

}

func TestCreateEventSubscription(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID: "project-event-sub-create",
		Name:      "sub-source",
		Enabled:   true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))

	sub := &domain.EventSubscription{
		SourceID:   src.ID,
		TargetType: "job",
		TargetID:   newID(),
		FilterExpr: json.RawMessage(`{"event":"push"}`),
		Enabled:    true,
	}
	require.NoError(t, q.CreateEventSubscription(ctx, sub))
	require.NotEqual(t, "",

		sub.ID)
	require.False(t, sub.CreatedAt.
		IsZero())

	subs, err := q.ListEventSubscriptionsBySource(ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, sub.ID,

		subs[0].
			ID)
	require.Equal(t, src.ID,

		subs[0].
			SourceID)
	require.Equal(t, "job",

		subs[0].TargetType,
	)
	require.Equal(t, true,
		subs[0].Enabled,
	)
	require.True(t, jsonEqual(subs[0].
		FilterExpr,
		json.RawMessage(`{"event":"push"}`)),
	)

}

func TestDeleteEventSubscription(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	src := &domain.EventSource{
		ProjectID: "project-event-sub-delete",
		Name:      "sub-delete-source",
		Enabled:   true,
	}
	require.NoError(t, q.CreateEventSource(ctx,
		src))

	sub := &domain.EventSubscription{
		SourceID:   src.ID,
		TargetType: "job",
		TargetID:   newID(),
		Enabled:    true,
	}
	require.NoError(t, q.CreateEventSubscription(ctx, sub))
	require.NoError(t, q.DeleteEventSubscription(ctx, sub.ID))

	subs, err := q.ListEventSubscriptionsBySource(ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, subs, 0)

	// Deleting the same subscription again should return ErrEventSubscriptionNotFound.
	err = q.DeleteEventSubscription(ctx, sub.ID)
	require.True(t, errors.Is(err, store.
		ErrEventSubscriptionNotFound,
	))

}
