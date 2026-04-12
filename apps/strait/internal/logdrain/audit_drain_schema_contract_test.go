package logdrain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// TestAuditSIEMDrain_PayloadMatchesSchema constructs one sample AuditEvent
// per registered action, serializes it through the drain's NDJSON wire
// format, and validates the details payload against the committed
// audit_schema_generated.json contract. This guards against drift between
// the Go registry and the schema consumed by SIEM integrations.
func TestAuditSIEMDrain_PayloadMatchesSchema(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// apps/strait/internal/logdrain -> apps/strait/internal/api/audit_schema_generated.json
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	schemaPath := filepath.Join(repoRoot, "internal", "api", "audit_schema_generated.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("audit_schema.json", bytes.NewReader(schemaBytes)); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}

	actions := domain.KnownAuditActions()
	sort.Strings(actions)
	if len(actions) == 0 {
		t.Fatal("no registered audit actions")
	}

	events := make([]domain.AuditEvent, 0, len(actions))
	for _, action := range actions {
		events = append(events, buildSampleEvent(t, action))
	}

	payload, err := encodeNDJSONBatch(events)
	if err != nil {
		t.Fatalf("encode NDJSON: %v", err)
	}

	lines := splitNDJSON(payload)
	if len(lines) != len(events) {
		t.Fatalf("ndjson line count mismatch: got %d want %d", len(lines), len(events))
	}

	for i, line := range lines {
		action := events[i].Action
		var ev map[string]any
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("%s: unmarshal wire line: %v", action, err)
		}
		details, _ := ev["details"].(map[string]any)
		if details == nil {
			details = map[string]any{}
		}
		schema, err := compiler.Compile(fmt.Sprintf("audit_schema.json#/$defs/%s", action))
		if err != nil {
			t.Errorf("%s: compile schema: %v", action, err)
			continue
		}
		if err := schema.Validate(details); err != nil {
			t.Errorf("%s: payload fails schema validation: %v\npayload=%s", action, err, string(line))
		}
	}
}

// buildSampleEvent produces a minimal but schema-valid AuditEvent for a
// given action by filling every required details key with a dummy string
// value. The wire form is what ForwardBatch would emit.
func buildSampleEvent(t *testing.T, action string) domain.AuditEvent {
	t.Helper()
	schema, ok := domain.AuditActionSchemas[action]
	if !ok {
		t.Fatalf("no schema registered for action %q", action)
	}
	details := make(map[string]string, len(schema.Required))
	for _, key := range schema.Required {
		details[key] = "sample"
	}
	raw, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("marshal details for %s: %v", action, err)
	}
	return domain.AuditEvent{
		ID:            "ev-" + action,
		ProjectID:     "p1",
		ActorID:       "a1",
		ActorType:     "user",
		Action:        action,
		ResourceType:  "test",
		ResourceID:    "r1",
		Details:       raw,
		CreatedAt:     time.Unix(0, 0).UTC(),
		SchemaVersion: domain.AuditEventSchemaVersionCurrent,
	}
}

// splitNDJSON separates a NDJSON payload into non-empty lines.
func splitNDJSON(payload []byte) [][]byte {
	out := make([][]byte, 0)
	for _, raw := range bytes.Split(payload, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
