# frozen_string_literal: true

require "spec_helper"

RSpec.describe "Strait::Composition idempotency" do
  describe ".with_idempotency" do
    it "adds Idempotency-Key header" do
      headers = Strait::Composition.with_idempotency({ "Content-Type" => "application/json" }, "key_123")
      expect(headers["Idempotency-Key"]).to eq("key_123")
      expect(headers["Content-Type"]).to eq("application/json")
    end

    it "does not modify the original hash" do
      original = { "Content-Type" => "application/json" }
      result = Strait::Composition.with_idempotency(original, "key_123")
      expect(original).not_to have_key("Idempotency-Key")
      expect(result).to have_key("Idempotency-Key")
    end

    it "handles nil headers by creating a new hash" do
      headers = Strait::Composition.with_idempotency(nil, "key_456")
      expect(headers).to eq({ "Idempotency-Key" => "key_456" })
    end
  end

  describe ".with_idempotency_header" do
    it "adds custom header name" do
      headers = Strait::Composition.with_idempotency_header({}, "key_789", "X-Custom-Idempotency")
      expect(headers["X-Custom-Idempotency"]).to eq("key_789")
    end

    it "does not modify the original hash" do
      original = { "Accept" => "application/json" }
      result = Strait::Composition.with_idempotency_header(original, "key_789", "X-Idem")
      expect(original).not_to have_key("X-Idem")
      expect(result["X-Idem"]).to eq("key_789")
    end
  end
end
