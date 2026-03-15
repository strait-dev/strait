import { describe, expect, test } from "bun:test";
import { createActor } from "xstate";

import {
  stepRunMachine,
  workflowRunMachine,
} from "../src/fsm/index";

describe("workflowRunMachine", () => {
  test("starts in pending state", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    expect(actor.getSnapshot().value).toBe("pending");
    actor.stop();
  });

  test("pending → running via START", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    expect(actor.getSnapshot().value).toBe("running");
    actor.stop();
  });

  test("running → paused → running", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "PAUSE" });
    expect(actor.getSnapshot().value).toBe("paused");
    actor.send({ type: "RESUME" });
    expect(actor.getSnapshot().value).toBe("running");
    actor.stop();
  });

  test("running → completed", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "COMPLETE" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });

  test("running → failed", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "FAIL" });
    expect(actor.getSnapshot().value).toBe("failed");
    actor.stop();
  });

  test("running → timed_out", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "TIMEOUT" });
    expect(actor.getSnapshot().value).toBe("timed_out");
    actor.stop();
  });

  test("pending → canceled", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "CANCEL" });
    expect(actor.getSnapshot().value).toBe("canceled");
    actor.stop();
  });

  test("paused → canceled", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "PAUSE" });
    actor.send({ type: "CANCEL" });
    expect(actor.getSnapshot().value).toBe("canceled");
    actor.stop();
  });

  test("paused → completed", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "PAUSE" });
    actor.send({ type: "COMPLETE" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });

  test("completed is terminal", () => {
    const actor = createActor(workflowRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "COMPLETE" });
    actor.send({ type: "FAIL" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });
});

describe("stepRunMachine", () => {
  test("starts in pending state", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    expect(actor.getSnapshot().value).toBe("pending");
    actor.stop();
  });

  test("pending → running → completed", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "START" });
    expect(actor.getSnapshot().value).toBe("running");
    actor.send({ type: "COMPLETE" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });

  test("pending → waiting → running → completed", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "WAIT" });
    expect(actor.getSnapshot().value).toBe("waiting");
    actor.send({ type: "START" });
    expect(actor.getSnapshot().value).toBe("running");
    actor.send({ type: "COMPLETE" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });

  test("pending → skipped", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "SKIP" });
    expect(actor.getSnapshot().value).toBe("skipped");
    actor.stop();
  });

  test("pending → canceled", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "CANCEL" });
    expect(actor.getSnapshot().value).toBe("canceled");
    actor.stop();
  });

  test("running → failed", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "FAIL" });
    expect(actor.getSnapshot().value).toBe("failed");
    actor.stop();
  });

  test("running → canceled", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "CANCEL" });
    expect(actor.getSnapshot().value).toBe("canceled");
    actor.stop();
  });

  test("waiting → skipped", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "WAIT" });
    actor.send({ type: "SKIP" });
    expect(actor.getSnapshot().value).toBe("skipped");
    actor.stop();
  });

  test("waiting → canceled", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "WAIT" });
    actor.send({ type: "CANCEL" });
    expect(actor.getSnapshot().value).toBe("canceled");
    actor.stop();
  });

  test("completed is terminal", () => {
    const actor = createActor(stepRunMachine);
    actor.start();
    actor.send({ type: "START" });
    actor.send({ type: "COMPLETE" });
    actor.send({ type: "FAIL" });
    expect(actor.getSnapshot().value).toBe("completed");
    actor.stop();
  });
});
