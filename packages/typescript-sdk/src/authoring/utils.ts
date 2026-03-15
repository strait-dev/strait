/**
 * Resolves a project ID from definition-time or registration-time values.
 *
 * @param definitionProjectId - Project ID from the definition options.
 * @param registrationProjectId - Project ID from the register() call.
 * @param entityLabel - Label used in the error message (e.g. "defineJob(my-slug)").
 * @returns The resolved project ID.
 * @throws {Error} If neither project ID is provided.
 */
export const requireProjectId = (
  definitionProjectId: string | undefined,
  registrationProjectId: string | undefined,
  entityLabel: string
): string => {
  const resolved = registrationProjectId ?? definitionProjectId;
  if (!resolved) {
    throw new Error(
      `${entityLabel} requires projectId in definition or register() call`
    );
  }

  return resolved;
};

/**
 * Extracts an `id` field from an unknown API response object.
 *
 * @param value - The API response to extract from.
 * @returns The `id` string if present, otherwise `undefined`.
 */
export const extractEntityId = (value: unknown): string | undefined => {
  if (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof (value as { readonly id: unknown }).id === "string"
  ) {
    return (value as { readonly id: string }).id;
  }

  return undefined;
};
