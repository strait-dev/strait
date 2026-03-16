"""Tests for authoring job definitions."""

from dataclasses import dataclass

import pytest

from strait.authoring._job import JobOptions, TriggerJobInput, define_job


class FakeJobClient:
    def __init__(self):
        self.calls = []

    def create_job(self, body):
        self.calls.append(("create", body))
        return {"id": "job-123"}

    def trigger_job(self, job_id, body):
        self.calls.append(("trigger", job_id, body))
        return {"id": "run-1"}

    def bulk_trigger_job(self, job_id, body):
        self.calls.append(("bulk_trigger", job_id, body))
        return {"count": len(body.get("items", []))}

    def get_run(self, run_id):
        return {"id": run_id, "status": "completed"}


class TestDefineJob:
    def test_creates_job_definition(self):
        job = define_job(JobOptions(name="My Job", slug="my-job"))
        assert job.kind == "job"
        assert job.slug == "my-job"

    def test_registration_body(self):
        job = define_job(JobOptions(
            name="My Job", slug="my-job",
            endpoint_url="https://example.com/run",
            project_id="proj-1",
            description="A test job",
            max_concurrency=5,
            timeout_secs=300,
        ))
        body = job.to_registration_body()
        assert body["project_id"] == "proj-1"
        assert body["name"] == "My Job"
        assert body["slug"] == "my-job"
        assert body["endpoint_url"] == "https://example.com/run"
        assert body["description"] == "A test job"
        assert body["max_concurrency"] == 5
        assert body["timeout_secs"] == 300

    def test_registration_body_project_id_override(self):
        job = define_job(JobOptions(name="J", slug="j", project_id="p1"))
        body = job.to_registration_body("p2")
        assert body["project_id"] == "p2"

    def test_registration_body_requires_project_id(self):
        job = define_job(JobOptions(name="J", slug="j"))
        with pytest.raises(ValueError, match="requires project_id"):
            job.to_registration_body()

    def test_register(self):
        job = define_job(JobOptions(name="J", slug="j", project_id="p1"))
        client = FakeJobClient()
        result = job.register(client)
        assert result["id"] == "job-123"
        assert job._last_registered_job_id == "job-123"

    def test_trigger_with_job_id(self):
        job = define_job(JobOptions(name="J", slug="j"))
        client = FakeJobClient()
        result = job.trigger(client, TriggerJobInput(
            payload={"key": "val"}, job_id="job-1",
        ))
        assert result["id"] == "run-1"
        assert client.calls[0][1] == "job-1"

    def test_trigger_after_register(self):
        job = define_job(JobOptions(name="J", slug="j", project_id="p1"))
        client = FakeJobClient()
        job.register(client)
        result = job.trigger(client, TriggerJobInput(payload={"k": "v"}))
        assert result["id"] == "run-1"

    def test_trigger_requires_job_id(self):
        job = define_job(JobOptions(name="J", slug="j"))
        with pytest.raises(ValueError, match="requires job_id"):
            job.trigger(FakeJobClient(), TriggerJobInput(payload={}))

    def test_trigger_with_all_options(self):
        job = define_job(JobOptions(name="J", slug="j"))
        client = FakeJobClient()
        job.trigger(client, TriggerJobInput(
            payload={"k": "v"},
            job_id="job-1",
            idempotency_key="idem-1",
            priority=5,
            dry_run=True,
            metadata={"env": "test"},
            scheduled_at="2025-01-01T00:00:00Z",
        ))
        _, _, body = client.calls[0]
        assert body["idempotency_key"] == "idem-1"
        assert body["priority"] == 5
        assert body["dry_run"] is True
        assert body["metadata"] == {"env": "test"}
        assert body["scheduled_at"] == "2025-01-01T00:00:00Z"

    def test_batch_trigger(self):
        job = define_job(JobOptions(name="J", slug="j", project_id="p1"))
        client = FakeJobClient()
        job.register(client)
        result = job.batch_trigger(client, [
            TriggerJobInput(payload={"i": 1}),
            TriggerJobInput(payload={"i": 2}),
        ])
        assert result["count"] == 2

    def test_batch_trigger_requires_job_id(self):
        job = define_job(JobOptions(name="J", slug="j"))
        with pytest.raises(ValueError, match="requires job_id"):
            job.batch_trigger(FakeJobClient(), [])

    def test_dataclass_payload_serialized(self):
        @dataclass
        class Payload:
            name: str
            count: int

        job = define_job(JobOptions(name="J", slug="j"))
        client = FakeJobClient()
        job.trigger(client, TriggerJobInput(
            payload=Payload(name="test", count=5), job_id="job-1",
        ))
        _, _, body = client.calls[0]
        assert body["payload"] == {"name": "test", "count": 5}

    def test_optional_fields_omitted_from_body(self):
        job = define_job(JobOptions(name="J", slug="j", project_id="p1"))
        body = job.to_registration_body()
        assert "description" not in body
        assert "cron" not in body
        assert "max_concurrency" not in body

    def test_tags_included(self):
        job = define_job(JobOptions(
            name="J", slug="j", project_id="p1",
            tags={"env": "prod", "team": "backend"},
        ))
        body = job.to_registration_body()
        assert body["tags"] == {"env": "prod", "team": "backend"}

    def test_retry_delays_included(self):
        job = define_job(JobOptions(
            name="J", slug="j", project_id="p1",
            retry_delays_secs=[1, 5, 30],
        ))
        body = job.to_registration_body()
        assert body["retry_delays_secs"] == [1, 5, 30]

    def test_callbacks_stored(self):
        def run_fn(payload, ctx):
            return "result"
        def on_success(payload, output, ctx):
            pass

        job = define_job(JobOptions(
            name="J", slug="j",
            run=run_fn, on_success=on_success,
        ))
        assert job.run is run_fn
        assert job.on_success is on_success
