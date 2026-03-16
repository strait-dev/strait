# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::RunContext do
  it "creates a RunContext with run_id and attempt" do
    ctx = Strait::Authoring::RunContext.new(run_id: "run_123", attempt: 2)
    expect(ctx.run_id).to eq("run_123")
    expect(ctx.attempt).to eq(2)
  end

  it "defaults attempt to 1" do
    ctx = Strait::Authoring::RunContext.new(run_id: "run_1")
    expect(ctx.attempt).to eq(1)
  end

  it "defaults all optional fields to nil" do
    ctx = Strait::Authoring::RunContext.new(run_id: "run_1")
    expect(ctx.checkpoint).to be_nil
    expect(ctx.report_progress).to be_nil
    expect(ctx.heartbeat).to be_nil
    expect(ctx.report_usage).to be_nil
    expect(ctx.log_tool_call).to be_nil
    expect(ctx.save_output).to be_nil
    expect(ctx.state).to be_nil
    expect(ctx.stream_chunk).to be_nil
    expect(ctx.wait_for_event).to be_nil
    expect(ctx.spawn).to be_nil
    expect(ctx.continue_run).to be_nil
    expect(ctx.annotate).to be_nil
    expect(ctx.complete).to be_nil
    expect(ctx.fail).to be_nil
  end

  it "accepts all fields via keyword arguments" do
    checkpoint_fn = ->(s) { s }
    progress_fn = ->(p, m) { [p, m] }
    heartbeat_fn = -> { true }
    usage_fn = ->(**kw) { kw }
    tool_fn = ->(**kw) { kw }
    output_fn = ->(k, v) { [k, v] }
    state = Strait::Authoring::RunContextState.new(
      get: ->(k) { k }, set: ->(k, v) { [k, v] },
      delete: ->(k) { k }, list: -> { [] }
    )
    stream_fn = ->(c) { c }
    wait_fn = ->(e) { e }
    spawn_fn = ->(**kw) { kw }
    continue_fn = ->(p) { p }
    annotate_fn = ->(a) { a }
    complete_fn = ->(r) { r }
    fail_fn = ->(e) { e }

    ctx = Strait::Authoring::RunContext.new(
      run_id: "run_1", attempt: 3,
      checkpoint: checkpoint_fn, report_progress: progress_fn,
      heartbeat: heartbeat_fn, report_usage: usage_fn,
      log_tool_call: tool_fn, save_output: output_fn,
      state: state, stream_chunk: stream_fn,
      wait_for_event: wait_fn, spawn: spawn_fn,
      continue_run: continue_fn, annotate: annotate_fn,
      complete: complete_fn, fail: fail_fn
    )

    expect(ctx.checkpoint).to eq(checkpoint_fn)
    expect(ctx.report_progress).to eq(progress_fn)
    expect(ctx.heartbeat).to eq(heartbeat_fn)
    expect(ctx.report_usage).to eq(usage_fn)
    expect(ctx.log_tool_call).to eq(tool_fn)
    expect(ctx.save_output).to eq(output_fn)
    expect(ctx.state).to eq(state)
    expect(ctx.stream_chunk).to eq(stream_fn)
    expect(ctx.wait_for_event).to eq(wait_fn)
    expect(ctx.spawn).to eq(spawn_fn)
    expect(ctx.continue_run).to eq(continue_fn)
    expect(ctx.annotate).to eq(annotate_fn)
    expect(ctx.complete).to eq(complete_fn)
    expect(ctx.fail).to eq(fail_fn)
  end

  it "allows setting fields after initialization" do
    ctx = Strait::Authoring::RunContext.new(run_id: "run_1")
    handler = ->(s) { s }
    ctx.checkpoint = handler
    expect(ctx.checkpoint).to eq(handler)
  end

  it "allows calling lambda fields" do
    called = false
    ctx = Strait::Authoring::RunContext.new(
      run_id: "run_1",
      heartbeat: -> { called = true }
    )
    ctx.heartbeat.call
    expect(called).to be true
  end
end

RSpec.describe Strait::Authoring::RunContextState do
  it "creates with get, set, delete, list" do
    get_fn = ->(k) { k }
    set_fn = ->(k, v) { [k, v] }
    delete_fn = ->(k) { k }
    list_fn = -> { [] }

    state = Strait::Authoring::RunContextState.new(
      get: get_fn, set: set_fn, delete: delete_fn, list: list_fn
    )

    expect(state.get).to eq(get_fn)
    expect(state.set).to eq(set_fn)
    expect(state.delete).to eq(delete_fn)
    expect(state.list).to eq(list_fn)
  end

  it "can call lambdas to get/set/delete/list" do
    store = {}
    state = Strait::Authoring::RunContextState.new(
      get: ->(k) { store[k] },
      set: ->(k, v) { store[k] = v },
      delete: ->(k) { store.delete(k) },
      list: -> { store.keys }
    )

    state.set.call("foo", "bar")
    expect(state.get.call("foo")).to eq("bar")
    expect(state.list.call).to eq(["foo"])
    state.delete.call("foo")
    expect(state.get.call("foo")).to be_nil
    expect(state.list.call).to eq([])
  end
end
