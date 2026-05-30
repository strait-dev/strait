package api

import "strconv"

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

// parseLimitParam parses a limit query string for endpoints whose cursor is an
// opaque id rather than a timestamp (so parsePaginationFromStrings does not
// apply). Invalid or absent values fall back to defaultPageLimit; values above
// maxPageLimit are clamped.
func parseLimitParam(limitStr string) int {
	limit := defaultPageLimit
	if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
		limit = parsed
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	return limit
}

type paginationError struct {
	msg string
}

func (e *paginationError) Error() string {
	return e.msg
}
