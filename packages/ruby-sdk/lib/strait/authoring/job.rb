# frozen_string_literal: true

module Strait
  module Authoring
    JobOptions = Struct.new(
      :name, :slug, :endpoint_url, :project_id,
      :description, :group_id, :tags, :environment_id,
      :cron, :timezone, :execution_window_cron,
      :max_concurrency, :rate_limit_max, :rate_limit_window_secs,
      :max_attempts, :retry_strategy, :retry_delays_secs,
      :timeout_secs, :run_ttl_secs, :dedup_window_secs,
      :webhook_url, :webhook_secret, :fallback_endpoint_url,
      :run, :on_success, :on_failure, :on_start,
      keyword_init: true
    )

    TriggerJobInput = Struct.new(
      :job_id, :payload, :idempotency_key, :priority,
      :dry_run, :metadata, :scheduled_at,
      keyword_init: true
    )

    class JobDefinition
      attr_reader :kind, :slug, :run, :on_success, :on_failure, :on_start
      attr_accessor :last_registered_job_id

      def initialize(opts)
        @kind = "job"
        @slug = opts.slug
        @opts = opts
        @run = opts.run
        @on_success = opts.on_success
        @on_failure = opts.on_failure
        @on_start = opts.on_start
        @last_registered_job_id = nil
      end

      def to_registration_body(project_id = nil)
        pid = project_id || @opts.project_id
        body = {}
        body["name"] = @opts.name if @opts.name
        body["slug"] = @opts.slug if @opts.slug
        body["endpoint_url"] = @opts.endpoint_url if @opts.endpoint_url
        body["project_id"] = pid if pid
        body["description"] = @opts.description if @opts.description
        body["group_id"] = @opts.group_id if @opts.group_id
        body["tags"] = @opts.tags if @opts.tags
        body["environment_id"] = @opts.environment_id if @opts.environment_id
        body["cron"] = @opts.cron if @opts.cron
        body["timezone"] = @opts.timezone if @opts.timezone
        body["execution_window_cron"] = @opts.execution_window_cron if @opts.execution_window_cron
        body["max_concurrency"] = @opts.max_concurrency if @opts.max_concurrency
        body["rate_limit_max"] = @opts.rate_limit_max if @opts.rate_limit_max
        body["rate_limit_window_secs"] = @opts.rate_limit_window_secs if @opts.rate_limit_window_secs
        body["max_attempts"] = @opts.max_attempts if @opts.max_attempts
        body["retry_strategy"] = @opts.retry_strategy if @opts.retry_strategy
        body["retry_delays_secs"] = @opts.retry_delays_secs if @opts.retry_delays_secs
        body["timeout_secs"] = @opts.timeout_secs if @opts.timeout_secs
        body["run_ttl_secs"] = @opts.run_ttl_secs if @opts.run_ttl_secs
        body["dedup_window_secs"] = @opts.dedup_window_secs if @opts.dedup_window_secs
        body["webhook_url"] = @opts.webhook_url if @opts.webhook_url
        body["webhook_secret"] = @opts.webhook_secret if @opts.webhook_secret
        body["fallback_endpoint_url"] = @opts.fallback_endpoint_url if @opts.fallback_endpoint_url
        body
      end

      def register(client, project_id: nil)
        body = to_registration_body(project_id)
        result = client.jobs.create(body)
        @last_registered_job_id = result["id"] if result.is_a?(Hash)
        result
      end

      def trigger(client, input)
        job_id = input.job_id || @last_registered_job_id
        raise ArgumentError, "job_id is required" unless job_id

        body = {}
        body["payload"] = input.payload if input.payload
        body["idempotency_key"] = input.idempotency_key if input.idempotency_key
        body["priority"] = input.priority if input.priority
        body["dry_run"] = input.dry_run unless input.dry_run.nil?
        body["metadata"] = input.metadata if input.metadata
        body["scheduled_at"] = input.scheduled_at if input.scheduled_at
        client.jobs.trigger(job_id, body)
      end

      def batch_trigger(client, items, job_id: nil)
        jid = job_id || @last_registered_job_id
        raise ArgumentError, "job_id is required" unless jid

        body = {
          "items" => items.map { |item|
            h = {}
            h["payload"] = item.payload if item.payload
            h["idempotency_key"] = item.idempotency_key if item.idempotency_key
            h["priority"] = item.priority if item.priority
            h["dry_run"] = item.dry_run unless item.dry_run.nil?
            h["metadata"] = item.metadata if item.metadata
            h["scheduled_at"] = item.scheduled_at if item.scheduled_at
            h
          }
        }
        client.jobs.bulk_trigger(jid, body)
      end
    end

    def self.define_job(**opts)
      JobDefinition.new(JobOptions.new(**opts))
    end
  end
end
