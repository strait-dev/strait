import { describe, expect, test } from "bun:test";
import { createActor } from "xstate";

import {
  canTransitionRun,
  isTerminalRunStatus,
  type RunStatus,
  runMachine,
} from "../src/fsm/index";

describe("runMachine", () => {
  test("starts in delayed state", () => {
    const actor = createActor(runMachine);
    actor.start();
    expect(actor.getSnapshot().value).toBe("delayed");
    actor.stop();
  });

  test("delayed → queued via ENQUEUE", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    expect(actor.getSnapshot().value).toBe("queued");
    actor.stop();
  });

  test("queued → dequeued → executing → completed", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    expect(actor.getSnapshot().value).toBe("dequeued");
    actor.send({ type: "EXECUTE" });
    expect(actor.getSnapshot().value).toBe("executing");
    actor.send({ type: "COMPLETE" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });

  test("executing → failed", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "EXECUTE" });
    actor.send({ type: "FAIL" });
    expect(actor.getSnapshot().value).toBe("failed");
    actor.stop();
  });

  test("executing → timed_out", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "EXECUTE" });
    actor.send({ type: "TIMEOUT" });
    expect(actor.getSnapshot().value).toBe("timed_out");
    actor.stop();
  });

  test("executing → crashed", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "EXECUTE" });
    actor.send({ type: "CRASH" });
    expect(actor.getSnapshot().value).toBe("crashed");
    actor.stop();
  });

  test("executing → waiting → executing", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "EXECUTE" });
    actor.send({ type: "WAIT" });
    expect(actor.getSnapshot().value).toBe("waiting");
    actor.send({ type: "EXECUTE" });
    expect(actor.getSnapshot().value).toBe("executing");
    actor.stop();
  });

  test("executing → dead_letter → replay_staged → queued", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "EXECUTE" });
    actor.send({ type: "DEAD_LETTER" });
    expect(actor.getSnapshot().value).toBe("dead_letter");
    actor.send({ type: "REPLAY" });
    expect(actor.getSnapshot().value).toBe("replay_staged");
    actor.send({ type: "ENQUEUE" });
    expect(actor.getSnapshot().value).toBe("queued");
    actor.stop();
  });

  test("delayed → canceled", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "CANCEL" });
    expect(actor.getSnapshot().value).toBe("canceled");
    actor.stop();
  });

  test("delayed → expired", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "EXPIRE" });
    expect(actor.getSnapshot().value).toBe("expired");
    actor.stop();
  });

  test("dequeued → system_failed", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "SYSTEM_FAIL" });
    expect(actor.getSnapshot().value).toBe("system_failed");
    actor.stop();
  });

  test("dequeued → queued via REQUEUE", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "REQUEUE" });
    expect(actor.getSnapshot().value).toBe("queued");
    actor.stop();
  });

  test("completed is a terminal state (ignores events)", () => {
    const actor = createActor(runMachine);
    actor.start();
    actor.send({ type: "ENQUEUE" });
    actor.send({ type: "DEQUEUE" });
    actor.send({ type: "EXECUTE" });
    actor.send({ type: "COMPLETE" });
    actor.send({ type: "FAIL" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });
});

describe("canTransitionRun", () => {
  test("valid transitions return true", () => {
    expect(canTransitionRun("delayed", "ENQUEUE")).toBe(true);
    expect(canTransitionRun("queued", "DEQUEUE")).toBe(true);
    expect(canTransitionRun("executing", "COMPLETE")).toBe(true);
    expect(canTransitionRun("executing", "FAIL")).toBe(true);
    expect(canTransitionRun("dead_letter", "REPLAY")).toBe(true);
  });

  test("invalid transitions return false", () => {
    expect(canTransitionRun("completed", "EXECUTE")).toBe(false);
    expect(canTransitionRun("failed", "COMPLETE")).toBe(false);
    expect(canTransitionRun("delayed", "COMPLETE")).toBe(false);
    expect(canTransitionRun("queued", "EXECUTE")).toBe(false);
  });
});

describe("isTerminalRunStatus", () => {
  test("terminal statuses return true", () => {
    const terminals: RunStatus[] = [
      "completed",
      "failed",
      "timed_out",
      "crashed",
      "system_failed",
      "canceled",
      "expired",
    ];

    for (const status of terminals) {
      expect(isTerminalRunStatus(status)).toBe(true);
    }
  });

  test("non-terminal statuses return false", () => {
    const nonTerminals: RunStatus[] = [
      "delayed",
      "queued",
      "dequeued",
      "executing",
      "waiting",
      "dead_letter",
      "replay_staged",
    ];

    for (const status of nonTerminals) {
      expect(isTerminalRunStatus(status)).toBe(false);
    }
  });
});
