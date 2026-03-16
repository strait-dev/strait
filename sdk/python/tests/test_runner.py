"""Tests for Strait SDK runner and client."""

from __future__ import annotations

import os
import threading
import time
from typing import Any
from unittest.mock import MagicMock, call, patch

import pytest

from strait.client import StraitClient
from strait.runner import RunContext, StraitRunner


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _base_env() -> dict[str, str]:
    return {
        "STRAIT_RUN_ID": "run-123",
        "STRAIT_SDK_TOKEN": "tok-secret",
        "STRAIT_API_URL": "https://api.test.com",
        "STRAIT_JOB_SLUG": "my-job",
        "STRAIT_ATTEMPT": "2",
        "STRAIT_PAYLOAD_MODE": "inline",
        "STRAIT_PAYLOAD": '{"key":"value"}',
    }


def _make_runner_with_mock(
    env_overrides: dict[str, str] | None = None,
) -> tuple[StraitRunner, MagicMock]:
    """Create a runner with a mocked client, bypassing real HTTP."""
    env = _base_env()
    if env_overrides:
        env.update(env_overrides)

    with patch.dict(os.environ, env, clear=False):
        runner = StraitRunner.from_env()

    mock_client = MagicMock(spec=StraitClient)
    mock_client.heartbeat = MagicMock()
    mock_client.complete = MagicMock()
    mock_client.fail = MagicMock()
    mock_client.log = MagicMock()
    mock_client.fetch_payload = MagicMock(return_value={"fetched": True})

    runner._client = mock_client
    return runner, mock_client


# ---------------------------------------------------------------------------
# 1. Happy path: handler returns value -> /complete called
# ---------------------------------------------------------------------------

class TestHappyPath:
    def test_complete_called_on_success(self) -> None:
        runner, mock_client = _make_runner_with_mock()

        def handler(ctx: RunContext) -> dict[str, int]:
            return {"answer": 42}

        with pytest.raises(SystemExit) as exc_info:
            runner.run(handler)

        assert exc_info.value.code == 0
        mock_client.complete.assert_called_once_with("run-123", {"answer": 42})


# ---------------------------------------------------------------------------
# 2. Handler throws -> /fail called
# ---------------------------------------------------------------------------

class TestHandlerFailure:
    def test_fail_called_on_exception(self) -> None:
        runner, mock_client = _make_runner_with_mock()

        def handler(ctx: RunContext) -> None:
            raise ValueError("handler broke")

        with pytest.raises(SystemExit) as exc_info:
            runner.run(handler)

        assert exc_info.value.code == 0
        mock_client.fail.assert_called_once_with(
            "run-123", "handler broke", "ValueError"
        )


# ---------------------------------------------------------------------------
# 3. Payload inline from env
# ---------------------------------------------------------------------------

class TestInlinePayload:
    def test_inline_payload_parsed_from_env(self) -> None:
        runner, mock_client = _make_runner_with_mock()
        captured: dict[str, Any] = {}

        def handler(ctx: RunContext) -> str:
            captured["payload"] = ctx.payload
            return "ok"

        with pytest.raises(SystemExit):
            runner.run(handler)

        assert captured["payload"] == {"key": "value"}


# ---------------------------------------------------------------------------
# 4. Payload fetch mode
# ---------------------------------------------------------------------------

class TestFetchPayload:
    def test_fetches_payload_when_mode_is_fetch(self) -> None:
        runner, mock_client = _make_runner_with_mock(
            {"STRAIT_PAYLOAD_MODE": "fetch"}
        )
        captured: dict[str, Any] = {}

        def handler(ctx: RunContext) -> str:
            captured["payload"] = ctx.payload
            return "ok"

        with pytest.raises(SystemExit):
            runner.run(handler)

        mock_client.fetch_payload.assert_called_once_with("run-123")
        assert captured["payload"] == {"fetched": True}


# ---------------------------------------------------------------------------
# 5. Heartbeat fires at intervals
# ---------------------------------------------------------------------------

class TestHeartbeat:
    def test_heartbeat_fires(self) -> None:
        runner, mock_client = _make_runner_with_mock()
        barrier = threading.Event()

        def handler(ctx: RunContext) -> str:
            # Wait enough time for at least one heartbeat (10s interval).
            # We can't truly wait 10s in tests, so we patch the interval.
            barrier.wait(timeout=2.0)
            return "ok"

        # Patch heartbeat interval to be very short for testing.
        with patch("strait.runner.HEARTBEAT_INTERVAL_SECS", 0.1):
            # Run in a thread since runner calls sys.exit.
            def run_in_thread() -> None:
                try:
                    runner.run(handler)
                except SystemExit:
                    pass

            t = threading.Thread(target=run_in_thread)
            t.start()

            # Let some heartbeats fire.
            time.sleep(0.5)
            barrier.set()
            t.join(timeout=5.0)

        assert mock_client.heartbeat.call_count >= 2


# ---------------------------------------------------------------------------
# 6. Missing STRAIT_RUN_ID -> throws
# ---------------------------------------------------------------------------

class TestMissingRunId:
    def test_raises_without_run_id(self) -> None:
        env = _base_env()
        del env["STRAIT_RUN_ID"]

        with patch.dict(os.environ, env, clear=True):
            with pytest.raises(RuntimeError, match="STRAIT_RUN_ID"):
                StraitRunner.from_env()


# ---------------------------------------------------------------------------
# 7. Missing STRAIT_SDK_TOKEN -> throws
# ---------------------------------------------------------------------------

class TestMissingToken:
    def test_raises_without_sdk_token(self) -> None:
        env = _base_env()
        del env["STRAIT_SDK_TOKEN"]

        with patch.dict(os.environ, env, clear=True):
            with pytest.raises(RuntimeError, match="STRAIT_SDK_TOKEN"):
                StraitRunner.from_env()


# ---------------------------------------------------------------------------
# 8. All HTTP calls include Authorization header
# ---------------------------------------------------------------------------

class TestAuthHeader:
    def test_client_created_with_bearer_token(self) -> None:
        env = _base_env()
        with patch.dict(os.environ, env, clear=False):
            with patch("strait.runner.StraitClient") as MockClient:
                StraitRunner.from_env()
                MockClient.assert_called_once_with(
                    "https://api.test.com", "tok-secret"
                )


class TestClientAuth:
    def test_headers_include_authorization(self) -> None:
        """Verify the StraitClient is constructed with auth headers."""
        with patch("httpx.Client") as mock_httpx:
            StraitClient("https://api.test.com", "tok-secret")
            mock_httpx.assert_called_once()
            headers = mock_httpx.call_args.kwargs["headers"]
            assert headers["Authorization"] == "Bearer tok-secret"


# ---------------------------------------------------------------------------
# 9. Secrets from STRAIT_SECRET_* env vars
# ---------------------------------------------------------------------------

class TestSecrets:
    def test_reads_secret_env_vars(self) -> None:
        runner, mock_client = _make_runner_with_mock()
        captured: dict[str, Any] = {}

        # Inject secret env vars.
        extra_env = {
            "STRAIT_SECRET_DB_URL": "postgres://localhost/db",
            "STRAIT_SECRET_API_KEY": "sk-123",
        }

        def handler(ctx: RunContext) -> str:
            captured["secrets"] = ctx.secrets
            return "ok"

        with patch.dict(os.environ, extra_env, clear=False):
            # Re-create runner to pick up secrets.
            runner, mock_client = _make_runner_with_mock()
            with pytest.raises(SystemExit):
                with patch.dict(os.environ, extra_env, clear=False):
                    runner.run(handler)

        assert captured["secrets"]["DB_URL"] == "postgres://localhost/db"
        assert captured["secrets"]["API_KEY"] == "sk-123"


# ---------------------------------------------------------------------------
# Client unit tests
# ---------------------------------------------------------------------------

class TestStraitClient:
    def test_complete_posts_result(self) -> None:
        with patch("httpx.Client") as MockHttpx:
            mock_instance = MagicMock()
            mock_resp = MagicMock()
            mock_resp.raise_for_status = MagicMock()
            mock_instance.post.return_value = mock_resp
            MockHttpx.return_value = mock_instance

            client = StraitClient("https://api.test.com", "tok-123")
            client.complete("run-1", {"done": True})

            mock_instance.post.assert_called_once_with(
                "https://api.test.com/sdk/v1/runs/run-1/complete",
                json={"result": {"done": True}},
            )

    def test_fail_posts_error(self) -> None:
        with patch("httpx.Client") as MockHttpx:
            mock_instance = MagicMock()
            mock_resp = MagicMock()
            mock_resp.raise_for_status = MagicMock()
            mock_instance.post.return_value = mock_resp
            MockHttpx.return_value = mock_instance

            client = StraitClient("https://api.test.com", "tok-123")
            client.fail("run-1", "oops", "RuntimeError")

            mock_instance.post.assert_called_once_with(
                "https://api.test.com/sdk/v1/runs/run-1/fail",
                json={"error": "oops", "error_class": "RuntimeError"},
            )

    def test_heartbeat_posts(self) -> None:
        with patch("httpx.Client") as MockHttpx:
            mock_instance = MagicMock()
            mock_resp = MagicMock()
            mock_resp.raise_for_status = MagicMock()
            mock_instance.post.return_value = mock_resp
            MockHttpx.return_value = mock_instance

            client = StraitClient("https://api.test.com", "tok-123")
            client.heartbeat("run-1")

            mock_instance.post.assert_called_once_with(
                "https://api.test.com/sdk/v1/runs/run-1/heartbeat",
            )

    def test_fetch_payload_gets_and_returns_payload(self) -> None:
        with patch("httpx.Client") as MockHttpx:
            mock_instance = MagicMock()
            mock_resp = MagicMock()
            mock_resp.raise_for_status = MagicMock()
            mock_resp.json.return_value = {"payload": {"data": 1}}
            mock_instance.get.return_value = mock_resp
            MockHttpx.return_value = mock_instance

            client = StraitClient("https://api.test.com", "tok-123")
            result = client.fetch_payload("run-1")

            assert result == {"data": 1}
            mock_instance.get.assert_called_once_with(
                "https://api.test.com/sdk/v1/runs/run-1/payload",
            )

    def test_log_posts_level_and_message(self) -> None:
        with patch("httpx.Client") as MockHttpx:
            mock_instance = MagicMock()
            mock_resp = MagicMock()
            mock_resp.raise_for_status = MagicMock()
            mock_instance.post.return_value = mock_resp
            MockHttpx.return_value = mock_instance

            client = StraitClient("https://api.test.com", "tok-123")
            client.log("run-1", "info", "hello")

            mock_instance.post.assert_called_once_with(
                "https://api.test.com/sdk/v1/runs/run-1/log",
                json={"level": "info", "message": "hello"},
            )

    def test_strips_trailing_slash(self) -> None:
        with patch("httpx.Client") as MockHttpx:
            mock_instance = MagicMock()
            mock_resp = MagicMock()
            mock_resp.raise_for_status = MagicMock()
            mock_instance.post.return_value = mock_resp
            MockHttpx.return_value = mock_instance

            client = StraitClient("https://api.test.com/", "tok-123")
            client.heartbeat("run-1")

            mock_instance.post.assert_called_once_with(
                "https://api.test.com/sdk/v1/runs/run-1/heartbeat",
            )
