"""Strait Event Triggers SDK — Python client."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any
from urllib.parse import quote

import httpx


@dataclass
class WaitForEventOptions:
    """Options for waiting on an external event."""

    event_key: str
    timeout_secs: int = 3600
    notify_url: str | None = None


@dataclass
class SendEventOptions:
    """Options for sending an event to resolve a waiting trigger."""

    event_key: str
    payload: dict[str, Any] | None = None


@dataclass
class EventTrigger:
    """Represents a durable event trigger."""

    id: str
    event_key: str
    project_id: str
    source_type: str
    trigger_type: str
    status: str
    timeout_secs: int
    requested_at: str
    expires_at: str
    workflow_run_id: str | None = None
    workflow_step_run_id: str | None = None
    job_run_id: str | None = None
    request_payload: Any = None
    response_payload: Any = None
    received_at: str | None = None
    error: str | None = None
    notify_url: str | None = None
    notify_status: str | None = None
    sent_by: str | None = None


class EventsClient:
    """Event trigger client for the Strait SDK."""

    def __init__(self, base_url: str, headers: dict[str, str]) -> None:
        self._base_url = base_url.rstrip("/")
        self._client = httpx.Client(headers=headers, timeout=30.0)

    def wait_for_event(self, run_id: str, options: WaitForEventOptions) -> EventTrigger:
        """Pause the current run and wait for an external event."""
        url = f"{self._base_url}/sdk/v1/runs/{run_id}/wait-for-event"
        body: dict[str, Any] = {
            "event_key": options.event_key,
            "timeout_secs": options.timeout_secs,
        }
        if options.notify_url:
            body["notify_url"] = options.notify_url

        resp = self._client.post(url, json=body)
        resp.raise_for_status()
        return EventTrigger(**resp.json())

    def send_event(self, options: SendEventOptions) -> EventTrigger:
        """Send an event to resolve a waiting trigger."""
        url = f"{self._base_url}/v1/events/{quote(options.event_key, safe='')}/send"
        body: dict[str, Any] = {}
        if options.payload:
            body["payload"] = options.payload

        resp = self._client.post(url, json=body)
        resp.raise_for_status()
        return EventTrigger(**resp.json())

    def get_event_trigger(self, event_key: str) -> EventTrigger:
        """Get an event trigger by its event key."""
        url = f"{self._base_url}/v1/events/{quote(event_key, safe='')}"
        resp = self._client.get(url)
        resp.raise_for_status()
        return EventTrigger(**resp.json())

    def cancel_event_trigger(self, event_key: str) -> EventTrigger:
        """Cancel a waiting event trigger."""
        url = f"{self._base_url}/v1/events/{quote(event_key, safe='')}"
        resp = self._client.delete(url)
        resp.raise_for_status()
        return EventTrigger(**resp.json())

    def close(self) -> None:
        """Close the underlying HTTP client."""
        self._client.close()
