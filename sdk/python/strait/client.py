"""Strait SDK -- HTTP client for managed execution containers."""

from __future__ import annotations

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

    def complete(self, run_id: str, result: Any) -> None:
        """Mark a run as completed with a result."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/complete"
        resp = self._client.post(url, json={"result": result})
        resp.raise_for_status()

    def fail(self, run_id: str, error: str, error_class: str | None = None) -> None:
        """Mark a run as failed with an error."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/fail"
        body: dict[str, Any] = {"error": error}
        if error_class is not None:
            body["error_class"] = error_class
        resp = self._client.post(url, json=body)
        resp.raise_for_status()

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
