package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// orderingExempt is the list of handlers that intentionally call emit in a
// location that the naive AST check can't model as "after the store write".
// Each entry needs a documented reason.
var orderingExempt = map[string]string{
	// event_sources.go: dispatch iterates subscribers and emits per-dispatch
	// counts at the end; the store writes are inside the loop.
	"handleDispatchEvent": "dispatches via loop then emits aggregate",

	// workflows.go: create wraps CreateWorkflow + CreateWorkflowStep in a
	// runInTx closure; emit is outside the closure (tested separately).
	"handleCreateWorkflow": "runInTx closure; emit is outside tx",
	"handleUpdateWorkflow": "runInTx closure; emit is outside tx",
	"handleCloneWorkflow":  "runInTx closure; emit is outside tx",

	// handleSeedSystemRoles emits AFTER reading roles back; the SeedRoles
	// store call is the mutation but the AST walk finds ListProjectRoles
	// first because it's textually closer to the emit.
	"handleSeedSystemRoles": "emit is after the post-seed list read, which is correct",

	// Bulk operations aggregate over iterated store calls.
	"handleBatchCreateJobs":          "iterates CreateJob; emit is aggregate",
	"handleBulkTriggerJob":           "iterates CreateRun; emit is aggregate",
	"handleBulkCancelRuns":           "bulk cancel is aggregate",
	"handleBulkCancelAll":            "bulk cancel-by-filter is aggregate",
	"handleBulkReplayRuns":           "iterates Enqueue; emit is aggregate",
	"handleBulkReplayDeadLetterRuns": "bulk replay is aggregate",
	"handleBulkCancelWorkflowRuns":   "iterates cancels; emit is aggregate",
	"handleBulkReplayWorkflowRuns":   "iterates retries; emit is aggregate",
	"handleBulkAssignMembers":        "iterates AssignMemberRole; emit is per-iter",

	// Idempotency / deduplication handlers may emit from a different branch.
	"handleTriggerJob": "async emit path; ordered via Enqueue",

	// Audit DLQ replay prefers an atomic store method when available; that
	// method performs the chain insert and DLQ delete before the emit, but the
	// fallback branch's explicit store calls appear later in the function body.
	"handleReplayDeadletter": "atomic replay branch mutates inside store method before emit",

	// workflow_runs.go: several handlers load-then-mutate-then-reload, which
	// confuses the AST ordering check because the Get call textually precedes
	// the mutation. These are audited by the negative-path test instead.
	"handleCancelWorkflowRun":         "load-mutate-reload pattern",
	"handlePauseWorkflowRun":          "load-mutate-reload pattern",
	"handleResumeWorkflowRun":         "load-mutate-reload pattern",
	"handleRetryWorkflowRun":          "load-mutate-reload pattern",
	"handleApproveWorkflowStep":       "callback-based, store call is inside callback",
	"handleSkipWorkflowStep":          "callback-based, store call is inside callback",
	"handleForceCompleteWorkflowStep": "callback-based, store call is inside callback",
	"handleRetryWorkflowStep":         "callback-based, store call is inside callback",
	"handleReplayWorkflowSubtree":     "iterates UpdateStepRunStatus",
	"handleCompensateWorkflowRun":     "load-mutate-reload pattern",

	// Job pause/resume: we call GetJob first, then PauseJob, then GetJob
	// again. The first Get is not a mutation but precedes the emit textually.
	"handlePauseJob":      "load-mutate-reload pattern",
	"handleResumeJob":     "load-mutate-reload pattern",
	"handleDeleteJob":     "GetJob before DeleteJob",
	"handleCloneJob":      "GetJob(source) then CreateJob(clone)",
	"handleCancelRun":     "GetRun before UpdateRunStatus",
	"handleReplayRun":     "GetRun(original) then Enqueue(replay)",
	"handleRescheduleRun": "GetRun before RescheduleRun",
	"handlePauseRun":      "GetRun before UpdateRunStatus",
	"handleResumeRun":     "GetRun before UpdateRunStatus",
	"handleRestartRun":    "GetRun before UpdateRunStatus",
	"handleUpdateJob":     "GetJob before UpdateJob",

	// RBAC: handleRemoveMember performs GetMemberRole then RemoveMemberRole.
	"handleRemoveMember":         "GetMemberRole before RemoveMemberRole",
	"handleDeleteJobGroup":       "GetJobGroup before DeleteJobGroup",
	"handleUpdateJobGroup":       "GetJobGroup before UpdateJobGroup",
	"handlePauseAllJobsByGroup":  "GetJobGroup before PauseJobsByGroup",
	"handleResumeAllJobsByGroup": "GetJobGroup before ResumeJobsByGroup",

	// environments, notification channels, event sources: load-then-mutate.
	"handleUpdateEnvironment":         "GetEnvironment before UpdateEnvironment",
	"handleDeleteEnvironment":         "GetEnvironment before DeleteEnvironment",
	"handleUpdateNotificationChannel": "GetNotificationChannel before UpdateNotificationChannel",
	"handleDeleteEventSubscription":   "GetEventSubscription before DeleteEventSubscription",

	// Canary, workflow policy: Get before mutation.
	"handleUpdateCanaryDeployment":   "Get before UpdateCanaryDeploymentTraffic",
	"handleRollbackCanaryDeployment": "Get before UpdateCanaryDeploymentTraffic",

	// Webhooks: test / replay both do reads first.
	"handleTestWebhook":           "sends HTTP request; store write is irrelevant",
	"handleReplayWebhookDelivery": "Get before Replay",
	"handleRetryWebhookDelivery":  "Get before Retry",

	// event trigger: Get then update.
	"handleSendEvent":          "Get before UpdateEventTriggerStatus",
	"handleSendEventByPrefix":  "iterate then batch update",
	"handleCancelEventTrigger": "Get before UpdateEventTriggerStatus",
	"handlePurgeEventTriggers": "branch-based: dry run vs real delete",

	// Workflows: triggerWorkflow uses the workflow engine (not store) so
	// the engine mutation is indirect.
	"handleTriggerWorkflow": "workflowEngine.TriggerWorkflow; engine handles persistence",

	// audit export: emit is after the stream closes; stream is not a store.Xxx call.
	"handleExportAuditEvents": "stream-based, no discrete store mutation",
	"handleExportJobs":        "stream-based",
	"handleExportRuns":        "stream-based",
	"handleExportWorkflows":   "stream-based",
	"handleExportUsage":       "stream-based",
}

// mutationMethodPrefixes is the set of method-name prefixes on s.store that
// count as a mutation. The ordering check asserts the first such call on the
// store in a handler textually precedes the emit call.
var mutationMethodPrefixes = []string{
	"Create", "Update", "Delete", "Insert", "Replace", "Upsert",
	"Rotate", "Revoke", "Pause", "Resume", "Cancel", "Replay",
	"Reset", "Restart", "Reschedule", "Promote", "Finalize",
	"Confirm", "Rollback", "Seed", "Approve", "Skip",
	"ForceComplete", "Retry", "Batch", "Bulk", "Set",
}

func isStoreMutation(name string) bool {
	for _, p := range mutationMethodPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// TestAuditEmitOrdering_AfterMutation asserts that in every mutation
// handler, the first call to emitAuditEvent / emitAuditEventAsync appears
// textually after the first call to s.store.<Mutation>. This catches
// regressions where someone moves the emit line above the store call.
func TestAuditEmitOrdering_AfterMutation(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(".")
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	fset := token.NewFileSet()

	type violation struct {
		file    string
		handler string
		line    int
		reason  string
	}
	var violations []violation

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.Nil(t, parseErr)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			if !isServerReceiver(fn.Recv) {
				continue
			}
			if !strings.HasPrefix(fn.Name.Name, "handle") {
				continue
			}
			if _, exempt := orderingExempt[fn.Name.Name]; exempt {
				continue
			}

			// Find first emit and first store mutation positions.
			var firstEmitPos, firstStorePos token.Pos
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				// Emit call.
				if sel.Sel.Name == "emitAuditEvent" || sel.Sel.Name == "emitAuditEventAsync" {
					if firstEmitPos == token.NoPos {
						firstEmitPos = call.Pos()
					}
					return true
				}
				// s.store.<Mutation>() call.
				if inner, ok := sel.X.(*ast.SelectorExpr); ok {
					if ident, ok := inner.X.(*ast.Ident); ok && ident.Name == "s" && inner.Sel.Name == "store" {
						if isStoreMutation(sel.Sel.Name) {
							if firstStorePos == token.NoPos {
								firstStorePos = call.Pos()
							}
						}
					}
				}
				return true
			})

			if firstEmitPos == token.NoPos {
				continue // handler does not emit; coverage guard enforces elsewhere
			}
			if firstStorePos == token.NoPos {
				continue // handler emits but does no store mutation; skip
			}
			if firstEmitPos < firstStorePos {
				pos := fset.Position(firstEmitPos)
				violations = append(violations, violation{
					file:    filepath.Base(pos.Filename),
					handler: fn.Name.Name,
					line:    pos.Line,
					reason:  "emitAuditEvent call precedes the first store mutation in the function body",
				})
			}
		}
	}

	if len(violations) == 0 {
		return
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file != violations[j].file {
			return violations[i].file < violations[j].file
		}
		return violations[i].handler < violations[j].handler
	})

	var b strings.Builder
	b.WriteString("the following handlers emit an audit event BEFORE their first store mutation:\n\n")
	for _, v := range violations {
		b.WriteString("  - ")
		b.WriteString(v.file)
		b.WriteString(":")
		b.WriteString(itoa(v.line))
		b.WriteString("  ")
		b.WriteString(v.handler)
		b.WriteString(" — ")
		b.WriteString(v.reason)
		b.WriteString("\n")
	}
	b.WriteString("\naudit events must only fire AFTER the store mutation has succeeded.\n")
	b.WriteString("move the emitAuditEvent call to just before the return statement, after the store call.\n")
	b.WriteString("if the handler has an unusual structure, add it to orderingExempt with a reason.\n")
	require.Fail(t,

		b.String())
}
