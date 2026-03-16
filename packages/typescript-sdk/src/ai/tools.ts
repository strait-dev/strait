import { jsonSchema, type Tool, tool } from "ai";
import type { RunContext } from "../authoring/job";

export type CreateStraitToolsOptions = {
  readonly checkpoint?: boolean;
  readonly spawn?: boolean;
  readonly saveOutput?: boolean;
  readonly waitForEvent?: boolean;
  readonly stateGet?: boolean;
  readonly stateSet?: boolean;
  readonly complete?: boolean;
};

type StraitToolSet = Record<string, Tool>;

const addCheckpointTool = (tools: StraitToolSet, ctx: RunContext) => {
  tools.strait_checkpoint = tool({
    description:
      "Save agent state for durable execution. The state will be available if the run is resumed.",
    inputSchema: jsonSchema<{ state: Record<string, unknown> }>({
      type: "object",
      properties: {
        state: {
          type: "object",
          description: "The state to checkpoint",
        },
      },
      required: ["state"],
    }),
    execute: async ({ state }) => {
      await ctx.checkpoint(state);
      return { success: true };
    },
  });
};

const addSpawnTool = (tools: StraitToolSet, ctx: RunContext) => {
  const spawn = ctx.spawn;
  if (!spawn) {
    return;
  }
  tools.strait_spawn = tool({
    description: "Spawn a child job run.",
    inputSchema: jsonSchema<{
      jobSlug: string;
      projectId: string;
      payload?: Record<string, unknown>;
      priority?: number;
    }>({
      type: "object",
      properties: {
        jobSlug: {
          type: "string",
          description: "The slug of the job to spawn",
        },
        projectId: {
          type: "string",
          description: "The project ID",
        },
        payload: {
          type: "object",
          description: "Optional payload for the child job",
        },
        priority: {
          type: "number",
          description: "Optional priority (higher = sooner)",
        },
      },
      required: ["jobSlug", "projectId"],
    }),
    execute: ({ jobSlug, projectId, payload, priority }) =>
      spawn({ jobSlug, projectId, payload, priority }),
  });
};

const addSaveOutputTool = (tools: StraitToolSet, ctx: RunContext) => {
  const saveOutput = ctx.saveOutput;
  if (!saveOutput) {
    return;
  }
  tools.strait_save_output = tool({
    description: "Save a structured output for this run.",
    inputSchema: jsonSchema<{
      key: string;
      value: Record<string, unknown>;
    }>({
      type: "object",
      properties: {
        key: { type: "string", description: "Output key" },
        value: { type: "object", description: "Output value" },
      },
      required: ["key", "value"],
    }),
    execute: async ({ key, value }) => {
      await saveOutput(key, value);
      return { success: true };
    },
  });
};

const addWaitForEventTool = (tools: StraitToolSet, ctx: RunContext) => {
  const waitForEvent = ctx.waitForEvent;
  if (!waitForEvent) {
    return;
  }
  tools.strait_wait_for_event = tool({
    description: "Pause and wait for an external event.",
    inputSchema: jsonSchema<{ eventKey: string; timeoutSecs?: number }>({
      type: "object",
      properties: {
        eventKey: {
          type: "string",
          description: "The event key to wait for",
        },
        timeoutSecs: {
          type: "number",
          description: "Maximum wait time in seconds",
        },
      },
      required: ["eventKey"],
    }),
    execute: ({ eventKey, timeoutSecs }) =>
      waitForEvent(eventKey, { timeoutSecs }),
  });
};

const addStateGetTool = (tools: StraitToolSet, ctx: RunContext) => {
  const state = ctx.state;
  if (!state) {
    return;
  }
  tools.strait_state_get = tool({
    description: "Read a value from the durable KV state store.",
    inputSchema: jsonSchema<{ key: string }>({
      type: "object",
      properties: {
        key: {
          type: "string",
          description: "The state key to read",
        },
      },
      required: ["key"],
    }),
    execute: async ({ key }) => {
      const value = await state.get(key);
      return { key, value };
    },
  });
};

const addStateSetTool = (tools: StraitToolSet, ctx: RunContext) => {
  const state = ctx.state;
  if (!state) {
    return;
  }
  tools.strait_state_set = tool({
    description: "Write a value to the durable KV state store.",
    inputSchema: jsonSchema<{ key: string; value: unknown }>({
      type: "object",
      properties: {
        key: {
          type: "string",
          description: "The state key to write",
        },
        value: { description: "The value to store" },
      },
      required: ["key", "value"],
    }),
    execute: async ({ key, value }) => {
      await state.set(key, value);
      return { success: true };
    },
  });
};

const addCompleteTool = (tools: StraitToolSet, ctx: RunContext) => {
  const complete = ctx.complete;
  if (!complete) {
    return;
  }
  tools.strait_complete = tool({
    description: "Mark the current run as completed.",
    inputSchema: jsonSchema<{ result?: Record<string, unknown> }>({
      type: "object",
      properties: {
        result: {
          type: "object",
          description: "Optional result data",
        },
      },
    }),
    execute: async ({ result }) => {
      await complete(result);
      return { success: true };
    },
  });
};

type ToolEntry = {
  readonly key: string;
  readonly enabled: boolean;
  readonly add: (tools: StraitToolSet, ctx: RunContext) => void;
};

const getToolEntries = (
  ctx: RunContext,
  options?: CreateStraitToolsOptions
): ToolEntry[] => [
  {
    key: "checkpoint",
    enabled: (options?.checkpoint ?? true) && !!ctx.checkpoint,
    add: addCheckpointTool,
  },
  {
    key: "spawn",
    enabled: (options?.spawn ?? true) && !!ctx.spawn,
    add: addSpawnTool,
  },
  {
    key: "saveOutput",
    enabled: (options?.saveOutput ?? true) && !!ctx.saveOutput,
    add: addSaveOutputTool,
  },
  {
    key: "waitForEvent",
    enabled: (options?.waitForEvent ?? true) && !!ctx.waitForEvent,
    add: addWaitForEventTool,
  },
  {
    key: "stateGet",
    enabled: (options?.stateGet ?? true) && !!ctx.state,
    add: addStateGetTool,
  },
  {
    key: "stateSet",
    enabled: (options?.stateSet ?? true) && !!ctx.state,
    add: addStateSetTool,
  },
  {
    key: "complete",
    enabled: (options?.complete ?? false) && !!ctx.complete,
    add: addCompleteTool,
  },
];

export const createStraitTools = (
  ctx: RunContext,
  options?: CreateStraitToolsOptions
): StraitToolSet => {
  const tools: StraitToolSet = {};
  for (const entry of getToolEntries(ctx, options)) {
    if (entry.enabled) {
      entry.add(tools, ctx);
    }
  }
  return tools;
};
