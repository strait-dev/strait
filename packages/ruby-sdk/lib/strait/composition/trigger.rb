# frozen_string_literal: true

module Strait
  module Composition
    def self.trigger_and_wait(input, trigger_fn:, get_run:, get_id:, get_status:, opts: nil)
      result = trigger_fn.call(input)
      run_id = get_id.call(result)

      wait_for_run(
        run_id,
        get_run: get_run,
        get_status: get_status,
        opts: opts
      )
    end
  end
end
