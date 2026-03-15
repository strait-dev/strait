# frozen_string_literal: true

module Strait
  # Base error for all Strait SDK errors.
  class Error < StandardError; end

  # Raised when the HTTP transport layer fails (network error, DNS, etc.).
  class TransportError < Error
    attr_reader :cause_error

    def initialize(message, cause_error: nil)
      @cause_error = cause_error
      super(message)
    end
  end

  # Raised when the response body cannot be decoded as JSON.
  class DecodeError < Error
    attr_reader :body, :cause_error

    def initialize(message, body: nil, cause_error: nil)
      @body = body
      @cause_error = cause_error
      super(message)
    end
  end

  # Raised when request validation fails before sending.
  class ValidationError < Error
    attr_reader :issues

    def initialize(message, issues: [])
      @issues = issues
      super(message)
    end
  end

  # Raised on HTTP 401.
  class UnauthorizedError < Error
    attr_reader :status, :body

    def initialize(status, message, body: nil)
      @status = status
      @body = body
      super(message)
    end
  end

  # Raised on HTTP 404.
  class NotFoundError < Error
    attr_reader :status, :body

    def initialize(status, message, body: nil)
      @status = status
      @body = body
      super(message)
    end
  end

  # Raised on HTTP 409.
  class ConflictError < Error
    attr_reader :status, :body

    def initialize(status, message, body: nil)
      @status = status
      @body = body
      super(message)
    end
  end

  # Raised on HTTP 429.
  class RateLimitedError < Error
    attr_reader :status, :body

    def initialize(status, message, body: nil)
      @status = status
      @body = body
      super(message)
    end
  end

  # Raised on any other non-2xx HTTP status.
  class ApiError < Error
    attr_reader :status, :body

    def initialize(status, message, body: nil)
      @status = status
      @body = body
      super(message)
    end
  end

  # Raised when a polling/wait operation times out.
  class TimeoutError < Error
    attr_reader :run_id, :elapsed_ms

    def initialize(message, run_id: nil, elapsed_ms: nil)
      @run_id = run_id
      @elapsed_ms = elapsed_ms
      super(message)
    end
  end

  # Raised when DAG validation fails (cycles, missing refs, duplicates).
  class DagValidationError < Error
    attr_reader :cycles, :missing_refs, :duplicate_refs

    def initialize(message, cycles: [], missing_refs: [], duplicate_refs: [])
      @cycles = cycles
      @missing_refs = missing_refs
      @duplicate_refs = duplicate_refs
      super(message)
    end
  end

  # Map an HTTP status code to the appropriate error class.
  #
  # @param status  [Integer] HTTP status code
  # @param message [String]  error message
  # @param body    [String, nil] raw response body
  # @return [Error]
  def self.map_http_error(status, message, body = nil)
    case status
    when 401
      UnauthorizedError.new(status, message, body: body)
    when 404
      NotFoundError.new(status, message, body: body)
    when 409
      ConflictError.new(status, message, body: body)
    when 429
      RateLimitedError.new(status, message, body: body)
    else
      ApiError.new(status, message, body: body)
    end
  end
end
