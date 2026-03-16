# frozen_string_literal: true

module Strait
  module Composition
    class Result
      attr_reader :value, :error

      def initialize(value: nil, error: nil)
        @value = value
        @error = error
        @ok = error.nil?
      end

      def ok? = @ok
      def err? = !@ok

      def unwrap
        raise @error if err?
        @value
      end

      def unwrap_err
        raise "Result is ok, not an error" if ok?
        @error
      end

      def match(on_ok:, on_err:)
        if ok?
          on_ok.call(@value)
        else
          on_err.call(@error)
        end
      end

      def self.ok(value)
        new(value: value)
      end

      def self.err(error)
        new(error: error)
      end

      def self.from_block
        value = yield
        ok(value)
      rescue => e
        err(e)
      end
    end
  end
end
