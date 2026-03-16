# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::AgentRunContext do
  it "initializes with iteration 0" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1")
    expect(ctx.iteration).to eq(0)
  end

  it "initializes with accumulated_cost_microusd 0" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1")
    expect(ctx.accumulated_cost_microusd).to eq(0)
  end

  it "tracks cost with add_cost" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1")
    ctx.add_cost(100)
    ctx.add_cost(200)
    expect(ctx.accumulated_cost_microusd).to eq(300)
  end

  it "budget_exceeded? returns false when under budget" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1", max_cost_microusd: 1000)
    ctx.add_cost(500)
    expect(ctx.budget_exceeded?).to be false
  end

  it "budget_exceeded? returns true when at budget" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1", max_cost_microusd: 1000)
    ctx.add_cost(1000)
    expect(ctx.budget_exceeded?).to be true
  end

  it "budget_exceeded? returns true when over budget" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1", max_cost_microusd: 1000)
    ctx.add_cost(1500)
    expect(ctx.budget_exceeded?).to be true
  end

  it "defaults max_cost_microusd to infinity" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1")
    ctx.add_cost(999_999_999)
    expect(ctx.budget_exceeded?).to be false
  end

  it "inherits RunContext fields" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1", attempt: 3)
    expect(ctx.run_id).to eq("run_1")
    expect(ctx.attempt).to eq(3)
  end

  it "allows setting iteration" do
    ctx = Strait::Authoring::AgentRunContext.new(run_id: "run_1")
    ctx.iteration = 5
    expect(ctx.iteration).to eq(5)
  end
end

RSpec.describe Strait::Authoring::AgentOptions do
  it "creates with keyword arguments" do
    opts = Strait::Authoring::AgentOptions.new(
      name: "My Agent", slug: "my-agent", max_cost_microusd: 5000
    )
    expect(opts.name).to eq("My Agent")
    expect(opts.slug).to eq("my-agent")
    expect(opts.max_cost_microusd).to eq(5000)
  end

  it "defaults auto_checkpoint to true" do
    opts = Strait::Authoring::AgentOptions.new(name: "A", slug: "a")
    expect(opts.auto_checkpoint).to be true
  end

  it "allows setting auto_checkpoint to false" do
    opts = Strait::Authoring::AgentOptions.new(name: "A", slug: "a", auto_checkpoint: false)
    expect(opts.auto_checkpoint).to be false
  end
end

RSpec.describe "Strait::Authoring.define_agent" do
  it "returns a JobDefinition" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent")
    job = Strait::Authoring.define_agent(opts)
    expect(job).to be_a(Strait::Authoring::JobDefinition)
  end

  it "sets kind to job" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent")
    job = Strait::Authoring.define_agent(opts)
    expect(job.kind).to eq("job")
  end

  it "includes strait.kind=agent tag in registration body" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent", tags: { "env" => "prod" })
    job = Strait::Authoring.define_agent(opts)
    body = job.to_registration_body
    expect(body["tags"]).to include("strait.kind" => "agent", "env" => "prod")
  end

  it "defaults timeout_secs to 600" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent")
    job = Strait::Authoring.define_agent(opts)
    body = job.to_registration_body
    expect(body["timeout_secs"]).to eq(600)
  end

  it "defaults max_attempts to 5" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent")
    job = Strait::Authoring.define_agent(opts)
    body = job.to_registration_body
    expect(body["max_attempts"]).to eq(5)
  end

  it "defaults retry_strategy to exponential" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent")
    job = Strait::Authoring.define_agent(opts)
    body = job.to_registration_body
    expect(body["retry_strategy"]).to eq("exponential")
  end

  it "allows overriding timeout_secs" do
    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent", timeout_secs: 1200)
    job = Strait::Authoring.define_agent(opts)
    body = job.to_registration_body
    expect(body["timeout_secs"]).to eq(1200)
  end

  it "wraps run to create AgentRunContext with cost tracking" do
    usage_calls = []
    checkpoint_calls = []

    user_run = ->(payload, ctx) {
      ctx.report_usage.call(provider: "openai", model: "gpt-4", cost_microusd: 100)
      ctx.checkpoint.call({ "step" => 1 })
      { "result" => payload }
    }

    opts = Strait::Authoring::AgentOptions.new(
      name: "Agent", slug: "agent",
      max_cost_microusd: 5000,
      run: user_run
    )
    job = Strait::Authoring.define_agent(opts)

    # Create a mock RunContext to pass to the agent's run
    inner_ctx = Strait::Authoring::RunContext.new(
      run_id: "run_1",
      attempt: 1,
      report_usage: ->(**kwargs) { usage_calls << kwargs },
      checkpoint: ->(state) { checkpoint_calls << state }
    )

    job.run.call({ "input" => "hello" }, inner_ctx)

    expect(usage_calls.length).to eq(1)
    expect(checkpoint_calls.length).to eq(1)
    expect(checkpoint_calls.first).to eq({ "step" => 1 })
  end

  it "tracks iterations through checkpoint calls" do
    iterations = []

    user_run = ->(payload, ctx) {
      ctx.checkpoint.call({ "step" => 1 })
      iterations << ctx.iteration
      ctx.checkpoint.call({ "step" => 2 })
      iterations << ctx.iteration
    }

    opts = Strait::Authoring::AgentOptions.new(name: "Agent", slug: "agent", run: user_run)
    job = Strait::Authoring.define_agent(opts)

    inner_ctx = Strait::Authoring::RunContext.new(
      run_id: "run_1", attempt: 1,
      checkpoint: ->(state) { }
    )

    job.run.call({}, inner_ctx)
    expect(iterations).to eq([1, 2])
  end

  it "skips original checkpoint when auto_checkpoint is false" do
    checkpoint_calls = []

    user_run = ->(payload, ctx) {
      ctx.checkpoint.call({ "step" => 1 })
    }

    opts = Strait::Authoring::AgentOptions.new(
      name: "Agent", slug: "agent",
      auto_checkpoint: false,
      run: user_run
    )
    job = Strait::Authoring.define_agent(opts)

    inner_ctx = Strait::Authoring::RunContext.new(
      run_id: "run_1", attempt: 1,
      checkpoint: ->(state) { checkpoint_calls << state }
    )

    job.run.call({}, inner_ctx)
    expect(checkpoint_calls).to be_empty
  end

  it "stores on_success and on_failure callbacks" do
    on_success = ->(ctx) { "success" }
    on_failure = ->(ctx) { "failure" }
    opts = Strait::Authoring::AgentOptions.new(
      name: "Agent", slug: "agent",
      on_success: on_success, on_failure: on_failure
    )
    job = Strait::Authoring.define_agent(opts)
    expect(job.on_success).to eq(on_success)
    expect(job.on_failure).to eq(on_failure)
  end
end
