# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::JobStep do
  it "creates a JobStep with ref and job_id" do
    step = Strait::Authoring.job_step("step-a", "job_123")
    expect(step.ref).to eq("step-a")
    expect(step.job_id).to eq("job_123")
    expect(step.type).to eq("job")
    expect(step.depends_on).to eq([])
  end

  it "to_api includes ref, type, and job_id" do
    step = Strait::Authoring.job_step("step-a", "job_123")
    api = step.to_api
    expect(api["ref"]).to eq("step-a")
    expect(api["type"]).to eq("job")
    expect(api["job_id"]).to eq("job_123")
    expect(api).not_to have_key("depends_on")
  end

  it "to_api includes depends_on when non-empty" do
    step = Strait::Authoring.job_step("step-b", "job_123", depends_on: ["step-a"])
    api = step.to_api
    expect(api["depends_on"]).to eq(["step-a"])
  end

  it "to_api includes all optional fields" do
    step = Strait::Authoring.job_step(
      "step-a", "job_123",
      condition: "{{ steps.prev.output.success }}",
      on_failure: "skip_dependents",
      payload: { "key" => "value" },
      retry_max_attempts: 3,
      retry_backoff: "exponential",
      retry_initial_delay_secs: 1,
      retry_max_delay_secs: 60,
      timeout_secs_override: 120,
      output_transform: "{{ output.result }}",
      concurrency_key: "my-key",
      resource_class: "large"
    )
    api = step.to_api
    expect(api["condition"]).to eq("{{ steps.prev.output.success }}")
    expect(api["on_failure"]).to eq("skip_dependents")
    expect(api["payload"]).to eq({ "key" => "value" })
    expect(api["retry_max_attempts"]).to eq(3)
    expect(api["retry_backoff"]).to eq("exponential")
    expect(api["retry_initial_delay_secs"]).to eq(1)
    expect(api["retry_max_delay_secs"]).to eq(60)
    expect(api["timeout_secs_override"]).to eq(120)
    expect(api["output_transform"]).to eq("{{ output.result }}")
    expect(api["concurrency_key"]).to eq("my-key")
    expect(api["resource_class"]).to eq("large")
  end
end

RSpec.describe Strait::Authoring::ApprovalStep do
  it "creates an ApprovalStep" do
    step = Strait::Authoring.approval_step("approve-1")
    expect(step.ref).to eq("approve-1")
    expect(step.type).to eq("approval")
    expect(step.depends_on).to eq([])
  end

  it "to_api includes ref and type" do
    step = Strait::Authoring.approval_step("approve-1")
    api = step.to_api
    expect(api["ref"]).to eq("approve-1")
    expect(api["type"]).to eq("approval")
  end

  it "to_api includes approval_timeout_secs and approvers" do
    step = Strait::Authoring.approval_step(
      "approve-1",
      approval_timeout_secs: 3600,
      approvers: ["user_1", "user_2"],
      depends_on: ["step-a"]
    )
    api = step.to_api
    expect(api["approval_timeout_secs"]).to eq(3600)
    expect(api["approvers"]).to eq(["user_1", "user_2"])
    expect(api["depends_on"]).to eq(["step-a"])
  end
end

RSpec.describe Strait::Authoring::SubWorkflowStep do
  it "creates a SubWorkflowStep" do
    step = Strait::Authoring.sub_workflow_step("sub-1", "wf_nested")
    expect(step.ref).to eq("sub-1")
    expect(step.sub_workflow_id).to eq("wf_nested")
    expect(step.type).to eq("sub_workflow")
  end

  it "to_api includes sub_workflow_id" do
    step = Strait::Authoring.sub_workflow_step("sub-1", "wf_nested")
    api = step.to_api
    expect(api["ref"]).to eq("sub-1")
    expect(api["type"]).to eq("sub_workflow")
    expect(api["sub_workflow_id"]).to eq("wf_nested")
  end

  it "to_api includes max_nesting_depth" do
    step = Strait::Authoring.sub_workflow_step("sub-1", "wf_nested", max_nesting_depth: 3)
    api = step.to_api
    expect(api["max_nesting_depth"]).to eq(3)
  end

  it "to_api includes payload" do
    step = Strait::Authoring.sub_workflow_step("sub-1", "wf_nested", payload: { "x" => 1 })
    api = step.to_api
    expect(api["payload"]).to eq({ "x" => 1 })
  end
end

RSpec.describe Strait::Authoring::WaitForEventStep do
  it "creates a WaitForEventStep" do
    step = Strait::Authoring.wait_for_event_step("wait-1", "payment.completed")
    expect(step.ref).to eq("wait-1")
    expect(step.event_key).to eq("payment.completed")
    expect(step.type).to eq("wait_for_event")
  end

  it "to_api includes event_key" do
    step = Strait::Authoring.wait_for_event_step("wait-1", "payment.completed")
    api = step.to_api
    expect(api["ref"]).to eq("wait-1")
    expect(api["type"]).to eq("wait_for_event")
    expect(api["event_key"]).to eq("payment.completed")
  end

  it "to_api includes timeout and notify_url" do
    step = Strait::Authoring.wait_for_event_step(
      "wait-1", "payment.completed",
      event_timeout_secs: 7200,
      event_notify_url: "https://notify.example.com"
    )
    api = step.to_api
    expect(api["event_timeout_secs"]).to eq(7200)
    expect(api["event_notify_url"]).to eq("https://notify.example.com")
  end
end

RSpec.describe Strait::Authoring::SleepStep do
  it "creates a SleepStep" do
    step = Strait::Authoring.sleep_step("sleep-1", 300)
    expect(step.ref).to eq("sleep-1")
    expect(step.sleep_duration_secs).to eq(300)
    expect(step.type).to eq("sleep")
  end

  it "to_api includes sleep_duration_secs" do
    step = Strait::Authoring.sleep_step("sleep-1", 300)
    api = step.to_api
    expect(api["ref"]).to eq("sleep-1")
    expect(api["type"]).to eq("sleep")
    expect(api["sleep_duration_secs"]).to eq(300)
  end

  it "to_api includes depends_on" do
    step = Strait::Authoring.sleep_step("sleep-1", 300, depends_on: ["step-a"])
    api = step.to_api
    expect(api["depends_on"]).to eq(["step-a"])
  end

  it "to_api includes condition and on_failure" do
    step = Strait::Authoring.sleep_step(
      "sleep-1", 300,
      condition: "{{ true }}",
      on_failure: "continue"
    )
    api = step.to_api
    expect(api["condition"]).to eq("{{ true }}")
    expect(api["on_failure"]).to eq("continue")
  end
end

RSpec.describe "Step type constants" do
  it "defines STEP_TYPE_JOB" do
    expect(Strait::Authoring::STEP_TYPE_JOB).to eq("job")
  end

  it "defines STEP_TYPE_APPROVAL" do
    expect(Strait::Authoring::STEP_TYPE_APPROVAL).to eq("approval")
  end

  it "defines STEP_TYPE_SUB_WORKFLOW" do
    expect(Strait::Authoring::STEP_TYPE_SUB_WORKFLOW).to eq("sub_workflow")
  end

  it "defines STEP_TYPE_WAIT_FOR_EVENT" do
    expect(Strait::Authoring::STEP_TYPE_WAIT_FOR_EVENT).to eq("wait_for_event")
  end

  it "defines STEP_TYPE_SLEEP" do
    expect(Strait::Authoring::STEP_TYPE_SLEEP).to eq("sleep")
  end
end

RSpec.describe "On failure constants" do
  it "defines ON_FAILURE_FAIL_WORKFLOW" do
    expect(Strait::Authoring::ON_FAILURE_FAIL_WORKFLOW).to eq("fail_workflow")
  end

  it "defines ON_FAILURE_SKIP_DEPENDENTS" do
    expect(Strait::Authoring::ON_FAILURE_SKIP_DEPENDENTS).to eq("skip_dependents")
  end

  it "defines ON_FAILURE_CONTINUE" do
    expect(Strait::Authoring::ON_FAILURE_CONTINUE).to eq("continue")
  end
end
