"""Tests for strait._errors."""


from strait._errors import (
    ApiError,
    ConflictError,
    DagValidationError,
    DecodeError,
    NotFoundError,
    RateLimitedError,
    StraitError,
    StraitTimeoutError,
    TransportError,
    UnauthorizedError,
    ValidationError,
    map_http_error,
)


class TestErrorHierarchy:
    def test_all_errors_inherit_from_strait_error(self):
        errors = [
            TransportError("t"),
            DecodeError("d", body="{}"),
            ValidationError("v"),
            UnauthorizedError(401, "u"),
            NotFoundError(404, "n"),
            ConflictError(409, "c"),
            RateLimitedError(429, "r"),
            ApiError(500, "a"),
            StraitTimeoutError("to", run_id="r1", elapsed_ms=100),
            DagValidationError("dag"),
        ]
        for err in errors:
            assert isinstance(err, StraitError)

    def test_transport_error_wraps_cause(self):
        cause = OSError("network down")
        err = TransportError("failed", cause=cause)
        assert err.__cause__ is cause
        assert str(err) == "failed"

    def test_decode_error_has_body(self):
        err = DecodeError("bad json", body="not json", cause=ValueError("x"))
        assert err.body == "not json"

    def test_validation_error_has_issues(self):
        err = ValidationError("bad", issues=["a", "b"])
        assert err.issues == ["a", "b"]

    def test_validation_error_default_issues(self):
        err = ValidationError("bad")
        assert err.issues == []

    def test_unauthorized_error_attrs(self):
        err = UnauthorizedError(403, "forbidden", body={"msg": "no"})
        assert err.status == 403
        assert err.body == {"msg": "no"}

    def test_timeout_error_attrs(self):
        err = StraitTimeoutError("timed out", run_id="run-1", elapsed_ms=5000)
        assert err.run_id == "run-1"
        assert err.elapsed_ms == 5000

    def test_dag_validation_error_attrs(self):
        err = DagValidationError(
            "cycle", cycles=["a", "b"], missing_refs=["c"], duplicate_refs=["d"],
        )
        assert err.cycles == ["a", "b"]
        assert err.missing_refs == ["c"]
        assert err.duplicate_refs == ["d"]

    def test_dag_validation_error_defaults(self):
        err = DagValidationError("empty")
        assert err.cycles == []
        assert err.missing_refs == []
        assert err.duplicate_refs == []


class TestMapHttpError:
    def test_401_returns_unauthorized(self):
        err = map_http_error(401, "unauthorized", None)
        assert isinstance(err, UnauthorizedError)
        assert err.status == 401

    def test_403_returns_unauthorized(self):
        err = map_http_error(403, "forbidden", None)
        assert isinstance(err, UnauthorizedError)

    def test_404_returns_not_found(self):
        err = map_http_error(404, "not found", None)
        assert isinstance(err, NotFoundError)

    def test_409_returns_conflict(self):
        err = map_http_error(409, "conflict", None)
        assert isinstance(err, ConflictError)

    def test_429_returns_rate_limited(self):
        err = map_http_error(429, "rate limited", None)
        assert isinstance(err, RateLimitedError)

    def test_500_returns_api_error(self):
        err = map_http_error(500, "server error", None)
        assert isinstance(err, ApiError)
        assert err.status == 500

    def test_empty_message_defaults(self):
        err = map_http_error(500, "", None)
        assert str(err) == "HTTP 500"

    def test_422_returns_api_error(self):
        err = map_http_error(422, "unprocessable", {"detail": "bad"})
        assert isinstance(err, ApiError)
        assert err.body == {"detail": "bad"}
