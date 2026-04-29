import { CheckmarkCircle02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";

// Homepage shows the 3 most recognizable competitors; full list lives in compare/data.ts
const COMPETITORS = ["Strait", "Trigger.dev", "Inngest", "Temporal"] as const;

const ROWS = [
  {
    feature: "Language SDKs",
    values: [
      "5 (TypeScript, Python, Go, Ruby, Rust)",
      "1 (TypeScript)",
      "4 (TypeScript, Python, Go, Kotlin)",
      "7 (Go, Java, PHP, Python, TypeScript, .NET, Ruby)",
    ],
  },
  {
    feature: "Self-hosting",
    values: ["Simple", "Docker/K8s", "Available", "Complex"],
  },
  {
    feature: "AI cost tracking",
    values: ["Built-in per-run budgets", "No", "Limited", "No"],
  },
  {
    feature: "License",
    values: ["Apache 2.0", "Apache 2.0", "SSPL (server)", "MIT"],
  },
  {
    feature: "Execution model",
    values: [
      "Containers with warm pools",
      "Serverless",
      "Serverless",
      "Bring your own",
    ],
  },
  {
    feature: "Workflow approvals",
    values: [
      "Built-in gates",
      "wait.forToken()",
      "waitForEvent()",
      "Via Signals",
    ],
  },
] as const;

const ComparisonSection = () => (
  <section className="border-border/40 border-y py-16 sm:py-20">
    <Shell variant="wide">
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            How Strait compares.
          </h2>
          <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
            More SDKs, simpler self-hosting, and AI cost tracking that
            competitors don&apos;t offer.
          </p>
        </div>
      </Reveal>

      {/* Desktop table */}
      <Reveal className="hidden md:block" delay={0.1} spring>
        <div className="overflow-hidden rounded-2xl border border-border/40">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-border/40 border-b bg-muted/30">
                <th
                  className="px-6 py-4 text-left font-medium text-muted-foreground"
                  scope="col"
                >
                  Feature
                </th>
                {COMPETITORS.map((name, i) => (
                  <th
                    className={`px-6 py-4 text-left font-semibold ${
                      i === 0 ? "bg-primary/5 text-primary" : "text-foreground"
                    }`}
                    key={name}
                    scope="col"
                  >
                    {name}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {ROWS.map((row) => (
                <tr
                  className="border-border/40 border-b transition-colors last:border-b-0 hover:bg-muted/20"
                  key={row.feature}
                >
                  <th
                    className="px-6 py-4 text-left font-medium text-foreground"
                    scope="row"
                  >
                    {row.feature}
                  </th>
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
                      className="flex min-w-0 items-start justify-between gap-4 text-sm"
                      key={`${competitor}-${row.feature}`}
                    >
                      <span className="shrink-0 text-muted-foreground">
                        {row.feature}
                      </span>
                      <div className="min-w-0 text-right">
                        <div className="truncate font-medium text-primary">
                          {row.values[0]}
                        </div>
                        <div className="truncate text-muted-foreground/60 text-xs">
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
