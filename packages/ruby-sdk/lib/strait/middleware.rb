# frozen_string_literal: true

module Strait
  # Middleware hooks for request/response/error lifecycle.
  Middleware = Struct.new(:on_request, :on_response, :on_error, keyword_init: true)

  # Context passed to on_request middleware hooks.
  MiddlewareRequestContext = Struct.new(:method, :url, :headers, keyword_init: true)

  # Context passed to on_response middleware hooks.
  MiddlewareResponseContext = Struct.new(:method, :url, :status, :duration_ms, keyword_init: true)

  # Context passed to on_error middleware hooks.
  MiddlewareErrorContext = Struct.new(:method, :url, :error, keyword_init: true)
end
