import { CheckmarkCircle02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";

const COMPETITORS = ["Strait", "Trigger.dev", "Inngest", "Temporal"] as const;

const ROWS = [
  {
    feature: "SDKs",
    values: ["5", "1", "1", "4"],
  },
  {
    feature: "Self-hosting",
    values: ["Simple (Postgres + Redis)", "Limited", "No", "Complex"],
  },
  {
    feature: "AI cost tracking",
    values: ["Built-in", "No", "Limited", "No"],
  },
  {
    feature: "License",
    values: ["Apache 2.0", "Apache 2.0", "SSPL", "MIT"],
  },
  {
    feature: "Managed execution",
    values: ["Containers with warm pools", "Serverless", "Serverless", "BYO"],
  },
] as const;

const ComparisonSection = () => (
  <section className="border-border/40 border-y py-20 sm:py-28">
    <Shell variant="wide">
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-foreground">Why Strait?</span>{" "}
            <span className="text-muted-foreground">
              See how Strait compares to other orchestration platforms.
            </span>
          </h2>
        </div>
      </Reveal>

      {/* Desktop table */}
      <Reveal className="hidden md:block" delay={0.1} spring>
        <div className="overflow-hidden rounded-2xl border border-border/40">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-border/40 border-b bg-muted/30">
                <th className="px-6 py-4 text-left font-medium text-muted-foreground">
                  Feature
                </th>
                {COMPETITORS.map((name, i) => (
                  <th
                    className={`px-6 py-4 text-left font-semibold ${
                      i === 0 ? "bg-primary/5 text-primary" : "text-foreground"
                    }`}
                    key={name}
                  >
                    {name}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {ROWS.map((row) => (
                <tr
                  className="border-border/40 border-b last:border-b-0"
                  key={row.feature}
                >
                  <td className="px-6 py-4 font-medium text-foreground">
                    {row.feature}
                  </td>
                  {row.values.map((value, i) => (
                    <td
                      className={`px-6 py-4 ${
                        i === 0
                          ? "bg-primary/5 font-medium text-foreground"
                          : "text-muted-foreground"
                      }`}
                      key={`${row.feature}-${COMPETITORS[i]}`}
                    >
                      <div className="flex items-center gap-2">
                        {i === 0 && (
                          <HugeiconsIcon
                            className="size-4 shrink-0 text-primary"
                            icon={CheckmarkCircle02Icon}
                          />
                        )}
                        {value}
                      </div>
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Reveal>

      {/* Mobile cards */}
      <div className="flex flex-col gap-6 md:hidden">
        {COMPETITORS.slice(1).map((competitor) => (
          <Reveal key={competitor} spring>
            <div className="rounded-2xl border border-border/40 bg-card/50 p-5">
              <h3 className="mb-4 font-semibold text-foreground">
                Strait vs. {competitor}
              </h3>
              <div className="flex flex-col gap-3">
                {ROWS.map((row) => {
                  const competitorIdx = COMPETITORS.indexOf(competitor);
                  return (
                    <div
                      className="flex items-start justify-between gap-4 text-sm"
                      key={`${competitor}-${row.feature}`}
                    >
                      <span className="text-muted-foreground">
                        {row.feature}
                      </span>
                      <div className="text-right">
                        <div className="font-medium text-primary">
                          {row.values[0]}
                        </div>
                        <div className="text-muted-foreground/60 text-xs">
                          vs. {row.values[competitorIdx]}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </Reveal>
        ))}
      </div>
    </Shell>
  </section>
);

export default ComparisonSection;
