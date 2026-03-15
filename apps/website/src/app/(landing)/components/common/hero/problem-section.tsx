"use client";

import { useEffect, useRef } from "react";
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

const PAIN_QUOTES = [
  {
    quote:
      "We have 4 different queue systems and none agree on retry semantics.",
    role: "Platform Engineer",
  },
  {
    quote:
      "When a job fails, I spend 30 minutes joining logs from three services.",
    role: "SRE Lead",
  },
  {
    quote:
      "Our workflow dependencies are shell scripts calling other shell scripts.",
    role: "Backend Tech Lead",
  },
  {
    quote:
      "Every new service reinvents the same job infrastructure from scratch.",
    role: "Engineering Manager",
  },
];

const ProblemSection = () => {
  const sectionRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          el.setAttribute("data-visible", "");
          observer.disconnect();
        }
      },
      { threshold: 0.15 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <section
      aria-label="The problem with background job systems"
      className="border-border/40 border-y py-20 sm:py-28"
      ref={sectionRef}
    >
      <Shell variant="wide">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              Your current stack is held together with duct tape.
            </span>{" "}
            <span className="text-muted-foreground">
              Every team that runs background work hits the same wall — and
              patches it with more tools until the system becomes the problem.
            </span>
          </h2>
        </div>

        {/* Sprawl diagram */}
        <div className="grid grid-cols-1 gap-8 lg:grid-cols-[1fr_auto_1fr] lg:gap-12">
          {/* Left: sprawl */}
          <div className="flex flex-wrap items-center justify-center gap-3">
            {SPRAWL_BOXES.map((box, i) => (
              <div
                className="demo-line rounded-lg border border-border/60 border-dashed bg-muted/30 px-4 py-2.5 text-muted-foreground text-sm"
                key={box}
                style={
                  {
                    "--line-delay": `${0.2 + i * 0.12}s`,
                  } as React.CSSProperties
                }
              >
                {box}
              </div>
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
          </div>

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
            <div
              className="demo-line flex flex-col items-center gap-4"
              style={{ "--line-delay": "1.4s" } as React.CSSProperties}
            >
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
          </div>
        </div>

        {/* Pain-point quotes */}
        <div className="mt-16 grid grid-cols-1 gap-4 sm:grid-cols-2">
          {PAIN_QUOTES.map((item) => (
            <div
              className="rounded-xl border border-border/60 bg-card p-5"
              key={item.role}
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
          ))}
        </div>
      </Shell>
    </section>
  );
};

export default ProblemSection;
