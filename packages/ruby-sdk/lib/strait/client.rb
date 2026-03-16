# frozen_string_literal: true

require "faraday"
require "json"

module Strait
  # Main API client for the Strait platform.
  #
  # Provides access to all 19 operation services and handles authentication,
  # HTTP transport, middleware hooks, and error mapping.
  class Client
    attr_reader :base_url, :auth, :default_headers, :timeout_ms

    # @param base_url        [String]           API base URL
    # @param bearer_token    [String, nil]      Bearer token (shorthand for auth)
    # @param api_key         [String, nil]      API key (shorthand for auth)
    # @param run_token       [String, nil]      Run token (shorthand for auth)
    # @param auth            [AuthMode, nil]    Explicit auth mode
    # @param default_headers [Hash, nil]        Headers sent with every request
    # @param timeout_ms      [Integer]          Request timeout in milliseconds
    # @param http_client     [Faraday::Connection, nil] Custom Faraday connection
    # @param middleware       [Array<Middleware>, nil]    Middleware hooks
    def initialize(
      base_url:,
      bearer_token: nil,
      api_key: nil,
      run_token: nil,
      auth: nil,
      default_headers: nil,
      timeout_ms: 30_000,
      http_client: nil,
      middleware: nil
    )
      @base_url = ConfigHelper.normalize_base_url(base_url)
      @auth = resolve_auth(auth, bearer_token, api_key, run_token)
      @default_headers = default_headers || {}
      @timeout_ms = timeout_ms
      @middleware = middleware || []
      @http_client = http_client || build_http_client
    end

    # Build a Client from environment variables.
    #
    # @param overrides [Hash] keyword arguments forwarded to initialize
    # @return [Client]
    def self.from_env(**overrides)
      config = ConfigHelper.config_from_env
      new(
        base_url: overrides.fetch(:base_url, config.base_url),
        auth: overrides.fetch(:auth, config.auth),
        default_headers: overrides.fetch(:default_headers, config.default_headers),
        timeout_ms: overrides.fetch(:timeout_ms, config.timeout_ms),
        **overrides.except(:base_url, :auth, :default_headers, :timeout_ms)
      )
    end

    # Build a Client from a strait.json config file.
    #
    # @param path       [String, nil] explicit path to strait.json
    # @param search_dir [String, nil] directory to search upward from
    # @param overrides  [Hash]        keyword arguments forwarded to initialize
    # @return [Client]
    def self.from_file(path: nil, search_dir: nil, **overrides)
      config = ConfigFile.config_from_file(path: path, search_dir: search_dir)
      new(
        base_url: overrides.fetch(:base_url, config.base_url),
        auth: overrides.fetch(:auth, config.auth),
        default_headers: overrides.fetch(:default_headers, config.default_headers),
        timeout_ms: overrides.fetch(:timeout_ms, config.timeout_ms),
        **overrides.except(:base_url, :auth, :default_headers, :timeout_ms)
      )
    end

    # Execute an HTTP request against the Strait API.
    #
    # @param method  [Symbol]     HTTP method (:get, :post, :patch, :put, :delete)
    # @param path    [String]     URL path (appended to base_url)
    # @param query   [Hash, nil]  query-string parameters
    # @param headers [Hash, nil]  additional request headers
    # @param body    [Hash, nil]  request body (will be JSON-encoded)
    # @return [Hash, nil] parsed JSON response body
    def do_request(method, path, query: nil, headers: nil, body: nil)
      url = "#{@base_url}#{path}"

      request_headers = build_headers(headers)

      # Invoke on_request middleware hooks.
      @middleware.each do |mw|
        mw.on_request&.call(MiddlewareRequestContext.new(method: method, url: url, headers: request_headers))
      end

      start_time = Process.clock_gettime(Process::CLOCK_MONOTONIC)

      begin
        response = @http_client.run_request(method, url, nil, request_headers) do |req|
          req.params.update(query) if query
          req.body = JSON.generate(body) if body
        end
      rescue Faraday::Error => e
        # Invoke on_error middleware hooks.
        @middleware.each do |mw|
          mw.on_error&.call(MiddlewareErrorContext.new(method: method, url: url, error: e))
        end
        raise TransportError.new(e.message, cause_error: e)
      end

      elapsed_ms = ((Process.clock_gettime(Process::CLOCK_MONOTONIC) - start_time) * 1000).round

      # Invoke on_response middleware hooks.
      @middleware.each do |mw|
        mw.on_response&.call(MiddlewareResponseContext.new(
          method: method, url: url, status: response.status, duration_ms: elapsed_ms
        ))
      end

      # Handle non-2xx responses.
      unless (200..299).include?(response.status)
        error_message = extract_error_message(response)
        error = Strait.map_http_error(response.status, error_message, response.body)

        @middleware.each do |mw|
          mw.on_error&.call(MiddlewareErrorContext.new(method: method, url: url, error: error))
        end

        raise error
      end

      # Return nil for 204 No Content.
      return nil if response.status == 204 || response.body.nil? || response.body.empty?

      begin
        JSON.parse(response.body)
      rescue JSON::ParserError => e
        raise DecodeError.new("Failed to decode response JSON: #{e.message}", body: response.body, cause_error: e)
      end
    end

    # -- Service accessors --------------------------------------------------

    # @return [Operations::HealthService]
    def health
      @health ||= Operations::HealthService.new(self)
    end

    # @return [Operations::JobsService]
    def jobs
      @jobs ||= Operations::JobsService.new(self)
    end

    # @return [Operations::RunsService]
    def runs
      @runs ||= Operations::RunsService.new(self)
    end

    # @return [Operations::WorkflowsService]
    def workflows
      @workflows ||= Operations::WorkflowsService.new(self)
    end

    # @return [Operations::WorkflowRunsService]
    def workflow_runs
      @workflow_runs ||= Operations::WorkflowRunsService.new(self)
    end

    # @return [Operations::DeploymentsService]
    def deployments
      @deployments ||= Operations::DeploymentsService.new(self)
    end

    # @return [Operations::EnvironmentsService]
    def environments
      @environments ||= Operations::EnvironmentsService.new(self)
    end

    # @return [Operations::SecretsService]
    def secrets
      @secrets ||= Operations::SecretsService.new(self)
    end

    # @return [Operations::APIKeysService]
    def api_keys
      @api_keys ||= Operations::APIKeysService.new(self)
    end

    # @return [Operations::WebhooksService]
    def webhooks
      @webhooks ||= Operations::WebhooksService.new(self)
    end

    # @return [Operations::EventTriggersService]
    def event_triggers
      @event_triggers ||= Operations::EventTriggersService.new(self)
    end

    # @return [Operations::EventSourcesService]
    def event_sources
      @event_sources ||= Operations::EventSourcesService.new(self)
    end

    # @return [Operations::BatchOperationsService]
    def batch_operations
      @batch_operations ||= Operations::BatchOperationsService.new(self)
    end

    # @return [Operations::StatsService]
    def stats
      @stats ||= Operations::StatsService.new(self)
    end

    # @return [Operations::AnalyticsService]
    def analytics
      @analytics ||= Operations::AnalyticsService.new(self)
    end

    # @return [Operations::LogDrainsService]
    def log_drains
      @log_drains ||= Operations::LogDrainsService.new(self)
    end

    # @return [Operations::SDKRunsService]
    def sdk_runs
      @sdk_runs ||= Operations::SDKRunsService.new(self)
    end

    # @return [Operations::RBACService]
    def rbac
      @rbac ||= Operations::RBACService.new(self)
    end

    # @return [Operations::JobGroupsService]
    def job_groups
      @job_groups ||= Operations::JobGroupsService.new(self)
    end

    private

    def resolve_auth(auth, bearer_token, api_key, run_token)
      return auth if auth

      if bearer_token
        AuthMode.new(type: AuthType::BEARER, token: bearer_token)
      elsif api_key
        AuthMode.new(type: AuthType::API_KEY, token: api_key)
      elsif run_token
        AuthMode.new(type: AuthType::RUN_TOKEN, token: run_token)
      end
    end

    def build_http_client
      timeout_secs = @timeout_ms / 1000.0
      Faraday.new do |f|
        f.options.timeout = timeout_secs
        f.options.open_timeout = timeout_secs
        f.adapter Faraday.default_adapter
      end
    end

    def build_headers(extra)
      hdrs = {
        "Content-Type" => "application/json",
        "Accept" => "application/json"
      }
      hdrs.merge!(@default_headers)
      hdrs["Authorization"] = ConfigHelper.get_authorization_header(@auth.token) if @auth
      hdrs.merge!(extra) if extra
      hdrs
    end

    def extract_error_message(response)
      parsed = JSON.parse(response.body)
      parsed["message"] || parsed["error"] || "HTTP #{response.status}"
    rescue StandardError
      "HTTP #{response.status}"
    end
  end
end
