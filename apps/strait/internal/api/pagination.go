package api

import (
	"strconv"
	"time"
)

// PaginatedResponse wraps a list response with cursor-based pagination metadata.
type PaginatedResponse struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

// paginatedResult builds a PaginatedResponse from a slice that was fetched with limit+1.
// If len(items) > limit, the extra item is trimmed and next_cursor is set.
func paginatedResult[T any](items []T, limit int, cursorFn func(T) string) PaginatedResponse {
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		c := cursorFn(last)
		return PaginatedResponse{
			Data:       items,
			NextCursor: &c,
			HasMore:    true,
		}
	}
	return PaginatedResponse{
		Data:    items,
		HasMore: false,
	}
}

type paginationError struct {
	msg string
}

func (e *paginationError) Error() string {
	return e.msg
}

// parsePaginationFromStrings parses limit and cursor from string query params.
func parsePaginationFromStrings(limitStr, cursorStr string) (int, *time.Time, error) {
	return parsePaginationParams(limitStr, cursorStr, "invalid cursor format")
}

// parsePaginationParamsTyped is a typed-handler variant that preserves typed
// endpoint validation messages for cursor parse failures.
func parsePaginationParamsTyped(limitStr, cursorStr string) (int, *time.Time, error) {
	return parsePaginationParams(limitStr, cursorStr, "cursor must be a valid RFC3339 timestamp")
}

func parsePaginationParams(limitStr, cursorStr, cursorErrMsg string) (int, *time.Time, error) {
	limit := defaultPageLimit
	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			return 0, nil, &paginationError{msg: "limit must be a positive integer"}
		}
		if parsed > maxPageLimit {
			parsed = maxPageLimit
		}
		limit = parsed
	}

	var cursor *time.Time
	if cursorStr != "" {
		parsed, err := time.Parse(time.RFC3339Nano, cursorStr)
		if err != nil {
			return 0, nil, &paginationError{msg: cursorErrMsg}
		}
		cursor = &parsed
	}

	return limit, cursor, nil
}
