# frozen_string_literal: true

module Strait
  module HTTP
    # Replace {paramName} placeholders in a URL path with actual values.
    #
    # @param path   [String] URL path with placeholders, e.g. "/v1/jobs/{jobID}"
    # @param params [Hash]   mapping of placeholder names to values
    # @return [String] path with substituted values
    def self.substitute_path_params(path, params)
      result = path.dup
      params.each do |key, value|
        result.gsub!("{#{key}}", value.to_s)
      end
      result
    end
  end
end
