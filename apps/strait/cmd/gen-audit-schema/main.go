// gen-audit-schema emits a JSON Schema Draft 2020-12 document describing
// every registered audit action in strait's audit log. Used by external
// SIEM integrations and compliance reviewers to validate exported audit
// payloads.
//
// Usage:
//
//	go run ./cmd/gen-audit-schema > internal/api/audit_schema_generated.json
//
// The output is deterministic — every run with the same registry produces
// byte-identical output. A companion test (TestAuditSchemaGeneratedIsFresh)
// verifies the committed file matches the current registry; CI fails if
// someone adds an action without regenerating.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"strait/internal/domain"
)

// schemaDoc is the top-level JSON Schema document.
type schemaDoc struct {
	Schema  string                 `json:"$schema"`
	Title   string                 `json:"title"`
	Type    string                 `json:"type"`
	Defs    map[string]actionEntry `json:"$defs"`
	Version string                 `json:"version"`
}

type actionEntry struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Properties  map[string]schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	NotAllowed  []string          `json:"x-forbidden-keys,omitempty"`
}

type schema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func main() {
	os.Exit(exitCode(os.Stdout, os.Stderr))
}

func exitCode(stdout io.Writer, stderr io.Writer) int {
	if err := run(stdout, stderr); err != nil {
		return 1
	}
	return 0
}

func run(stdout io.Writer, stderr io.Writer) error {
	actions := domain.KnownAuditActions()
	sort.Strings(actions)

	defs := make(map[string]actionEntry, len(actions))
	for _, action := range actions {
		entry, ok := domain.AuditActionSchemas[action]
		if !ok {
			fmt.Fprintf(stderr, "warning: no schema entry for action %q\n", action)
			continue
		}
		props := make(map[string]schema, len(entry.Required))
		for _, key := range entry.Required {
			props[key] = schema{Type: "string", Description: "required"}
		}
		defs[action] = actionEntry{
			Type:        "object",
			Description: entry.Description,
			Properties:  props,
			Required:    entry.Required,
			NotAllowed:  domain.ForbiddenKeysFor(action),
		}
	}

	doc := schemaDoc{
		Schema:  "https://json-schema.org/draft/2020-12/schema",
		Title:   "Strait audit event registry",
		Type:    "object",
		Defs:    defs,
		Version: fmt.Sprintf("%d", domain.AuditEventSchemaVersionCurrent),
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintln(stderr, "encode:", err)
		return err
	}
	return nil
}
