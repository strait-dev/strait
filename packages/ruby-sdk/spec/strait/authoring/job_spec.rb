# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::JobDefinition do
  describe ".define_job" do
    it "creates a JobDefinition" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job")
      expect(job).to be_a(Strait::Authoring::JobDefinition)
      expect(job.kind).to eq("job")
      expect(job.slug).to eq("my-job")
    end

    it "stores run callback" do
      handler = ->(_ctx) { "result" }
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job", run: handler)
      expect(job.run).to eq(handler)
    end

    it "stores on_success callback" do
      handler = ->(_ctx) { "success" }
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job", on_success: handler)
      expect(job.on_success).to eq(handler)
    end

    it "stores on_failure callback" do
      handler = ->(_ctx) { "failure" }
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job", on_failure: handler)
      expect(job.on_failure).to eq(handler)
    end

    it "stores on_start callback" do
      handler = ->(_ctx) { "start" }
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job", on_start: handler)
      expect(job.on_start).to eq(handler)
    end

    it "initializes last_registered_job_id to nil" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job")
      expect(job.last_registered_job_id).to be_nil
    end
  end

  describe "#to_registration_body" do
    it "includes name and slug" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job")
      body = job.to_registration_body
      expect(body["name"]).to eq("My Job")
      expect(body["slug"]).to eq("my-job")
    end

    it "includes project_id from parameter" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job")
      body = job.to_registration_body("proj_123")
      expect(body["project_id"]).to eq("proj_123")
    end

    it "includes project_id from options" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job", project_id: "proj_456")
      body = job.to_registration_body
      expect(body["project_id"]).to eq("proj_456")
    end

    it "parameter project_id overrides options project_id" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job", project_id: "proj_456")
      body = job.to_registration_body("proj_override")
      expect(body["project_id"]).to eq("proj_override")
    end

    it "includes all optional fields when set" do
      job = Strait::Authoring.define_job(
        name: "My Job",
        slug: "my-job",
        endpoint_url: "https://worker.example.com/run",
        description: "A test job",
        group_id: "grp_1",
        tags: ["tag1", "tag2"],
        environment_id: "env_1",
        cron: "0 * * * *",
        timezone: "UTC",
        max_concurrency: 5,
        max_attempts: 3,
        retry_strategy: "exponential",
        timeout_secs: 60,
        webhook_url: "https://hooks.example.com"
      )
      body = job.to_registration_body
      expect(body["endpoint_url"]).to eq("https://worker.example.com/run")
      expect(body["description"]).to eq("A test job")
      expect(body["group_id"]).to eq("grp_1")
      expect(body["tags"]).to eq(["tag1", "tag2"])
      expect(body["environment_id"]).to eq("env_1")
      expect(body["cron"]).to eq("0 * * * *")
      expect(body["timezone"]).to eq("UTC")
      expect(body["max_concurrency"]).to eq(5)
      expect(body["max_attempts"]).to eq(3)
      expect(body["retry_strategy"]).to eq("exponential")
      expect(body["timeout_secs"]).to eq(60)
      expect(body["webhook_url"]).to eq("https://hooks.example.com")
    end

    it "omits nil fields" do
      job = Strait::Authoring.define_job(name: "My Job", slug: "my-job")
      body = job.to_registration_body
      expect(body.keys).to contain_exactly("name", "slug")
    end

    it "includes rate limit fields" do
      job = Strait::Authoring.define_job(
        name: "My Job",
        slug: "my-job",
        rate_limit_max: 100,
        rate_limit_window_secs: 60
      )
      body = job.to_registration_body
      expect(body["rate_limit_max"]).to eq(100)
      expect(body["rate_limit_window_secs"]).to eq(60)
    end

    it "includes retry delays" do
      job = Strait::Authoring.define_job(
        name: "My Job",
        slug: "my-job",
        retry_delays_secs: [1, 5, 30]
      )
      body = job.to_registration_body
      expect(body["retry_delays_secs"]).to eq([1, 5, 30])
    end
  end
end

RSpec.describe Strait::Authoring::TriggerJobInput do
  it "creates a TriggerJobInput struct" do
    input = Strait::Authoring::TriggerJobInput.new(
      job_id: "job_1",
      payload: { "key" => "value" },
      idempotency_key: "idem_1",
      priority: 1
    )
    expect(input.job_id).to eq("job_1")
    expect(input.payload).to eq({ "key" => "value" })
    expect(input.idempotency_key).to eq("idem_1")
    expect(input.priority).to eq(1)
  end
end
