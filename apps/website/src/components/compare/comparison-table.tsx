import type { ComparisonCategory } from "@/data/comparisons.ts";

import ComparisonTableClient from "./comparison-table.client.tsx";
import type { ComparisonSection } from "./comparison-table.types.ts";

function transformCategories(
  categories: ComparisonCategory[]
): ComparisonSection[] {
  return categories.map((category) => ({
    name: category.name,
    rows: category.features.map((feature) => ({
      label: feature.feature,
      type: (typeof feature.strait === "boolean" &&
      typeof feature.competitor === "boolean"
        ? "boolean"
        : "text") as "text" | "boolean",
      values: {
        strait: feature.strait,
        competitor: feature.competitor,
      },
      ...(feature.tooltip ? { tooltip: feature.tooltip } : {}),
    })),
  }));
}

type ComparisonTableProps = {
  competitorName: string;
  categories: ComparisonCategory[];
};

const ComparisonTable = ({
  competitorName,
  categories,
}: ComparisonTableProps) => {
  const sections = transformCategories(categories);

  return (
    <ComparisonTableClient
      competitorName={competitorName}
      sections={sections}
    />
  );
};

export default ComparisonTable;
