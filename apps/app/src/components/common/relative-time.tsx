import { formatDistanceToNow } from "date-fns";

type RelativeTimeProps = {
  value: string;
};

export function RelativeTime({ value }: RelativeTimeProps) {
  return (
    <time dateTime={value} suppressHydrationWarning>
      {formatDistanceToNow(new Date(value), { addSuffix: true })}
    </time>
  );
}
