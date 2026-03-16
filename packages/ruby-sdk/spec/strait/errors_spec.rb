# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Error do
  it "inherits from StandardError" do
    expect(Strait::Error.ancestors).to include(StandardError)
  end

  it "can be instantiated with a message" do
    err = Strait::Error.new("something went wrong")
    expect(err.message).to eq("something went wrong")
  end
end

RSpec.describe Strait::TransportError do
  it "has message and cause_error" do
    cause = RuntimeError.new("connection refused")
    err = Strait::TransportError.new("transport failure", cause_error: cause)
    expect(err.message).to eq("transport failure")
    expect(err.cause_error).to eq(cause)
  end

  it "defaults cause_error to nil" do
    err = Strait::TransportError.new("no cause")
    expect(err.cause_error).to be_nil
  end
end

RSpec.describe Strait::DecodeError do
  it "has message, body, and cause_error" do
    cause = JSON::ParserError.new("unexpected token")
    err = Strait::DecodeError.new("decode failed", body: "not json", cause_error: cause)
    expect(err.message).to eq("decode failed")
    expect(err.body).to eq("not json")
    expect(err.cause_error).to eq(cause)
  end

  it "defaults body and cause_error to nil" do
    err = Strait::DecodeError.new("decode failed")
    expect(err.body).to be_nil
    expect(err.cause_error).to be_nil
  end
end

RSpec.describe Strait::ValidationError do
  it "has message and issues" do
    err = Strait::ValidationError.new("invalid input", issues: ["field required", "too short"])
    expect(err.message).to eq("invalid input")
    expect(err.issues).to eq(["field required", "too short"])
  end

  it "defaults issues to empty array" do
    err = Strait::ValidationError.new("invalid")
    expect(err.issues).to eq([])
  end
end

RSpec.describe Strait::UnauthorizedError do
  it "has status, message, and body" do
    err = Strait::UnauthorizedError.new(401, "unauthorized", body: '{"error":"bad token"}')
    expect(err.status).to eq(401)
    expect(err.message).to eq("unauthorized")
    expect(err.body).to eq('{"error":"bad token"}')
  end
end

RSpec.describe Strait::NotFoundError do
  it "has status, message, and body" do
    err = Strait::NotFoundError.new(404, "not found", body: nil)
    expect(err.status).to eq(404)
    expect(err.message).to eq("not found")
    expect(err.body).to be_nil
  end
end

RSpec.describe Strait::ConflictError do
  it "has status, message, and body" do
    err = Strait::ConflictError.new(409, "conflict", body: '{"detail":"duplicate"}')
    expect(err.status).to eq(409)
    expect(err.message).to eq("conflict")
    expect(err.body).to eq('{"detail":"duplicate"}')
  end
end

RSpec.describe Strait::RateLimitedError do
  it "has status, message, and body" do
    err = Strait::RateLimitedError.new(429, "rate limited", body: '{"retry_after":60}')
    expect(err.status).to eq(429)
    expect(err.message).to eq("rate limited")
    expect(err.body).to eq('{"retry_after":60}')
  end
end

RSpec.describe Strait::ApiError do
  it "has status, message, and body" do
    err = Strait::ApiError.new(500, "internal error", body: "oops")
    expect(err.status).to eq(500)
    expect(err.message).to eq("internal error")
    expect(err.body).to eq("oops")
  end
end

RSpec.describe Strait::TimeoutError do
  it "has message, run_id, and elapsed_ms" do
    err = Strait::TimeoutError.new("timed out", run_id: "run_123", elapsed_ms: 30_000)
    expect(err.message).to eq("timed out")
    expect(err.run_id).to eq("run_123")
    expect(err.elapsed_ms).to eq(30_000)
  end

  it "defaults run_id and elapsed_ms to nil" do
    err = Strait::TimeoutError.new("timed out")
    expect(err.run_id).to be_nil
    expect(err.elapsed_ms).to be_nil
  end
end

RSpec.describe Strait::DagValidationError do
  it "has message, cycles, missing_refs, and duplicate_refs" do
    err = Strait::DagValidationError.new(
      "dag invalid",
      cycles: ["A", "B"],
      missing_refs: ["C"],
      duplicate_refs: ["D"]
    )
    expect(err.message).to eq("dag invalid")
    expect(err.cycles).to eq(["A", "B"])
    expect(err.missing_refs).to eq(["C"])
    expect(err.duplicate_refs).to eq(["D"])
  end

  it "defaults collections to empty arrays" do
    err = Strait::DagValidationError.new("dag invalid")
    expect(err.cycles).to eq([])
    expect(err.missing_refs).to eq([])
    expect(err.duplicate_refs).to eq([])
  end
end

RSpec.describe "Strait.map_http_error" do
  it "maps 401 to UnauthorizedError" do
    err = Strait.map_http_error(401, "unauthorized", "body")
    expect(err).to be_a(Strait::UnauthorizedError)
    expect(err.status).to eq(401)
    expect(err.body).to eq("body")
  end

  it "maps 404 to NotFoundError" do
    err = Strait.map_http_error(404, "not found")
    expect(err).to be_a(Strait::NotFoundError)
    expect(err.status).to eq(404)
  end

  it "maps 409 to ConflictError" do
    err = Strait.map_http_error(409, "conflict")
    expect(err).to be_a(Strait::ConflictError)
    expect(err.status).to eq(409)
  end

  it "maps 429 to RateLimitedError" do
    err = Strait.map_http_error(429, "rate limited")
    expect(err).to be_a(Strait::RateLimitedError)
    expect(err.status).to eq(429)
  end

  it "maps 500 to ApiError" do
    err = Strait.map_http_error(500, "server error")
    expect(err).to be_a(Strait::ApiError)
    expect(err.status).to eq(500)
  end

  it "maps 502 to ApiError" do
    err = Strait.map_http_error(502, "bad gateway")
    expect(err).to be_a(Strait::ApiError)
    expect(err.status).to eq(502)
  end

  it "maps 403 to ApiError (not UnauthorizedError)" do
    err = Strait.map_http_error(403, "forbidden")
    expect(err).to be_a(Strait::ApiError)
    expect(err.status).to eq(403)
  end

  it "uses provided message" do
    err = Strait.map_http_error(500, "custom message")
    expect(err.message).to eq("custom message")
  end
end
