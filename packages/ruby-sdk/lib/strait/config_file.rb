# frozen_string_literal: true

require "json"

module Strait
  module ConfigFile
    FILENAME = "strait.json"

    # Load a Config by reading strait.json and layering env-var overrides on top.
    #
    # @param path       [String, nil] explicit path to strait.json
    # @param search_dir [String, nil] directory to search upward from (default: Dir.pwd)
    # @return [Config]
    def self.config_from_file(path: nil, search_dir: nil)
      data = read_config_file(path: path, search_dir: search_dir)

      sdk = data["sdk"] || {}

      base_url = ENV.fetch("STRAIT_BASE_URL", sdk["base_url"])
      raise ArgumentError, "base_url is required (set STRAIT_BASE_URL or sdk.base_url in strait.json)" if base_url.nil? || base_url.empty?

      api_key = ENV.fetch("STRAIT_API_KEY", nil)
      auth_type = ENV.fetch("STRAIT_AUTH_TYPE", sdk.fetch("auth_type", AuthType::API_KEY))
      timeout_ms = Integer(ENV.fetch("STRAIT_TIMEOUT_MS", sdk.fetch("timeout_ms", 30_000).to_s))

      auth = api_key ? AuthMode.new(type: auth_type, token: api_key) : nil

      Config.new(
        base_url: ConfigHelper.normalize_base_url(base_url),
        auth: auth,
        default_headers: {},
        timeout_ms: timeout_ms
      )
    end

    # Extract the project ID from strait.json.
    #
    # @param path       [String, nil] explicit path to strait.json
    # @param search_dir [String, nil] directory to search upward from
    # @return [String, nil]
    def self.project_id_from_file(path: nil, search_dir: nil)
      data = read_config_file(path: path, search_dir: search_dir)
      project = data["project"] || {}
      project["id"]
    end

    # Read and parse the strait.json file.
    #
    # @param path       [String, nil] explicit file path
    # @param search_dir [String, nil] directory to begin upward search
    # @return [Hash]
    def self.read_config_file(path: nil, search_dir: nil)
      file_path = path || find_config_file(search_dir || Dir.pwd)
      raise ArgumentError, "strait.json not found" if file_path.nil?

      JSON.parse(File.read(file_path))
    end

    # Walk up from search_dir looking for strait.json.
    #
    # @param dir [String]
    # @return [String, nil]
    def self.find_config_file(dir)
      current = File.expand_path(dir)
      loop do
        candidate = File.join(current, FILENAME)
        return candidate if File.exist?(candidate)

        parent = File.dirname(current)
        return nil if parent == current

        current = parent
      end
    end

    private_class_method :read_config_file, :find_config_file
  end
end
