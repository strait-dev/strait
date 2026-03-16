package composition

import "iter"

// PaginatedQuery represents pagination query parameters.
type PaginatedQuery struct {
	Cursor string
	Limit  int
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse[T any] struct {
	Data       []T    `json:"data,omitempty"`
	Items      []T    `json:"items,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    *bool  `json:"has_more,omitempty"`
}

// PaginateOptions configures pagination behavior.
type PaginateOptions struct {
	Limit int
}

// Paginate returns an iterator that automatically paginates through list API responses.
func Paginate[T any](
	listFn func(query PaginatedQuery) (PaginatedResponse[T], error),
	opts *PaginateOptions,
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var cursor string
		limit := 0
		if opts != nil {
			limit = opts.Limit
		}

		for {
			q := PaginatedQuery{Cursor: cursor, Limit: limit}
			resp, err := listFn(q)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}

			items := resp.Data
			if len(items) == 0 {
				items = resp.Items
			}

			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}

			if resp.NextCursor == "" || (resp.HasMore != nil && !*resp.HasMore) || len(items) == 0 {
				return
			}

			cursor = resp.NextCursor
		}
	}
}

// CollectAll collects all items from a paginated iterator into a slice.
func CollectAll[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var result []T
	for item, err := range seq {
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
