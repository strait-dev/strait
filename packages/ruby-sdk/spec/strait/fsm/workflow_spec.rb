# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::FSM workflow run transitions" do
  describe "WORKFLOW_RUN_TRANSITIONS" do
    it "has the expected number of transitions" do
      expect(Strait::FSM::WORKFLOW_RUN_TRANSITIONS.size).to eq(9)
    end

    {
      ["pending", "START"] => "running",
      ["pending", "CANCEL"] => "canceled",
      ["running", "PAUSE"] => "paused",
      ["running", "COMPLETE"] => "completed",
      ["running", "FAIL"] => "failed",
      ["running", "TIMEOUT"] => "timed_out",
      ["running", "CANCEL"] => "canceled",
      ["paused", "RESUME"] => "running",
      ["paused", "CANCEL"] => "canceled",
    }.each do |(from, event), expected_to|
      it "transitions #{from} + #{event} => #{expected_to}" do
        result = Strait::FSM.transition_workflow_run(from, event)
        expect(result).to eq(expected_to)
      end
    end
  end

  describe ".can_transition_workflow_run?" do
    it "returns true for valid transitions" do
      expect(Strait::FSM.can_transition_workflow_run?("pending", "START")).to be true
      expect(Strait::FSM.can_transition_workflow_run?("running", "COMPLETE")).to be true
      expect(Strait::FSM.can_transition_workflow_run?("paused", "RESUME")).to be true
    end

    it "returns false for invalid transitions" do
      expect(Strait::FSM.can_transition_workflow_run?("completed", "START")).to be false
      expect(Strait::FSM.can_transition_workflow_run?("pending", "COMPLETE")).to be false
      expect(Strait::FSM.can_transition_workflow_run?("failed", "RESUME")).to be false
    end
  end

  describe ".transition_workflow_run" do
    it "raises ArgumentError for invalid transition" do
      expect {
        Strait::FSM.transition_workflow_run("completed", "START")
      }.to raise_error(ArgumentError, /invalid workflow run transition/)
    end
  end

  describe ".terminal_workflow_run_status?" do
    %w[completed failed timed_out canceled].each do |status|
      it "returns true for #{status}" do
        expect(Strait::FSM.terminal_workflow_run_status?(status)).to be true
      end
    end

    %w[pending running paused].each do |status|
      it "returns false for #{status}" do
        expect(Strait::FSM.terminal_workflow_run_status?(status)).to be false
      end
    end
  end
end
