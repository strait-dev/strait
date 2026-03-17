"""Pagination helpers."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Callable, Generic, Iterator, TypeVar

T = TypeVar("T")


@dataclass
class PaginatedQuery:
    cursor: str = ""
    limit: int = 0


@dataclass
class PaginatedResponse(Generic[T]):
    data: list[T] = field(default_factory=list)
    items: list[T] = field(default_factory=list)
    next_cursor: str = ""
    has_more: bool | None = None


def paginate(
    list_fn: Callable[[PaginatedQuery], PaginatedResponse[T]],
    *,
    limit: int = 0,
) -> Iterator[T]:
    cursor = ""
    while True:
        q = PaginatedQuery(cursor=cursor, limit=limit)
        resp = list_fn(q)

        items = resp.data if resp.data else resp.items
        yield from items

        if not resp.next_cursor or (resp.has_more is not None and not resp.has_more) or not items:
            return

        cursor = resp.next_cursor


def collect_all(iterator: Iterator[T]) -> list[T]:
    return list(iterator)
