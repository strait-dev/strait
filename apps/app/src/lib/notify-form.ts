export const notifyScopedKeyPattern =
  /^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$/;

export const isNotifyScopedKey = (value: string) =>
  notifyScopedKeyPattern.test(value.trim());

export type NotifyJSONRecordParseResult =
  | { ok: true; data: Record<string, object> }
  | { ok: false; reason: "invalid_json" | "invalid_shape" };

export const parseNotifyJSONRecord = (
  raw: string
): NotifyJSONRecordParseResult => {
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { ok: false, reason: "invalid_shape" };
    }

    return { ok: true, data: parsed as Record<string, object> };
  } catch {
    return { ok: false, reason: "invalid_json" };
  }
};
