# frozen_string_literal: true

module Strait
  module Composition
    JITTER_FULL = "full"
    JITTER_NONE = "none"

    RetryOptions = Struct.new(
      :attempts, :delay_ms, :factor, :max_delay_ms, :jitter, :should_retry,
      keyword_init: true
    ) do
      def initialize(**kwargs)
        super
        self.attempts ||= 3
        self.delay_ms ||= 250
        self.factor ||= 2.0
        self.max_delay_ms ||= 30_000
        self.jitter ||= JITTER_FULL
      end
    end

    def self.with_retry(opts = nil, &block)
      opts ||= RetryOptions.new
      last_error = nil

      opts.attempts.times do |attempt|
        begin
          return block.call
        rescue => e
          last_error = e

          if opts.should_retry
            next if opts.should_retry.call(e, attempt + 1, opts.attempts)
            raise e
          end

          raise e if attempt + 1 >= opts.attempts

          delay = [opts.delay_ms * (opts.factor ** attempt), opts.max_delay_ms].min
          if opts.jitter == JITTER_FULL
            delay = rand(0..delay.to_i)
          end
          sleep(delay / 1000.0) if delay > 0
        end
      end

      raise last_error
    end
  end
end
