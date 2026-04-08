import type {
  NotifyDigestPolicy,
  NotifyPreference,
} from "@/hooks/api/types";

export const notifyDigestPolicyOptions: readonly NotifyDigestPolicy[] = [
  "instant",
  "hourly",
  "daily",
] as const;

export const parsePreferenceChannel = (
  preference: NotifyPreference | undefined,
  channel: "email" | "inbox",
  fallback: boolean
) => {
  if (
    !preference?.channel_prefs ||
    typeof preference.channel_prefs !== "object"
  ) {
    return fallback;
  }

  const value = (
    preference.channel_prefs as Record<
      string,
      object | string | number | boolean | null
    >
  )[channel];

  return typeof value === "boolean" ? value : fallback;
};

export const normalizePreferenceDigest = (
  preference: NotifyPreference | undefined
) => {
  const value = preference?.digest_policy;
  return (
    notifyDigestPolicyOptions.find((option) => option === value) ?? "instant"
  );
};

export const listPreferenceScopes = (
  preferences: NotifyPreference[],
  selectedScope: string
) => {
  const scopes = new Set<string>(["global", selectedScope]);
  for (const item of preferences) {
    scopes.add(item.scope);
  }
  return Array.from(scopes);
};
