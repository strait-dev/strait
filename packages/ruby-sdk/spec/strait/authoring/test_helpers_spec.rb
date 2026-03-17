# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Authoring.create_test_context" do
  let(:ctx_and_record) { Strait::Authoring.create_test_context }
  let(:ctx) { ctx_and_record[0] }
  let(:record) { ctx_and_record[1] }

  it "returns a RunContext and TestRunRecord" do
    expect(ctx).to be_a(Strait::Authoring::RunContext)
    expect(record).to be_a(Strait::Authoring::TestRunRecord)
  end

  it "sets default run_id and attempt" do
    expect(ctx.run_id).to eq("test-run")
    expect(ctx.attempt).to eq(1)
  end

  it "allows custom run_id and attempt" do
    ctx2, = Strait::Authoring.create_test_context(run_id: "custom-run", attempt: 3)
    expect(ctx2.run_id).to eq("custom-run")
    expect(ctx2.attempt).to eq(3)
  end

  it "records checkpoints" do
    ctx.checkpoint.call({ "step" => 1 })
    ctx.checkpoint.call({ "step" => 2 })
    expect(record.checkpoints).to eq([{ "step" => 1 }, { "step" => 2 }])
  end

  it "records progress updates" do
    ctx.report_progress.call(50, "halfway")
    ctx.report_progress.call(100)
    expect(record.progress_updates).to eq([
      { "percent" => 50, "message" => "halfway" },
      { "percent" => 100, "message" => nil }
    ])
  end

  it "records heartbeats" do
    ctx.heartbeat.call
    ctx.heartbeat.call
    ctx.heartbeat.call
    expect(record.heartbeats).to eq(3)
  end

  it "records usage reports" do
    ctx.report_usage.call(provider: "openai", model: "gpt-4", cost_microusd: 500)
    expect(record.usage_reports).to eq([{ provider: "openai", model: "gpt-4", cost_microusd: 500 }])
  end

  it "records tool calls" do
    ctx.log_tool_call.call(tool_name: "search", input: "query", output: "result")
    expect(record.tool_calls).to eq([{ tool_name: "search", input: "query", output: "result" }])
  end

  it "records outputs" do
    ctx.save_output.call("key1", "val1")
    expect(record.outputs).to eq([{ "key" => "key1", "value" => "val1" }])
  end

  it "manages state store" do
    ctx.state.set.call("foo", "bar")
    expect(ctx.state.get.call("foo")).to eq("bar")
    expect(record.state_store).to eq({ "foo" => "bar" })

    list = ctx.state.list.call
    expect(list).to eq([{ "key" => "foo", "value" => "bar" }])

    ctx.state.delete.call("foo")
    expect(ctx.state.get.call("foo")).to be_nil
    expect(record.state_store).to eq({})
  end

  it "records stream chunks" do
    ctx.stream_chunk.call("hello")
    ctx.stream_chunk.call("world", stream_id: "s1", done: true)
    expect(record.stream_chunks).to eq([
      { "chunk" => "hello" },
      { "chunk" => "world", "stream_id" => "s1", "done" => true }
    ])
  end

  it "records events and returns mock response" do
    result = ctx.wait_for_event.call("payment.done", timeout_secs: 300)
    expect(record.events).to eq([{ "event_key" => "payment.done", "timeout_secs" => 300, "notify_url" => nil }])
    expect(result["status"]).to eq("waiting")
    expect(result["trigger_id"]).to eq("trigger_test")
  end

  it "records spawns and returns mock response" do
    result = ctx.spawn.call(job_slug: "child", project_id: "proj_1", payload: { "x" => 1 })
    expect(record.spawns).to eq([{ "job_slug" => "child", "project_id" => "proj_1", "payload" => { "x" => 1 } }])
    expect(result["id"]).to eq("spawn_1")
  end

  it "records continuations" do
    result = ctx.continue_run.call({ "next" => true })
    expect(record.continuations).to eq([{ "payload" => { "next" => true } }])
    expect(result["id"]).to eq("continue_1")
  end

  it "records annotations" do
    ctx.annotate.call([{ "key" => "note", "value" => "hello" }])
    expect(record.annotations).to eq([[{ "key" => "note", "value" => "hello" }]])
  end

  it "records complete" do
    ctx.complete.call({ "answer" => 42 })
    expect(record.completed).to be true
    expect(record.result).to eq({ "answer" => 42 })
  end

  it "records complete without result" do
    ctx.complete.call
    expect(record.completed).to be true
    expect(record.result).to be_nil
  end

  it "records fail" do
    ctx.fail.call("something broke")
    expect(record.failed).to be true
    expect(record.fail_error).to eq("something broke")
  end

  it "initializes record with empty defaults" do
    expect(record.checkpoints).to eq([])
    expect(record.logs).to eq([])
    expect(record.usage_reports).to eq([])
    expect(record.tool_calls).to eq([])
    expect(record.outputs).to eq([])
    expect(record.progress_updates).to eq([])
    expect(record.state_store).to eq({})
    expect(record.stream_chunks).to eq([])
    expect(record.heartbeats).to eq(0)
    expect(record.spawns).to eq([])
    expect(record.events).to eq([])
    expect(record.annotations).to eq([])
    expect(record.continuations).to eq([])
    expect(record.completed).to be false
    expect(record.failed).to be false
    expect(record.fail_error).to be_nil
    expect(record.result).to be_nil
  end
end
