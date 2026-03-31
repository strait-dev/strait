import { describe, expect, it, vi } from "vitest";
import { StraitPlatformClient } from "./platform";

function mockClient() {
  return {
    post: vi.fn().mockResolvedValue({}),
  };
}

describe("StraitPlatformClient", () => {
  describe("jobs", () => {
    it("trigger sends correct payload", async () => {
      const client = mockClient();
      client.post.mockResolvedValue({
        run_id: "run-1",
        job_id: "job-1",
        status: "queued",
      });
      const platform = new StraitPlatformClient(client);

      const result = await platform.jobs.trigger("etl-pipeline", {
        data: "test",
      });

      expect(client.post).toHaveBeenCalledWith(
        "/platform/trigger-job",
        { job_slug: "etl-pipeline", payload: { data: "test" } },
        { retryable: false, signal: undefined }
      );
      expect(result.run_id).toBe("run-1");
    });

    it("triggerAndWait polls until terminal", async () => {
      const client = mockClient();
      client.post
        .mockResolvedValueOnce({
          run_id: "run-1",
          job_id: "job-1",
          status: "queued",
        })
        .mockResolvedValueOnce({
          run_id: "run-1",
          status: "completed",
          result: { ok: true },
        });

      const platform = new StraitPlatformClient(client);
      const result = await platform.jobs.triggerAndWait("etl-pipeline");

      expect(client.post).toHaveBeenCalledTimes(2);
      expect(result.status).toBe("completed");
    });
  });

  describe("workflows", () => {
    it("trigger sends correct payload", async () => {
      const client = mockClient();
      client.post.mockResolvedValue({
        workflow_run_id: "wfr-1",
        status: "running",
      });
      const platform = new StraitPlatformClient(client);

      const result = await platform.workflows.trigger("approval-flow", {
        request: "data",
      });

      expect(client.post).toHaveBeenCalledWith(
        "/platform/trigger-workflow",
        { workflow_slug: "approval-flow", payload: { request: "data" } },
        { retryable: false, signal: undefined }
      );
      expect(result.workflow_run_id).toBe("wfr-1");
    });
  });

  describe("agents", () => {
    it("run sends correct payload", async () => {
      const client = mockClient();
      client.post.mockResolvedValue({
        run_id: "run-2",
        agent_id: "agent-1",
        status: "queued",
      });
      const platform = new StraitPlatformClient(client);

      const result = await platform.agents.run("classifier", { text: "hello" });

      expect(client.post).toHaveBeenCalledWith(
        "/platform/trigger-agent",
        { agent_slug: "classifier", payload: { text: "hello" } },
        { retryable: false, signal: undefined }
      );
      expect(result.agent_id).toBe("agent-1");
    });

    it("runAndWait polls until terminal", async () => {
      const client = mockClient();
      client.post
        .mockResolvedValueOnce({
          run_id: "run-2",
          agent_id: "agent-1",
          status: "queued",
        })
        .mockResolvedValueOnce({
          run_id: "run-2",
          status: "completed",
          result: { answer: 42 },
        });

      const platform = new StraitPlatformClient(client);
      const result = await platform.agents.runAndWait("classifier");

      expect(result.status).toBe("completed");
    });
  });

  describe("awaitRun", () => {
    it("clamps timeout to 300000ms", async () => {
      const client = mockClient();
      client.post.mockResolvedValue({ run_id: "r", status: "completed" });
      const platform = new StraitPlatformClient(client);

      await platform.awaitRun("r", 999_999);

      expect(client.post).toHaveBeenCalledWith(
        "/platform/await-run",
        { run_id: "r", timeout_ms: 300_000 },
        { retryable: false, signal: undefined }
      );
    });

    it("clamps negative timeout to 0", async () => {
      const client = mockClient();
      client.post.mockResolvedValue({ run_id: "r", status: "queued" });
      const platform = new StraitPlatformClient(client);

      await platform.awaitRun("r", -100);

      expect(client.post).toHaveBeenCalledWith(
        "/platform/await-run",
        { run_id: "r", timeout_ms: 0 },
        { retryable: false, signal: undefined }
      );
    });

    it("defaults to 60000ms timeout", async () => {
      const client = mockClient();
      client.post.mockResolvedValue({ run_id: "r", status: "completed" });
      const platform = new StraitPlatformClient(client);

      await platform.awaitRun("r");

      expect(client.post).toHaveBeenCalledWith(
        "/platform/await-run",
        { run_id: "r", timeout_ms: 60_000 },
        { retryable: false, signal: undefined }
      );
    });
  });
});
