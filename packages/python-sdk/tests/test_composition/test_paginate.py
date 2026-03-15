"""Tests for composition paginate."""

from strait.composition._paginate import (
    PaginatedResponse,
    collect_all,
    paginate,
)


class TestPaginate:
    def test_single_page(self):
        def list_fn(q):
            return PaginatedResponse(data=[1, 2, 3])

        items = collect_all(paginate(list_fn))
        assert items == [1, 2, 3]

    def test_multiple_pages(self):
        pages = [
            PaginatedResponse(data=[1, 2], next_cursor="c1", has_more=True),
            PaginatedResponse(data=[3, 4], next_cursor="c2", has_more=True),
            PaginatedResponse(data=[5], next_cursor="", has_more=False),
        ]
        call_count = [0]

        def list_fn(q):
            idx = call_count[0]
            call_count[0] += 1
            if idx > 0:
                assert q.cursor != ""
            return pages[idx]

        items = collect_all(paginate(list_fn))
        assert items == [1, 2, 3, 4, 5]

    def test_empty_page_stops(self):
        def list_fn(q):
            return PaginatedResponse(data=[])

        items = collect_all(paginate(list_fn))
        assert items == []

    def test_items_field_used_when_data_empty(self):
        def list_fn(q):
            return PaginatedResponse(items=["a", "b"])

        items = collect_all(paginate(list_fn))
        assert items == ["a", "b"]

    def test_limit_passed_through(self):
        captured_limits: list[int] = []

        def list_fn(q):
            captured_limits.append(q.limit)
            return PaginatedResponse(data=[1])

        collect_all(paginate(list_fn, limit=25))
        assert captured_limits == [25]

    def test_iterator_can_be_broken_early(self):
        call_count = [0]

        def list_fn(q):
            call_count[0] += 1
            return PaginatedResponse(data=[call_count[0]], next_cursor="next", has_more=True)

        items = []
        for item in paginate(list_fn):
            items.append(item)
            if len(items) >= 2:
                break
        assert items == [1, 2]
