export type ComparisonColumnKey = "strait" | "competitor";

export type ComparisonSectionRow = {
  label: string;
  type: "text" | "boolean";
  values: Record<ComparisonColumnKey, string | boolean | null>;
  tooltip?: string;
};

export type ComparisonSection = {
  name: string;
  rows: ComparisonSectionRow[];
};

export type ComparisonTableProps = {
  competitorName: string;
  sections: ComparisonSection[];
};
