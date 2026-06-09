package api

import (
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
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
	require.NoError(t,
		err)
	require.Nil(t, query.statusQuery)

	for _, status := range []domain.RunStatus{domain.StatusFailed, domain.StatusTimedOut} {
		if _, ok := query.statuses[status]; !ok {
			require.Failf(t, "test failure",

				"missing status %q in %#v", status, query.statuses)
		}
	}
	require.Equal(t, "team", query.tagKey)
	require.Equal(t, "infra", query.tagValue)

	assertStringPtr(t, "triggeredBy", query.triggeredBy, "api")
	assertStringPtr(t, "batchID", query.batchID, "batch-1")
	require.True(t, json.Valid(query.payloadContains))
	require.JSONEq(t, `{"customer":"acme"}`, string(query.payloadContains))
	require.NotNil(t, query.executionMode)
	require.Equal(t, domain.ExecutionModeWorker, *query.executionMode)

	assertStringPtr(t, "errorClass", query.errorClass, "timeout")
	require.Equal(t, 17,
		query.limit,
	)
	require.NotNil(t, query.cursor)
	require.True(t, query.cursor.Equal(cursor))
}

func TestNewListRunsQuery_MetadataFilters(t *testing.T) {
	t.Parallel()

	query, err := newListRunsQuery(&ListRunsInput{
		MetadataKey:   "env",
		MetadataValue: "prod",
	})
	require.NoError(t,
		err)

	assertStringPtr(t, "metadataKey", query.metadataKey, "env")
	assertStringPtr(t, "metadataValue", query.metadataValue, "prod")
}

func TestNewListRunsQuery_SingleStatus(t *testing.T) {
	t.Parallel()

	query, err := newListRunsQuery(&ListRunsInput{Status: string(domain.StatusQueued)})
	require.NoError(t,
		err)
	require.NotNil(t, query.statusQuery)
	require.Equal(t, domain.StatusQueued, *query.statusQuery)

	if _, ok := query.statuses[domain.StatusQueued]; !ok {
		require.Failf(t, "test failure",

			"statuses = %#v, want queued", query.statuses)
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
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.
				want)
		})
	}
}

func TestListRunsQuery_UsesFilteredStorePath(t *testing.T) {
	t.Parallel()
	require.False(t, (listRunsQuery{}).usesFilteredStorePath(""))
	require.True(t, (listRunsQuery{}).usesFilteredStorePath("env-prod"))
	require.True(t, (listRunsQuery{tagKey: "team"}).usesFilteredStorePath(""))
	require.True(t, (listRunsQuery{statuses: map[domain.
		RunStatus]struct{}{domain.
		StatusFailed: {}, domain.StatusTimedOut: {}}}).usesFilteredStorePath(""),
	)
}

func assertStringPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	require.NotNil(t, got, "%s", name)
	require.Equal(t, want, *got, "%s", name)
}
