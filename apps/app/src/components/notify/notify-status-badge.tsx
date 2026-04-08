import { Badge } from "@strait/ui/components/badge";

type Props = {
  status: string;
};

const statusStyles = {
  delivered: "success-light",
  failed: "destructive-light",
  bounced: "destructive-light",
  processing: "info-light",
  pending: "secondary-light",
  scheduled: "warning-light",
  active: "success-light",
  unsubscribed: "warning-light",
  deleted: "secondary-light",
  acknowledged: "warning-light",
  completed: "success-light",
} as const;

const formatStatus = (status: string) =>
  status.replaceAll("_", " ").replace(/\b\w/g, (char) => char.toUpperCase());

const NotifyStatusBadge = ({ status }: Props) => {
  const normalized = (status || "unknown").toLowerCase();
  const variant =
    statusStyles[normalized as keyof typeof statusStyles] ?? "secondary-light";

  return <Badge variant={variant}>{formatStatus(normalized)}</Badge>;
};

export default NotifyStatusBadge;
