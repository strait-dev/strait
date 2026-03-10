/** Options for waiting on an external event. */
export interface WaitForEventOptions {
  /** Globally unique event key. Use namespaced patterns like "aml:user-123". */
  eventKey: string;
  /** Timeout in seconds (default: 3600). */
  timeoutSecs?: number;
  /** Optional webhook URL to notify when the event arrives. */
  notifyUrl?: string;
}

/** Options for sending an event to resolve a waiting trigger. */
export interface SendEventOptions {
  /** Globally unique event key to resolve. */
  eventKey: string;
  /** Optional JSON payload to deliver with the event. */
  payload?: Record<string, unknown>;
}

/** Represents a durable event trigger. */
export interface EventTrigger {
  id: string;
  event_key: string;
  project_id: string;
  source_type: "workflow_step" | "job_run";
  trigger_type: "event" | "sleep";
  workflow_run_id?: string;
  workflow_step_run_id?: string;
  job_run_id?: string;
  status: "waiting" | "received" | "timed_out" | "canceled";
  timeout_secs: number;
  request_payload?: unknown;
  response_payload?: unknown;
  requested_at: string;
  received_at?: string;
  expires_at: string;
  error?: string;
  notify_url?: string;
  notify_status?: string;
  sent_by?: string;
}
