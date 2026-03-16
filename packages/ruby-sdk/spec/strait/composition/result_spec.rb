# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Composition::Result do
  describe ".ok" do
    it "creates an ok result" do
      result = Strait::Composition::Result.ok(42)
      expect(result.ok?).to be true
      expect(result.err?).to be false
      expect(result.value).to eq(42)
    end
  end

  describe ".err" do
    it "creates an err result" do
      error = RuntimeError.new("boom")
      result = Strait::Composition::Result.err(error)
      expect(result.ok?).to be false
      expect(result.err?).to be true
      expect(result.error).to eq(error)
    end
  end

  describe "#unwrap" do
    it "returns value on ok" do
      result = Strait::Composition::Result.ok("hello")
      expect(result.unwrap).to eq("hello")
    end

    it "raises on err" do
      error = RuntimeError.new("boom")
      result = Strait::Composition::Result.err(error)
      expect { result.unwrap }.to raise_error(RuntimeError, "boom")
    end
  end

  describe "#unwrap_err" do
    it "returns error on err" do
      error = RuntimeError.new("boom")
      result = Strait::Composition::Result.err(error)
      expect(result.unwrap_err).to eq(error)
    end

    it "raises on ok" do
      result = Strait::Composition::Result.ok("hello")
      expect { result.unwrap_err }.to raise_error(RuntimeError, /ok/)
    end
  end

  describe ".from_block" do
    it "captures success" do
      result = Strait::Composition::Result.from_block { 42 }
      expect(result.ok?).to be true
      expect(result.value).to eq(42)
    end

    it "captures error" do
      result = Strait::Composition::Result.from_block { raise "oops" }
      expect(result.err?).to be true
      expect(result.error.message).to eq("oops")
    end
  end

  describe "#match" do
    it "calls on_ok for ok result" do
      result = Strait::Composition::Result.ok(42)
      output = result.match(
        on_ok: ->(v) { v * 2 },
        on_err: ->(_e) { -1 }
      )
      expect(output).to eq(84)
    end

    it "calls on_err for err result" do
      error = RuntimeError.new("fail")
      result = Strait::Composition::Result.err(error)
      output = result.match(
        on_ok: ->(_v) { -1 },
        on_err: ->(e) { e.message }
      )
      expect(output).to eq("fail")
    end
  end
end
