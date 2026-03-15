import { describe, expect, test } from "bun:test";

import { createClient } from "../src/client";
import {
  createAndFinalizeDeployment,
  createAndFinalizeDeploymentResult,
  promoteDeploymentVersion,
  rollbackDeploymentVersion,
} from "../src/composition/index";
import type { FetchLike } from "../src/runtime";

const makeJsonResponse = (status: number, body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });

describe("deployment SDK support", () => {
  test("createClient exposes generated deployment operations", async () => {
    const calls: Array<{ readonly method: string; readonly url: string }> = [];

    const fetchImpl: FetchLike = (input, init) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const url = String(input);
      calls.push({ method, url });

      if (method === "GET") {
        return Promise.resolve(
          makeJsonResponse(200, { data: [{ id: "dep_1" }], has_more: false })
        );
      }

      if (method === "POST" && url.endsWith("/promote")) {
        return Promise.resolve(
          makeJsonResponse(200, { id: "dep_1", status: "promoted" })
        );
      }

      return Promise.resolve(
        makeJsonResponse(201, { id: "dep_1", status: "draft" })
      );
    };

    const client = createClient(
      {
        baseUrl: "https://strait.dev",
        auth: { type: "bearer", token: "strait_key" },
      },
      { fetch: fetchImpl }
    );

    const listed = await client.deployments.list({
      query: { project_id: "proj_1", environment: "staging" },
    });
    const created = await client.operationsPromise.postV1Deployments<
      unknown,
      { readonly id: string; readonly status: string }
    >({
      body: {
        project_id: "proj_1",
        environment: "staging",
        runtime: "node",
        artifact_uri: "file:///tmp/manifest.json",
      },
    });
    const promoted =
      await client.operationsPromise.postV1DeploymentsByDeploymentIDPromote<
        unknown,
        { readonly id: string; readonly status: string }
      >({
        pathParams: { deploymentID: "dep_1" },
        body: {
          project_id: "proj_1",
          environment: "staging",
        },
      });

    expect(typeof client.createDeployment).toBe("function");
    expect(typeof client.deployments.create).toBe("function");
    expect(Array.isArray(listed.data)).toBe(true);
    expect(created.id).toBe("dep_1");
    expect(promoted.status).toBe("promoted");
    expect(
      calls.some(
        (call) =>
          call.method === "GET" &&
          call.url ===
            "https://strait.dev/v1/deployments?project_id=proj_1&environment=staging"
      )
    ).toBe(true);
    expect(
      calls.some(
        (call) =>
          call.method === "POST" &&
          call.url === "https://strait.dev/v1/deployments/dep_1/promote"
      )
    ).toBe(true);
  });

  test("createAndFinalizeDeployment chains create and finalize with inferred body", async () => {
    const recorded: string[] = [];

    const client = {
      createDeployment: () => {
        recorded.push("create");
        return Promise.resolve({ id: "dep_42", status: "draft" });
      },
      finalizeDeployment: (input: {
        readonly pathParams: { readonly deploymentID: string };
        readonly body: {
          readonly project_id: string;
          readonly environment: string;
        };
      }) => {
        recorded.push("finalize");
        expect(input.pathParams.deploymentID).toBe("dep_42");
        expect(input.body).toEqual({
          project_id: "proj_1",
          environment: "staging",
        });
        return Promise.resolve({ id: "dep_42", status: "finalized" });
      },
      promoteDeployment: () =>
        Promise.resolve({ id: "dep_42", status: "promoted" }),
      rollbackDeployment: () =>
        Promise.resolve({ id: "dep_42", status: "promoted" }),
    };

    const output = await createAndFinalizeDeployment(client, {
      create: {
        body: {
          project_id: "proj_1",
          environment: "staging",
          runtime: "node",
          artifact_uri: "file:///tmp/manifest.json",
        },
      },
    });

    expect(recorded).toEqual(["create", "finalize"]);
    expect(output.created.id).toBe("dep_42");
    expect(output.finalized.status).toBe("finalized");
  });

  test("createAndFinalizeDeployment result variant captures missing id failures", async () => {
    const client = {
      createDeployment: () => Promise.resolve({ status: "draft" }),
      finalizeDeployment: () =>
        Promise.resolve({ id: "dep", status: "finalized" }),
      promoteDeployment: () =>
        Promise.resolve({ id: "dep", status: "promoted" }),
      rollbackDeployment: () =>
        Promise.resolve({ id: "dep", status: "promoted" }),
    };

    const result = await createAndFinalizeDeploymentResult(client, {
      create: {
        body: {
          project_id: "proj_1",
          environment: "staging",
          runtime: "node",
          artifact_uri: "file:///tmp/manifest.json",
        },
      },
    });

    expect(result.ok).toBe(false);
    expect(() => result.unwrap()).toThrow(
      "deployment response is missing a usable id"
    );
  });

  test("promote and rollback helpers map deploymentID into path params", async () => {
    const inputs: Array<{
      readonly action: string;
      readonly deploymentID: string;
    }> = [];

    const client = {
      createDeployment: () => Promise.resolve({ id: "dep_1" }),
      finalizeDeployment: () => Promise.resolve({ id: "dep_1" }),
      promoteDeployment: (input: {
        readonly pathParams: { readonly deploymentID: string };
      }) => {
        inputs.push({
          action: "promote",
          deploymentID: input.pathParams.deploymentID,
        });
        return Promise.resolve({ id: "dep_2", status: "promoted" });
      },
      rollbackDeployment: (input: {
        readonly pathParams: { readonly deploymentID: string };
      }) => {
        inputs.push({
          action: "rollback",
          deploymentID: input.pathParams.deploymentID,
        });
        return Promise.resolve({ id: "dep_1", status: "promoted" });
      },
    };

    const promoted = await promoteDeploymentVersion(client, {
      deploymentID: "dep_2",
      body: {
        project_id: "proj_1",
        environment: "prod",
      },
    });
    const rolledBack = await rollbackDeploymentVersion(client, {
      deploymentID: "dep_1",
      body: {
        project_id: "proj_1",
        environment: "prod",
      },
    });

    expect(promoted.status).toBe("promoted");
    expect(rolledBack.id).toBe("dep_1");
    expect(inputs).toEqual([
      { action: "promote", deploymentID: "dep_2" },
      { action: "rollback", deploymentID: "dep_1" },
    ]);
  });
});
