package api

import (
	"net/http"
	"strconv"
	"time"
)

// PaginatedResponse wraps a list response with cursor-based pagination metadata.
type PaginatedResponse struct {
	Data       any     `json:"data"`
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

// parsePaginationParams extracts limit and cursor from query parameters.
// Returns validated limit (clamped to [1, maxPageLimit]) and optional cursor.
func parsePaginationParams(r *http.Request) (limit int, cursor *time.Time, err error) {
	query := r.URL.Query()

	limit = defaultPageLimit
	if limitRaw := query.Get("limit"); limitRaw != "" {
		parsed, parseErr := strconv.Atoi(limitRaw)
		if parseErr != nil || parsed <= 0 {
			return 0, nil, &paginationError{msg: "limit must be a positive integer"}
		}
		if parsed > maxPageLimit {
			parsed = maxPageLimit
		}
		limit = parsed
	}

	if cursorRaw := query.Get("cursor"); cursorRaw != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, cursorRaw)
		if parseErr != nil {
			return 0, nil, &paginationError{msg: "cursor must be a valid RFC3339 timestamp"}
		}
		cursor = &parsed
	}

	return limit, cursor, nil
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
