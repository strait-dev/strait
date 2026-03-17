"""Strait SDK -- Runner for managed execution containers."""

from __future__ import annotations

import logging
import os
import resource
import signal
import sys
import threading
from dataclasses import dataclass, field
from typing import Any, Callable, TypeVar

from strait.client import StraitClient

T = TypeVar("T")

HEARTBEAT_INTERVAL_SECS = 10.0
SIGTERM_GRACE_SECS = 5.0
RESOURCE_MONITOR_INTERVAL_SECS = 5.0
MEMORY_WARN_PERCENT = 80.0
MEMORY_ERROR_PERCENT = 90.0

_logger = logging.getLogger("strait.runner")


@dataclass
class RunContext:
    """Context passed to the user's handler function."""

    run_id: str
    job_slug: str
    attempt: int
    payload: Any
    secrets: dict[str, str] = field(default_factory=dict)

    _aborted: bool = field(default=False, repr=False)
    _client: StraitClient | None = field(default=None, repr=False)

    @property
    def aborted(self) -> bool:
        """Whether SIGTERM has been received."""
        return self._aborted

    def log(self, level: str, msg: str) -> None:
        """Send a log entry for this run."""
        if self._client is not None:
            self._client.log(self.run_id, level, msg)


def _read_secrets() -> dict[str, str]:
    """Read STRAIT_SECRET_* env vars into a plain dict."""
    prefix = "STRAIT_SECRET_"
    secrets: dict[str, str] = {}
    for key, value in os.environ.items():
        if key.startswith(prefix):
            name = key[len(prefix) :]
            secrets[name] = value
    return secrets


class StraitRunner:
    """Runner for managed execution containers."""

    def __init__(
        self,
        *,
        client: StraitClient,
        run_id: str,
        job_slug: str,
        attempt: int,
        payload_mode: str,
        inline_payload: Any,
    ) -> None:
        self._client = client
        self._run_id = run_id
        self._job_slug = job_slug
        self._attempt = attempt
        self._payload_mode = payload_mode
        self._inline_payload = inline_payload

    @classmethod
    def from_env(cls) -> StraitRunner:
        """Create a StraitRunner from environment variables."""
        run_id = os.environ.get("STRAIT_RUN_ID")
        if not run_id:
            raise RuntimeError("STRAIT_RUN_ID environment variable is required")

        token = os.environ.get("STRAIT_SDK_TOKEN")
        if not token:
            raise RuntimeError("STRAIT_SDK_TOKEN environment variable is required")

        base_url = os.environ.get("STRAIT_API_URL", "https://api.runstrait.com")
        job_slug = os.environ.get("STRAIT_JOB_SLUG", "")
        attempt = int(os.environ.get("STRAIT_ATTEMPT", "1"))
        payload_mode = os.environ.get("STRAIT_PAYLOAD_MODE", "inline")

        inline_payload: Any = None
        if payload_mode == "inline":
            raw = os.environ.get("STRAIT_PAYLOAD")
            if raw is not None:
                import json

                try:
                    inline_payload = json.loads(raw)
                except (json.JSONDecodeError, ValueError):
                    inline_payload = raw

        client = StraitClient(base_url, token)

        return cls(
            client=client,
            run_id=run_id,
            job_slug=job_slug,
            attempt=attempt,
            payload_mode=payload_mode,
            inline_payload=inline_payload,
        )

    def run(self, handler: Callable[[RunContext], T]) -> None:
        """Execute the handler with full lifecycle management."""
        heartbeat_stop = threading.Event()
        aborted = False

        def _heartbeat_loop() -> None:
            while not heartbeat_stop.wait(HEARTBEAT_INTERVAL_SECS):
                try:
                    self._client.heartbeat(self._run_id)
                except Exception:
                    pass  # Heartbeat failures are non-fatal.

        def _sigterm_handler(_signum: int, _frame: Any) -> None:
            nonlocal aborted
            aborted = True
            if hasattr(ctx, "_aborted"):
                ctx._aborted = True
            # Grace period, then force exit.
            grace = threading.Timer(SIGTERM_GRACE_SECS, lambda: os._exit(1))
            grace.daemon = True
            grace.start()

        # Placeholder context for sigterm handler reference.
        ctx = RunContext(
            run_id=self._run_id,
            job_slug=self._job_slug,
            attempt=self._attempt,
            payload=None,
            secrets=_read_secrets(),
            _client=self._client,
        )

        prev_handler = signal.getsignal(signal.SIGTERM)
        try:
            signal.signal(signal.SIGTERM, _sigterm_handler)
        except ValueError:
            pass  # Not in main thread; signal handling unavailable.

        # Start heartbeat daemon thread.
        heartbeat_thread = threading.Thread(target=_heartbeat_loop, daemon=True)
        heartbeat_thread.start()

        # Start resource monitor daemon thread.
        resource_stop = threading.Event()
        memory_limit_mb = os.environ.get("STRAIT_MEMORY_LIMIT_MB")
        memory_limit: float | None = float(memory_limit_mb) if memory_limit_mb else None

        def _resource_monitor_loop() -> None:
            while not resource_stop.wait(RESOURCE_MONITOR_INTERVAL_SECS):
                try:
                    usage = resource.getrusage(resource.RUSAGE_SELF)
                    rss_bytes = usage.ru_maxrss
                    # macOS reports in bytes, Linux in KB.
                    if sys.platform == "darwin":
                        rss_mb = rss_bytes / (1024 * 1024)
                    else:
                        rss_mb = rss_bytes / 1024

                    mem_pct: float | None = None
                    if memory_limit and memory_limit > 0:
                        mem_pct = (rss_mb / memory_limit) * 100
                        if mem_pct >= MEMORY_ERROR_PERCENT:
                            _logger.error("memory pressure critical: %.1fMB (%.1f%%)", rss_mb, mem_pct)
                        elif mem_pct >= MEMORY_WARN_PERCENT:
                            _logger.warning("memory pressure warning: %.1fMB (%.1f%%)", rss_mb, mem_pct)

                    self._client.report_resources(
                        self._run_id,
                        memory_mb=rss_mb,
                        memory_percent=mem_pct,
                    )
                except Exception:
                    pass  # Resource monitoring is non-fatal.

        resource_thread = threading.Thread(target=_resource_monitor_loop, daemon=True)
        resource_thread.start()

        exit_code = 0
        try:
            # Resolve payload.
            payload = self._inline_payload
            if self._payload_mode == "fetch":
                payload = self._client.fetch_payload(self._run_id)

            ctx.payload = payload

            # Run the handler.
            result = handler(ctx)

            # Report success.
            self._client.complete(self._run_id, result)
        except Exception as exc:
            exit_code = 1
            # Report failure.
            error_message = str(exc)
            error_class = type(exc).__name__

            try:
                self._client.fail(self._run_id, error_message, error_class)
            except Exception:
                pass  # If fail reporting itself fails, we still exit.
        finally:
            heartbeat_stop.set()
            resource_stop.set()
            heartbeat_thread.join(timeout=1.0)
            resource_thread.join(timeout=1.0)
            self._client.close()
            try:
                signal.signal(signal.SIGTERM, prev_handler)
            except ValueError:
                pass  # Not in main thread.
            sys.exit(exit_code)
