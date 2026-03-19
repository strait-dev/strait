type FieldDiff = {
  field: string;
  local: unknown;
  remote: unknown;
};

type DiffEntry = {
  kind: "job" | "workflow";
  slug: string;
  action: "add" | "modify" | "remove";
  fields?: FieldDiff[];
};

type DiffResult = {
  additions: DiffEntry[];
  modifications: DiffEntry[];
  removals: DiffEntry[];
  warnings: string[];
};

export type { DiffEntry, DiffResult, FieldDiff };

const JOB_COMPARE_FIELDS = [
  "name",
  "endpointUrl",
  "endpoint_url",
  "timeout",
  "maxConcurrency",
  "max_concurrency",
] as const;

const WORKFLOW_COMPARE_FIELDS = ["name", "steps"] as const;

const getSlug = (record: Record<string, unknown>): string => {
  if (typeof record.slug === "string") {
    return record.slug;
  }
  if (typeof record.name === "string") {
    return record.name;
  }
  return "";
};

const deepEqual = (a: unknown, b: unknown): boolean =>
  JSON.stringify(a) === JSON.stringify(b);

const compareFields = (
  local: Record<string, unknown>,
  remote: Record<string, unknown>,
  fields: readonly string[]
): FieldDiff[] => {
  const diffs: FieldDiff[] = [];
  for (const field of fields) {
    const localVal = local[field];
    const remoteVal = remote[field];
    if (localVal !== undefined && remoteVal !== undefined) {
      if (!deepEqual(localVal, remoteVal)) {
        diffs.push({ field, local: localVal, remote: remoteVal });
      }
    } else if (localVal !== undefined && remoteVal === undefined) {
      diffs.push({ field, local: localVal, remote: undefined });
    } else if (localVal === undefined && remoteVal !== undefined) {
      diffs.push({ field, local: undefined, remote: remoteVal });
    }
  }
  return diffs;
};

const indexBySlug = (
  records: readonly Record<string, unknown>[]
): Map<string, Record<string, unknown>> => {
  const map = new Map<string, Record<string, unknown>>();
  for (const record of records) {
    const slug = getSlug(record);
    if (slug) {
      map.set(slug, record);
    }
  }
  return map;
};

export const computeDiff = (
  localJobs: readonly Record<string, unknown>[],
  localWorkflows: readonly Record<string, unknown>[],
  remoteJobs: readonly Record<string, unknown>[],
  remoteWorkflows: readonly Record<string, unknown>[]
): DiffResult => {
  const additions: DiffEntry[] = [];
  const modifications: DiffEntry[] = [];
  const removals: DiffEntry[] = [];
  const warnings: string[] = [];

  const diffCollection = (
    kind: "job" | "workflow",
    locals: readonly Record<string, unknown>[],
    remotes: readonly Record<string, unknown>[],
    compareFieldList: readonly string[]
  ) => {
    const localIndex = indexBySlug(locals);
    const remoteIndex = indexBySlug(remotes);

    for (const [slug, local] of localIndex) {
      const remote = remoteIndex.get(slug);
      if (remote) {
        const fields = compareFields(local, remote, compareFieldList);
        if (fields.length > 0) {
          modifications.push({ kind, slug, action: "modify", fields });
        }
      } else {
        additions.push({ kind, slug, action: "add" });
      }
    }

    for (const [slug, remote] of remoteIndex) {
      if (!localIndex.has(slug)) {
        removals.push({ kind, slug, action: "remove" });
        const status = remote.status;
        if (typeof status === "string" && status === "active") {
          warnings.push(`Removing ${kind} "${slug}" which has status "active"`);
        }
      }
    }
  };

  diffCollection("job", localJobs, remoteJobs, [...JOB_COMPARE_FIELDS]);
  diffCollection("workflow", localWorkflows, remoteWorkflows, [
    ...WORKFLOW_COMPARE_FIELDS,
  ]);

  return { additions, modifications, removals, warnings };
};
