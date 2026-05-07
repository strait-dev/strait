//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAuditForensic_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("forensic-roundtrip-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
	q.SetAuditSigningKey(signingKey)

	projectID := "proj-forensic-" + newID()

	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ActorID:       "user:u-1",
		ActorType:     "user",
		Action:        domain.AuditActionJobCreated,
		ResourceType:  "job",
		ResourceID:    "job-1",
		Details:       json.RawMessage(`{"name":"test-job"}`),
		RemoteIP:      "198.51.100.42",
		UserAgent:     "TestAgent/1.0",
		RequestID:     "req-abc-123",
		TraceID:       "trace-def-456",
		SchemaVersion: 2,
	}
	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("CreateAuditEvent: %v", err)
	}
	if ev.ID == "" {
		t.Fatal("event ID was not populated after insert")
	}

	events, err := q.ListAuditEvents(ctx, projectID, "", "", "", 10, nil, nil, nil, false)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]

	if got.RemoteIP != ev.RemoteIP {
		t.Errorf("remote_ip = %q, want %q", got.RemoteIP, ev.RemoteIP)
	}
	if got.UserAgent != ev.UserAgent {
		t.Errorf("user_agent = %q, want %q", got.UserAgent, ev.UserAgent)
	}
	if got.RequestID != ev.RequestID {
		t.Errorf("request_id = %q, want %q", got.RequestID, ev.RequestID)
	}
	if got.TraceID != ev.TraceID {
		t.Errorf("trace_id = %q, want %q", got.TraceID, ev.TraceID)
	}
	if got.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", got.SchemaVersion)
	}
	if got.Signature == "" {
		t.Error("signature is empty, expected HMAC to be computed")
	}
}

func TestAuditForensic_ChainVerifiesWithForensicFields(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("forensic-chain-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
	q.SetAuditSigningKey(signingKey)

	projectID := "proj-forensic-chain-" + newID()

	for i := range 5 {
		ev := &domain.AuditEvent{
			ProjectID:     projectID,
			ActorID:       "user:u-1",
			ActorType:     "user",
			Action:        domain.AuditActionJobCreated,
			ResourceType:  "job",
			ResourceID:    "job-" + newID(),
			Details:       json.RawMessage(`{"seq":` + strconv.Itoa(i) + `}`),
			RemoteIP:      "10.0.0.1",
			UserAgent:     "ChainTest/2.0",
			RequestID:     "req-" + newID(),
			TraceID:       "trace-" + newID(),
			SchemaVersion: 2,
		}
		if err := q.CreateAuditEvent(ctx, ev); err != nil {
			t.Fatalf("CreateAuditEvent[%d]: %v", i, err)
		}
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid: %s (broken at %q)", result.Error, result.BrokenAtID)
	}
	if result.EventsChecked != 5 {
		t.Errorf("events_checked = %d, want 5", result.EventsChecked)
	}
}

func TestAuditForensic_DefaultsForOldEvents(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	signingKey, err := store.DeriveAuditSigningKey("forensic-compat-secret")
	if err != nil {
		t.Fatalf("derive signing key: %v", err)
	}
	q.SetAuditSigningKey(signingKey)

	projectID := "proj-forensic-compat-" + newID()

	ev := &domain.AuditEvent{
		ProjectID:     projectID,
		ActorID:       "user:u-1",
		ActorType:     "user",
		Action:        domain.AuditActionJobCreated,
		ResourceType:  "job",
		ResourceID:    "job-compat",
		Details:       json.RawMessage(`{"legacy":true}`),
		SchemaVersion: 1,
	}
	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("CreateAuditEvent (v1): %v", err)
	}

	result, err := q.VerifyAuditChain(ctx, projectID)
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("chain invalid for v1 event: %s (broken at %q)", result.Error, result.BrokenAtID)
	}
}
