import { describe, expect, test } from "bun:test";
import {
  createCostTracker,
  withCostBudget,
} from "../src/composition/cost-budget";
import { CostBudgetExceededError } from "../src/errors";

describe("createCostTracker", () => {
  test("tracks costs correctly", () => {
    const tracker = createCostTracker({ maxCostMicrousd: 10_000 });

    tracker.add(3000);
    expect(tracker.current()).toBe(3000);
    expect(tracker.remaining()).toBe(7000);
    expect(tracker.isExceeded()).toBe(false);
  });

  test("throws CostBudgetExceededError when exceeded", () => {
    const tracker = createCostTracker({ maxCostMicrousd: 1000 });

    tracker.add(500);
    expect(() => tracker.add(600)).toThrow();

    try {
      const t2 = createCostTracker({ maxCostMicrousd: 100 });
      t2.add(200);
    } catch (e) {
      expect(e).toBeInstanceOf(CostBudgetExceededError);
      const err = e as CostBudgetExceededError;
      expect(err.currentCostMicrousd).toBe(200);
      expect(err.maxCostMicrousd).toBe(100);
    }
  });

  test("fires warning callback at threshold", () => {
    let warningCurrent = 0;
    let warningMax = 0;

    const tracker = createCostTracker({
      maxCostMicrousd: 1000,
      warningThreshold: 0.5,
      onWarning: (current, max) => {
        warningCurrent = current;
        warningMax = max;
      },
    });

    tracker.add(400);
    expect(warningCurrent).toBe(0); // below 50%

    tracker.add(200);
    expect(warningCurrent).toBe(600); // crossed 50%
    expect(warningMax).toBe(1000);
  });

  test("warning fires only once", () => {
    let warningCount = 0;

    const tracker = createCostTracker({
      maxCostMicrousd: 1000,
      warningThreshold: 0.5,
      onWarning: () => {
        warningCount++;
      },
    });

    tracker.add(600);
    tracker.add(200);
    // Should not throw yet (800 < 1000), but warning should only fire once
    expect(warningCount).toBe(1);
  });

  test("default warning threshold is 0.8", () => {
    let warned = false;

    const tracker = createCostTracker({
      maxCostMicrousd: 1000,
      onWarning: () => {
        warned = true;
      },
    });

    tracker.add(700);
    expect(warned).toBe(false);

    tracker.add(200);
    expect(warned).toBe(true);
  });

  test("isExceeded returns true at exact budget", () => {
    const tracker = createCostTracker({ maxCostMicrousd: 100 });

    // Adding exactly the max should throw
    expect(() => tracker.add(100)).toThrow();
  });

  test("remaining returns 0 when at budget", () => {
    const tracker = createCostTracker({ maxCostMicrousd: 100 });

    tracker.add(50);
    expect(tracker.remaining()).toBe(50);
  });
});

describe("withCostBudget", () => {
  test("wraps function with tracker", async () => {
    const result = await withCostBudget(
      (tracker) => {
        tracker.add(100);
        return Promise.resolve(tracker.current());
      },
      { maxCostMicrousd: 1000 }
    );

    expect(result).toBe(100);
  });

  test("budget exceeded propagates out", () => {
    expect(() =>
      withCostBudget(
        (tracker) => {
          tracker.add(2000);
          return Promise.resolve();
        },
        { maxCostMicrousd: 1000 }
      )
    ).toThrow();
  });
});
