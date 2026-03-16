# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Composition::CostBudgetOptions do
  it "creates with keyword arguments" do
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 10_000)
    expect(opts.max_cost_microusd).to eq(10_000)
  end

  it "defaults warning_threshold to 0.8" do
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 10_000)
    expect(opts.warning_threshold).to eq(0.8)
  end

  it "allows custom warning_threshold" do
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 10_000, warning_threshold: 0.9)
    expect(opts.warning_threshold).to eq(0.9)
  end

  it "accepts on_warning callback" do
    cb = ->(current, max) { [current, max] }
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 10_000, on_warning: cb)
    expect(opts.on_warning).to eq(cb)
  end
end

RSpec.describe Strait::Composition::CostTracker do
  let(:options) { Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 1000) }
  let(:tracker) { Strait::Composition::CostTracker.new(options) }

  it "starts with zero current cost" do
    expect(tracker.current).to eq(0)
  end

  it "starts with full remaining budget" do
    expect(tracker.remaining).to eq(1000)
  end

  it "starts not exceeded" do
    expect(tracker.exceeded?).to be false
  end

  it "tracks cost with add" do
    tracker.add(300)
    expect(tracker.current).to eq(300)
    expect(tracker.remaining).to eq(700)
  end

  it "accumulates multiple adds" do
    tracker.add(100)
    tracker.add(200)
    tracker.add(300)
    expect(tracker.current).to eq(600)
    expect(tracker.remaining).to eq(400)
  end

  it "raises CostBudgetExceededError when budget exceeded" do
    expect {
      tracker.add(1000)
    }.to raise_error(Strait::CostBudgetExceededError) { |e|
      expect(e.current_cost_microusd).to eq(1000)
      expect(e.max_cost_microusd).to eq(1000)
    }
  end

  it "raises CostBudgetExceededError when budget overshot" do
    tracker.add(500)
    expect {
      tracker.add(600)
    }.to raise_error(Strait::CostBudgetExceededError) { |e|
      expect(e.current_cost_microusd).to eq(1100)
      expect(e.max_cost_microusd).to eq(1000)
    }
  end

  it "exceeded? returns true at budget" do
    begin
      tracker.add(1000)
    rescue Strait::CostBudgetExceededError
      # expected
    end
    expect(tracker.exceeded?).to be true
  end

  it "remaining returns 0 when over budget" do
    begin
      tracker.add(1500)
    rescue Strait::CostBudgetExceededError
      # expected
    end
    expect(tracker.remaining).to eq(0)
  end

  context "with warning callback" do
    let(:warnings) { [] }
    let(:options_with_warning) do
      Strait::Composition::CostBudgetOptions.new(
        max_cost_microusd: 1000,
        on_warning: ->(current, max) { warnings << [current, max] },
        warning_threshold: 0.8
      )
    end
    let(:tracker_with_warning) { Strait::Composition::CostTracker.new(options_with_warning) }

    it "does not fire warning below threshold" do
      tracker_with_warning.add(700)
      expect(warnings).to be_empty
    end

    it "fires warning at threshold" do
      tracker_with_warning.add(800)
      expect(warnings).to eq([[800, 1000]])
    end

    it "fires warning only once" do
      tracker_with_warning.add(800)
      tracker_with_warning.add(50)
      expect(warnings.length).to eq(1)
    end

    it "fires warning before budget exceeded error" do
      tracker_with_warning.add(900)
      expect(warnings.length).to eq(1)
      expect {
        tracker_with_warning.add(200)
      }.to raise_error(Strait::CostBudgetExceededError)
      expect(warnings.length).to eq(1)
    end
  end
end

RSpec.describe "Strait::Composition.create_cost_tracker" do
  it "returns a CostTracker" do
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 5000)
    tracker = Strait::Composition.create_cost_tracker(opts)
    expect(tracker).to be_a(Strait::Composition::CostTracker)
  end
end

RSpec.describe "Strait::Composition.with_cost_budget" do
  it "yields a tracker to the block" do
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 5000)
    Strait::Composition.with_cost_budget(opts) do |tracker|
      expect(tracker).to be_a(Strait::Composition::CostTracker)
      tracker.add(100)
      expect(tracker.current).to eq(100)
    end
  end

  it "returns the block result" do
    opts = Strait::Composition::CostBudgetOptions.new(max_cost_microusd: 5000)
    result = Strait::Composition.with_cost_budget(opts) do |tracker|
      tracker.add(100)
      "done"
    end
    expect(result).to eq("done")
  end
end

RSpec.describe Strait::CostBudgetExceededError do
  it "has message, current_cost_microusd, and max_cost_microusd" do
    err = Strait::CostBudgetExceededError.new(
      "budget exceeded",
      current_cost_microusd: 1500,
      max_cost_microusd: 1000
    )
    expect(err.message).to eq("budget exceeded")
    expect(err.current_cost_microusd).to eq(1500)
    expect(err.max_cost_microusd).to eq(1000)
  end

  it "inherits from Strait::Error" do
    expect(Strait::CostBudgetExceededError.ancestors).to include(Strait::Error)
  end
end
