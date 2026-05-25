import ConfigRow from "@/components/common/config-row";
import { KeyIcon, LayersIcon } from "@/lib/icons";
import { singletonConflictLabel, singletonKeyTemplate } from "@/lib/singleton";

type SingletonConfigRowsProps = {
  onConflict?: string;
  keyExpr?: unknown;
  maxQueueDepth?: number | null;
};

/**
 * Read-only config rows describing a job or workflow's singleton settings.
 * Renders nothing when no on-conflict policy is set. Shared by the job and
 * workflow detail pages.
 */
const SingletonConfigRows = ({
  onConflict,
  keyExpr,
  maxQueueDepth,
}: SingletonConfigRowsProps) => {
  if (!onConflict) {
    return null;
  }

  return (
    <>
      <ConfigRow
        icon={LayersIcon}
        label="Singleton Mode"
        value={singletonConflictLabel(onConflict)}
      />
      <ConfigRow
        icon={KeyIcon}
        label="Singleton Key"
        value={singletonKeyTemplate(keyExpr) || "—"}
      />
      {onConflict === "queue" && (
        <ConfigRow
          icon={LayersIcon}
          label="Max Queue Depth"
          value={maxQueueDepth == null ? "Unbounded" : String(maxQueueDepth)}
        />
      )}
    </>
  );
};

export default SingletonConfigRows;
