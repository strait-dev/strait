"""Tests for composition Result type."""

import pytest

from strait.composition._result import Result


class TestResult:
    def test_ok_value(self):
        r = Result.ok(42)
        assert r.is_ok
        assert not r.is_err
        assert r.unwrap() == 42

    def test_err_value(self):
        r = Result.err(ValueError("bad"))
        assert r.is_err
        assert not r.is_ok

    def test_unwrap_err_raises(self):
        r = Result.err(ValueError("bad"))
        with pytest.raises(ValueError, match="bad"):
            r.unwrap()

    def test_unwrap_err_tuple(self):
        r = Result.ok(42)
        val, err = r.unwrap_err()
        assert val == 42
        assert err is None

    def test_unwrap_err_tuple_failure(self):
        exc = ValueError("bad")
        r = Result.err(exc)
        val, err = r.unwrap_err()
        assert val is None
        assert err is exc

    def test_match_ok(self):
        results: list[int] = []
        r = Result.ok(42)
        r.match(on_ok=lambda v: results.append(v), on_err=lambda e: None)
        assert results == [42]

    def test_match_err(self):
        errors: list[str] = []
        r = Result.err(ValueError("bad"))
        r.match(on_ok=lambda v: None, on_err=lambda e: errors.append(str(e)))
        assert errors == ["bad"]

    def test_from_func_success(self):
        r = Result.from_func(lambda: 42)
        assert r.is_ok
        assert r.unwrap() == 42

    def test_from_func_failure(self):
        r = Result.from_func(lambda: 1 / 0)
        assert r.is_err
        with pytest.raises(ZeroDivisionError):
            r.unwrap()

    def test_ok_with_none(self):
        r = Result.ok(None)
        assert r.is_ok
        assert r.unwrap() is None

    def test_ok_with_complex_type(self):
        data = {"key": [1, 2, 3]}
        r = Result.ok(data)
        assert r.unwrap() == data
