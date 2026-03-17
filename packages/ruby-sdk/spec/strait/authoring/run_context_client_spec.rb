# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::RunContextFactory do
  let(:calls) { [] }
  let(:mock_client) do
    c = Object.new
    local_calls = calls

    # Define all methods the factory wires to
    c.define_singleton_method(:checkpoint_run) { |run_id, body| local_calls << [:checkpoint_run, run_id, body]; {} }
    c.define_singleton_method(:progress_run) { |run_id, body| local_calls << [:progress_run, run_id, body]; {} }
    c.define_singleton_method(:heartbeat_run) { |run_id| local_calls << [:heartbeat_run, run_id]; {} }
    c.define_singleton_method(:usage_run) { |run_id, body| local_calls << [:usage_run, run_id, body]; {} }
    c.define_singleton_method(:tool_call_run) { |run_id, body| local_calls << [:tool_call_run, run_id, body]; {} }
    c.define_singleton_method(:output_run) { |run_id, body| local_calls << [:output_run, run_id, body]; {} }
    c.define_singleton_method(:get_state) { |run_id, key| local_calls << [:get_state, run_id, key]; { "value" => "v" } }
    c.define_singleton_method(:set_state) { |run_id, body| local_calls << [:set_state, run_id, body]; {} }
    c.define_singleton_method(:delete_state) { |run_id, key| local_calls << [:delete_state, run_id, key]; {} }
    c.define_singleton_method(:list_state) { |run_id| local_calls << [:list_state, run_id]; [] }
    c.define_singleton_method(:stream_run) { |run_id, body| local_calls << [:stream_run, run_id, body]; {} }
    c.define_singleton_method(:wait_for_event_run) { |run_id, body| local_calls << [:wait_for_event_run, run_id, body]; {} }
    c.define_singleton_method(:spawn_run) { |run_id, body| local_calls << [:spawn_run, run_id, body]; {} }
    c.define_singleton_method(:continue_run) { |run_id, body| local_calls << [:continue_run, run_id, body]; {} }
    c.define_singleton_method(:annotate_run) { |run_id, body| local_calls << [:annotate_run, run_id, body]; {} }
    c.define_singleton_method(:complete_run) { |run_id, body| local_calls << [:complete_run, run_id, body]; {} }
    c.define_singleton_method(:fail_run) { |run_id, body| local_calls << [:fail_run, run_id, body]; {} }
    c
  end

  let(:ctx) { described_class.create_run_context(mock_client, "run_42", attempt: 2) }

  it "sets run_id and attempt" do
    expect(ctx.run_id).to eq("run_42")
    expect(ctx.attempt).to eq(2)
  end

  it "wires checkpoint to checkpoint_run" do
    ctx.checkpoint.call({ "step" => 1 })
    expect(calls.last).to eq([:checkpoint_run, "run_42", { "state" => { "step" => 1 }, "source" => "sdk" }])
  end

  it "wires report_progress with message" do
    ctx.report_progress.call(50, "halfway")
    expect(calls.last).to eq([:progress_run, "run_42", { "percent" => 50, "message" => "halfway" }])
  end

  it "wires report_progress without message" do
    ctx.report_progress.call(100)
    expect(calls.last).to eq([:progress_run, "run_42", { "percent" => 100 }])
  end

  it "wires heartbeat" do
    ctx.heartbeat.call
    expect(calls.last).to eq([:heartbeat_run, "run_42"])
  end

  it "wires report_usage" do
    ctx.report_usage.call(provider: "openai", model: "gpt-4", prompt_tokens: 10, cost_microusd: 500)
    expect(calls.last).to eq([
      :usage_run, "run_42",
      { "provider" => "openai", "model" => "gpt-4", "prompt_tokens" => 10, "cost_microusd" => 500 }
    ])
  end

  it "wires log_tool_call" do
    ctx.log_tool_call.call(tool_name: "search", input: "q", output: "r", duration_ms: 100, status: "ok")
    expect(calls.last).to eq([
      :tool_call_run, "run_42",
      { "tool_name" => "search", "input" => "q", "output" => "r", "duration_ms" => 100, "status" => "ok" }
    ])
  end

  it "wires save_output" do
    ctx.save_output.call("key1", "val1")
    expect(calls.last).to eq([:output_run, "run_42", { "key" => "key1", "value" => "val1" }])
  end

  it "wires save_output with schema" do
    ctx.save_output.call("key1", "val1", { "type" => "string" })
    expect(calls.last).to eq([:output_run, "run_42", { "key" => "key1", "value" => "val1", "schema" => { "type" => "string" } }])
  end

  it "wires state.get" do
    result = ctx.state.get.call("mykey")
    expect(calls.last).to eq([:get_state, "run_42", "mykey"])
    expect(result).to eq({ "value" => "v" })
  end

  it "wires state.set" do
    ctx.state.set.call("mykey", "myval")
    expect(calls.last).to eq([:set_state, "run_42", { "key" => "mykey", "value" => "myval" }])
  end

  it "wires state.delete" do
    ctx.state.delete.call("mykey")
    expect(calls.last).to eq([:delete_state, "run_42", "mykey"])
  end

  it "wires state.list" do
    ctx.state.list.call
    expect(calls.last).to eq([:list_state, "run_42"])
  end

  it "wires stream_chunk" do
    ctx.stream_chunk.call("hello", stream_id: "s1", done: false)
    expect(calls.last).to eq([:stream_run, "run_42", { "chunk" => "hello", "stream_id" => "s1", "done" => false }])
  end

  it "wires wait_for_event" do
    ctx.wait_for_event.call("payment.done", timeout_secs: 300)
    expect(calls.last).to eq([:wait_for_event_run, "run_42", { "event_key" => "payment.done", "timeout_secs" => 300 }])
  end

  it "wires spawn" do
    ctx.spawn.call(job_slug: "child-job", project_id: "proj_1", payload: { "x" => 1 })
    expect(calls.last).to eq([:spawn_run, "run_42", { "job_slug" => "child-job", "project_id" => "proj_1", "payload" => { "x" => 1 } }])
  end

  it "wires continue_run" do
    ctx.continue_run.call({ "next" => true })
    expect(calls.last).to eq([:continue_run, "run_42", { "payload" => { "next" => true } }])
  end

  it "wires continue_run without payload" do
    ctx.continue_run.call
    expect(calls.last).to eq([:continue_run, "run_42", nil])
  end

  it "wires annotate" do
    ctx.annotate.call([{ "key" => "note", "value" => "hello" }])
    expect(calls.last).to eq([:annotate_run, "run_42", { "annotations" => [{ "key" => "note", "value" => "hello" }] }])
  end

  it "wires complete" do
    ctx.complete.call({ "answer" => 42 })
    expect(calls.last).to eq([:complete_run, "run_42", { "result" => { "answer" => 42 } }])
  end

  it "wires complete without result" do
    ctx.complete.call
    expect(calls.last).to eq([:complete_run, "run_42", nil])
  end

  it "wires fail" do
    ctx.fail.call("something broke")
    expect(calls.last).to eq([:fail_run, "run_42", { "error" => "something broke" }])
  end
end
