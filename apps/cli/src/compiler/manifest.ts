import type { StraitProjectConfig, StraitProjectManifest } from "./types";

type DslDefinition = {
  readonly kind?: string;
  readonly slug?: string;
  readonly toRegistrationBody?: (
    projectId?: string
  ) => Readonly<Record<string, unknown>>;
};

const asRecord = (
  value: unknown
): Readonly<Record<string, unknown>> | undefined =>
  typeof value === "object" && value !== null
    ? (value as Readonly<Record<string, unknown>>)
    : undefined;

const toRegistrationRecord = (
  value: unknown,
  projectId: string
): Readonly<Record<string, unknown>> | undefined => {
  const dslDefinition = value as DslDefinition;
  if (typeof dslDefinition.toRegistrationBody === "function") {
    return dslDefinition.toRegistrationBody(projectId);
  }

  return asRecord(value);
};

const slugForRecord = (record: Readonly<Record<string, unknown>>): string => {
  if (typeof record.slug === "string") {
    return record.slug;
  }
  if (typeof record.name === "string") {
    return record.name;
  }
  return "";
};

const sortBySlug = (
  entries: readonly Readonly<Record<string, unknown>>[]
): readonly Readonly<Record<string, unknown>>[] =>
  [...entries].sort((left, right) => {
    const leftSlug = slugForRecord(left);
    const rightSlug = slugForRecord(right);

    return leftSlug.localeCompare(rightSlug);
  });

/**
 * Builds stable project manifest payload from code-first config.
 */
export const buildProjectManifest = (
  config: StraitProjectConfig,
  options?: {
    readonly now?: Date;
    readonly outDirOverride?: string;
  }
): StraitProjectManifest => {
  const projectId = config.project.id;

  const jobs = sortBySlug(
    (config.jobs ?? [])
      .map((entry) => toRegistrationRecord(entry, projectId))
      .filter((entry): entry is Readonly<Record<string, unknown>> =>
        Boolean(entry)
      )
  );

  const workflows = sortBySlug(
    (config.workflows ?? [])
      .map((entry) => toRegistrationRecord(entry, projectId))
      .filter((entry): entry is Readonly<Record<string, unknown>> =>
        Boolean(entry)
      )
  );

  const generatedAt = (options?.now ?? new Date()).toISOString();

  return {
    version: 1,
    project: config.project,
    runtime: config.runtime?.kind ?? "node",
    build: {
      outDir: options?.outDirOverride ?? config.build?.outDir ?? ".strait",
      generatedAt,
    },
    jobs,
    workflows,
  };
};
