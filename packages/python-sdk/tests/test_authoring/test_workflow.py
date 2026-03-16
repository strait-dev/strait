"""Tests for authoring workflow definitions."""

import pytest

from strait._errors import DagValidationError
from strait.authoring._steps import job_step
from strait.authoring._workflow import (
    TriggerWorkflowInput,
    WorkflowOptions,
    define_workflow,
)


class FakeWorkflowClient:
    def __init__(self):
        self.calls = []

    def create_workflow(self, body):
        self.calls.append(("create", body))
        return {"id": "wf-123"}

    def trigger_workflow(self, workflow_id, body):
        self.calls.append(("trigger", workflow_id, body))
        return {"id": "wfr-1"}

    def get_run(self, run_id):
        return {"id": run_id, "status": "completed"}


class TestDefineWorkflow:
    def test_creates_workflow_definition(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="my-wf"))
        assert wf.kind == "workflow"
        assert wf.slug == "my-wf"

    def test_registration_body(self):
        wf = define_workflow(WorkflowOptions(
            name="WF", slug="my-wf", project_id="p1",
            steps=[
                job_step("a", "j1"),
                job_step("b", "j2", depends_on=["a"]),
            ],
        ))
        body = wf.to_registration_body()
        assert body["project_id"] == "p1"
        assert body["name"] == "WF"
        assert len(body["steps"]) == 2
        assert body["steps"][0]["step_ref"] == "a"
        assert body["steps"][1]["depends_on"] == ["a"]

    def test_registration_validates_dag(self):
        wf = define_workflow(WorkflowOptions(
            name="WF", slug="wf", project_id="p1",
            steps=[
                job_step("a", "j1", depends_on=["b"]),
                job_step("b", "j2", depends_on=["a"]),
            ],
        ))
        with pytest.raises(DagValidationError):
            wf.to_registration_body()

    def test_registration_requires_project_id(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="wf"))
        with pytest.raises(ValueError, match="requires project_id"):
            wf.to_registration_body()

    def test_register(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="wf", project_id="p1"))
        client = FakeWorkflowClient()
        result = wf.register(client)
        assert result["id"] == "wf-123"
        assert wf._last_registered_workflow_id == "wf-123"

    def test_trigger_with_workflow_id(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="wf"))
        client = FakeWorkflowClient()
        result = wf.trigger(client, TriggerWorkflowInput(
            payload={"k": "v"}, workflow_id="wf-1",
        ))
        assert result["id"] == "wfr-1"

    def test_trigger_after_register(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="wf", project_id="p1"))
        client = FakeWorkflowClient()
        wf.register(client)
        result = wf.trigger(client, TriggerWorkflowInput(payload={}))
        assert result["id"] == "wfr-1"

    def test_trigger_requires_workflow_id(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="wf"))
        with pytest.raises(ValueError, match="requires workflow_id"):
            wf.trigger(FakeWorkflowClient(), TriggerWorkflowInput(payload={}))

    def test_trigger_all_options(self):
        wf = define_workflow(WorkflowOptions(name="WF", slug="wf"))
        client = FakeWorkflowClient()
        wf.trigger(client, TriggerWorkflowInput(
            payload={"k": "v"}, workflow_id="wf-1",
            idempotency_key="idem",
            priority=3,
            dry_run=True,
            metadata={"env": "test"},
            step_overrides={"s1": {"timeout": 60}},
        ))
        _, _, body = client.calls[0]
        assert body["idempotency_key"] == "idem"
        assert body["priority"] == 3
        assert body["dry_run"] is True
        assert body["step_overrides"] == {"s1": {"timeout": 60}}

    def test_optional_fields_in_body(self):
        wf = define_workflow(WorkflowOptions(
            name="WF", slug="wf", project_id="p1",
            description="desc", max_concurrent_runs=5, timeout_secs=600,
            cron="*/5 * * * *", timezone="UTC",
        ))
        body = wf.to_registration_body()
        assert body["description"] == "desc"
        assert body["max_concurrent_runs"] == 5
        assert body["timeout_secs"] == 600
        assert body["cron"] == "*/5 * * * *"

    def test_empty_steps(self):
        wf = define_workflow(WorkflowOptions(
            name="WF", slug="wf", project_id="p1", steps=[],
        ))
        body = wf.to_registration_body()
        assert body["steps"] == []

    def test_callbacks_stored(self):
        def run_fn(payload, ctx):
            return "result"

        wf = define_workflow(WorkflowOptions(name="WF", slug="wf", run=run_fn))
        assert wf.run is run_fn
