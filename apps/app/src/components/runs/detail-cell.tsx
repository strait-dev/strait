const DetailCell = ({ label, value }: { label: string; value: string }) => (
  <div className="flex flex-col gap-0.5">
    <span className="text-muted-foreground text-xs">{label}</span>
    <span className="font-mono text-sm">{value}</span>
  </div>
);

export default DetailCell;
