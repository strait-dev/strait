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
)

// auditActionReferenceAllowlist lists AuditAction* constants that are NOT
// referenced from internal/api/ nor from the two known non-api emitters
// (internal/store/audit_events.go for audit.retention_trimmed and
// internal/store/audit_key_rotation.go for audit.key_rotated). Every entry
// in this map needs a reason — the default stance is "a defined audit
// action const must be emitted by at least one call site".
var auditActionReferenceAllowlist = map[string]string{}

// TestEveryAuditActionConstHasCallSite walks the full set of files that are
// allowed to emit audit events and asserts that every AuditAction* constant
// defined in internal/domain/audit_actions.go is referenced by at least one
// of them. This guards against "phantom" constants — entries that live in
// the taxonomy and schema but are never emitted in practice.
//
// Scanned files (non-test):
//   - apps/strait/internal/api/*.go (all handlers)
//   - apps/strait/internal/store/audit_events.go (retention tombstone)
//   - apps/strait/internal/store/audit_key_rotation.go (key rotation anchor)
//
// A constant that is emitted only from a file outside that set must be added
// to auditActionReferenceAllowlist with a documented reason.
func TestEveryAuditActionConstHasCallSite(t *testing.T) {
	t.Parallel()

	apiDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	// apiDir = .../apps/strait/internal/api — walk up to apps/strait.
	straitRoot := filepath.Clean(filepath.Join(apiDir, "..", ".."))

	defined := collectAuditActionConsts(t, filepath.Join(straitRoot, "internal", "domain", "audit_actions.go"))
	if len(defined) == 0 {
		t.Fatal("no AuditAction* constants found in internal/domain/audit_actions.go — parser broken?")
	}

	scanFiles := []string{
		filepath.Join(straitRoot, "internal", "store", "audit_events.go"),
		filepath.Join(straitRoot, "internal", "store", "audit_key_rotation.go"),
	}
	// Add every non-test .go in internal/api.
	apiEntries, err := os.ReadDir(apiDir)
	if err != nil {
		t.Fatalf("read api dir: %v", err)
	}
	for _, e := range apiEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanFiles = append(scanFiles, filepath.Join(apiDir, name))
	}

	referenced := map[string]struct{}{}
	fset := token.NewFileSet()
	for _, path := range scanFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name != "domain" {
				return true
			}
			name := sel.Sel.Name
			if strings.HasPrefix(name, "AuditAction") {
				referenced[name] = struct{}{}
			}
			return true
		})
	}

	var missing []string
	for c := range defined {
		if _, ok := referenced[c]; ok {
			continue
		}
		if _, allowed := auditActionReferenceAllowlist[c]; allowed {
			continue
		}
		missing = append(missing, c)
	}

	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)

	var b strings.Builder
	b.WriteString("the following AuditAction* constants are defined in internal/domain/audit_actions.go ")
	b.WriteString("but never referenced from internal/api/, internal/store/audit_events.go, ")
	b.WriteString("or internal/store/audit_key_rotation.go:\n\n")
	for _, name := range missing {
		b.WriteString("  - ")
		b.WriteString(name)
		b.WriteString("\n")
	}
	b.WriteString("\neither add an emitAuditEventAsync/CreateAuditEvent call using the const, ")
	b.WriteString("or (if the action is emitted from a file outside the scanned set) add it ")
	b.WriteString("to auditActionReferenceAllowlist with a documented reason.\n")
	t.Fatal(b.String())
}

// collectAuditActionConsts parses audit_actions.go and returns the set of
// const names matching AuditAction*.
func collectAuditActionConsts(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]struct{}{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, nameIdent := range vs.Names {
				if strings.HasPrefix(nameIdent.Name, "AuditAction") {
					out[nameIdent.Name] = struct{}{}
				}
			}
		}
	}
	return out
}
