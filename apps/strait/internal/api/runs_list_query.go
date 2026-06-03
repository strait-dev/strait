package api

import (
	"encoding/json"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/domain"
)

type listRunsQuery struct {
	statuses        map[domain.RunStatus]struct{}
	statusQuery     *domain.RunStatus
	tagKey          string
	tagValue        string
	metadataKey     *string
	metadataValue   *string
	triggeredBy     *string
	batchID         *string
	payloadContains json.RawMessage
	executionMode   *domain.ExecutionMode
	errorClass      *string
	limit           int
	cursor          *time.Time
}

func newListRunsQuery(input *ListRunsInput) (listRunsQuery, error) {
	statuses, statusQuery, err := buildRunStatusFilter(input.Status, input.Statuses)
	if err != nil {
		return listRunsQuery{}, err
	}

	if input.TagValue != "" && input.TagKey == "" {
		return listRunsQuery{}, huma.Error400BadRequest("tag_key is required when tag_value is provided")
	}
	if input.TagKey != "" {
		if err := validateTags(map[string]string{input.TagKey: input.TagValue}); err != nil {
			return listRunsQuery{}, huma.Error400BadRequest(err.Error())
		}
	}

	if input.MetadataValue != "" && input.MetadataKey == "" {
		return listRunsQuery{}, huma.Error400BadRequest("metadata_key is required when metadata_value is provided")
	}
	if input.TagKey != "" && input.MetadataKey != "" {
		return listRunsQuery{}, huma.Error400BadRequest("tag_key and metadata_key filters are mutually exclusive")
	}

	payloadContains, err := parsePayloadContains(input.PayloadContains)
	if err != nil {
		return listRunsQuery{}, err
	}
	executionMode, err := parseExecutionModeFilter(input.ExecutionMode)
	if err != nil {
		return listRunsQuery{}, err
	}
	errorClass, err := parseErrorClassFilter(input.ErrorClass)
	if err != nil {
		return listRunsQuery{}, err
	}
	limit, cursor, err := parsePaginationParamsTyped(input.Limit, input.Cursor)
	if err != nil {
		return listRunsQuery{}, huma.Error400BadRequest(err.Error())
	}

	return listRunsQuery{
		statuses:        statuses,
		statusQuery:     statusQuery,
		tagKey:          input.TagKey,
		tagValue:        input.TagValue,
		metadataKey:     optionalString(input.MetadataKey),
		metadataValue:   optionalString(input.MetadataValue),
		triggeredBy:     optionalString(input.TriggeredBy),
		batchID:         optionalString(input.BatchID),
		payloadContains: payloadContains,
		executionMode:   executionMode,
		errorClass:      errorClass,
		limit:           limit,
		cursor:          cursor,
	}, nil
}

func (q listRunsQuery) usesFilteredStorePath(environmentID string) bool {
	return environmentID != "" || q.tagKey != "" || len(q.statuses) > 1
}

func parsePayloadContains(raw string) (json.RawMessage, error) {
	if raw == "" {
		return nil, nil
	}
	if !json.Valid([]byte(raw)) {
		return nil, huma.Error400BadRequest("payload_contains must be valid JSON")
	}
	return json.RawMessage(raw), nil
}

func parseExecutionModeFilter(raw string) (*domain.ExecutionMode, error) {
	if raw == "" {
		return nil, nil
	}
	parsed := domain.ExecutionMode(raw)
	if !parsed.IsValid() {
		return nil, huma.Error400BadRequest("execution_mode is invalid")
	}
	return &parsed, nil
}

func parseErrorClassFilter(raw string) (*string, error) {
	if raw == "" {
		return nil, nil
	}
	if !domain.ValidErrorClasses[raw] {
		return nil, huma.Error400BadRequest("error_class is invalid")
	}
	return &raw, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
