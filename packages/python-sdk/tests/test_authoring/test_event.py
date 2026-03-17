"""Tests for defineEvent and EventDefinition."""

from __future__ import annotations

import pytest

from strait.authoring._event import EventDefinition, define_event


class TestDefineEvent:
    def test_creates_event_definition(self):
        event = define_event("order.paid")
        assert isinstance(event, EventDefinition)
        assert event.key == "order.paid"

    def test_no_validator_by_default(self):
        event = define_event("order.paid")
        assert event.validate is None

    def test_with_validator(self):
        def validate(data: dict) -> dict:
            if "amount" not in data:
                raise ValueError("missing amount")
            return data

        event = define_event("order.paid", validate=validate)
        assert event.validate is validate

    def test_parse_passthrough_without_validator(self):
        event = define_event("user.created")
        data = {"name": "Alice", "email": "alice@test.com"}
        result = event.parse(data)
        assert result == data

    def test_parse_returns_same_object_without_validator(self):
        event = define_event("test.event")
        data = {"key": "value"}
        result = event.parse(data)
        assert result is data

    def test_parse_with_validator_transforms(self):
        def validate(data: dict) -> dict:
            return {**data, "validated": True}

        event = define_event("order.paid", validate=validate)
        result = event.parse({"amount": 100})
        assert result["validated"] is True
        assert result["amount"] == 100

    def test_parse_with_validator_raises(self):
        def validate(data: dict) -> dict:
            if "required_field" not in data:
                raise ValueError("missing required_field")
            return data

        event = define_event("order.paid", validate=validate)
        with pytest.raises(ValueError, match="missing required_field"):
            event.parse({"other": "data"})

    def test_parse_string_input(self):
        event = define_event("simple.event")
        result = event.parse("hello")
        assert result == "hello"

    def test_parse_none_input(self):
        event = define_event("nullable.event")
        result = event.parse(None)
        assert result is None

    def test_parse_list_input(self):
        event = define_event("batch.event")
        data = [1, 2, 3]
        result = event.parse(data)
        assert result == [1, 2, 3]

    def test_validator_receives_correct_input(self):
        received: list = []

        def validate(data: dict) -> dict:
            received.append(data)
            return data

        event = define_event("test.event", validate=validate)
        original = {"x": 42}
        event.parse(original)
        assert len(received) == 1
        assert received[0] is original

    def test_different_keys(self):
        e1 = define_event("a.b.c")
        e2 = define_event("x.y.z")
        assert e1.key != e2.key
        assert e1.key == "a.b.c"
        assert e2.key == "x.y.z"
