"""Tests for strait._http."""

import json

import httpx
import pytest

from strait._config import AuthMode, AuthType, Config
from strait._errors import ApiError, DecodeError, NotFoundError, TransportError, UnauthorizedError
from strait._http import do_request, substitute_path_params


class TestSubstitutePathParams:
    def test_single_param(self):
        assert substitute_path_params("/v1/jobs/{jobID}", {"jobID": "j1"}) == "/v1/jobs/j1"

    def test_multiple_params(self):
        result = substitute_path_params(
            "/v1/jobs/{jobID}/versions/{versionID}",
            {"jobID": "j1", "versionID": "v2"},
        )
        assert result == "/v1/jobs/j1/versions/v2"

    def test_no_params(self):
        assert substitute_path_params("/v1/jobs", {}) == "/v1/jobs"

    def test_missing_param_left_intact(self):
        assert substitute_path_params("/v1/jobs/{jobID}", {"other": "x"}) == "/v1/jobs/{jobID}"


class TestDoRequest:
    @pytest.fixture()
    def config(self):
        return Config(
            base_url="https://api.example.com",
            auth=AuthMode(type=AuthType.API_KEY, token="test-key"),
        )

    def test_get_success(self, config):
        def handler(request: httpx.Request) -> httpx.Response:
            assert request.method == "GET"
            assert str(request.url) == "https://api.example.com/v1/jobs"
            assert request.headers["Authorization"] == "Bearer test-key"
            return httpx.Response(200, json={"data": []})

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        result = do_request(client, config, method="GET", path="/v1/jobs")
        assert result == {"data": []}

    def test_post_with_body(self, config):
        def handler(request: httpx.Request) -> httpx.Response:
            body = json.loads(request.content)
            assert body == {"name": "test"}
            return httpx.Response(200, json={"id": "j1"})

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        result = do_request(client, config, method="POST", path="/v1/jobs", body={"name": "test"})
        assert result == {"id": "j1"}

    def test_query_params(self, config):
        def handler(request: httpx.Request) -> httpx.Response:
            assert request.url.params["limit"] == "10"
            return httpx.Response(200, json={})

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        do_request(client, config, method="GET", path="/v1/jobs", query={"limit": "10"})

    def test_custom_headers(self, config):
        def handler(request: httpx.Request) -> httpx.Response:
            assert request.headers["X-Custom"] == "value"
            return httpx.Response(200, json={})

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        do_request(client, config, method="GET", path="/v1/jobs", headers={"X-Custom": "value"})

    def test_default_headers(self, config):
        config.default_headers = {"X-Project": "proj-1"}

        def handler(request: httpx.Request) -> httpx.Response:
            assert request.headers["X-Project"] == "proj-1"
            return httpx.Response(200, json={})

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        do_request(client, config, method="GET", path="/v1/jobs")

    def test_path_params(self, config):
        def handler(request: httpx.Request) -> httpx.Response:
            assert str(request.url) == "https://api.example.com/v1/jobs/j1"
            return httpx.Response(200, json={"id": "j1"})

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        result = do_request(
            client, config, method="GET", path="/v1/jobs/{jobID}",
            path_params={"jobID": "j1"},
        )
        assert result["id"] == "j1"

    def test_401_raises_unauthorized(self, config):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(401, json={"message": "unauthorized"}),
        )
        client = httpx.Client(transport=transport)
        with pytest.raises(UnauthorizedError):
            do_request(client, config, method="GET", path="/v1/jobs")

    def test_404_raises_not_found(self, config):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(404, json={"message": "not found"}),
        )
        client = httpx.Client(transport=transport)
        with pytest.raises(NotFoundError):
            do_request(client, config, method="GET", path="/v1/jobs/missing")

    def test_500_raises_api_error(self, config):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(500, text="internal error"),
        )
        client = httpx.Client(transport=transport)
        with pytest.raises(ApiError):
            do_request(client, config, method="GET", path="/v1/jobs")

    def test_error_extracts_message_from_body(self, config):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(400, json={"message": "bad request detail"}),
        )
        client = httpx.Client(transport=transport)
        with pytest.raises(ApiError, match="bad request detail"):
            do_request(client, config, method="POST", path="/v1/jobs", body={})

    def test_empty_response_returns_empty_dict(self, config):
        transport = httpx.MockTransport(lambda r: httpx.Response(204, text=""))
        client = httpx.Client(transport=transport)
        result = do_request(client, config, method="DELETE", path="/v1/jobs/j1")
        assert result == {}

    def test_network_error_raises_transport_error(self, config):
        def handler(request: httpx.Request) -> httpx.Response:
            raise httpx.ConnectError("connection refused")

        transport = httpx.MockTransport(handler)
        client = httpx.Client(transport=transport)
        with pytest.raises(TransportError, match="request failed"):
            do_request(client, config, method="GET", path="/v1/jobs")

    def test_invalid_json_raises_decode_error(self, config):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(200, text="not json", headers={"content-type": "text/plain"}),
        )
        client = httpx.Client(transport=transport)
        with pytest.raises(DecodeError):
            do_request(client, config, method="GET", path="/v1/jobs")
