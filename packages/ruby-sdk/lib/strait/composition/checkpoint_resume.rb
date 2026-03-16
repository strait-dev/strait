# frozen_string_literal: true

module Strait
  module Composition
    # Wraps a block with checkpoint/resume state management.
    #
    # @param ctx [Strait::Authoring::RunContext] The run context
    # @param last_checkpoint [Hash, nil] Restored state from prior run
    # @param initial_state [Hash] Default state if no checkpoint
    # @param checkpoint_interval [Integer] How often to checkpoint (default 1)
    # @yield [state, update_state] The user function
    # @return The result of the block
    def self.with_checkpoint_resume(ctx, last_checkpoint, initial_state:, checkpoint_interval: 1, &block)
      current_state = last_checkpoint || initial_state
      step_count = 0

      update_state = ->(new_state) {
        current_state = new_state
        step_count += 1
        if (step_count % checkpoint_interval).zero? && ctx.checkpoint
          begin
            ctx.checkpoint.call(current_state)
          rescue StandardError
            # fire-and-forget
          end
        end
      }

      result = block.call(current_state, update_state)

      # Final checkpoint
      ctx.checkpoint&.call(current_state)

      result
    end
  end
end
