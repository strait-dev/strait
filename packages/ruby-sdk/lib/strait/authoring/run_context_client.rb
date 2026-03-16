# frozen_string_literal: true

module Strait
  module Authoring
    # Factory for creating a RunContext wired to HTTP endpoints.
    module RunContextFactory
      # Creates a RunContext wired to the given client's SDK runs service.
      # @param client [Object] An object responding to SDK run methods
      # @param run_id [String] The run identifier
      # @param attempt [Integer] The attempt number
      # @return [RunContext]
      def self.create_run_context(client, run_id, attempt: 1)
        RunContext.new(
          run_id: run_id,
          attempt: attempt,

          checkpoint: ->(state) {
            client.checkpoint_run(run_id, { "state" => state, "source" => "sdk" })
          },

          report_progress: ->(percent, message = nil) {
            body = { "percent" => percent }
            body["message"] = message if message
            client.progress_run(run_id, body)
          },

          heartbeat: -> { client.heartbeat_run(run_id) },

          report_usage: ->(provider:, model:, prompt_tokens: nil, completion_tokens: nil, total_tokens: nil, cost_microusd: nil) {
            body = { "provider" => provider, "model" => model }
            body["prompt_tokens"] = prompt_tokens if prompt_tokens
            body["completion_tokens"] = completion_tokens if completion_tokens
            body["total_tokens"] = total_tokens if total_tokens
            body["cost_microusd"] = cost_microusd if cost_microusd
            client.usage_run(run_id, body)
          },

          log_tool_call: ->(tool_name:, input: nil, output: nil, duration_ms: nil, status: nil) {
            body = { "tool_name" => tool_name }
            body["input"] = input if input
            body["output"] = output if output
            body["duration_ms"] = duration_ms if duration_ms
            body["status"] = status if status
            client.tool_call_run(run_id, body)
          },

          save_output: ->(key, value, schema = nil) {
            body = { "key" => key, "value" => value }
            body["schema"] = schema if schema
            client.output_run(run_id, body)
          },

          state: RunContextState.new(
            get: ->(key) { client.get_state(run_id, key) },
            set: ->(key, value) { client.set_state(run_id, { "key" => key, "value" => value }) },
            delete: ->(key) { client.delete_state(run_id, key) },
            list: -> { client.list_state(run_id) }
          ),

          stream_chunk: ->(chunk, stream_id: nil, done: nil) {
            body = { "chunk" => chunk }
            body["stream_id"] = stream_id if stream_id
            body["done"] = done unless done.nil?
            client.stream_run(run_id, body)
          },

          wait_for_event: ->(event_key, timeout_secs: nil, notify_url: nil) {
            body = { "event_key" => event_key }
            body["timeout_secs"] = timeout_secs if timeout_secs
            body["notify_url"] = notify_url if notify_url
            client.wait_for_event_run(run_id, body)
          },

          spawn: ->(job_slug:, project_id:, payload: nil, priority: nil) {
            body = { "job_slug" => job_slug, "project_id" => project_id }
            body["payload"] = payload if payload
            body["priority"] = priority if priority
            client.spawn_run(run_id, body)
          },

          continue_run: ->(payload = nil) {
            body = payload ? { "payload" => payload } : nil
            client.continue_run(run_id, body)
          },

          annotate: ->(annotations) {
            client.annotate_run(run_id, { "annotations" => annotations })
          },

          complete: ->(result = nil) {
            body = result ? { "result" => result } : nil
            client.complete_run(run_id, body)
          },

          fail: ->(error) {
            client.fail_run(run_id, { "error" => error })
          }
        )
      end
    end
  end
end
