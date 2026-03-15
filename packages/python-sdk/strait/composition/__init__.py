"""Composition helpers: Result, retry, wait, paginate, idempotency, deployments."""

from strait.composition._deployments import (
    CreateAndFinalizeOutput,
    CreateFinalizePromoteOutput,
    create_and_finalize_deployment,
    create_finalize_promote_deployment,
)
from strait.composition._idempotency import with_idempotency, with_idempotency_header
from strait.composition._paginate import (
    PaginatedQuery,
    PaginatedResponse,
    collect_all,
    paginate,
)
from strait.composition._result import Result
from strait.composition._retry import RetryOptions, with_retry, with_retry_async
from strait.composition._trigger import trigger_and_wait, trigger_and_wait_async
from strait.composition._wait import WaitForRunOptions, wait_for_run, wait_for_run_async

__all__ = [
    "Result",
    "RetryOptions",
    "with_retry",
    "with_retry_async",
    "WaitForRunOptions",
    "wait_for_run",
    "wait_for_run_async",
    "trigger_and_wait",
    "trigger_and_wait_async",
    "PaginatedQuery",
    "PaginatedResponse",
    "paginate",
    "collect_all",
    "with_idempotency",
    "with_idempotency_header",
    "CreateAndFinalizeOutput",
    "CreateFinalizePromoteOutput",
    "create_and_finalize_deployment",
    "create_finalize_promote_deployment",
]
