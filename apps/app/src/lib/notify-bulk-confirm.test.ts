import { describe, expect, it } from "vitest";
import {
  bulkPrefConfirmDescription,
  bulkTopicConfirmDescription,
  bulkTopicConfirmTitle,
} from "./notify-bulk-confirm";

describe("bulkTopicConfirmTitle", () => {
  it("returns add title for add action", () => {
    expect(bulkTopicConfirmTitle("add")).toBe(
      "Bulk add subscribers to topic"
    );
  });

  it("returns remove title for remove action", () => {
    expect(bulkTopicConfirmTitle("remove")).toBe(
      "Bulk remove subscribers from topic"
    );
  });
});

describe("bulkTopicConfirmDescription", () => {
  it("uses singular noun for one subscriber (add)", () => {
    expect(bulkTopicConfirmDescription("add", 1, "alerts.critical")).toBe(
      'Add 1 subscriber to topic "alerts.critical"?'
    );
  });

  it("uses plural noun for multiple subscribers (add)", () => {
    expect(bulkTopicConfirmDescription("add", 42, "alerts.critical")).toBe(
      'Add 42 subscribers to topic "alerts.critical"?'
    );
  });

  it("includes undone warning for remove action", () => {
    const desc = bulkTopicConfirmDescription("remove", 5, "ops.team");
    expect(desc).toContain("Remove 5 subscribers");
    expect(desc).toContain("cannot be undone");
  });

  it("uses singular noun for one subscriber (remove)", () => {
    expect(bulkTopicConfirmDescription("remove", 1, "ops.team")).toContain(
      "1 subscriber from"
    );
  });
});

describe("bulkPrefConfirmDescription", () => {
  it("uses singular noun for one subscriber", () => {
    expect(bulkPrefConfirmDescription(1, "global")).toBe(
      'Apply preference updates to 1 subscriber for scope "global"?'
    );
  });

  it("uses plural noun for multiple subscribers", () => {
    expect(bulkPrefConfirmDescription(10, "marketing.emails")).toBe(
      'Apply preference updates to 10 subscribers for scope "marketing.emails"?'
    );
  });
});
