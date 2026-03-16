"""Event definition DSL."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable


@dataclass
class EventDefinition:
    key: str
    validate: Callable[[Any], Any] | None = None

    def parse(self, input: Any) -> Any:
        if self.validate is not None:
            return self.validate(input)
        return input


def define_event(
    key: str,
    validate: Callable[[Any], Any] | None = None,
) -> EventDefinition:
    return EventDefinition(key=key, validate=validate)
