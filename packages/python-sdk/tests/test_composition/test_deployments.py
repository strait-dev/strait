"""Tests for composition deployment helpers."""

import pytest

from strait.composition._deployments import (
    create_and_finalize_deployment,
    create_finalize_promote_deployment,
)


class FakeDeploymentClient:
    def __init__(self):
        self.calls = []

    def create_deployment(self, body):
        self.calls.append(("create", body))
        return {"id": "dep-1"}

    def finalize_deployment(self, deployment_id, body):
        self.calls.append(("finalize", deployment_id, body))
        return {"id": "dep-1-final"}

    def promote_deployment(self, deployment_id, body):
        self.calls.append(("promote", deployment_id, body))
        return {"id": "dep-1-promoted"}

    def rollback_deployment(self, deployment_id, body):
        self.calls.append(("rollback", deployment_id, body))
        return {"id": "dep-1-rollback"}


class TestCreateAndFinalize:
    def test_basic_flow(self):
        client = FakeDeploymentClient()
        result = create_and_finalize_deployment(
            client,
            {"project_id": "p1", "environment": "prod", "runtime": "python", "artifact_uri": "s3://x"},
        )
        assert result.created["id"] == "dep-1"
        assert result.finalized["id"] == "dep-1-final"
        assert len(client.calls) == 2

    def test_infers_mutation_body(self):
        client = FakeDeploymentClient()
        create_and_finalize_deployment(
            client,
            {"project_id": "p1", "environment": "staging", "runtime": "go", "artifact_uri": "s3://y"},
        )
        _, _, finalize_body = client.calls[1]
        assert finalize_body == {"project_id": "p1", "environment": "staging"}

    def test_custom_finalize_body(self):
        client = FakeDeploymentClient()
        create_and_finalize_deployment(
            client,
            {"project_id": "p1", "environment": "prod", "runtime": "go", "artifact_uri": "s3://y"},
            finalize_body={"project_id": "p2", "environment": "dev"},
        )
        _, _, finalize_body = client.calls[1]
        assert finalize_body == {"project_id": "p2", "environment": "dev"}

    def test_missing_id_raises(self):
        class NoIDClient(FakeDeploymentClient):
            def create_deployment(self, body):
                return {}

        with pytest.raises(ValueError, match="missing a usable id"):
            create_and_finalize_deployment(NoIDClient(), {"project_id": "p1", "environment": "e"})


class TestCreateFinalizePromote:
    def test_full_flow(self):
        client = FakeDeploymentClient()
        result = create_finalize_promote_deployment(
            client,
            {"project_id": "p1", "environment": "prod", "runtime": "go", "artifact_uri": "s3://x"},
        )
        assert result.created["id"] == "dep-1"
        assert result.finalized["id"] == "dep-1-final"
        assert result.promoted["id"] == "dep-1-promoted"
        assert len(client.calls) == 3

    def test_custom_promote_body(self):
        client = FakeDeploymentClient()
        create_finalize_promote_deployment(
            client,
            {"project_id": "p1", "environment": "prod", "runtime": "go", "artifact_uri": "s3://x"},
            promote_body={"project_id": "p3", "environment": "production"},
        )
        _, _, promote_body = client.calls[2]
        assert promote_body == {"project_id": "p3", "environment": "production"}
