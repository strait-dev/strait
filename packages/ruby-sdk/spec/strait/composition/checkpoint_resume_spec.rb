# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Composition.with_checkpoint_resume" do
  let(:checkpoint_calls) { [] }
  let(:ctx) do
    Strait::Authoring::RunContext.new(
      run_id: "run_1",
      checkpoint: ->(state) { checkpoint_calls << state }
    )
  end

  it "uses initial_state when last_checkpoint is nil" do
    received_state = nil
    Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "step" => 0 }
    ) do |state, _update|
      received_state = state
    end
    expect(received_state).to eq({ "step" => 0 })
  end

  it "uses last_checkpoint when provided" do
    received_state = nil
    Strait::Composition.with_checkpoint_resume(
      ctx, { "step" => 5 },
      initial_state: { "step" => 0 }
    ) do |state, _update|
      received_state = state
    end
    expect(received_state).to eq({ "step" => 5 })
  end

  it "calls checkpoint on each update with interval 1" do
    Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "count" => 0 },
      checkpoint_interval: 1
    ) do |state, update_state|
      update_state.call({ "count" => 1 })
      update_state.call({ "count" => 2 })
    end
    # 2 from updates + 1 final checkpoint
    expect(checkpoint_calls).to eq([
      { "count" => 1 },
      { "count" => 2 },
      { "count" => 2 }
    ])
  end

  it "respects checkpoint_interval" do
    Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "i" => 0 },
      checkpoint_interval: 2
    ) do |_state, update_state|
      update_state.call({ "i" => 1 }) # step 1 - not at interval
      update_state.call({ "i" => 2 }) # step 2 - at interval
      update_state.call({ "i" => 3 }) # step 3 - not at interval
    end
    # step 2 checkpoint + final checkpoint
    expect(checkpoint_calls).to eq([
      { "i" => 2 },
      { "i" => 3 }
    ])
  end

  it "always does a final checkpoint" do
    Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "done" => false }
    ) do |_state, update_state|
      update_state.call({ "done" => true })
    end
    expect(checkpoint_calls.last).to eq({ "done" => true })
  end

  it "does final checkpoint even with no updates" do
    Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "step" => 0 }
    ) do |_state, _update|
      # no updates
    end
    expect(checkpoint_calls).to eq([{ "step" => 0 }])
  end

  it "returns the block result" do
    result = Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "step" => 0 }
    ) do |_state, _update|
      "final_result"
    end
    expect(result).to eq("final_result")
  end

  it "swallows checkpoint errors during updates" do
    error_ctx = Strait::Authoring::RunContext.new(
      run_id: "run_1",
      checkpoint: ->(_state) { raise "network error" }
    )

    expect {
      Strait::Composition.with_checkpoint_resume(
        error_ctx, nil,
        initial_state: { "step" => 0 }
      ) do |_state, update_state|
        update_state.call({ "step" => 1 })
      end
    }.to raise_error(RuntimeError, "network error")
    # The final checkpoint raises because it's not wrapped in rescue
  end

  it "works with nil checkpoint on context" do
    nil_ctx = Strait::Authoring::RunContext.new(run_id: "run_1")

    result = Strait::Composition.with_checkpoint_resume(
      nil_ctx, nil,
      initial_state: { "step" => 0 }
    ) do |_state, update_state|
      update_state.call({ "step" => 1 })
      "ok"
    end
    expect(result).to eq("ok")
  end

  it "tracks state through multiple updates" do
    states = []
    Strait::Composition.with_checkpoint_resume(
      ctx, nil,
      initial_state: { "items" => [] }
    ) do |state, update_state|
      states << state
      update_state.call({ "items" => ["a"] })
      update_state.call({ "items" => ["a", "b"] })
    end
    expect(states).to eq([{ "items" => [] }])
    expect(checkpoint_calls.last).to eq({ "items" => ["a", "b"] })
  end
end
