# frozen_string_literal: true

require "spec_helper"
require "tmpdir"
require "json"

RSpec.describe Strait::ConfigFile do
  let(:saved_env) { {} }

  before do
    %w[STRAIT_BASE_URL STRAIT_API_KEY STRAIT_AUTH_TYPE STRAIT_TIMEOUT_MS].each do |key|
      saved_env[key] = ENV[key]
      ENV.delete(key)
    end
  end

  after do
    saved_env.each { |key, val| val.nil? ? ENV.delete(key) : ENV[key] = val }
  end

  describe ".config_from_file" do
    it "reads config from strait.json" do
      Dir.mktmpdir do |dir|
        config_data = {
          "sdk" => {
            "base_url" => "https://api.example.com/",
            "auth_type" => "bearer",
            "timeout_ms" => 5000
          }
        }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))
        ENV["STRAIT_API_KEY"] = "key_123"

        config = Strait::ConfigFile.config_from_file(path: File.join(dir, "strait.json"))
        expect(config.base_url).to eq("https://api.example.com")
        expect(config.auth.type).to eq("bearer")
        expect(config.auth.token).to eq("key_123")
        expect(config.timeout_ms).to eq(5000)
      end
    end

    it "reads config with custom path" do
      Dir.mktmpdir do |dir|
        custom_path = File.join(dir, "custom.json")
        config_data = { "sdk" => { "base_url" => "https://custom.example.com" } }
        File.write(custom_path, JSON.generate(config_data))
        ENV["STRAIT_API_KEY"] = "key_456"

        config = Strait::ConfigFile.config_from_file(path: custom_path)
        expect(config.base_url).to eq("https://custom.example.com")
      end
    end

    it "env vars override file values" do
      Dir.mktmpdir do |dir|
        config_data = {
          "sdk" => {
            "base_url" => "https://file.example.com",
            "auth_type" => "apiKey",
            "timeout_ms" => 1000
          }
        }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))
        ENV["STRAIT_BASE_URL"] = "https://env.example.com/"
        ENV["STRAIT_API_KEY"] = "env_key"
        ENV["STRAIT_AUTH_TYPE"] = "bearer"
        ENV["STRAIT_TIMEOUT_MS"] = "9999"

        config = Strait::ConfigFile.config_from_file(path: File.join(dir, "strait.json"))
        expect(config.base_url).to eq("https://env.example.com")
        expect(config.auth.type).to eq("bearer")
        expect(config.auth.token).to eq("env_key")
        expect(config.timeout_ms).to eq(9999)
      end
    end

    it "raises when file is missing" do
      expect {
        Strait::ConfigFile.config_from_file(path: "/nonexistent/strait.json")
      }.to raise_error(Errno::ENOENT)
    end

    it "raises when base_url is missing from file and env" do
      Dir.mktmpdir do |dir|
        config_data = { "sdk" => {} }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))

        expect {
          Strait::ConfigFile.config_from_file(path: File.join(dir, "strait.json"))
        }.to raise_error(ArgumentError, /base_url is required/)
      end
    end

    it "returns nil auth when no API key in env" do
      Dir.mktmpdir do |dir|
        config_data = { "sdk" => { "base_url" => "https://api.example.com" } }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))

        config = Strait::ConfigFile.config_from_file(path: File.join(dir, "strait.json"))
        expect(config.auth).to be_nil
      end
    end

    it "defaults timeout_ms to 30000" do
      Dir.mktmpdir do |dir|
        config_data = { "sdk" => { "base_url" => "https://api.example.com" } }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))

        config = Strait::ConfigFile.config_from_file(path: File.join(dir, "strait.json"))
        expect(config.timeout_ms).to eq(30_000)
      end
    end
  end

  describe ".project_id_from_file" do
    it "extracts project.id from strait.json" do
      Dir.mktmpdir do |dir|
        config_data = { "project" => { "id" => "proj_abc123" } }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))

        result = Strait::ConfigFile.project_id_from_file(path: File.join(dir, "strait.json"))
        expect(result).to eq("proj_abc123")
      end
    end

    it "returns nil when project section is missing" do
      Dir.mktmpdir do |dir|
        config_data = { "sdk" => { "base_url" => "https://api.example.com" } }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))

        result = Strait::ConfigFile.project_id_from_file(path: File.join(dir, "strait.json"))
        expect(result).to be_nil
      end
    end

    it "returns nil when project.id is missing" do
      Dir.mktmpdir do |dir|
        config_data = { "project" => { "name" => "my-project" } }
        File.write(File.join(dir, "strait.json"), JSON.generate(config_data))

        result = Strait::ConfigFile.project_id_from_file(path: File.join(dir, "strait.json"))
        expect(result).to be_nil
      end
    end
  end
end
