# frozen_string_literal: true

module Strait
  # Authentication type constants.
  module AuthType
    BEARER = "bearer"
    API_KEY = "apiKey"
    RUN_TOKEN = "runToken"
  end

  # Holds authentication mode (type + token).
  AuthMode = Struct.new(:type, :token, keyword_init: true)

  # SDK configuration.
  Config = Struct.new(:base_url, :auth, :default_headers, :timeout_ms, keyword_init: true)

  module ConfigHelper
    # Build a Config from environment variables.
    #
    # Required env vars:
    #   STRAIT_BASE_URL  - API base URL
    #   STRAIT_API_KEY   - API key (used as token)
    #
    # Optional env vars:
    #   STRAIT_AUTH_TYPE  - one of "bearer", "apiKey", "runToken" (default "apiKey")
    #   STRAIT_TIMEOUT_MS - request timeout in milliseconds (default 30000)
    def self.config_from_env
      base_url = ENV.fetch("STRAIT_BASE_URL") { raise ArgumentError, "STRAIT_BASE_URL environment variable is required" }
      api_key = ENV.fetch("STRAIT_API_KEY") { raise ArgumentError, "STRAIT_API_KEY environment variable is required" }
      auth_type = ENV.fetch("STRAIT_AUTH_TYPE", AuthType::API_KEY)
      timeout_ms = Integer(ENV.fetch("STRAIT_TIMEOUT_MS", "30000"))

      Config.new(
        base_url: normalize_base_url(base_url),
        auth: AuthMode.new(type: auth_type, token: api_key),
        default_headers: {},
        timeout_ms: timeout_ms
      )
    end

    # Strip trailing slashes from a base URL.
    def self.normalize_base_url(url)
      url.chomp("/")
    end

    # Return the Authorization header value for the given token.
    def self.get_authorization_header(token)
      "Bearer #{token}"
    end
  end
end
