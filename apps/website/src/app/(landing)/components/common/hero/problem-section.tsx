"use client";

import Highlighter from "@/components/cultui/highlighter.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";

const SPRAWL_BOXES = [
  "Redis Queue",
  "Cron Service",
  "Custom Retry",
  "SQS + Lambda",
  "Airflow",
  "Slack Alerts",
  "Manual DB Queries",
  "Shell Scripts",
];

const PAIN_QUOTES: Array<{ quote: React.ReactNode; role: string }> = [
  {
    quote: (
      <>
        We have{" "}
        <Highlighter color="var(--destructive)" isView type="underline">
          4 different queue systems
        </Highlighter>{" "}
        and none agree on retry semantics.
      </>
    ),
    role: "Platform Engineer",
  },
  {
    quote: (
      <>
        When a job fails, I spend{" "}
        <Highlighter color="var(--destructive)" isView type="underline">
          30 minutes joining logs
        </Highlighter>{" "}
        from three services.
      </>
    ),
    role: "SRE Lead",
  },
  {
    quote: (
      <>
        Our workflow dependencies are{" "}
        <Highlighter color="var(--destructive)" isView type="underline">
          shell scripts calling other shell scripts
        </Highlighter>
        .
      </>
    ),
    role: "Backend Tech Lead",
  },
  {
    quote: (
      <>
        Every new service{" "}
        <Highlighter color="var(--destructive)" isView type="underline">
          reinvents the same job infrastructure
        </Highlighter>{" "}
        from scratch.
      </>
    ),
    role: "Engineering Manager",
  },
];

const ProblemSection = () => (
  <section
    aria-label="The problem with background job systems"
    className="border-border/40 border-y py-20 sm:py-28"
  >
    <Shell variant="wide">
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              Your current stack is held together with duct tape.
            </span>{" "}
            <span className="text-muted-foreground">
              Every team that runs background work hits the same wall — and
              patches it with more tools until the system becomes the problem.
            </span>
          </h2>
        </div>
      </Reveal>

      {/* Sprawl diagram */}
      <div className="grid grid-cols-1 gap-8 lg:grid-cols-[1fr_auto_1fr] lg:gap-12">
        {/* Left: sprawl */}
        <StaggerGroup className="flex flex-wrap items-center justify-center gap-3">
          {SPRAWL_BOXES.map((box) => (
            <StaggerItem key={box}>
              <div className="rounded-lg border border-border/60 border-dashed bg-muted/30 px-4 py-2.5 text-muted-foreground text-sm">
                {box}
              </div>
            </StaggerItem>
          ))}
          <svg
            className="pointer-events-none absolute inset-0 hidden h-full w-full opacity-20 lg:block"
            viewBox="0 0 400 300"
          >
            <path
              d="M50,80 Q200,20 350,90 M80,150 Q180,200 320,130 M60,220 Q250,260 380,180"
              fill="none"
              stroke="var(--border)"
              strokeDasharray="4 4"
              strokeWidth={1}
            />
          </svg>
        </StaggerGroup>

        {/* Divider */}
        <div className="hidden items-center lg:flex">
          <div className="flex flex-col items-center gap-2">
            <div className="h-20 w-px bg-border/60" />
            <span className="rounded-full bg-muted px-3 py-1 font-medium text-muted-foreground text-xs">
              vs
            </span>
            <div className="h-20 w-px bg-border/60" />
          </div>
        </div>

        {/* Right: Strait */}
        <div className="flex items-center justify-center">
          <Reveal spring variant="scale">
            <div className="flex flex-col items-center gap-4">
              <div className="rounded-xl border border-foreground/10 bg-card px-8 py-6 text-center shadow-sm">
                <p className="font-heading font-semibold text-foreground text-lg">
                  Strait
                </p>
                <p className="mt-1 text-muted-foreground text-sm">
                  One runtime. One queue. One dashboard.
                </p>
              </div>
              <div className="flex gap-3">
                {["Queue", "Orchestrate", "Observe"].map((label) => (
                  <span
                    className="rounded-full border border-success/30 bg-success/8 px-3 py-1 text-sm text-success"
                    key={label}
                  >
                    {label}
                  </span>
                ))}
              </div>
            </div>
          </Reveal>
        </div>
      </div>

      {/* Pain-point quotes */}
      <div className="mt-16 grid grid-cols-1 gap-4 sm:grid-cols-2">
        {PAIN_QUOTES.map((item, i) => (
          <Reveal
            delay={i * 0.08}
            key={item.role}
            variant={i % 2 === 0 ? "fade-left" : "fade-right"}
          >
            <div
              className="rounded-xl border border-border/60 bg-card p-5"
              style={{
                borderLeftColor: "var(--destructive)",
                borderLeftWidth: 3,
              }}
            >
              <p className="text-foreground text-sm italic leading-relaxed">
                &ldquo;{item.quote}&rdquo;
              </p>
              <p className="mt-3 text-muted-foreground text-xs">
                — {item.role}
              </p>
            </div>
          </Reveal>
        ))}
      </div>
    </Shell>
  </section>
);

export default ProblemSection;
