type JsonValue = Record<string, unknown>;

function diffKeys(
  a: JsonValue,
  b: JsonValue
): Array<{
  key: string;
  status: "added" | "changed" | "removed" | "unchanged";
  newValue?: unknown;
  oldValue?: unknown;
}> {
  const allKeys = new Set([...Object.keys(a), ...Object.keys(b)]);
  const result: Array<{
    key: string;
    status: "added" | "changed" | "removed" | "unchanged";
    newValue?: unknown;
    oldValue?: unknown;
  }> = [];

  for (const key of [...allKeys].sort()) {
    const inA = key in a;
    const inB = key in b;
    if (inA && !inB) {
      result.push({ key, status: "removed", oldValue: a[key] });
    } else if (!inA && inB) {
      result.push({ key, status: "added", newValue: b[key] });
    } else if (JSON.stringify(a[key]) === JSON.stringify(b[key])) {
      result.push({
        key,
        status: "unchanged",
        oldValue: a[key],
        newValue: b[key],
      });
    } else {
      result.push({
        key,
        status: "changed",
        oldValue: a[key],
        newValue: b[key],
      });
    }
  }

  return result;
}

const STATUS_COLORS = {
  added: "text-green-600 dark:text-green-400",
  changed: "text-amber-600 dark:text-amber-400",
  removed: "text-red-600 dark:text-red-400",
  unchanged: "text-muted-foreground",
};

const STATUS_BG = {
  added: "bg-green-500/10",
  changed: "bg-amber-500/10",
  removed: "bg-red-500/10",
  unchanged: "",
};

export default function ConfigDiff({
  left,
  right,
}: {
  left: JsonValue;
  right: JsonValue;
}) {
  const entries = diffKeys(left, right);
  const hasChanges = entries.some((e) => e.status !== "unchanged");

  if (!hasChanges) {
    return (
      <p className="text-muted-foreground text-sm">
        No configuration differences between these versions.
      </p>
    );
  }

  return (
    <div className="space-y-1 font-mono text-xs">
      {entries.map((entry) => (
        <div
          className={`flex gap-2 rounded px-2 py-1 ${STATUS_BG[entry.status]}`}
          key={entry.key}
        >
          <span
            className={`min-w-[18ch] font-medium ${STATUS_COLORS[entry.status]}`}
          >
            {entry.status === "added" && "+ "}
            {entry.status === "removed" && "- "}
            {entry.status === "changed" && "~ "}
            {entry.key}
          </span>
          <span className={STATUS_COLORS[entry.status]}>
            {entry.status === "removed" && JSON.stringify(entry.oldValue)}
            {entry.status === "added" && JSON.stringify(entry.newValue)}
            {entry.status === "changed" && (
              <>
                <span className="line-through opacity-60">
                  {JSON.stringify(entry.oldValue)}
                </span>
                {" -> "}
                {JSON.stringify(entry.newValue)}
              </>
            )}
            {entry.status === "unchanged" && JSON.stringify(entry.oldValue)}
          </span>
        </div>
      ))}
    </div>
  );
}
