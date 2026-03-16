# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::FSM step run transitions" do
  describe "STEP_RUN_TRANSITIONS" do
    it "has the expected number of transitions" do
      expect(Strait::FSM::STEP_RUN_TRANSITIONS.size).to eq(9)
    end

    {
      ["pending", "WAIT"] => "waiting",
      ["pending", "START"] => "running",
      ["pending", "SKIP"] => "skipped",
      ["pending", "CANCEL"] => "canceled",
      ["waiting", "START"] => "running",
      ["waiting", "CANCEL"] => "canceled",
      ["running", "COMPLETE"] => "completed",
      ["running", "FAIL"] => "failed",
      ["running", "CANCEL"] => "canceled",
    }.each do |(from, event), expected_to|
      it "transitions #{from} + #{event} => #{expected_to}" do
        result = Strait::FSM.transition_step_run(from, event)
        expect(result).to eq(expected_to)
      end
    end
  end

  describe ".can_transition_step_run?" do
    it "returns true for valid transitions" do
      expect(Strait::FSM.can_transition_step_run?("pending", "START")).to be true
      expect(Strait::FSM.can_transition_step_run?("running", "COMPLETE")).to be true
      expect(Strait::FSM.can_transition_step_run?("waiting", "START")).to be true
    end

    it "returns false for invalid transitions" do
      expect(Strait::FSM.can_transition_step_run?("completed", "START")).to be false
      expect(Strait::FSM.can_transition_step_run?("pending", "COMPLETE")).to be false
      expect(Strait::FSM.can_transition_step_run?("skipped", "START")).to be false
    end
  end

  describe ".transition_step_run" do
    it "raises ArgumentError for invalid transition" do
      expect {
        Strait::FSM.transition_step_run("completed", "START")
      }.to raise_error(ArgumentError, /invalid step run transition/)
    end
  end

  describe ".terminal_step_run_status?" do
    %w[completed failed skipped canceled].each do |status|
      it "returns true for #{status}" do
        expect(Strait::FSM.terminal_step_run_status?(status)).to be true
      end
    end

    %w[pending waiting running].each do |status|
      it "returns false for #{status}" do
        expect(Strait::FSM.terminal_step_run_status?(status)).to be false
      end
    end
  end
end
