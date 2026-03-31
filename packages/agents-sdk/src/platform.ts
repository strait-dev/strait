/**
 * Platform API client for cross-product triggers from the agent runtime.
 *
 * Exposes typed methods to trigger jobs, workflows, and agents from within
 * an agent's execution context. All calls are authenticated via the run's
 * JWT token and scoped to the same project.
 */

import type { JsonValue } from "./types";

type InternalClient = {
  post<T>(
    path: string,
    body: unknown,
    opts?: { retryable?: boolean; signal?: AbortSignal }
  ): Promise<T>;
};

export type TriggerJobResult = {
  job_id: string;
  run_id: string;
  status: string;
};

export type TriggerWorkflowResult = {
  status: string;
  workflow_run_id: string;
};

export type TriggerAgentResult = {
  agent_id: string;
  run_id: string;
  status: string;
};

export type AwaitRunResult = {
  error?: string;
  result?: JsonValue;
  run_id: string;
  status: string;
};

export class StraitPlatformClient {
  readonly #client: InternalClient;

  constructor(client: InternalClient) {
    this.#client = client;
  }

  jobs = {
    trigger: (
      jobSlug: string,
      payload?: JsonValue,
      signal?: AbortSignal
    ): Promise<TriggerJobResult> =>
      this.#client.post<TriggerJobResult>(
        "/platform/trigger-job",
        { job_slug: jobSlug, payload },
        { retryable: false, signal }
      ),

    triggerAndWait: async (
      jobSlug: string,
      payload?: JsonValue,
      opts?: { signal?: AbortSignal; timeoutMs?: number }
    ): Promise<AwaitRunResult> => {
      const { run_id } = await this.jobs.trigger(
        jobSlug,
        payload,
        opts?.signal
      );
      return this.awaitRun(run_id, opts?.timeoutMs ?? 60_000, opts?.signal);
    },
  };

  workflows = {
    trigger: (
      workflowSlug: string,
      payload?: JsonValue,
      signal?: AbortSignal
    ): Promise<TriggerWorkflowResult> =>
      this.#client.post<TriggerWorkflowResult>(
        "/platform/trigger-workflow",
        { workflow_slug: workflowSlug, payload },
        { retryable: false, signal }
      ),
  };

  agents = {
    run: (
      agentSlug: string,
      payload?: JsonValue,
      signal?: AbortSignal
    ): Promise<TriggerAgentResult> =>
      this.#client.post<TriggerAgentResult>(
        "/platform/trigger-agent",
        { agent_slug: agentSlug, payload },
        { retryable: false, signal }
      ),

    runAndWait: async (
      agentSlug: string,
      payload?: JsonValue,
      opts?: { signal?: AbortSignal; timeoutMs?: number }
    ): Promise<AwaitRunResult> => {
      const { run_id } = await this.agents.run(
        agentSlug,
        payload,
        opts?.signal
      );
      return this.awaitRun(run_id, opts?.timeoutMs ?? 120_000, opts?.signal);
    },
  };

  awaitRun = (
    runId: string,
    timeoutMs = 60_000,
    signal?: AbortSignal
  ): Promise<AwaitRunResult> => {
    const clampedTimeout = Math.max(0, Math.min(timeoutMs, 300_000));
    return this.#client.post<AwaitRunResult>(
      "/platform/await-run",
      { run_id: runId, timeout_ms: clampedTimeout },
      { retryable: false, signal }
    );
  };
}
