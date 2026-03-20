import { Card, CardContent } from "@strait/ui/components/card";

type Props = {
  label: string;
  value: string;
};

export function MetricsCard({ label, value }: Props) {
  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-muted-foreground text-xs">{label}</p>
        <p className="mt-1 font-medium text-foreground text-lg tabular-nums">
          {value}
        </p>
      </CardContent>
    </Card>
  );
}
