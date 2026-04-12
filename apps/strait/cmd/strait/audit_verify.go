package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

// auditVerifyDeps carries injectable collaborators for the `strait audit
// verify` subcommand so the command can be unit-tested without a live
// database or signing key.
type auditVerifyDeps struct {
	// verify runs the chain verification. Production wiring calls
	// store.Queries.VerifyAuditChain.
	verify func(ctx context.Context, projectID string) (*domain.AuditChainVerification, error)
	// emit records the self-audit event. Production wiring inserts via
	// store.Queries.CreateAuditEvent; tests capture the call.
	emit func(ctx context.Context, ev *domain.AuditEvent) error
	// now is a clock for deterministic duration measurement in tests.
	now func() time.Time
	// stdout / stderr are injected for output capture in tests.
	stdout io.Writer
	stderr io.Writer
	// actorID is the CLI operator identifier written on the self-audit
	// event's actor_id column.
	actorID string
	// recordVerify increments the chain-verify total counter; called
	// exactly once per verification attempt regardless of outcome.
	// Optional — nil means metrics are disabled for this invocation
	// (e.g. unit tests that don't care).
	recordVerify func(ctx context.Context)
	// recordVerifyFailed increments the chain-verify failed counter,
	// labeled by reason. Called only on a non-passing outcome. Optional.
	recordVerifyFailed func(ctx context.Context, reason string)
}

// auditVerifyOutput is the machine-readable shape produced when
// --output=json. Mirrors the plan spec.
type auditVerifyOutput struct {
	ProjectID     string                  `json:"project_id"`
	Status        string                  `json:"status"`
	EventsChecked int                     `json:"events_checked"`
	FirstBreak    *auditVerifyBreakDetail `json:"first_break"`
	DurationMS    int64                   `json:"duration_ms"`
}

type auditVerifyBreakDetail struct {
	EventID string `json:"event_id"`
	Reason  string `json:"reason"`
}

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit subsystem operations",
	}
	cmd.AddCommand(newAuditVerifyCommand())
	return cmd
}

func newAuditVerifyCommand() *cobra.Command {
	var (
		projectID string
		sinceStr  string
		output    string
	)

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the HMAC audit chain for a project (out-of-band)",
		Long: "Replays the signed audit event chain and verifies HMAC linkage.\n" +
			"Exits 0 on a passing chain, 1 on any break or error. Emits a\n" +
			"self-audit event (audit.chain_verified) recording the outcome.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if projectID == "" {
				return fmt.Errorf("--project is required")
			}
			if output != "text" && output != "json" {
				return fmt.Errorf("--output must be text or json")
			}
			if sinceStr != "" {
				if _, err := time.Parse(time.RFC3339, sinceStr); err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
			}

			deps, cleanup, err := buildAuditVerifyDeps(cmd.Context())
			if err != nil {
				return err
			}
			defer cleanup()

			return runAuditVerify(cmd.Context(), deps, projectID, output)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID to verify (required)")
	cmd.Flags().StringVar(&sinceStr, "since", "", "RFC3339 lower bound (reserved; currently informational)")
	cmd.Flags().StringVar(&output, "output", "text", "output format: text or json")

	return cmd
}

// buildAuditVerifyDeps wires production dependencies: opens the pgx pool,
// derives the audit signing key from INTERNAL_SECRET, and returns a deps
// bundle plus a cleanup closure.
func buildAuditVerifyDeps(ctx context.Context) (auditVerifyDeps, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return auditVerifyDeps{}, func() {}, fmt.Errorf("load config: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return auditVerifyDeps{}, func() {}, fmt.Errorf("connect database: %w", err)
	}

	q := store.New(pool)
	if cfg.InternalSecret != "" {
		key, keyErr := store.DeriveAuditSigningKey(cfg.InternalSecret)
		if keyErr != nil {
			pool.Close()
			return auditVerifyDeps{}, func() {}, fmt.Errorf("derive audit signing key: %w", keyErr)
		}
		q.SetAuditSigningKey(key)
	}

	actor := os.Getenv("USER")
	if actor == "" {
		actor = "cli"
	}

	deps := auditVerifyDeps{
		verify:  q.VerifyAuditChain,
		emit:    q.CreateAuditEvent,
		now:     time.Now,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		actorID: actor,
	}
	return deps, pool.Close, nil
}

// runAuditVerify executes the verification using injected deps. Returns
// a non-nil error when the caller should exit non-zero (chain break,
// verifier error, or I/O failure). Cobra translates the error into a
// non-zero process exit via the existing run() wrapper.
func runAuditVerify(ctx context.Context, deps auditVerifyDeps, projectID, output string) error {
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.stdout == nil {
		deps.stdout = os.Stdout
	}
	if deps.stderr == nil {
		deps.stderr = os.Stderr
	}

	start := deps.now()
	result, verr := deps.verify(ctx, projectID)
	duration := deps.now().Sub(start)

	if deps.recordVerify != nil {
		deps.recordVerify(ctx)
	}

	passed := verr == nil && result != nil && result.Valid
	status := "PASS"
	if !passed {
		status = "FAIL"
	}

	if !passed && deps.recordVerifyFailed != nil {
		reason := "chain_broken"
		if verr != nil {
			reason = "verifier_error"
		}
		deps.recordVerifyFailed(ctx, reason)
	}

	// Emit self-audit first so the outcome is recorded regardless of
	// downstream printing. Errors here are logged but do not change
	// the command exit code (the verification result is authoritative).
	if deps.emit != nil {
		eventsChecked := 0
		if result != nil {
			eventsChecked = result.EventsChecked
		}
		details := map[string]any{
			"events_checked": eventsChecked,
			"passed":         passed,
			// "valid" is retained alongside "passed" for parity with
			// the existing API-path emit in internal/api/rbac.go so
			// consumers (SIEM, dashboards) see one consistent key.
			"valid": passed,
		}
		if verr != nil {
			details["error"] = verr.Error()
		}
		detailsJSON, _ := json.Marshal(details)
		ev := &domain.AuditEvent{
			ProjectID:    projectID,
			ActorID:      deps.actorID,
			ActorType:    "cli",
			Action:       domain.AuditActionAuditChainVerified,
			ResourceType: "audit",
			ResourceID:   projectID,
			Details:      detailsJSON,
		}
		if emitErr := deps.emit(ctx, ev); emitErr != nil {
			fmt.Fprintf(deps.stderr, "warning: self-audit emit failed: %v\n", emitErr)
		}
	}

	// Print summary.
	if output == "json" {
		out := auditVerifyOutput{
			ProjectID:  projectID,
			Status:     status,
			DurationMS: duration.Milliseconds(),
		}
		if result != nil {
			out.EventsChecked = result.EventsChecked
		}
		if !passed {
			reason := "chain_broken"
			eventID := ""
			switch {
			case verr != nil:
				reason = verr.Error()
			case result != nil && result.Error != "":
				reason = result.Error
				eventID = result.BrokenAtID
			case result != nil && result.BrokenAtID != "":
				eventID = result.BrokenAtID
			}
			out.FirstBreak = &auditVerifyBreakDetail{EventID: eventID, Reason: reason}
		}
		if err := json.NewEncoder(deps.stdout).Encode(out); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
	} else {
		fmt.Fprintf(deps.stdout, "project: %s\n", projectID)
		fmt.Fprintf(deps.stdout, "status:  %s\n", status)
		if result != nil {
			fmt.Fprintf(deps.stdout, "events:  %d\n", result.EventsChecked)
		}
		fmt.Fprintf(deps.stdout, "took:    %dms\n", duration.Milliseconds())
		if !passed {
			switch {
			case verr != nil:
				fmt.Fprintf(deps.stdout, "error:   %v\n", verr)
			case result != nil && result.Error != "":
				fmt.Fprintf(deps.stdout, "error:   %s\n", result.Error)
				if result.BrokenAtID != "" {
					fmt.Fprintf(deps.stdout, "broken_at: %s\n", result.BrokenAtID)
				}
			}
		}
	}

	if !passed {
		if verr != nil {
			return fmt.Errorf("audit chain verification failed: %w", verr)
		}
		return fmt.Errorf("audit chain verification failed for project %s", projectID)
	}
	return nil
}
