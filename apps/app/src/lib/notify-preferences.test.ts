import { describe, expect, it } from "vitest";
import type { NotifyPreference } from "@/hooks/api/types";
import {
  listPreferenceScopes,
  normalizePreferenceDigest,
  parsePreferenceChannel,
} from "./notify-preferences";

const makePreference = (
  fields?: Partial<NotifyPreference>
): NotifyPreference => ({
  id: "pref_123",
  recipient_type: "subscriber",
  recipient_id: "sub_123",
  scope: "global",
  channel_prefs: { email: true, inbox: false },
  critical_override: true,
  created_at: "2026-04-08T00:00:00Z",
  updated_at: "2026-04-08T00:00:00Z",
  ...fields,
});

describe("notify preference helpers", () => {
  it("parses boolean channel prefs", () => {
    const preference = makePreference();

    expect(parsePreferenceChannel(preference, "email", false)).toBe(true);
    expect(parsePreferenceChannel(preference, "inbox", true)).toBe(false);
  });

  it("falls back when channel prefs are missing", () => {
    const preference = makePreference({ channel_prefs: undefined });

    expect(parsePreferenceChannel(preference, "email", true)).toBe(true);
  });

  it("normalizes digest policy", () => {
    expect(
      normalizePreferenceDigest(makePreference({ digest_policy: "daily" }))
    ).toBe("daily");
    expect(
      normalizePreferenceDigest(
        makePreference({
          digest_policy:
            "unknown" as unknown as NotifyPreference["digest_policy"],
        })
      )
    ).toBe("instant");
  });

  it("collects scope options with global and selected scope", () => {
    const scopes = listPreferenceScopes(
      [
        makePreference({ scope: "global" }),
        makePreference({ scope: "workflow.approvals" }),
      ],
      "custom.scope"
    );

    expect(scopes).toEqual(
      expect.arrayContaining(["global", "workflow.approvals", "custom.scope"])
    );
  });
});
