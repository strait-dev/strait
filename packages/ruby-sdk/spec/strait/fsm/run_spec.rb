# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::FSM run transitions" do
  describe "RUN_TRANSITIONS" do
    it "has the expected number of transitions" do
      expect(Strait::FSM::RUN_TRANSITIONS.size).to eq(27)
    end

    {
      ["delayed", "ENQUEUE"] => "queued",
      ["delayed", "CANCEL"] => "canceled",
      ["delayed", "EXPIRE"] => "expired",
      ["queued", "DEQUEUE"] => "dequeued",
      ["queued", "CANCEL"] => "canceled",
      ["queued", "EXPIRE"] => "expired",
      ["dequeued", "EXECUTE"] => "executing",
      ["dequeued", "CANCEL"] => "canceled",
      ["dequeued", "REQUEUE"] => "queued",
      ["executing", "COMPLETE"] => "completed",
      ["executing", "FAIL"] => "failed",
      ["executing", "TIMEOUT"] => "timed_out",
      ["executing", "CRASH"] => "crashed",
      ["executing", "SYSTEM_FAIL"] => "system_failed",
      ["executing", "CANCEL"] => "canceled",
      ["executing", "WAIT"] => "waiting",
      ["waiting", "EXECUTE"] => "executing",
      ["waiting", "CANCEL"] => "canceled",
      ["waiting", "TIMEOUT"] => "timed_out",
      ["failed", "REQUEUE"] => "queued",
      ["failed", "DEAD_LETTER"] => "dead_letter",
      ["failed", "REPLAY"] => "replay_staged",
      ["timed_out", "REPLAY"] => "replay_staged",
      ["crashed", "REPLAY"] => "replay_staged",
      ["system_failed", "REPLAY"] => "replay_staged",
      ["dead_letter", "REPLAY"] => "replay_staged",
    }.each do |(from, event), expected_to|
      it "transitions #{from} + #{event} => #{expected_to}" do
        result = Strait::FSM.transition_run(from, event)
        expect(result).to eq(expected_to)
      end
    end

    it "transitions replay_staged + ENQUEUE => queued" do
      expect(Strait::FSM.transition_run("replay_staged", "ENQUEUE")).to eq("queued")
    end
  end

  describe ".can_transition_run?" do
    it "returns true for valid transitions" do
      expect(Strait::FSM.can_transition_run?("delayed", "ENQUEUE")).to be true
      expect(Strait::FSM.can_transition_run?("executing", "COMPLETE")).to be true
    end

    it "returns false for invalid transitions" do
      expect(Strait::FSM.can_transition_run?("completed", "EXECUTE")).to be false
      expect(Strait::FSM.can_transition_run?("delayed", "COMPLETE")).to be false
      expect(Strait::FSM.can_transition_run?("nonexistent", "ENQUEUE")).to be false
    end
  end

  describe ".transition_run" do
    it "raises ArgumentError for invalid transition" do
      expect {
        Strait::FSM.transition_run("completed", "EXECUTE")
      }.to raise_error(ArgumentError, /invalid run transition/)
    end
  end

  describe ".terminal_run_status?" do
    %w[completed failed timed_out crashed system_failed canceled expired].each do |status|
      it "returns true for #{status}" do
        expect(Strait::FSM.terminal_run_status?(status)).to be true
      end
    end

    %w[delayed queued dequeued executing waiting dead_letter replay_staged].each do |status|
      it "returns false for #{status}" do
        expect(Strait::FSM.terminal_run_status?(status)).to be false
      end
    end
  end
end
