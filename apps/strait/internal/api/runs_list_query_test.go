package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"
)

func TestNewListRunsQuery_BuildsFilters(t *testing.T) {
	t.Parallel()

	cursor := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	query, err := newListRunsQuery(&ListRunsInput{
		Statuses:        []string{string(domain.StatusFailed), string(domain.StatusTimedOut)},
		TagKey:          "team",
		TagValue:        "infra",
		TriggeredBy:     "api",
		BatchID:         "batch-1",
		PayloadContains: `{"customer":"acme"}`,
		ExecutionMode:   string(domain.ExecutionModeWorker),
		ErrorClass:      "timeout",
		Limit:           "17",
		Cursor:          cursor.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("newListRunsQuery: %v", err)
	}

	if query.statusQuery != nil {
		t.Fatalf("statusQuery = %v, want nil for multi-status query", *query.statusQuery)
	}
	for _, status := range []domain.RunStatus{domain.StatusFailed, domain.StatusTimedOut} {
		if _, ok := query.statuses[status]; !ok {
			t.Fatalf("missing status %q in %#v", status, query.statuses)
		}
	}
	if query.tagKey != "team" || query.tagValue != "infra" {
		t.Fatalf("tag filter = (%q, %q), want team/infra", query.tagKey, query.tagValue)
	}
	assertStringPtr(t, "triggeredBy", query.triggeredBy, "api")
	assertStringPtr(t, "batchID", query.batchID, "batch-1")
	if !json.Valid(query.payloadContains) || string(query.payloadContains) != `{"customer":"acme"}` {
		t.Fatalf("payloadContains = %s", query.payloadContains)
	}
	if query.executionMode == nil || *query.executionMode != domain.ExecutionModeWorker {
		t.Fatalf("executionMode = %v, want worker", query.executionMode)
	}
	assertStringPtr(t, "errorClass", query.errorClass, "timeout")
	if query.limit != 17 {
		t.Fatalf("limit = %d, want 17", query.limit)
	}
	if query.cursor == nil || !query.cursor.Equal(cursor) {
		t.Fatalf("cursor = %v, want %v", query.cursor, cursor)
	}
}

func TestNewListRunsQuery_MetadataFilters(t *testing.T) {
	t.Parallel()

	query, err := newListRunsQuery(&ListRunsInput{
		MetadataKey:   "env",
		MetadataValue: "prod",
	})
	if err != nil {
		t.Fatalf("newListRunsQuery: %v", err)
	}

	assertStringPtr(t, "metadataKey", query.metadataKey, "env")
	assertStringPtr(t, "metadataValue", query.metadataValue, "prod")
}

func TestNewListRunsQuery_SingleStatus(t *testing.T) {
	t.Parallel()

	query, err := newListRunsQuery(&ListRunsInput{Status: string(domain.StatusQueued)})
	if err != nil {
		t.Fatalf("newListRunsQuery: %v", err)
	}

	if query.statusQuery == nil || *query.statusQuery != domain.StatusQueued {
		t.Fatalf("statusQuery = %v, want queued", query.statusQuery)
	}
	if _, ok := query.statuses[domain.StatusQueued]; !ok {
		t.Fatalf("statuses = %#v, want queued", query.statuses)
	}
}

func TestNewListRunsQuery_InvalidFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input ListRunsInput
		want  string
	}{
		{name: "tag value without key", input: ListRunsInput{TagValue: "infra"}, want: "tag_key is required"},
		{name: "metadata value without key", input: ListRunsInput{MetadataValue: "prod"}, want: "metadata_key is required"},
		{name: "tag and metadata", input: ListRunsInput{TagKey: "team", MetadataKey: "env"}, want: "mutually exclusive"},
		{name: "invalid payload", input: ListRunsInput{PayloadContains: "{"}, want: "payload_contains must be valid JSON"},
		{name: "invalid execution mode", input: ListRunsInput{ExecutionMode: "invalid"}, want: "execution_mode is invalid"},
		{name: "invalid error class", input: ListRunsInput{ErrorClass: "invalid"}, want: "error_class is invalid"},
		{name: "invalid cursor", input: ListRunsInput{Cursor: "not-time"}, want: "cursor must be a valid RFC3339 timestamp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := newListRunsQuery(&tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestListRunsQuery_UsesFilteredStorePath(t *testing.T) {
	t.Parallel()

	if (listRunsQuery{}).usesFilteredStorePath("") {
		t.Fatal("empty query without env should use direct store path")
	}
	if !(listRunsQuery{}).usesFilteredStorePath("env-prod") {
		t.Fatal("environment-scoped query must use filtered path")
	}
	if !(listRunsQuery{tagKey: "team"}).usesFilteredStorePath("") {
		t.Fatal("tag query must use filtered path")
	}
	if !(listRunsQuery{statuses: map[domain.RunStatus]struct{}{
		domain.StatusFailed:   {},
		domain.StatusTimedOut: {},
	}}).usesFilteredStorePath("") {
		t.Fatal("multi-status query must use filtered path")
	}
}

func assertStringPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %q", name, got, want)
	}
}
