"use client";

import { MinusSignIcon, Tick01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@strait/ui/components/tooltip";
import { cn } from "@strait/ui/utils";
import { Fragment } from "react";

import type { ComparisonTableProps } from "./comparison-table.types.ts";

function CellContent({
  type,
  value,
}: {
  type: "text" | "boolean";
  value: string | boolean | null;
}) {
  if (type === "text") {
    if (value) {
      return <span>{value as string}</span>;
    }
    return <span className="text-muted-foreground/40">&mdash;</span>;
  }
  if (value) {
    return (
      <HugeiconsIcon
        className="mx-auto size-5 text-foreground"
        icon={Tick01Icon}
      />
    );
  }
  return (
    <HugeiconsIcon
      className="mx-auto size-5 text-muted-foreground/40"
      icon={MinusSignIcon}
    />
  );
}

function RowLabel({ label, tooltip }: { label: string; tooltip?: string }) {
  if (tooltip) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger className="cursor-help text-foreground text-sm underline decoration-muted-foreground/40 decoration-dashed underline-offset-4">
            {label}
          </TooltipTrigger>
          <TooltipContent>{tooltip}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }
  return <span className="text-foreground text-sm">{label}</span>;
}

const ComparisonTableClient = ({
  competitorName,
  sections,
}: ComparisonTableProps) => {
  return (
    <>
      {/* Desktop table */}
      <div className="hidden lg:block">
        <div className="border-border/50 border-y">
          <div className="mx-auto border-border/50 border-x">
            <table className="w-full border-collapse">
              <caption className="sr-only">
                Feature comparison: Strait vs {competitorName}
              </caption>
              <thead>
                <tr className="border-border/50 border-b">
                  <th
                    className="border-border/50 border-r px-6 py-4 text-left font-medium text-muted-foreground text-sm"
                    scope="col"
                  >
                    Feature
                  </th>
                  <th
                    className="border-border/50 border-r bg-muted/50 px-6 py-4 text-center font-semibold text-primary text-sm"
                    scope="col"
                  >
                    Strait
                  </th>
                  <th
                    className="px-6 py-4 text-center font-semibold text-foreground text-sm"
                    scope="col"
                  >
                    {competitorName}
                  </th>
                </tr>
              </thead>
              <tbody>
                {sections.map((section, sectionIdx) => (
                  <Fragment key={section.name}>
                    <tr className="border-border/50 border-b bg-muted/30">
                      <th
                        className="px-6 py-4 text-left"
                        colSpan={3}
                        scope="colgroup"
                      >
                        <span className="font-semibold text-foreground text-sm">
                          {section.name}
                        </span>
                      </th>
                    </tr>

                    {section.rows.map((row, rowIdx) => {
                      const isLastRow = rowIdx === section.rows.length - 1;
                      const isLastSection = sectionIdx === sections.length - 1;

                      return (
                        <tr
                          className={cn(
                            "transition-colors hover:bg-muted/20",
                            !(isLastRow && isLastSection) &&
                              "border-border/50 border-b"
                          )}
                          key={row.label}
                        >
                          <th
                            className="border-border/50 border-r px-6 py-4 text-left font-normal"
                            scope="row"
                          >
                            <RowLabel label={row.label} tooltip={row.tooltip} />
                          </th>
                          <td className="border-border/50 border-r bg-muted/50 px-6 py-4 text-center text-sm">
                            <CellContent
                              type={row.type}
                              value={row.values.strait}
                            />
                          </td>
                          <td className="px-6 py-4 text-center text-muted-foreground text-sm">
                            <CellContent
                              type={row.type}
                              value={row.values.competitor}
                            />
                          </td>
                        </tr>
                      );
                    })}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>

      {/* Mobile layout */}
      <div className="flex flex-col gap-6 lg:hidden">
        {sections.map((section) => (
          <div
            className="overflow-hidden rounded-xl border border-border/40"
            key={section.name}
          >
            <div className="bg-muted/30 px-5 py-3">
              <h3 className="font-semibold text-foreground text-sm">
                {section.name}
              </h3>
            </div>
            <div className="divide-y divide-border/30">
              {section.rows.map((row) => (
                <div className="px-5 py-3" key={row.label}>
                  <p className="mb-2 text-foreground text-sm">
                    <RowLabel label={row.label} tooltip={row.tooltip} />
                  </p>
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <p className="mb-1 font-medium text-primary text-xs">
                        Strait
                      </p>
                      <CellContent type={row.type} value={row.values.strait} />
                    </div>
                    <div>
                      <p className="mb-1 text-muted-foreground text-xs">
                        {competitorName}
                      </p>
                      <span className="text-muted-foreground">
                        <CellContent
                          type={row.type}
                          value={row.values.competitor}
                        />
                      </span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </>
  );
};

export default ComparisonTableClient;
