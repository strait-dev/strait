//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
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
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource() error = %v", err)
	}

	if src.ID == "" {
		t.Fatal("CreateEventSource() did not set ID")
	}
	if src.CreatedAt.IsZero() {
		t.Fatal("CreateEventSource() did not set CreatedAt")
	}
	if src.UpdatedAt.IsZero() {
		t.Fatal("CreateEventSource() did not set UpdatedAt")
	}

	got, err := q.GetEventSource(ctx, src.ID, src.ProjectID)
	if err != nil {
		t.Fatalf("GetEventSource() error = %v", err)
	}
	if got.ID != src.ID {
		t.Fatalf("GetEventSource() ID = %q, want %q", got.ID, src.ID)
	}
	if got.ProjectID != src.ProjectID {
		t.Fatalf("GetEventSource() ProjectID = %q, want %q", got.ProjectID, src.ProjectID)
	}
	if got.Name != src.Name {
		t.Fatalf("GetEventSource() Name = %q, want %q", got.Name, src.Name)
	}
	if got.Description != src.Description {
		t.Fatalf("GetEventSource() Description = %q, want %q", got.Description, src.Description)
	}
	if !jsonEqual(got.Schema, src.Schema) {
		t.Fatalf("GetEventSource() Schema = %s, want %s", got.Schema, src.Schema)
	}
	if got.Enabled != src.Enabled {
		t.Fatalf("GetEventSource() Enabled = %v, want %v", got.Enabled, src.Enabled)
	}
}

func TestGetEventSource_NotFound(t *testing.T) {
	ctx := context.Background()
	q := mustStore(t)
	mustClean(t, ctx)

	_, err := q.GetEventSource(ctx, newID(), "project-not-found")
	if !errors.Is(err, store.ErrEventSourceNotFound) {
		t.Fatalf("GetEventSource() error = %v, want ErrEventSourceNotFound", err)
	}
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
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource() error = %v", err)
	}

	got, err := q.GetEventSourceByName(ctx, src.ProjectID, src.Name)
	if err != nil {
		t.Fatalf("GetEventSourceByName() error = %v", err)
	}
	if got.ID != src.ID {
		t.Fatalf("GetEventSourceByName() ID = %q, want %q", got.ID, src.ID)
	}
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
	if err := q.CreateEventSource(ctx, src1); err != nil {
		t.Fatalf("CreateEventSource(1) error = %v", err)
	}

	src2 := &domain.EventSource{
		ProjectID: projectID,
		Name:      "source-beta",
		Enabled:   true,
	}
	if err := q.CreateEventSource(ctx, src2); err != nil {
		t.Fatalf("CreateEventSource(2) error = %v", err)
	}

	list, err := q.ListEventSources(ctx, projectID)
	if err != nil {
		t.Fatalf("ListEventSources() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListEventSources() len = %d, want 2", len(list))
	}
	// Ordered by created_at DESC: src2 first.
	if list[0].ID != src2.ID {
		t.Fatalf("ListEventSources()[0].ID = %q, want %q (most recent)", list[0].ID, src2.ID)
	}
	if list[1].ID != src1.ID {
		t.Fatalf("ListEventSources()[1].ID = %q, want %q (oldest)", list[1].ID, src1.ID)
	}
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
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource() error = %v", err)
	}

	originalUpdatedAt := src.UpdatedAt

	if err := q.UpdateEventSource(ctx, src.ID, src.ProjectID, map[string]any{
		"description": "Updated desc",
		"enabled":     false,
	}); err != nil {
		t.Fatalf("UpdateEventSource() error = %v", err)
	}

	got, err := q.GetEventSource(ctx, src.ID, src.ProjectID)
	if err != nil {
		t.Fatalf("GetEventSource() error = %v", err)
	}
	if got.Description != "Updated desc" {
		t.Fatalf("GetEventSource() Description = %q, want %q", got.Description, "Updated desc")
	}
	if got.Enabled != false {
		t.Fatalf("GetEventSource() Enabled = %v, want false", got.Enabled)
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Fatalf("GetEventSource() UpdatedAt not advanced: got %v, original %v", got.UpdatedAt, originalUpdatedAt)
	}
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
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource() error = %v", err)
	}

	if err := q.DeleteEventSource(ctx, src.ID, src.ProjectID); err != nil {
		t.Fatalf("DeleteEventSource() error = %v", err)
	}

	_, err := q.GetEventSource(ctx, src.ID, src.ProjectID)
	if !errors.Is(err, store.ErrEventSourceNotFound) {
		t.Fatalf("GetEventSource() after delete error = %v, want ErrEventSourceNotFound", err)
	}
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
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource() error = %v", err)
	}

	sub := &domain.EventSubscription{
		SourceID:   src.ID,
		TargetType: "job",
		TargetID:   newID(),
		FilterExpr: json.RawMessage(`{"event":"push"}`),
		Enabled:    true,
	}
	if err := q.CreateEventSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateEventSubscription() error = %v", err)
	}

	if sub.ID == "" {
		t.Fatal("CreateEventSubscription() did not set ID")
	}
	if sub.CreatedAt.IsZero() {
		t.Fatal("CreateEventSubscription() did not set CreatedAt")
	}

	subs, err := q.ListEventSubscriptionsBySource(ctx, src.ID)
	if err != nil {
		t.Fatalf("ListEventSubscriptionsBySource() error = %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("ListEventSubscriptionsBySource() len = %d, want 1", len(subs))
	}
	if subs[0].ID != sub.ID {
		t.Fatalf("ListEventSubscriptionsBySource()[0].ID = %q, want %q", subs[0].ID, sub.ID)
	}
	if subs[0].SourceID != src.ID {
		t.Fatalf("ListEventSubscriptionsBySource()[0].SourceID = %q, want %q", subs[0].SourceID, src.ID)
	}
	if subs[0].TargetType != "job" {
		t.Fatalf("ListEventSubscriptionsBySource()[0].TargetType = %q, want %q", subs[0].TargetType, "job")
	}
	if subs[0].Enabled != true {
		t.Fatalf("ListEventSubscriptionsBySource()[0].Enabled = %v, want true", subs[0].Enabled)
	}
	if !jsonEqual(subs[0].FilterExpr, json.RawMessage(`{"event":"push"}`)) {
		t.Fatalf("ListEventSubscriptionsBySource()[0].FilterExpr = %s, want %s", subs[0].FilterExpr, `{"event":"push"}`)
	}
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
	if err := q.CreateEventSource(ctx, src); err != nil {
		t.Fatalf("CreateEventSource() error = %v", err)
	}

	sub := &domain.EventSubscription{
		SourceID:   src.ID,
		TargetType: "job",
		TargetID:   newID(),
		Enabled:    true,
	}
	if err := q.CreateEventSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateEventSubscription() error = %v", err)
	}

	if err := q.DeleteEventSubscription(ctx, sub.ID); err != nil {
		t.Fatalf("DeleteEventSubscription() error = %v", err)
	}

	subs, err := q.ListEventSubscriptionsBySource(ctx, src.ID)
	if err != nil {
		t.Fatalf("ListEventSubscriptionsBySource() error = %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("ListEventSubscriptionsBySource() len = %d, want 0", len(subs))
	}

	// Deleting the same subscription again should return ErrEventSubscriptionNotFound.
	err = q.DeleteEventSubscription(ctx, sub.ID)
	if !errors.Is(err, store.ErrEventSubscriptionNotFound) {
		t.Fatalf("DeleteEventSubscription() second call error = %v, want ErrEventSubscriptionNotFound", err)
	}
}
