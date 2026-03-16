"""Tests for composition retry."""

import pytest

from strait.composition._retry import JitterStrategy, RetryOptions, with_retry


class TestWithRetry:
    def test_success_on_first_attempt(self):
        result = with_retry(lambda: 42)
        assert result == 42

    def test_success_after_retries(self):
        attempts = [0]

        def fn():
            attempts[0] += 1
            if attempts[0] < 3:
                raise ValueError("not yet")
            return "done"

        result = with_retry(fn, RetryOptions(attempts=3, delay_ms=1, jitter=JitterStrategy.NONE))
        assert result == "done"
        assert attempts[0] == 3

    def test_exhausted_retries_raises(self):
        def fn():
            raise ValueError("always fails")

        with pytest.raises(ValueError, match="always fails"):
            with_retry(fn, RetryOptions(attempts=2, delay_ms=1, jitter=JitterStrategy.NONE))

    def test_should_retry_can_abort(self):
        attempts = [0]

        def fn():
            attempts[0] += 1
            raise ValueError("fail")

        opts = RetryOptions(
            attempts=5, delay_ms=1, jitter=JitterStrategy.NONE,
            should_retry=lambda err, attempt, max_: attempt < 2,
        )
        with pytest.raises(ValueError):
            with_retry(fn, opts)
        assert attempts[0] == 2

    def test_default_options(self):
        result = with_retry(lambda: "ok")
        assert result == "ok"

    def test_jitter_full_does_not_crash(self):
        result = with_retry(
            lambda: 42,
            RetryOptions(attempts=1, delay_ms=10, jitter=JitterStrategy.FULL),
        )
        assert result == 42

    def test_backoff_factor(self):
        attempts = [0]

        def fn():
            attempts[0] += 1
            if attempts[0] < 3:
                raise ValueError("retry")
            return "ok"

        result = with_retry(
            fn,
            RetryOptions(
                attempts=3, delay_ms=1, factor=2.0,
                max_delay_ms=100, jitter=JitterStrategy.NONE,
            ),
        )
        assert result == "ok"
