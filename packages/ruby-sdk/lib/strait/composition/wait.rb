# frozen_string_literal: true

module Strait
  module Composition
    WaitForRunOptions = Struct.new(
      :timeout_ms, :initial_delay_ms, :max_delay_ms, :factor, :is_terminal,
      keyword_init: true
    ) do
      def initialize(**kwargs)
        super
        self.timeout_ms ||= 300_000
        self.initial_delay_ms ||= 500
        self.max_delay_ms ||= 10_000
        self.factor ||= 1.5
        self.is_terminal ||= ->(status) { Strait::FSM.terminal_run_status?(status) }
      end
    end

    def self.wait_for_run(run_id, get_run:, get_status:, opts: nil)
      opts ||= WaitForRunOptions.new
      start = Process.clock_gettime(Process::CLOCK_MONOTONIC, :millisecond)
      delay = opts.initial_delay_ms

      loop do
        run = get_run.call(run_id)
        status = get_status.call(run)
        return run if opts.is_terminal.call(status)

        elapsed = Process.clock_gettime(Process::CLOCK_MONOTONIC, :millisecond) - start
        if elapsed >= opts.timeout_ms
          raise Strait::TimeoutError.new(
            "timed out waiting for run #{run_id}",
            run_id: run_id,
            elapsed_ms: elapsed
          )
        end

        sleep(delay / 1000.0)
        delay = [delay * opts.factor, opts.max_delay_ms].min
      end
    end
  end
end
