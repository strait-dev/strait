import { MinusSignIcon, Tick01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  formatPlanPrice,
  formatPriceWithCents,
  PLANS,
} from "@strait/billing/products";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@strait/ui/components/tooltip";
import { cn } from "@strait/ui/utils";
import { Fragment, useId, useState } from "react";

import Shell from "@/components/layout/shell.tsx";
import type {
  PlanSummary,
  PricingComparisonClientProps,
} from "./pricing-comparison.types.ts";

const patternClasses =
  "relative before:absolute before:inset-0 before:bg-[image:repeating-linear-gradient(315deg,_var(--color-primary)_0,_var(--color-primary)_1px,_transparent_0,_transparent_50%)] before:bg-[size:10px_10px] before:opacity-30";

const renderTableCellContent = (
  row: { type: "text" | "boolean" },
  value: string | boolean | null
) => {
  if (row.type === "text") {
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
};

function PlanPrice({
  plan,
  interval,
}: {
  plan: PlanSummary;
  interval: "monthly" | "yearly";
}) {
  const planData = PLANS[plan.key];
  const isCustom = plan.prices.monthly < 0;
  const isFree = plan.prices.monthly === 0;

  if (interval === "yearly" && !(isCustom || isFree)) {
    return (
      <div>
        <p className="flex items-baseline gap-x-1 text-foreground">
          <span className="font-semibold text-3xl">
            {formatPriceWithCents(plan.prices.yearly)}
          </span>
          <span className="font-light text-muted-foreground text-sm">/yr</span>
        </p>
        <p className="mt-1 text-muted-foreground/60 text-xs">
          {formatPlanPrice(planData, "yearly")}/mo
        </p>
      </div>
    );
  }

  const display = formatPlanPrice(planData, interval);

  return (
    <div>
      <p className="flex items-baseline gap-x-1 text-foreground">
        <span className="font-semibold text-3xl">{display}</span>
        {!(isCustom || isFree) && (
          <span className="font-light text-muted-foreground text-sm">/mo</span>
        )}
      </p>
    </div>
  );
}

function MobilePlanCard({
  plan,
  interval,
  sections,
}: {
  plan: PlanSummary;
  interval: "monthly" | "yearly";
  sections: PricingComparisonClientProps["sections"];
}) {
  return (
    <article className="flex flex-col">
      <div className="flex items-stretch border-border/50 border-b">
        <div
          className={cn(
            "flex aspect-square w-14 shrink-0 items-center justify-center border-border/50 border-r",
            patternClasses
          )}
        >
          {plan.highlight ? (
            <HugeiconsIcon
              className="relative z-10 size-5 text-foreground"
              icon={Tick01Icon}
            />
          ) : (
            <span className="relative z-10 font-semibold text-foreground text-sm">
              {plan.name.charAt(0)}
            </span>
          )}
        </div>
        <div className="flex flex-1 items-center justify-between px-4 py-3">
          <div>
            <h3 className="font-semibold text-base text-foreground">
              {plan.name}
            </h3>
            {plan.badge && (
              <Badge className="mt-1" variant="success-light">
                {plan.badge}
              </Badge>
            )}
          </div>
          <div className="text-right">
            <PlanPrice interval={interval} plan={plan} />
          </div>
        </div>
      </div>

      <div className="flex flex-1 flex-col p-6">
        <Button
          className="w-full"
          // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
          render={<a href={plan.cta.href} />}
          variant={plan.highlight ? "default" : "outline"}
        >
          {plan.cta.label}
        </Button>

        <ul className="mt-6 space-y-3 text-foreground text-sm">
          {sections.map((section) => (
            <li key={`${plan.key}-${section.name}`}>
              <ul className="space-y-3">
                {section.rows.map((row) => {
                  const value = row.values[plan.key];
                  if (!value) {
                    return null;
                  }

                  let label: React.ReactNode;
                  if (row.type === "text") {
                    label = value;
                  } else if (row.tooltip) {
                    label = (
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger className="cursor-help underline decoration-muted-foreground/40 decoration-dashed underline-offset-4">
                            {row.label}
                          </TooltipTrigger>
                          <TooltipContent>{row.tooltip}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    );
                  } else {
                    label = row.label;
                  }

                  return (
                    <li
                      className="flex gap-x-3"
                      key={`${plan.key}-${row.label}`}
                    >
                      <HugeiconsIcon
                        className="mt-0.5 size-4 shrink-0 text-success"
                        icon={Tick01Icon}
                      />
                      <span className="text-muted-foreground">{label}</span>
                    </li>
                  );
                })}
              </ul>
            </li>
          ))}
        </ul>
      </div>
    </article>
  );
}

const PricingComparisonClient = ({
  header,
  plans,
  sections,
}: PricingComparisonClientProps) => {
  const sectionId = useId();
  const headingId = `${sectionId}-title`;
  const [interval, setInterval] = useState<"monthly" | "yearly">("monthly");

  return (
    <section
      aria-labelledby={headingId}
      className="bg-background py-20 sm:py-28"
      id={sectionId}
    >
      <Shell variant="wide">
        <div className="mx-auto flex max-w-3xl flex-col items-center gap-4 text-center">
          <span className="kicker">{header.badge}</span>
          <h2
            className="text-balance font-semibold text-2xl text-foreground sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            {header.title}
          </h2>
          <p className="max-w-2xl text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
            {header.description}
          </p>
        </div>

        <div className="mt-10 flex justify-center">
          <div className="inline-flex items-center gap-1 rounded-full border border-border/50 bg-card p-1 shadow-sm">
            {[
              { label: "Monthly", value: "monthly" as const },
              {
                label: "Yearly",
                value: "yearly" as const,
                helper: "Save ~17%",
              },
            ].map((option) => (
              <button
                className={cn(
                  "relative flex items-center gap-2 rounded-full px-4 py-2 font-medium text-sm transition-colors",
                  interval === option.value
                    ? "bg-primary text-primary-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                )}
                key={option.value}
                onClick={() => setInterval(option.value)}
                type="button"
              >
                <span>{option.label}</span>
                {option.helper && option.value === "yearly" ? (
                  <span className="rounded-full bg-success px-2 py-0.5 font-medium text-white text-xs">
                    {option.helper}
                  </span>
                ) : null}
              </button>
            ))}
          </div>
        </div>

        {/* Mobile layout - tabbed plan switcher */}
        <div className="mx-auto mt-12 max-w-md sm:mt-16 lg:hidden">
          <Tabs defaultValue="pro">
            <TabsList className="w-full" variant="line">
              {plans.map((plan) => (
                <TabsTrigger key={plan.key} value={plan.key}>
                  {plan.name}
                </TabsTrigger>
              ))}
            </TabsList>
            {plans.map((plan) => (
              <TabsContent key={plan.key} value={plan.key}>
                <div className="mt-4 border border-border/50">
                  <MobilePlanCard
                    interval={interval}
                    plan={plan}
                    sections={sections}
                  />
                </div>
              </TabsContent>
            ))}
          </Tabs>
        </div>

        {/* Desktop table */}
        <div className="mt-16 hidden lg:block">
          <div className="border-border/50 border-y">
            <div className="mx-auto border-border/50 border-x">
              <table className="w-full border-collapse">
                <caption className="sr-only">
                  Pricing plan comparison table
                </caption>
                <thead>
                  <tr className="border-border/50 border-b">
                    <th className="border-border/50 border-r p-6" scope="col">
                      <span className="sr-only">Feature</span>
                    </th>

                    {plans.map((plan, index) => {
                      const isLast = index === plans.length - 1;

                      return (
                        <th
                          className={cn(
                            "p-6 text-left align-top font-normal",
                            !isLast && "border-border/50 border-r",
                            plan.highlight && "bg-muted/50"
                          )}
                          id={`desktop-${plan.key}`}
                          key={plan.key}
                          scope="col"
                        >
                          <div className="flex flex-col">
                            <div className="flex items-center gap-2">
                              <span className="kicker">{plan.name}</span>
                              {plan.badge && (
                                <Badge variant="success-light">
                                  {plan.badge}
                                </Badge>
                              )}
                            </div>
                            <div className="mt-2 min-h-[3.5rem]">
                              <PlanPrice interval={interval} plan={plan} />
                            </div>
                            <Button
                              className="mt-6 w-full"
                              render={
                                // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
                                <a
                                  aria-describedby={`desktop-${plan.key}`}
                                  href={plan.cta.href}
                                />
                              }
                              variant={plan.highlight ? "default" : "outline"}
                            >
                              {plan.cta.label}
                            </Button>
                          </div>
                        </th>
                      );
                    })}
                  </tr>
                </thead>
                <tbody>
                  {sections.map((section, sectionIdx) => (
                    <Fragment key={section.name}>
                      <tr className="border-border/50 border-b bg-muted/30">
                        <th
                          className="px-6 py-4 text-left"
                          colSpan={plans.length + 1}
                          scope="colgroup"
                        >
                          <span className="font-semibold text-foreground text-sm">
                            {section.name}
                          </span>
                        </th>
                      </tr>

                      {section.rows.map((row, rowIdx) => {
                        const isLastRow = rowIdx === section.rows.length - 1;
                        const isLastSection =
                          sectionIdx === sections.length - 1;

                        return (
                          <tr
                            className={cn(
                              !(isLastRow && isLastSection) &&
                                "border-border/50 border-b"
                            )}
                            key={row.label}
                          >
                            <th
                              className="border-border/50 border-r px-6 py-4 text-left font-normal"
                              scope="row"
                            >
                              {row.tooltip ? (
                                <TooltipProvider>
                                  <Tooltip>
                                    <TooltipTrigger className="cursor-help text-foreground text-sm underline decoration-muted-foreground/40 decoration-dashed underline-offset-4">
                                      {row.label}
                                    </TooltipTrigger>
                                    <TooltipContent>
                                      {row.tooltip}
                                    </TooltipContent>
                                  </Tooltip>
                                </TooltipProvider>
                              ) : (
                                <span className="text-foreground text-sm">
                                  {row.label}
                                </span>
                              )}
                            </th>

                            {plans.map((plan, index) => {
                              const value = row.values[plan.key];
                              const isLast = index === plans.length - 1;

                              return (
                                <td
                                  className={cn(
                                    "px-6 py-4 text-center text-muted-foreground text-sm",
                                    !isLast && "border-border/50 border-r",
                                    plan.highlight && "bg-muted/50"
                                  )}
                                  key={`${plan.key}-${row.label}`}
                                >
                                  {renderTableCellContent(row, value)}
                                </td>
                              );
                            })}
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
      </Shell>
    </section>
  );
};

export default PricingComparisonClient;
