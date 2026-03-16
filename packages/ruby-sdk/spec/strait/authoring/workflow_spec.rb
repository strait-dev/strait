# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::WorkflowDefinition do
  describe ".define_workflow" do
    it "creates a WorkflowDefinition" do
      wf = Strait::Authoring.define_workflow(name: "My Workflow", slug: "my-wf")
      expect(wf).to be_a(Strait::Authoring::WorkflowDefinition)
      expect(wf.kind).to eq("workflow")
      expect(wf.slug).to eq("my-wf")
    end

    it "stores run callback" do
      handler = ->(_ctx) { "result" }
      wf = Strait::Authoring.define_workflow(name: "My Workflow", slug: "my-wf", run: handler)
      expect(wf.run).to eq(handler)
    end

    it "initializes last_registered_workflow_id to nil" do
      wf = Strait::Authoring.define_workflow(name: "My Workflow", slug: "my-wf")
      expect(wf.last_registered_workflow_id).to be_nil
    end
  end

  describe "#to_registration_body" do
    it "includes name and slug" do
      wf = Strait::Authoring.define_workflow(name: "My Workflow", slug: "my-wf")
      body = wf.to_registration_body
      expect(body["name"]).to eq("My Workflow")
      expect(body["slug"]).to eq("my-wf")
    end

    it "includes project_id" do
      wf = Strait::Authoring.define_workflow(name: "My Workflow", slug: "my-wf")
      body = wf.to_registration_body("proj_123")
      expect(body["project_id"]).to eq("proj_123")
    end

    it "includes steps and validates DAG" do
      steps = [
        Strait::Authoring.job_step("step-a", "job_1"),
        Strait::Authoring.job_step("step-b", "job_2", depends_on: ["step-a"])
      ]
      wf = Strait::Authoring.define_workflow(name: "WF", slug: "wf", steps: steps)
      body = wf.to_registration_body
      expect(body["steps"]).to be_an(Array)
      expect(body["steps"].length).to eq(2)
      expect(body["steps"][0]["ref"]).to eq("step-a")
      expect(body["steps"][1]["ref"]).to eq("step-b")
    end

    it "raises DagValidationError for invalid DAG" do
      steps = [
        Strait::Authoring.job_step("step-a", "job_1", depends_on: ["step-b"]),
        Strait::Authoring.job_step("step-b", "job_2", depends_on: ["step-a"])
      ]
      wf = Strait::Authoring.define_workflow(name: "WF", slug: "wf", steps: steps)
      expect { wf.to_registration_body }.to raise_error(Strait::DagValidationError)
    end

    it "includes optional workflow fields" do
      wf = Strait::Authoring.define_workflow(
        name: "My Workflow",
        slug: "my-wf",
        description: "desc",
        tags: ["t1"],
        environment_id: "env_1",
        max_concurrent_runs: 10,
        max_parallel_steps: 5,
        timeout_secs: 3600,
        max_attempts: 2,
        retry_strategy: "fixed",
        cron: "0 0 * * *",
        timezone: "UTC",
        webhook_url: "https://hooks.example.com",
        webhook_secret: "secret123"
      )
      body = wf.to_registration_body
      expect(body["description"]).to eq("desc")
      expect(body["tags"]).to eq(["t1"])
      expect(body["environment_id"]).to eq("env_1")
      expect(body["max_concurrent_runs"]).to eq(10)
      expect(body["max_parallel_steps"]).to eq(5)
      expect(body["timeout_secs"]).to eq(3600)
      expect(body["max_attempts"]).to eq(2)
      expect(body["retry_strategy"]).to eq("fixed")
      expect(body["cron"]).to eq("0 0 * * *")
      expect(body["timezone"]).to eq("UTC")
      expect(body["webhook_url"]).to eq("https://hooks.example.com")
      expect(body["webhook_secret"]).to eq("secret123")
    end

    it "omits nil fields" do
      wf = Strait::Authoring.define_workflow(name: "WF", slug: "wf")
      body = wf.to_registration_body
      expect(body.keys).to contain_exactly("name", "slug")
    end
  end
end

RSpec.describe Strait::Authoring::TriggerWorkflowInput do
  it "creates a TriggerWorkflowInput struct" do
    input = Strait::Authoring::TriggerWorkflowInput.new(
      workflow_id: "wf_1",
      payload: { "key" => "value" },
      idempotency_key: "idem_1"
    )
    expect(input.workflow_id).to eq("wf_1")
    expect(input.payload).to eq({ "key" => "value" })
    expect(input.idempotency_key).to eq("idem_1")
  end
end
