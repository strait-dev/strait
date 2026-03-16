"""Result type for representing success or failure."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable, Generic, TypeVar

T = TypeVar("T")


@dataclass
class Result(Generic[T]):
    _ok: bool
    _value: T | None
    _error: Exception | None

    @staticmethod
    def ok(value: T) -> Result[T]:
        return Result(_ok=True, _value=value, _error=None)

    @staticmethod
    def err(error: Exception) -> Result[T]:
        return Result(_ok=False, _value=None, _error=error)

    @property
    def is_ok(self) -> bool:
        return self._ok

    @property
    def is_err(self) -> bool:
        return not self._ok

    def unwrap(self) -> T:
        if not self._ok:
            raise self._error  # type: ignore[misc]
        return self._value  # type: ignore[return-value]

    def unwrap_err(self) -> tuple[T | None, Exception | None]:
        return self._value, self._error

    def match(self, on_ok: Callable[[T], None], on_err: Callable[[Exception], None]) -> None:
        if self._ok:
            on_ok(self._value)  # type: ignore[arg-type]
        else:
            on_err(self._error)  # type: ignore[arg-type]

    @staticmethod
    def from_func(fn: Callable[[], T]) -> Result[T]:
        try:
            return Result.ok(fn())
        except Exception as e:
            return Result.err(e)
