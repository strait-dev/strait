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

// auditCoverageExemptHandlers lists handlers that must NOT emit audit events.
// These are read-only, SDK-scoped run-self operations, or dispatchers whose
// side effects are already audited by the underlying mutation call.
//
// Adding a new handler to this list is intentionally uncomfortable — every
// entry needs a reason comment, because the default stance is "audit it".
var auditCoverageExemptHandlers = map[string]string{
	// Auth flows — the caller has no actor yet, and the device code is audited
	// when approved (handleApproveDeviceCode).
	"handleCreateDeviceCode":   "pre-auth, no actor; issuance not a control-plane mutation",
	"handleExchangeDeviceCode": "pre-auth, no actor; the approval path is audited",
	"handleLogin":              "pre-auth, auth-lockout path; no actor yet",
	"handleLogout":             "session teardown; session auth already logged by middleware",

	// Stripe billing webhooks are verified inbound events, not user mutations.
	"handleStripeWebhook": "inbound Stripe webhook, not a user-initiated mutation",

	// CDC webhook receiver is internal-only.
	"handleCDCWebhook": "internal CDC push, not user-initiated",

	// Job trigger dry-run validates payload, limits, and scheduling without
	// creating runs, enqueueing work, or mutating control-plane state.
	"handleTriggerDryRun": "read-only validation path; successful job triggers are audited by the enqueue paths",

	// SDK endpoints: the run reports its own progress via run-token JWT.
	// These are protected by the SDK auth flow; audit logs would be dominated
	// by per-heartbeat noise with no security value.
	"handleSDKProgress":          "sdk run-token, self-report only",
	"handleSDKCheckpoint":        "sdk run-token, self-report only",
	"handleSDKComplete":          "sdk run-token, self-report only",
	"handleSDKOutput":            "sdk run-token, self-report only",
	"handleSDKEvent":             "sdk run-token, self-report only",
	"handleSDKWaitEvent":         "sdk run-token, self-report only",
	"handleSDKResources":         "sdk run-token, self-report only",
	"handleSDKHeartbeat":         "sdk run-token, self-report only",
	"handleSDKEmitEvent":         "sdk run-token, self-report only",
	"handleSDKLog":               "sdk run-token, self-report only",
	"handleSDKWorkflowStepDone":  "sdk run-token, self-report only",
	"handleSDKWorkflowStepFail":  "sdk run-token, self-report only",
	"handleSDKWorkflowStepSleep": "sdk run-token, self-report only",

	// Rotation handler — the store's RotateAuditSigningKey atomically emits
	// an is_anchor=TRUE audit.key_rotated event signed under the new epoch
	// key. Emitting an additional event from the handler would either
	// duplicate the anchor or log under the old key.
	"handleRotateAuditSigningKey": "store RotateAuditSigningKey emits the is_anchor audit event atomically",
}

// auditCoverageMutationPrefixes is the list of handler-name prefixes that the
// guard considers state-mutating. Any `handleX...` that matches one of these
// and is not in the exempt list must contain a call to emitAuditEvent or
// emitAuditEventAsync.
var auditCoverageMutationPrefixes = []string{
	"handleCreate",
	"handleUpdate",
	"handleDelete",
	"handleUpsert",
	"handlePause",
	"handleResume",
	"handleCancel",
	"handleApprove",
	"handleSkip",
	"handleForceComplete",
	"handleRetry",
	"handleReplay",
	"handleRollback",
	"handleRestart",
	"handleReschedule",
	"handleResetIdempotencyKey",
	"handleSetDebugMode",
	"handleClone",
	"handleBatch",
	"handleBulk",
	"handleTrigger",
	"handleSend",
	"handleRotate",
	"handleRevoke",
	"handleDispatch",
	"handleSubscribe",
	"handlePromote",
	"handleFinalize",
	"handleConfirm",
	"handleCompensate",
	"handleSeed",
	"handleExport",
	"handlePurge",
	"handleAssign",
	"handleRemove",
	"handleTest",
}

// TestAuditCoverageGuard walks every .go file in internal/api/ and asserts
// that every state-mutating *Server method on `handleX...` calls
// emitAuditEvent or emitAuditEventAsync somewhere in its body. Handlers that
// intentionally skip auditing must be listed in auditCoverageExemptHandlers
// with a documented reason.
//
// This test is the single mechanism that keeps audit coverage at ~100% going
// forward. Adding a new mutation handler without auditing it will fail CI.
func TestAuditCoverageGuard(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	type handlerInfo struct {
		name string
		file string
		line int
	}

	serverMethods := make(map[string]*ast.FuncDecl)
	files := make(map[string]*ast.File)
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
		require.NoError(t, parseErr)

		files[name] = file
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv != nil && fn.Body != nil && isServerReceiver(fn.Recv) {
				serverMethods[fn.Name.Name] = fn
			}
		}
	}

	var missing []handlerInfo

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			// Receiver must be *Server.
			if !isServerReceiver(fn.Recv) {
				continue
			}
			// Name must start with "handle" + mutation prefix.
			if !isMutationHandler(fn.Name.Name) {
				continue
			}
			// Exempt list is a non-audit allowlist.
			if _, exempt := auditCoverageExemptHandlers[fn.Name.Name]; exempt {
				continue
			}

			if !callsAuditEmit(fn.Body, serverMethods, map[string]bool{}) {
				pos := fset.Position(fn.Pos())
				missing = append(missing, handlerInfo{
					name: fn.Name.Name,
					file: filepath.Base(pos.Filename),
					line: pos.Line,
				})
			}
		}
	}

	if len(missing) == 0 {
		return
	}

	sort.Slice(missing, func(i, j int) bool {
		if missing[i].file != missing[j].file {
			return missing[i].file < missing[j].file
		}
		return missing[i].name < missing[j].name
	})

	var b strings.Builder
	b.WriteString("the following state-mutating handlers do not emit an audit event:\n\n")
	for _, m := range missing {
		b.WriteString("  - ")
		b.WriteString(m.file)
		b.WriteString(":")
		b.WriteString(itoa(m.line))
		b.WriteString("  ")
		b.WriteString(m.name)
		b.WriteString("\n")
	}
	b.WriteString("\nadd a call to s.emitAuditEvent (or s.emitAuditEventAsync for the hot path), ")
	b.WriteString("or — if the handler is truly read-only — add it to ")
	b.WriteString("auditCoverageExemptHandlers with a documented reason.\n")
	require.Fail(t,

		b.String())
}

// isServerReceiver reports whether the function's receiver is *api.Server.
func isServerReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "Server"
}

// isMutationHandler reports whether a handler name starts with any of the
// known mutation-intent prefixes.
func isMutationHandler(name string) bool {
	if !strings.HasPrefix(name, "handle") {
		return false
	}
	for _, prefix := range auditCoverageMutationPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// callsAuditEmit walks a function body looking for recognized audit-emission
// patterns, following calls into other *Server helper methods. This lets
// handlers stay small while preserving the guard's invariant that every
// mutating handler reaches an audit event on its success path.
//   - s.emitAuditEvent / s.emitAuditEventAsync (fire-and-forget)
//   - s.buildAuditEvent (paired with txStore.CreateAuditEvent inside
//     a runInTx closure for atomic-with-mutation audit inserts)
//
// Returns true on the first hit.
func callsAuditEmit(body *ast.BlockStmt, serverMethods map[string]*ast.FuncDecl, seen map[string]bool) bool {
	var found bool
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "emitAuditEvent", "emitAuditEventAsync", "buildAuditEvent":
			found = true
			return false
		}
		fn, ok := serverMethods[sel.Sel.Name]
		if !ok || seen[sel.Sel.Name] {
			return true
		}
		seen[sel.Sel.Name] = true
		if callsAuditEmit(fn.Body, serverMethods, seen) {
			found = true
			return false
		}
		return true
	})
	return found
}

// auditNegativePathFloor is the minimum number of "_StoreError" negative-path
// subtests that must exist in audit_negative_path_test.go. Raising this floor
// is the correct response to adding new mutation handlers — every new handler
// should bring at least one "no audit on store failure" subcase, and the
// floor serves as a tripwire when a contributor forgets. Lowering this floor
// requires justification in a commit message.
const auditNegativePathFloor = 10

// TestAuditNegativePathFloor walks audit_negative_path_test.go and asserts
// that at least auditNegativePathFloor test cases with names ending in
// "_StoreError" exist. This is the coarser companion to
// TestAuditCoverageGuard: it does not prove per-handler coverage, but it
// prevents the negative-path table from silently decaying.
func TestAuditNegativePathFloor(t *testing.T) {
	t.Parallel()

	path, err := filepath.Abs("audit_negative_path_test.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	count := 0
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "name" {
			return true
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if strings.HasSuffix(strings.Trim(lit.Value, `"`), "_StoreError") {
			count++
		}
		return true
	})
	require.GreaterOrEqual(t,
		count, auditNegativePathFloor,
	)
}

// itoa is a small int→string helper so the test has no runtime dependencies
// beyond the standard library parser.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
