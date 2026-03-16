"""Tests for authoring steps."""


from strait.authoring._steps import (
    OnFailureAction,
    ResourceClass,
    RetryBackoff,
    StepType,
    ai_step,
    approval_step,
    job_step,
    sleep_step,
    step_to_api,
    sub_workflow_step,
    wait_for_event_step,
)


class TestStepBuilders:
    def test_job_step(self):
        s = job_step("s1", "job-1")
        assert s.step_ref() == "s1"
        assert s.type() == StepType.JOB
        assert s.job_id == "job-1"

    def test_job_step_with_options(self):
        s = job_step(
            "s1", "job-1",
            depends_on=["s0"],
            on_failure=OnFailureAction.SKIP_DEPENDENTS,
            retry_max_attempts=3,
            retry_backoff=RetryBackoff.EXPONENTIAL,
            resource_class=ResourceClass.LARGE,
        )
        assert s.base_options().depends_on == ["s0"]
        assert s.base_options().on_failure == OnFailureAction.SKIP_DEPENDENTS
        assert s.base_options().retry_max_attempts == 3

    def test_approval_step(self):
        s = approval_step("approve-1", approval_timeout_secs=3600, approvers=["user@test.com"])
        assert s.type() == StepType.APPROVAL
        assert s.approval_timeout_secs == 3600
        assert s.approvers == ["user@test.com"]

    def test_sub_workflow_step(self):
        s = sub_workflow_step("sub-1", "wf-1", max_nesting_depth=3)
        assert s.type() == StepType.SUB_WORKFLOW
        assert s.sub_workflow_id == "wf-1"
        assert s.max_nesting_depth == 3

    def test_wait_for_event_step(self):
        s = wait_for_event_step("wait-1", "order.paid", event_timeout_secs=300)
        assert s.type() == StepType.WAIT_FOR_EVENT
        assert s.event_key == "order.paid"

    def test_sleep_step(self):
        s = sleep_step("sleep-1", 60)
        assert s.type() == StepType.SLEEP
        assert s.sleep_duration_secs == 60


class TestStepToApi:
    def test_job_step_minimal(self):
        s = job_step("s1", "job-1")
        out = step_to_api(s)
        assert out == {"step_ref": "s1", "type": "job", "job_id": "job-1"}

    def test_job_step_full(self):
        s = job_step(
            "s1", "job-1",
            depends_on=["s0"],
            on_failure=OnFailureAction.FAIL_WORKFLOW,
            payload={"key": "val"},
            retry_max_attempts=3,
            retry_backoff=RetryBackoff.EXPONENTIAL,
            retry_initial_delay_secs=5,
            retry_max_delay_secs=60,
            timeout_secs_override=300,
            output_transform="$.result",
            concurrency_key="tenant-1",
            resource_class=ResourceClass.MEDIUM,
            condition={"if": "true"},
        )
        out = step_to_api(s)
        assert out["depends_on"] == ["s0"]
        assert out["on_failure"] == "fail_workflow"
        assert out["payload"] == {"key": "val"}
        assert out["retry_max_attempts"] == 3
        assert out["retry_backoff"] == "exponential"
        assert out["retry_initial_delay_secs"] == 5
        assert out["retry_max_delay_secs"] == 60
        assert out["timeout_secs_override"] == 300
        assert out["output_transform"] == "$.result"
        assert out["concurrency_key"] == "tenant-1"
        assert out["resource_class"] == "medium"
        assert out["condition"] == {"if": "true"}

    def test_approval_step_api(self):
        s = approval_step("a1", approval_timeout_secs=600, approvers=["alice", "bob"])
        out = step_to_api(s)
        assert out["type"] == "approval"
        assert out["approval_timeout_secs"] == 600
        assert out["approvers"] == ["alice", "bob"]

    def test_sub_workflow_step_api(self):
        s = sub_workflow_step("sw1", "wf-2", max_nesting_depth=2)
        out = step_to_api(s)
        assert out["type"] == "sub_workflow"
        assert out["sub_workflow_id"] == "wf-2"
        assert out["max_nesting_depth"] == 2

    def test_wait_for_event_step_api(self):
        s = wait_for_event_step(
            "we1", "payment.done",
            event_timeout_secs=120,
            event_notify_url="https://hook.example.com",
        )
        out = step_to_api(s)
        assert out["type"] == "wait_for_event"
        assert out["event_key"] == "payment.done"
        assert out["event_timeout_secs"] == 120
        assert out["event_notify_url"] == "https://hook.example.com"

    def test_sleep_step_api(self):
        s = sleep_step("sl1", 30)
        out = step_to_api(s)
        assert out["type"] == "sleep"
        assert out["sleep_duration_secs"] == 30

    def test_optional_fields_omitted(self):
        s = job_step("s1", "job-1")
        out = step_to_api(s)
        assert "depends_on" not in out
        assert "on_failure" not in out
        assert "payload" not in out
        assert "retry_max_attempts" not in out

    def test_step_protocol_compliance(self):
        from strait.authoring._steps import Step
        steps = [
            job_step("a", "j1"),
            approval_step("b"),
            sub_workflow_step("c", "wf1"),
            wait_for_event_step("d", "key"),
            sleep_step("e", 10),
        ]
        for s in steps:
            assert isinstance(s, Step)


class TestAiStep:
    def test_basic_creation(self):
        s = ai_step("ai-1", "job-llm")
        assert s.step_ref() == "ai-1"
        assert s.type() == StepType.JOB
        assert s.job_id == "job-llm"

    def test_default_retry_max_attempts(self):
        s = ai_step("ai-1", "job-llm")
        assert s.base_options().retry_max_attempts == 5

    def test_default_retry_backoff(self):
        s = ai_step("ai-1", "job-llm")
        assert s.base_options().retry_backoff == RetryBackoff.EXPONENTIAL

    def test_default_retry_initial_delay(self):
        s = ai_step("ai-1", "job-llm")
        assert s.base_options().retry_initial_delay_secs == 2

    def test_default_retry_max_delay(self):
        s = ai_step("ai-1", "job-llm")
        assert s.base_options().retry_max_delay_secs == 120

    def test_default_timeout(self):
        s = ai_step("ai-1", "job-llm")
        assert s.base_options().timeout_secs_override == 600

    def test_default_resource_class(self):
        s = ai_step("ai-1", "job-llm")
        assert s.base_options().resource_class == ResourceClass.LARGE

    def test_custom_retry_attempts(self):
        s = ai_step("ai-1", "job-llm", retry_max_attempts=10)
        assert s.base_options().retry_max_attempts == 10

    def test_custom_timeout(self):
        s = ai_step("ai-1", "job-llm", timeout_secs_override=1200)
        assert s.base_options().timeout_secs_override == 1200

    def test_custom_resource_class(self):
        s = ai_step("ai-1", "job-llm", resource_class=ResourceClass.SMALL)
        assert s.base_options().resource_class == ResourceClass.SMALL

    def test_depends_on(self):
        s = ai_step("ai-1", "job-llm", depends_on=["prev-step"])
        assert s.base_options().depends_on == ["prev-step"]

    def test_payload(self):
        s = ai_step("ai-1", "job-llm", payload={"prompt": "hello"})
        assert s.base_options().payload == {"prompt": "hello"}

    def test_on_failure(self):
        s = ai_step("ai-1", "job-llm", on_failure=OnFailureAction.CONTINUE)
        assert s.base_options().on_failure == OnFailureAction.CONTINUE

    def test_api_serialization(self):
        s = ai_step("ai-1", "job-llm")
        out = step_to_api(s)
        assert out["step_ref"] == "ai-1"
        assert out["type"] == "job"
        assert out["job_id"] == "job-llm"
        assert out["retry_max_attempts"] == 5
        assert out["timeout_secs_override"] == 600

    def test_concurrency_key(self):
        s = ai_step("ai-1", "job-llm", concurrency_key="tenant-1")
        assert s.base_options().concurrency_key == "tenant-1"
