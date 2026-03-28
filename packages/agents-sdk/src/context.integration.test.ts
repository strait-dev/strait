import { createServer } from "node:http";
import { afterAll, beforeAll, describe, expect, it } from "vitest";

import { StraitContext } from "./context";

type CapturedRequest = {
  path: string;
  authorization: string | undefined;
  sdkVersion: string | undefined;
  body: unknown;
};

describe("StraitContext integration", () => {
  const requests: CapturedRequest[] = [];
  const server = createServer(async (req, res) => {
    const chunks: Uint8Array[] = [];
    for await (const chunk of req) {
      chunks.push(chunk);
    }
    const bodyText = Buffer.concat(chunks).toString("utf8");
    requests.push({
      path: req.url ?? "",
      authorization: req.headers.authorization,
      sdkVersion: req.headers["x-sdk-version"] as string | undefined,
      body: bodyText.length > 0 ? JSON.parse(bodyText) : undefined,
    });

    res.setHeader("content-type", "application/json");
    res.writeHead(200);
    res.end(JSON.stringify({ status: "ok" }));
  });

  beforeAll(async () => {
    await new Promise<void>((resolve) => {
      server.listen(0, "127.0.0.1", () => resolve());
    });
  });

  afterAll(async () => {
    await new Promise<void>((resolve, reject) => {
      server.close((error) => {
        if (error) {
          reject(error);
          return;
        }
        resolve();
      });
    });
  });

  it("uses environment wiring and hits the existing sdk run endpoints", async () => {
    const address = server.address();
    if (address == null || typeof address === "string") {
      throw new Error("server address unavailable");
    }

    const context = StraitContext.fromEnv({
      STRAIT_API_URL: `http://127.0.0.1:${address.port}`,
      STRAIT_RUN_ID: "run-env-1",
      STRAIT_RUN_TOKEN: "token-env-1",
    });

    await context.checkpoint({ phase: "planning" });
    await context.stream({ chunk: "hello", done: true });
    await context.workflow.state.set("shared-plan", { phase: "fan-out" });

    expect(requests).toHaveLength(3);
    expect(requests[0]).toMatchObject({
      path: "/sdk/v1/runs/run-env-1/checkpoint",
      authorization: "Bearer token-env-1",
      sdkVersion: "2.0.0",
      body: {
        state: {
          phase: "planning",
        },
      },
    });
    expect(requests[1]).toMatchObject({
      path: "/sdk/v1/runs/run-env-1/stream",
      authorization: "Bearer token-env-1",
      sdkVersion: "2.0.0",
      body: {
        chunk: "hello",
        done: true,
      },
    });
    expect(requests[2]).toMatchObject({
      path: "/sdk/v1/runs/run-env-1/workflow-state",
      authorization: "Bearer token-env-1",
      sdkVersion: "2.0.0",
      body: {
        key: "shared-plan",
        value: {
          phase: "fan-out",
        },
      },
    });
  });
});
