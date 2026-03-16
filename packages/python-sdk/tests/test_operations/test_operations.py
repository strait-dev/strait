"""Table-driven tests for all domain service operations."""


import httpx
import pytest

from strait._client import Client


@pytest.fixture()
def captured_client():
    """Create a client that captures requests and returns empty JSON."""
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={"ok": True})

    transport = httpx.MockTransport(handler)
    http = httpx.Client(transport=transport)
    client = Client(base_url="https://api.test", api_key="key", http_client=http)
    return client, requests


class TestHealthService:
    def test_list(self, captured_client):
        c, reqs = captured_client
        c.health.list()
        assert reqs[-1].method == "GET"
        assert reqs[-1].url.path == "/health"

    def test_get_ready(self, captured_client):
        c, reqs = captured_client
        c.health.get_ready()
        assert reqs[-1].url.path == "/health/ready"

    def test_list_metrics(self, captured_client):
        c, reqs = captured_client
        c.health.list_metrics()
        assert reqs[-1].url.path == "/metrics"


class TestJobsService:
    def test_list(self, captured_client):
        c, reqs = captured_client
        c.jobs.list()
        assert reqs[-1].method == "GET"
        assert reqs[-1].url.path == "/v1/jobs"

    def test_create(self, captured_client):
        c, reqs = captured_client
        c.jobs.create({"name": "test"})
        assert reqs[-1].method == "POST"

    def test_get(self, captured_client):
        c, reqs = captured_client
        c.jobs.get("j1")
        assert reqs[-1].url.path == "/v1/jobs/j1"

    def test_update(self, captured_client):
        c, reqs = captured_client
        c.jobs.update("j1", {"name": "new"})
        assert reqs[-1].method == "PATCH"

    def test_delete(self, captured_client):
        c, reqs = captured_client
        c.jobs.delete("j1")
        assert reqs[-1].method == "DELETE"

    def test_trigger(self, captured_client):
        c, reqs = captured_client
        c.jobs.trigger("j1", {"payload": {}})
        assert reqs[-1].url.path == "/v1/jobs/j1/trigger"

    def test_bulk_trigger(self, captured_client):
        c, reqs = captured_client
        c.jobs.bulk_trigger("j1", {"items": []})
        assert reqs[-1].url.path == "/v1/jobs/j1/trigger/bulk"

    def test_get_version(self, captured_client):
        c, reqs = captured_client
        c.jobs.get_version("j1", "v1")
        assert reqs[-1].url.path == "/v1/jobs/j1/versions/v1"


class TestRunsService:
    def test_list(self, captured_client):
        c, reqs = captured_client
        c.runs.list()
        assert reqs[-1].url.path == "/v1/runs"

    def test_get(self, captured_client):
        c, reqs = captured_client
        c.runs.get("r1")
        assert reqs[-1].url.path == "/v1/runs/r1"

    def test_bulk_cancel(self, captured_client):
        c, reqs = captured_client
        c.runs.bulk_cancel({"ids": ["r1"]})
        assert reqs[-1].url.path == "/v1/runs/bulk-cancel"

    def test_get_dlq(self, captured_client):
        c, reqs = captured_client
        c.runs.get_dlq()
        assert reqs[-1].url.path == "/v1/runs/dlq"


class TestWorkflowsService:
    def test_list(self, captured_client):
        c, reqs = captured_client
        c.workflows.list()
        assert reqs[-1].url.path == "/v1/workflows"

    def test_trigger(self, captured_client):
        c, reqs = captured_client
        c.workflows.trigger("wf1", {"payload": {}})
        assert reqs[-1].url.path == "/v1/workflows/wf1/trigger"

    def test_get_diff(self, captured_client):
        c, reqs = captured_client
        c.workflows.get_diff("wf1", "v1", "v2")
        assert reqs[-1].url.path == "/v1/workflows/wf1/versions/v1/diff/v2"


class TestWorkflowRunsService:
    def test_list(self, captured_client):
        c, reqs = captured_client
        c.workflow_runs.list()
        assert reqs[-1].url.path == "/v1/workflow-runs"

    def test_approve_step(self, captured_client):
        c, reqs = captured_client
        c.workflow_runs.approve_step("wr1", "s1", {"approved": True})
        assert reqs[-1].url.path == "/v1/workflow-runs/wr1/steps/s1/approve"

    def test_skip_step(self, captured_client):
        c, reqs = captured_client
        c.workflow_runs.skip_step("wr1", "s1")
        assert reqs[-1].url.path == "/v1/workflow-runs/wr1/steps/s1/skip"


class TestDeploymentsService:
    def test_create(self, captured_client):
        c, reqs = captured_client
        c.deployments.create({"project_id": "p1"})
        assert reqs[-1].url.path == "/v1/deployments"
        assert reqs[-1].method == "POST"

    def test_finalize(self, captured_client):
        c, reqs = captured_client
        c.deployments.finalize("d1", {"project_id": "p1"})
        assert reqs[-1].url.path == "/v1/deployments/d1/finalize"


class TestSDKRunsService:
    def test_complete_run(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.complete_run("r1", {"output": "done"})
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/complete"

    def test_heartbeat_run(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.heartbeat_run("r1")
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/heartbeat"

    def test_wait_for_event_run(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.wait_for_event_run("r1", {"event_key": "k"})
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/wait-for-event"


class TestSDKRunsStateAndStreamEndpoints:
    def test_set_state(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.set_state("r1", {"key": "k1", "value": "v1"})
        assert reqs[-1].method == "POST"
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/state"

    def test_list_state(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.list_state("r1")
        assert reqs[-1].method == "GET"
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/state"

    def test_get_state(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.get_state("r1", "my-key")
        assert reqs[-1].method == "GET"
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/state/my-key"

    def test_delete_state(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.delete_state("r1", "my-key")
        assert reqs[-1].method == "DELETE"
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/state/my-key"

    def test_stream_run(self, captured_client):
        c, reqs = captured_client
        c.sdk_runs.stream_run("r1", {"chunk": "hello"})
        assert reqs[-1].method == "POST"
        assert reqs[-1].url.path == "/sdk/v1/runs/r1/stream"


class TestRBACService:
    def test_list_roles(self, captured_client):
        c, reqs = captured_client
        c.rbac.list_roles()
        assert reqs[-1].url.path == "/v1/roles"

    def test_seed_roles(self, captured_client):
        c, reqs = captured_client
        c.rbac.seed_roles()
        assert reqs[-1].url.path == "/v1/seed-roles"


class TestJobGroupsService:
    def test_list(self, captured_client):
        c, reqs = captured_client
        c.job_groups.list()
        assert reqs[-1].url.path == "/v1/job-groups"

    def test_pause_all(self, captured_client):
        c, reqs = captured_client
        c.job_groups.pause_all("g1")
        assert reqs[-1].url.path == "/v1/job-groups/g1/pause-all"
