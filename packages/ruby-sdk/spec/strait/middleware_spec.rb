# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::Middleware do
  it "creates Middleware with all callbacks" do
    on_req = ->(ctx) { ctx }
    on_res = ->(ctx) { ctx }
    on_err = ->(ctx) { ctx }

    mw = Strait::Middleware.new(on_request: on_req, on_response: on_res, on_error: on_err)
    expect(mw.on_request).to eq(on_req)
    expect(mw.on_response).to eq(on_res)
    expect(mw.on_error).to eq(on_err)
  end

  it "creates Middleware with nil callbacks" do
    mw = Strait::Middleware.new
    expect(mw.on_request).to be_nil
    expect(mw.on_response).to be_nil
    expect(mw.on_error).to be_nil
  end
end

RSpec.describe Strait::MiddlewareRequestContext do
  it "has method, url, and headers members" do
    ctx = Strait::MiddlewareRequestContext.new(method: :get, url: "/v1/jobs", headers: { "Auth" => "Bearer x" })
    expect(ctx.method).to eq(:get)
    expect(ctx.url).to eq("/v1/jobs")
    expect(ctx.headers).to eq({ "Auth" => "Bearer x" })
  end
end

RSpec.describe Strait::MiddlewareResponseContext do
  it "has method, url, status, and duration_ms members" do
    ctx = Strait::MiddlewareResponseContext.new(method: :post, url: "/v1/jobs", status: 200, duration_ms: 150)
    expect(ctx.method).to eq(:post)
    expect(ctx.url).to eq("/v1/jobs")
    expect(ctx.status).to eq(200)
    expect(ctx.duration_ms).to eq(150)
  end
end

RSpec.describe Strait::MiddlewareErrorContext do
  it "has method, url, and error members" do
    error = RuntimeError.new("fail")
    ctx = Strait::MiddlewareErrorContext.new(method: :get, url: "/v1/health", error: error)
    expect(ctx.method).to eq(:get)
    expect(ctx.url).to eq("/v1/health")
    expect(ctx.error).to eq(error)
  end
end
