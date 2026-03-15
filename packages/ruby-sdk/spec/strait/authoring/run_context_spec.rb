# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::RunContext do
  it "creates a RunContext with run_id and attempt" do
    ctx = Strait::Authoring::RunContext.new(run_id: "run_123", attempt: 1)
    expect(ctx.run_id).to eq("run_123")
    expect(ctx.attempt).to eq(1)
  end

  it "defaults fields to nil" do
    ctx = Strait::Authoring::RunContext.new
    expect(ctx.run_id).to be_nil
    expect(ctx.attempt).to be_nil
  end
end
