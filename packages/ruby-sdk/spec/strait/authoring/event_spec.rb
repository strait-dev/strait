# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Authoring::EventDefinition do
  it "stores the event key" do
    event = Strait::Authoring::EventDefinition.new("payment.completed")
    expect(event.key).to eq("payment.completed")
  end

  it "parse returns input unchanged when no validator" do
    event = Strait::Authoring::EventDefinition.new("payment.completed")
    expect(event.parse({ "amount" => 100 })).to eq({ "amount" => 100 })
  end

  it "parse returns input unchanged for strings" do
    event = Strait::Authoring::EventDefinition.new("test.event")
    expect(event.parse("hello")).to eq("hello")
  end

  it "parse calls validator when provided" do
    validator = ->(input) {
      raise "amount required" unless input.is_a?(Hash) && input["amount"]
      input
    }
    event = Strait::Authoring::EventDefinition.new("payment.completed", validate: validator)
    expect(event.parse({ "amount" => 50 })).to eq({ "amount" => 50 })
  end

  it "parse raises when validator rejects input" do
    validator = ->(input) {
      raise "invalid" unless input.is_a?(Hash)
      input
    }
    event = Strait::Authoring::EventDefinition.new("payment.completed", validate: validator)
    expect { event.parse("not a hash") }.to raise_error(RuntimeError, "invalid")
  end

  it "parse applies transformation via validator" do
    transformer = ->(input) { input.merge("validated" => true) }
    event = Strait::Authoring::EventDefinition.new("order.created", validate: transformer)
    result = event.parse({ "id" => "ord_1" })
    expect(result).to eq({ "id" => "ord_1", "validated" => true })
  end
end

RSpec.describe "Strait::Authoring.define_event" do
  it "returns an EventDefinition" do
    event = Strait::Authoring.define_event("test.event")
    expect(event).to be_a(Strait::Authoring::EventDefinition)
  end

  it "sets the key" do
    event = Strait::Authoring.define_event("user.signup")
    expect(event.key).to eq("user.signup")
  end

  it "accepts a validate parameter" do
    validator = ->(input) { input.upcase }
    event = Strait::Authoring.define_event("test.event", validate: validator)
    expect(event.parse("hello")).to eq("HELLO")
  end

  it "works without validate" do
    event = Strait::Authoring.define_event("test.event")
    expect(event.parse(42)).to eq(42)
  end

  it "handles nil input" do
    event = Strait::Authoring.define_event("test.event")
    expect(event.parse(nil)).to be_nil
  end
end
