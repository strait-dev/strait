package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func baseDeps(stdout, stderr io.Writer) auditVerifyDeps {
	tick := time.Unix(0, 0)
	return auditVerifyDeps{
		verify: func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
			return &domain.AuditChainVerification{
				ProjectID:     projectID,
				Valid:         true,
				EventsChecked: 42,
			}, nil
		},
		emit: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
		now: func() time.Time {
			tick = tick.Add(5 * time.Millisecond)
			return tick
		},
		stdout:  stdout,
		stderr:  stderr,
		actorID: "test-user",
	}
}

func TestAuditVerify_CLI_PrintsPassForValidChain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := baseDeps(&stdout, &stderr)

	err := runAuditVerify(context.Background(), deps, "proj_123", "text")
	if err != nil {
		t.Fatalf("expected nil error for passing chain, got %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "PASS") {
		t.Errorf("expected PASS in output, got %q", out)
	}
	if !strings.Contains(out, "proj_123") {
		t.Errorf("expected project id in output, got %q", out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("expected events_checked=42 in output, got %q", out)
	}
}

func TestAuditVerify_CLI_ExitCodeNonZeroOnBreak(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := baseDeps(&stdout, &stderr)
	deps.verify = func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
		return &domain.AuditChainVerification{
			ProjectID:     projectID,
			Valid:         false,
			EventsChecked: 7,
			BrokenAtID:    "evt_bad",
			Error:         "signature mismatch",
		}, nil
	}

	err := runAuditVerify(context.Background(), deps, "proj_123", "text")
	if err == nil {
		t.Fatal("expected non-nil error on chain break (to signal non-zero exit)")
	}
	if !strings.Contains(stdout.String(), "FAIL") {
		t.Errorf("expected FAIL in output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "evt_bad") {
		t.Errorf("expected broken event id in output, got %q", stdout.String())
	}
}

func TestAuditVerify_CLI_JSONOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := baseDeps(&stdout, &stderr)

	if err := runAuditVerify(context.Background(), deps, "proj_json", "json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got auditVerifyOutput
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v — %s", err, stdout.String())
	}
	if got.ProjectID != "proj_json" {
		t.Errorf("project_id: got %q", got.ProjectID)
	}
	if got.Status != "PASS" {
		t.Errorf("status: got %q, want PASS", got.Status)
	}
	if got.EventsChecked != 42 {
		t.Errorf("events_checked: got %d, want 42", got.EventsChecked)
	}
	if got.FirstBreak != nil {
		t.Errorf("first_break should be nil on PASS, got %+v", got.FirstBreak)
	}
	if got.DurationMS < 0 {
		t.Errorf("duration_ms should be non-negative, got %d", got.DurationMS)
	}
}

func TestAuditVerify_CLI_JSONOutput_OnBreakIncludesFirstBreak(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := baseDeps(&stdout, &stderr)
	deps.verify = func(_ context.Context, projectID string) (*domain.AuditChainVerification, error) {
		return &domain.AuditChainVerification{
			ProjectID:  projectID,
			Valid:      false,
			BrokenAtID: "evt_bad",
			Error:      "signature mismatch",
		}, nil
	}

	err := runAuditVerify(context.Background(), deps, "proj_x", "json")
	if err == nil {
		t.Fatal("expected error on break")
	}
	var got auditVerifyOutput
	if decErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &got); decErr != nil {
		t.Fatalf("invalid JSON: %v — %s", decErr, stdout.String())
	}
	if got.Status != "FAIL" {
		t.Errorf("status: got %q", got.Status)
	}
	if got.FirstBreak == nil || got.FirstBreak.EventID != "evt_bad" {
		t.Errorf("first_break missing or wrong: %+v", got.FirstBreak)
	}
}

func TestAuditVerify_CLI_EmitsSelfAudit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := baseDeps(&stdout, &stderr)

	var captured *domain.AuditEvent
	deps.emit = func(_ context.Context, ev *domain.AuditEvent) error {
		captured = ev
		return nil
	}

	if err := runAuditVerify(context.Background(), deps, "proj_ae", "text"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured == nil {
		t.Fatal("self-audit emit was never called")
	}
	if captured.Action != domain.AuditActionAuditChainVerified {
		t.Errorf("action: got %q, want %q", captured.Action, domain.AuditActionAuditChainVerified)
	}
	if captured.ActorType != "cli" {
		t.Errorf("actor_type: got %q, want cli", captured.ActorType)
	}
	if captured.ProjectID != "proj_ae" {
		t.Errorf("project_id: got %q", captured.ProjectID)
	}

	var details map[string]any
	if err := json.Unmarshal(captured.Details, &details); err != nil {
		t.Fatalf("details not valid JSON: %v", err)
	}
	if passed, _ := details["passed"].(bool); !passed {
		t.Errorf("details.passed: got %v, want true", details["passed"])
	}
	if _, ok := details["events_checked"]; !ok {
		t.Error("details.events_checked missing")
	}
}

func TestAuditVerify_CLI_EmitsSelfAuditOnFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	deps := baseDeps(&stdout, &stderr)
	deps.verify = func(_ context.Context, _ string) (*domain.AuditChainVerification, error) {
		return nil, errors.New("db down")
	}
	var captured *domain.AuditEvent
	deps.emit = func(_ context.Context, ev *domain.AuditEvent) error {
		captured = ev
		return nil
	}

	_ = runAuditVerify(context.Background(), deps, "proj_err", "text")
	if captured == nil {
		t.Fatal("self-audit emit should still fire on verifier error")
	}
	var details map[string]any
	_ = json.Unmarshal(captured.Details, &details)
	if passed, _ := details["passed"].(bool); passed {
		t.Errorf("details.passed should be false on verifier error")
	}
}

func TestAuditVerify_CLI_RequiresProjectFlag(t *testing.T) {
	cmd := newAuditCommand()
	cmd.SetArgs([]string{"verify"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --project is missing")
	}
	if !strings.Contains(err.Error(), "--project") {
		t.Errorf("expected --project mention in error, got %v", err)
	}
}

func TestAuditVerify_CLI_InvalidOutputFlag(t *testing.T) {
	cmd := newAuditCommand()
	cmd.SetArgs([]string{"verify", "--project", "p", "--output", "xml"})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --output")
	}
}
