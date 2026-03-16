"""Strait SDK -- HTTP client for managed execution containers."""

from __future__ import annotations

import time as _time
from typing import Any

import httpx


class StraitClient:
    """HTTP client for communicating with the Strait API from managed containers."""

    def __init__(self, base_url: str, token: str) -> None:
        self._base_url = base_url.rstrip("/")
        self._token = token
        self._client = httpx.Client(
            headers={
                "Authorization": f"Bearer {token}",
                "Content-Type": "application/json",
            },
            timeout=30.0,
        )

    def _request_with_retry(
        self,
        method: str,
        url: str,
        max_attempts: int = 3,
        **kwargs: Any,
    ) -> httpx.Response:
        """Send request with exponential backoff retry for transient failures."""
        last_exc: Exception | None = None
        for attempt in range(max_attempts):
            try:
                resp = self._client.request(method, url, **kwargs)
                if resp.status_code < 500:
                    resp.raise_for_status()
                    return resp
                last_exc = httpx.HTTPStatusError(
                    f"server error ({resp.status_code})",
                    request=resp.request,
                    response=resp,
                )
            except (httpx.TransportError, httpx.HTTPStatusError) as exc:
                last_exc = exc
            if attempt < max_attempts - 1:
                backoff = (2**attempt) + (_time.monotonic() % 0.5)
                _time.sleep(backoff)
        raise last_exc  # type: ignore[misc]

    def complete(self, run_id: str, result: Any) -> None:
        """Mark a run as completed with a result."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/complete"
        self._request_with_retry("POST", url, json={"result": result})

    def fail(self, run_id: str, error: str, error_class: str | None = None) -> None:
        """Mark a run as failed with an error."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/fail"
        body: dict[str, Any] = {"error": error}
        if error_class is not None:
            body["error_class"] = error_class
        self._request_with_retry("POST", url, json=body)

    def heartbeat(self, run_id: str) -> None:
        """Send a heartbeat for a run."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/heartbeat"
        resp = self._client.post(url)
        resp.raise_for_status()

    def fetch_payload(self, run_id: str) -> Any:
        """Fetch the payload for a run."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/payload"
        resp = self._client.get(url)
        resp.raise_for_status()
        return resp.json().get("payload")

    def log(self, run_id: str, level: str, message: str) -> None:
        """Send a log entry for a run."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/log"
        resp = self._client.post(url, json={"level": level, "message": message})
        resp.raise_for_status()

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self._client.close()
