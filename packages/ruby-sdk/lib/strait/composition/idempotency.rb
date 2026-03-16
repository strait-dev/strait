# frozen_string_literal: true

module Strait
  module Composition
    def self.with_idempotency(headers, key)
      headers = (headers || {}).dup
      headers["Idempotency-Key"] = key
      headers
    end

    def self.with_idempotency_header(headers, key, header_name)
      headers = (headers || {}).dup
      headers[header_name] = key
      headers
    end
  end
end
