import { formatDistanceToNow } from "date-fns";
import { useEffect, useMemo, useState } from "react";

type RelativeTimeProps = {
  value: string | Date;
};

function formatStableTimestamp(date: Date) {
  if (Number.isNaN(date.getTime())) {
    return "Invalid date";
  }
  return `${date.toISOString().slice(0, 16).replace("T", " ")} UTC`;
}

const RelativeTime = ({ value }: RelativeTimeProps) => {
  const [isHydrated, setIsHydrated] = useState(false);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  const date = useMemo(
    () => (value instanceof Date ? value : new Date(value)),
    [value]
  );
  const fallback = useMemo(() => formatStableTimestamp(date), [date]);

  return (
    <time
      dateTime={Number.isNaN(date.getTime()) ? undefined : date.toISOString()}
    >
      {isHydrated && !Number.isNaN(date.getTime())
        ? formatDistanceToNow(date, { addSuffix: true })
        : fallback}
    </time>
  );
};

export default RelativeTime;
