"""Tests for strait._middleware."""

from strait._middleware import (
    Middleware,
    MiddlewareErrorContext,
    MiddlewareRequestContext,
    MiddlewareResponseContext,
)


class TestMiddleware:
    def test_middleware_with_all_hooks(self):
        events: list[str] = []
        mw = Middleware(
            on_request=lambda ctx: events.append(f"req:{ctx.method}"),
            on_response=lambda ctx: events.append(f"resp:{ctx.status}"),
            on_error=lambda ctx: events.append(f"err:{ctx.error}"),
        )
        mw.on_request(MiddlewareRequestContext(method="GET", url="/test", headers={}))
        mw.on_response(MiddlewareResponseContext(
            method="GET", url="/test", status=200, duration_ms=10,
        ))
        mw.on_error(MiddlewareErrorContext(method="GET", url="/test", error=Exception("boom")))
        assert events == ["req:GET", "resp:200", "err:boom"]

    def test_middleware_with_no_hooks(self):
        mw = Middleware()
        assert mw.on_request is None
        assert mw.on_response is None
        assert mw.on_error is None

    def test_request_context_attrs(self):
        ctx = MiddlewareRequestContext(
            method="POST", url="https://api.example.com/v1/jobs",
            headers={"Authorization": "Bearer x"}, body=b'{"name": "test"}',
        )
        assert ctx.method == "POST"
        assert ctx.body == b'{"name": "test"}'

    def test_response_context_attrs(self):
        ctx = MiddlewareResponseContext(
            method="GET", url="/test", status=200, duration_ms=42,
        )
        assert ctx.duration_ms == 42
