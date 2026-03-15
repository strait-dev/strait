package composition

import (
	"errors"
	"testing"
)

func TestPaginate_SinglePage(t *testing.T) {
	items, err := CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[string], error) {
		return PaginatedResponse[string]{Data: []string{"a", "b", "c"}}, nil
	}, nil))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestPaginate_MultiplePages(t *testing.T) {
	call := 0
	items, err := CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[int], error) {
		call++
		switch call {
		case 1:
			tr := true
			return PaginatedResponse[int]{Data: []int{1, 2}, NextCursor: "cur_2", HasMore: &tr}, nil
		case 2:
			return PaginatedResponse[int]{Data: []int{3, 4}}, nil
		}
		return PaginatedResponse[int]{}, nil
	}, nil))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("expected 4 items, got %d", len(items))
	}
}

func TestPaginate_ItemsField(t *testing.T) {
	items, err := CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[string], error) {
		return PaginatedResponse[string]{Items: []string{"x", "y"}}, nil
	}, nil))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestPaginate_Error(t *testing.T) {
	_, err := CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[string], error) {
		return PaginatedResponse[string]{}, errors.New("api error")
	}, nil))

	if err == nil || err.Error() != "api error" {
		t.Errorf("expected 'api error', got %v", err)
	}
}

func TestPaginate_EmptyResponse(t *testing.T) {
	items, err := CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[string], error) {
		return PaginatedResponse[string]{}, nil
	}, nil))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestPaginate_HasMoreFalseStops(t *testing.T) {
	f := false
	items, err := CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[int], error) {
		return PaginatedResponse[int]{Data: []int{1}, NextCursor: "cur_2", HasMore: &f}, nil
	}, nil))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestPaginate_WithLimit(t *testing.T) {
	var capturedLimit int
	_, _ = CollectAll(Paginate(func(q PaginatedQuery) (PaginatedResponse[string], error) {
		capturedLimit = q.Limit
		return PaginatedResponse[string]{Data: []string{"a"}}, nil
	}, &PaginateOptions{Limit: 5}))

	if capturedLimit != 5 {
		t.Errorf("expected limit 5, got %d", capturedLimit)
	}
}
