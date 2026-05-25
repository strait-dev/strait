/**
 * Helpers for displaying singleton (one-run-per-key) configuration.
 * Pure functions only, so they can be unit tested without rendering.
 */

import type { SingletonOnConflict } from "@/hooks/api/types";

/** Human-readable label for each on-conflict policy. */
export const SINGLETON_CONFLICT_LABELS: Record<SingletonOnConflict, string> = {
  queue: "Queue",
  drop: "Drop",
  replace: "Replace",
};

/**
 * Label for an on-conflict policy. The API types this as a bare string, so we
 * fall back to the raw value for any policy we do not have a label for.
 */
export function singletonConflictLabel(policy: string): string {
  return SINGLETON_CONFLICT_LABELS[policy as SingletonOnConflict] ?? policy;
}

/**
 * Extract the template string from a singleton_key_expr envelope.
 * The API returns either a JSON object `{ "template": "..." }` or, on older
 * payloads, a bare string. Returns "" when nothing usable is present.
 */
export function singletonKeyTemplate(expr: unknown): string {
  if (typeof expr === "string") {
    return expr;
  }
  if (expr && typeof expr === "object" && "template" in expr) {
    const { template } = expr as { template?: unknown };
    return typeof template === "string" ? template : "";
  }
  return "";
}

/** A job or workflow is a singleton when it has an on-conflict policy set. */
export function isSingletonConfigured(
  entity: { singleton_on_conflict?: string | null } | null | undefined
): boolean {
  return Boolean(entity?.singleton_on_conflict);
}
