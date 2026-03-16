"""Integration tests: register -> trigger -> wait flow with mocked HTTP."""

import json

import httpx
import pytest

from strait._client import Client
from strait._errors import ApiError, NotFoundError, UnauthorizedError
from strait._middleware import Middleware
from strait.authoring._job import JobOptions, TriggerJobInput, define_job
from strait.authoring._steps import job_step
from strait.authoring._workflow import TriggerWorkflowInput, WorkflowOptions, define_workflow
from strait.composition._wait import WaitForRunOptions, wait_for_run


class TestRegisterTriggerWait:
    def test_job_register_trigger_wait(self):
        poll_count = [0]

        def handler(request: httpx.Request) -> httpx.Response:
            path = request.url.path
            if request.method == "POST" and path == "/v1/jobs":
                return httpx.Response(200, json={"id": "job-1"})
            if request.method == "POST" and "/trigger" in path:
                return httpx.Response(200, json={"id": "run-1", "status": "queued"})
            if request.method == "GET" and "/runs/" in path:
                poll_count[0] += 1
                status = "completed" if poll_count[0] >= 2 else "executing"
                return httpx.Response(200, json={"id": "run-1", "status": status})
            return httpx.Response(404, json={"message": "not found"})

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        client = Client(base_url="https://api.test", api_key="key", http_client=http)

        job = define_job(JobOptions(
            name="Test Job", slug="test-job",
            endpoint_url="https://worker.test/run",
            project_id="proj-1",
        ))

        reg_result = job.register(
            type("C", (), {
                "create_job": lambda self, body: client.jobs.create(body),
                "trigger_job": lambda self, jid, body: client.jobs.trigger(jid, body),
                "bulk_trigger_job": lambda self, jid, body: client.jobs.bulk_trigger(jid, body),
                "get_run": lambda self, rid: client.runs.get(rid),
            })(),
        )
        assert reg_result["id"] == "job-1"

        dsl_client = type("C", (), {
            "create_job": lambda self, body: client.jobs.create(body),
            "trigger_job": lambda self, jid, body: client.jobs.trigger(jid, body),
            "bulk_trigger_job": lambda self, jid, body: client.jobs.bulk_trigger(jid, body),
            "get_run": lambda self, rid: client.runs.get(rid),
        })()

        trigger_result = job.trigger(dsl_client, TriggerJobInput(payload={"key": "val"}))
        assert trigger_result["id"] == "run-1"

        final = wait_for_run(
            get_run=lambda rid: client.runs.get(rid),
            get_status=lambda r: r["status"],
            run_id="run-1",
            opts=WaitForRunOptions(initial_delay_ms=1, timeout_ms=5000),
        )
        assert final["status"] == "completed"

    def test_workflow_register_trigger(self):
        def handler(request: httpx.Request) -> httpx.Response:
            if request.method == "POST" and request.url.path == "/v1/workflows":
                body = json.loads(request.content)
                assert len(body["steps"]) == 2
                return httpx.Response(200, json={"id": "wf-1"})
            if "trigger" in request.url.path:
                return httpx.Response(200, json={"id": "wfr-1"})
            return httpx.Response(200, json={})

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        client = Client(base_url="https://api.test", api_key="key", http_client=http)

        wf = define_workflow(WorkflowOptions(
            name="ETL", slug="etl", project_id="proj-1",
            steps=[
                job_step("extract", "j1"),
                job_step("load", "j2", depends_on=["extract"]),
            ],
        ))

        dsl_client = type("C", (), {
            "create_workflow": lambda self, body: client.workflows.create(body),
            "trigger_workflow": lambda self, wid, body: client.workflows.trigger(wid, body),
            "get_run": lambda self, rid: client.runs.get(rid),
        })()

        reg = wf.register(dsl_client)
        assert reg["id"] == "wf-1"

        result = wf.trigger(dsl_client, TriggerWorkflowInput(payload={}))
        assert result["id"] == "wfr-1"


class TestMiddlewareThroughClient:
    def test_request_response_hooks_fire(self):
        events: list[str] = []

        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(200, json={"status": "ok"})

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        mw = Middleware(
            on_request=lambda ctx: events.append(f"req:{ctx.method}"),
            on_response=lambda ctx: events.append(f"resp:{ctx.status}"),
        )
        client = Client(
            base_url="https://api.test", api_key="key",
            http_client=http, middleware=[mw],
        )

        client.health.list()
        client.jobs.list()

        assert events == ["req:GET", "resp:200", "req:GET", "resp:200"]


class TestErrorParsing:
    def test_api_error_with_message(self):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(422, json={"message": "validation failed", "errors": ["a"]}),
        )
        http = httpx.Client(transport=transport)
        client = Client(base_url="https://api.test", api_key="key", http_client=http)

        with pytest.raises(ApiError, match="validation failed") as exc_info:
            client.jobs.create({})
        assert exc_info.value.body["errors"] == ["a"]

    def test_unauthorized_error(self):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(401, json={"message": "invalid token"}),
        )
        http = httpx.Client(transport=transport)
        client = Client(base_url="https://api.test", api_key="bad", http_client=http)

        with pytest.raises(UnauthorizedError):
            client.runs.list()

    def test_not_found_error(self):
        transport = httpx.MockTransport(
            lambda r: httpx.Response(404, json={"message": "run not found"}),
        )
        http = httpx.Client(transport=transport)
        client = Client(base_url="https://api.test", api_key="key", http_client=http)

        with pytest.raises(NotFoundError):
            client.runs.get("nonexistent")


class TestQueryParams:
    def test_list_with_query(self):
        captured: list[httpx.Request] = []

        def handler(request: httpx.Request) -> httpx.Response:
            captured.append(request)
            return httpx.Response(200, json={"data": []})

        transport = httpx.MockTransport(handler)
        http = httpx.Client(transport=transport)
        client = Client(base_url="https://api.test", api_key="key", http_client=http)

        client.jobs.list(query={"limit": "10", "status": "active"})
        assert captured[-1].url.params["limit"] == "10"
        assert captured[-1].url.params["status"] == "active"
