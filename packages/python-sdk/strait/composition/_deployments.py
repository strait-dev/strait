"""Deployment workflow helpers."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Protocol


class DeploymentClient(Protocol):
    def create_deployment(self, body: dict[str, Any]) -> dict[str, Any]: ...
    def finalize_deployment(
        self, deployment_id: str, body: dict[str, Any],
    ) -> dict[str, Any]: ...
    def promote_deployment(
        self, deployment_id: str, body: dict[str, Any],
    ) -> dict[str, Any]: ...
    def rollback_deployment(
        self, deployment_id: str, body: dict[str, Any],
    ) -> dict[str, Any]: ...


@dataclass
class CreateAndFinalizeOutput:
    created: dict[str, Any]
    finalized: dict[str, Any]


@dataclass
class CreateFinalizePromoteOutput:
    created: dict[str, Any]
    finalized: dict[str, Any]
    promoted: dict[str, Any]


def _infer_mutation_body(create_body: dict[str, Any]) -> dict[str, Any]:
    return {
        "project_id": create_body.get("project_id", ""),
        "environment": create_body.get("environment", ""),
    }


def create_and_finalize_deployment(
    client: DeploymentClient,
    create_body: dict[str, Any],
    finalize_body: dict[str, Any] | None = None,
) -> CreateAndFinalizeOutput:
    created = client.create_deployment(create_body)
    deployment_id = created.get("id", "")
    if not deployment_id:
        raise ValueError("deployment response is missing a usable id")

    fb = finalize_body or _infer_mutation_body(create_body)
    finalized = client.finalize_deployment(deployment_id, fb)

    return CreateAndFinalizeOutput(created=created, finalized=finalized)


def create_finalize_promote_deployment(
    client: DeploymentClient,
    create_body: dict[str, Any],
    finalize_body: dict[str, Any] | None = None,
    promote_body: dict[str, Any] | None = None,
) -> CreateFinalizePromoteOutput:
    out = create_and_finalize_deployment(client, create_body, finalize_body)

    deployment_id = out.finalized.get("id") or out.created.get("id", "")

    if promote_body is not None:
        pb = promote_body
    elif finalize_body is not None:
        pb = finalize_body
    else:
        pb = _infer_mutation_body(create_body)

    promoted = client.promote_deployment(deployment_id, pb)

    return CreateFinalizePromoteOutput(
        created=out.created, finalized=out.finalized, promoted=promoted,
    )
