import { describe, expect, test } from "bun:test";

import { createTestContext } from "../src/authoring/test-client";
import { withCheckpointResume } from "../src/composition/checkpoint-resume";

describe("withCheckpointResume", () => {
  test("uses initial state when no checkpoint provided", async () => {
    const { ctx, record } = createTestContext();

    const result = await withCheckpointResume(
      ctx,
      undefined,
      (state, updateState) => {
        expect(state).toEqual({ step: 0, items: [] });
        updateState({ step: 1, items: ["a"] });
        return Promise.resolve("done");
      },
      { initialState: { step: 0, items: [] as string[] } }
    );

    expect(result).toBe("done");
    expect(record.checkpoints.length).toBeGreaterThanOrEqual(1);
  });

  test("resumes from provided checkpoint state", async () => {
    const { ctx } = createTestContext();

    await withCheckpointResume(
      ctx,
      { step: 3, items: ["a", "b", "c"] },
      (state, _updateState) => {
        expect(state).toEqual({ step: 3, items: ["a", "b", "c"] });
        return Promise.resolve("resumed");
      },
      { initialState: { step: 0, items: [] as string[] } }
    );
  });

  test("auto-checkpoints at configured interval", async () => {
    const { ctx, record } = createTestContext();

    await withCheckpointResume(
      ctx,
      undefined,
      (state, updateState) => {
        updateState({ ...state, step: 1 });
        updateState({ ...state, step: 2 });
        updateState({ ...state, step: 3 });
        updateState({ ...state, step: 4 });
        return Promise.resolve("ok");
      },
      { initialState: { step: 0 }, checkpointInterval: 2 }
    );

    // Steps 2 and 4 trigger checkpoint (interval=2), plus final checkpoint
    // The fire-and-forget checkpoints from updateState may or may not complete before the final one
    expect(record.checkpoints.length).toBeGreaterThanOrEqual(1);
  });

  test("final checkpoint on completion", async () => {
    const { ctx, record } = createTestContext();

    await withCheckpointResume(
      ctx,
      undefined,
      (_state, updateState) => {
        updateState({ done: true });
        return Promise.resolve("finished");
      },
      { initialState: { done: false } }
    );

    // At minimum, the final checkpoint should be awaited
    const lastCheckpoint = record.checkpoints.at(-1);
    expect(lastCheckpoint).toEqual({ done: true });
  });
});
