import type { RunContext } from "../authoring/job";

type JsonSchema = {
  readonly type: string;
  readonly properties?: Record<string, unknown>;
  readonly required?: readonly string[];
  readonly [key: string]: unknown;
};

type CoreTool = {
  readonly description: string;
  readonly parameters: JsonSchema;
  readonly execute: (args: Record<string, unknown>) => Promise<unknown>;
};

export type CreateStraitToolsOptions = {
  readonly checkpoint?: boolean;
  readonly spawn?: boolean;
  readonly saveOutput?: boolean;
  readonly waitForEvent?: boolean;
  readonly stateGet?: boolean;
  readonly stateSet?: boolean;
  readonly complete?: boolean;
};

const addCheckpointTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  tools.strait_checkpoint = {
    description:
      "Save agent state for durable execution. The state will be available if the run is resumed.",
    parameters: {
      type: "object",
      properties: {
        state: { type: "object", description: "The state to checkpoint" },
      },
      required: ["state"],
    },
    execute: async (args) => {
      await ctx.checkpoint(args.state as Record<string, unknown>);
      return { success: true };
    },
  };
};

const addSpawnTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  const spawn = ctx.spawn;
  if (!spawn) {
    return;
  }
  tools.strait_spawn = {
    description: "Spawn a child job run.",
    parameters: {
      type: "object",
      properties: {
        jobSlug: { type: "string", description: "The slug of the job to spawn" },
        projectId: { type: "string", description: "The project ID" },
        payload: { type: "object", description: "Optional payload for the child job" },
        priority: { type: "number", description: "Optional priority (higher = sooner)" },
      },
      required: ["jobSlug", "projectId"],
    },
    execute: (args) =>
      spawn({
        jobSlug: args.jobSlug as string,
        projectId: args.projectId as string,
        payload: args.payload as Record<string, unknown> | undefined,
        priority: args.priority as number | undefined,
      }),
  };
};

const addSaveOutputTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  const saveOutput = ctx.saveOutput;
  if (!saveOutput) {
    return;
  }
  tools.strait_save_output = {
    description: "Save a structured output for this run.",
    parameters: {
      type: "object",
      properties: {
        key: { type: "string", description: "Output key" },
        value: { type: "object", description: "Output value" },
      },
      required: ["key", "value"],
    },
    execute: async (args) => {
      await saveOutput(args.key as string, args.value as Record<string, unknown>);
      return { success: true };
    },
  };
};

const addWaitForEventTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  const waitForEvent = ctx.waitForEvent;
  if (!waitForEvent) {
    return;
  }
  tools.strait_wait_for_event = {
    description: "Pause and wait for an external event.",
    parameters: {
      type: "object",
      properties: {
        eventKey: { type: "string", description: "The event key to wait for" },
        timeoutSecs: { type: "number", description: "Maximum wait time in seconds" },
      },
      required: ["eventKey"],
    },
    execute: (args) =>
      waitForEvent(args.eventKey as string, {
        timeoutSecs: args.timeoutSecs as number | undefined,
      }),
  };
};

const addStateGetTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  const state = ctx.state;
  if (!state) {
    return;
  }
  tools.strait_state_get = {
    description: "Read a value from the durable KV state store.",
    parameters: {
      type: "object",
      properties: {
        key: { type: "string", description: "The state key to read" },
      },
      required: ["key"],
    },
    execute: async (args) => {
      const value = await state.get(args.key as string);
      return { key: args.key, value };
    },
  };
};

const addStateSetTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  const state = ctx.state;
  if (!state) {
    return;
  }
  tools.strait_state_set = {
    description: "Write a value to the durable KV state store.",
    parameters: {
      type: "object",
      properties: {
        key: { type: "string", description: "The state key to write" },
        value: { description: "The value to store" },
      },
      required: ["key", "value"],
    },
    execute: async (args) => {
      await state.set(args.key as string, args.value);
      return { success: true };
    },
  };
};

const addCompleteTool = (tools: Record<string, CoreTool>, ctx: RunContext) => {
  const complete = ctx.complete;
  if (!complete) {
    return;
  }
  tools.strait_complete = {
    description: "Mark the current run as completed.",
    parameters: {
      type: "object",
      properties: {
        result: { type: "object", description: "Optional result data" },
      },
    },
    execute: async (args) => {
      await complete(args.result as Record<string, unknown> | undefined);
      return { success: true };
    },
  };
};

type ToolEntry = {
  readonly key: string;
  readonly enabled: boolean;
  readonly add: (tools: Record<string, CoreTool>, ctx: RunContext) => void;
};

const getToolEntries = (ctx: RunContext, options?: CreateStraitToolsOptions): ToolEntry[] => [
  { key: "checkpoint", enabled: (options?.checkpoint ?? true) && !!ctx.checkpoint, add: addCheckpointTool },
  { key: "spawn", enabled: (options?.spawn ?? true) && !!ctx.spawn, add: addSpawnTool },
  { key: "saveOutput", enabled: (options?.saveOutput ?? true) && !!ctx.saveOutput, add: addSaveOutputTool },
  { key: "waitForEvent", enabled: (options?.waitForEvent ?? true) && !!ctx.waitForEvent, add: addWaitForEventTool },
  { key: "stateGet", enabled: (options?.stateGet ?? true) && !!ctx.state, add: addStateGetTool },
  { key: "stateSet", enabled: (options?.stateSet ?? true) && !!ctx.state, add: addStateSetTool },
  { key: "complete", enabled: (options?.complete ?? false) && !!ctx.complete, add: addCompleteTool },
];

export const createStraitTools = (
  ctx: RunContext,
  options?: CreateStraitToolsOptions
): Record<string, CoreTool> => {
  const tools: Record<string, CoreTool> = {};
  for (const entry of getToolEntries(ctx, options)) {
    if (entry.enabled) {
      entry.add(tools, ctx);
    }
  }
  return tools;
};
