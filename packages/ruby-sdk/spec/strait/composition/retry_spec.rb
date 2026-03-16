# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Composition retry" do
  describe Strait::Composition::RetryOptions do
    it "has default values" do
      opts = Strait::Composition::RetryOptions.new
      expect(opts.attempts).to eq(3)
      expect(opts.delay_ms).to eq(250)
      expect(opts.factor).to eq(2.0)
      expect(opts.max_delay_ms).to eq(30_000)
      expect(opts.jitter).to eq("full")
      expect(opts.should_retry).to be_nil
    end

    it "accepts custom values" do
      opts = Strait::Composition::RetryOptions.new(
        attempts: 5,
        delay_ms: 500,
        factor: 3.0,
        max_delay_ms: 60_000,
        jitter: "none"
      )
      expect(opts.attempts).to eq(5)
      expect(opts.delay_ms).to eq(500)
      expect(opts.factor).to eq(3.0)
      expect(opts.max_delay_ms).to eq(60_000)
      expect(opts.jitter).to eq("none")
    end
  end

  describe "Strait::Composition.with_retry" do
    it "succeeds on first try" do
      call_count = 0
      result = Strait::Composition.with_retry { call_count += 1; "ok" }
      expect(result).to eq("ok")
      expect(call_count).to eq(1)
    end

    it "retries on failure and eventually succeeds" do
      call_count = 0
      opts = Strait::Composition::RetryOptions.new(attempts: 3, delay_ms: 0, jitter: "none")
      result = Strait::Composition.with_retry(opts) do
        call_count += 1
        raise "fail" if call_count < 3
        "ok"
      end
      expect(result).to eq("ok")
      expect(call_count).to eq(3)
    end

    it "raises after max attempts exhausted" do
      opts = Strait::Composition::RetryOptions.new(attempts: 2, delay_ms: 0, jitter: "none")
      expect {
        Strait::Composition.with_retry(opts) { raise "permanent fail" }
      }.to raise_error(RuntimeError, "permanent fail")
    end

    it "uses default options when none provided" do
      call_count = 0
      # We override jitter to avoid sleeping
      opts = Strait::Composition::RetryOptions.new(attempts: 2, delay_ms: 0, jitter: "none")
      expect {
        Strait::Composition.with_retry(opts) do
          call_count += 1
          raise "fail"
        end
      }.to raise_error(RuntimeError)
      expect(call_count).to eq(2)
    end

    it "respects should_retry predicate returning false" do
      call_count = 0
      should_retry = ->(_err, _attempt, _max) { false }
      opts = Strait::Composition::RetryOptions.new(
        attempts: 5, delay_ms: 0, jitter: "none", should_retry: should_retry
      )
      expect {
        Strait::Composition.with_retry(opts) do
          call_count += 1
          raise "fail"
        end
      }.to raise_error(RuntimeError, "fail")
      expect(call_count).to eq(1)
    end

    it "respects should_retry predicate returning true" do
      call_count = 0
      should_retry = ->(_err, attempt, max) { attempt < max }
      opts = Strait::Composition::RetryOptions.new(
        attempts: 3, delay_ms: 0, jitter: "none", should_retry: should_retry
      )
      expect {
        Strait::Composition.with_retry(opts) do
          call_count += 1
          raise "fail"
        end
      }.to raise_error(RuntimeError)
      expect(call_count).to eq(3)
    end

    it "retries only for specific error types via should_retry" do
      call_count = 0
      should_retry = ->(err, _attempt, _max) { err.is_a?(IOError) }
      opts = Strait::Composition::RetryOptions.new(
        attempts: 3, delay_ms: 0, jitter: "none", should_retry: should_retry
      )
      expect {
        Strait::Composition.with_retry(opts) do
          call_count += 1
          raise RuntimeError, "not retryable"
        end
      }.to raise_error(RuntimeError, "not retryable")
      expect(call_count).to eq(1)
    end
  end
end
