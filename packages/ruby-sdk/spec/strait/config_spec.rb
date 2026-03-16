# frozen_string_literal: true

require "spec_helper"

RSpec.describe Strait::AuthType do
  it "defines BEARER constant" do
    expect(Strait::AuthType::BEARER).to eq("bearer")
  end

  it "defines API_KEY constant" do
    expect(Strait::AuthType::API_KEY).to eq("apiKey")
  end

  it "defines RUN_TOKEN constant" do
    expect(Strait::AuthType::RUN_TOKEN).to eq("runToken")
  end
end

RSpec.describe Strait::AuthMode do
  it "creates an AuthMode with type and token" do
    auth = Strait::AuthMode.new(type: "bearer", token: "tok_123")
    expect(auth.type).to eq("bearer")
    expect(auth.token).to eq("tok_123")
  end
end

RSpec.describe Strait::Config do
  it "creates a Config with all fields" do
    config = Strait::Config.new(
      base_url: "https://api.example.com",
      auth: Strait::AuthMode.new(type: "apiKey", token: "key_123"),
      default_headers: { "X-Custom" => "value" },
      timeout_ms: 5000
    )
    expect(config.base_url).to eq("https://api.example.com")
    expect(config.auth.type).to eq("apiKey")
    expect(config.auth.token).to eq("key_123")
    expect(config.default_headers).to eq({ "X-Custom" => "value" })
    expect(config.timeout_ms).to eq(5000)
  end

  it "creates a Config with defaults as nil" do
    config = Strait::Config.new
    expect(config.base_url).to be_nil
    expect(config.auth).to be_nil
    expect(config.default_headers).to be_nil
    expect(config.timeout_ms).to be_nil
  end
end

RSpec.describe Strait::ConfigHelper do
  describe ".normalize_base_url" do
    it "strips a single trailing slash" do
      expect(Strait::ConfigHelper.normalize_base_url("https://api.example.com/")).to eq("https://api.example.com")
    end

    it "returns url unchanged when no trailing slash" do
      expect(Strait::ConfigHelper.normalize_base_url("https://api.example.com")).to eq("https://api.example.com")
    end

    it "strips only the last trailing slash" do
      expect(Strait::ConfigHelper.normalize_base_url("https://api.example.com//")).to eq("https://api.example.com/")
    end

    it "handles empty string" do
      expect(Strait::ConfigHelper.normalize_base_url("")).to eq("")
    end
  end

  describe ".get_authorization_header" do
    it "returns Bearer token format" do
      expect(Strait::ConfigHelper.get_authorization_header("tok_abc")).to eq("Bearer tok_abc")
    end
  end

  describe ".config_from_env" do
    let(:saved_env) { {} }

    before do
      %w[STRAIT_BASE_URL STRAIT_API_KEY STRAIT_AUTH_TYPE STRAIT_TIMEOUT_MS].each do |key|
        saved_env[key] = ENV[key]
      end
    end

    after do
      saved_env.each { |key, val| val.nil? ? ENV.delete(key) : ENV[key] = val }
    end

    it "builds config from all env vars" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com/"
      ENV["STRAIT_API_KEY"] = "key_123"
      ENV["STRAIT_AUTH_TYPE"] = "bearer"
      ENV["STRAIT_TIMEOUT_MS"] = "5000"

      config = Strait::ConfigHelper.config_from_env
      expect(config.base_url).to eq("https://api.example.com")
      expect(config.auth.type).to eq("bearer")
      expect(config.auth.token).to eq("key_123")
      expect(config.timeout_ms).to eq(5000)
    end

    it "raises when STRAIT_BASE_URL is missing" do
      ENV.delete("STRAIT_BASE_URL")
      ENV["STRAIT_API_KEY"] = "key_123"

      expect { Strait::ConfigHelper.config_from_env }.to raise_error(ArgumentError, /STRAIT_BASE_URL/)
    end

    it "raises when STRAIT_API_KEY is missing" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV.delete("STRAIT_API_KEY")

      expect { Strait::ConfigHelper.config_from_env }.to raise_error(ArgumentError, /STRAIT_API_KEY/)
    end

    it "defaults auth type to apiKey" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV["STRAIT_API_KEY"] = "key_123"
      ENV.delete("STRAIT_AUTH_TYPE")
      ENV.delete("STRAIT_TIMEOUT_MS")

      config = Strait::ConfigHelper.config_from_env
      expect(config.auth.type).to eq("apiKey")
    end

    it "uses custom auth type from env" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV["STRAIT_API_KEY"] = "key_123"
      ENV["STRAIT_AUTH_TYPE"] = "runToken"
      ENV.delete("STRAIT_TIMEOUT_MS")

      config = Strait::ConfigHelper.config_from_env
      expect(config.auth.type).to eq("runToken")
    end

    it "parses STRAIT_TIMEOUT_MS as integer" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV["STRAIT_API_KEY"] = "key_123"
      ENV["STRAIT_TIMEOUT_MS"] = "10000"

      config = Strait::ConfigHelper.config_from_env
      expect(config.timeout_ms).to eq(10_000)
    end

    it "raises on invalid STRAIT_TIMEOUT_MS" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV["STRAIT_API_KEY"] = "key_123"
      ENV["STRAIT_TIMEOUT_MS"] = "not_a_number"

      expect { Strait::ConfigHelper.config_from_env }.to raise_error(ArgumentError)
    end

    it "defaults timeout to 30000" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV["STRAIT_API_KEY"] = "key_123"
      ENV.delete("STRAIT_TIMEOUT_MS")

      config = Strait::ConfigHelper.config_from_env
      expect(config.timeout_ms).to eq(30_000)
    end

    it "sets default_headers to empty hash" do
      ENV["STRAIT_BASE_URL"] = "https://api.example.com"
      ENV["STRAIT_API_KEY"] = "key_123"

      config = Strait::ConfigHelper.config_from_env
      expect(config.default_headers).to eq({})
    end
  end
end
