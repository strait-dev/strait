import { motion, useInView, useReducedMotion } from "motion/react";
import { useRef } from "react";
import Shell from "@/components/layout/shell.tsx";

const ARCHITECTURE = [
  "Reliable job queue",
  "Full run lifecycle tracking",
  "DAG workflow orchestration",
  "Automatic retries with backoff",
  "Failed job recovery and replay",
  "Secure signed webhooks",
  "TypeScript, Go, and Python SDKs",
  "Per-run and daily cost budgets",
];

const ArchitectureList = () => {
  const containerRef = useRef<HTMLDivElement>(null);
  const isInView = useInView(containerRef, { once: true, margin: "-64px" });
  const prefersReduced = useReducedMotion();

  return (
    <div className="flex flex-wrap gap-2" ref={containerRef}>
      {ARCHITECTURE.map((item, i) => (
        <motion.span
          animate={
            prefersReduced || isInView
              ? { opacity: 1, y: 0 }
              : { opacity: 0, y: 8 }
          }
          className="rounded-full border border-border/60 bg-card px-3 py-1.5 text-foreground text-sm"
          initial={prefersReduced ? { opacity: 1, y: 0 } : { opacity: 0, y: 8 }}
          key={item}
          transition={
            prefersReduced
              ? { duration: 0 }
              : { duration: 0.3, delay: i * 0.06, ease: [0.16, 1, 0.3, 1] }
          }
        >
          {item}
        </motion.span>
      ))}
    </div>
  );
};

const CredibilitySection = () => (
  <section
    className="infinity-border-y overflow-hidden py-20 sm:py-28"
    id="credibility"
  >
    <Shell variant="wide">
      <div className="mb-14 max-w-3xl">
        <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
          <span className="text-foreground">
            Built for production. Trusted by teams.
          </span>{" "}
          <span className="text-muted-foreground">
            Every design decision is documented. The architecture, the
            tradeoffs, and the source code are all public.
          </span>
        </h2>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3">
        {/* Transparent by default */}
        <div className="p-6 sm:p-8">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
              <svg
                className="size-5 text-foreground"
                fill="currentColor"
                viewBox="0 0 24 24"
              >
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
              </svg>
            </div>
            <div>
              <h3 className="font-semibold text-foreground">
                Transparent by default
              </h3>
              <p className="text-muted-foreground text-sm">
                Apache 2.0 license
              </p>
            </div>
          </div>
          <p className="text-muted-foreground text-sm leading-relaxed">
            Every component is open source. Review the job queue, workflow
            engine, and scheduler. Extend it or contribute back.
          </p>
          <div className="mt-4 flex items-center gap-4">
            <span className="rounded bg-muted px-2 py-0.5 font-mono text-muted-foreground text-xs">
              TypeScript · Go · Python
            </span>
            <span className="text-muted-foreground/60 text-xs">
              Apache 2.0 License
            </span>
          </div>
        </div>

        {/* Technical Foundation */}
        <div className="border-border border-t p-6 sm:p-8 lg:border-t-0 lg:border-l">
          <h3 className="mb-4 font-semibold text-foreground">
            Technical Foundation
          </h3>
          <svg
            className="mb-4 w-full rounded-lg"
            fill="none"
            viewBox="0 0 320 120"
            xmlns="http://www.w3.org/2000/svg"
          >
            {/* App node */}
            <rect
              fill="var(--muted)"
              height="36"
              rx="6"
              stroke="var(--border)"
              strokeWidth="1.5"
              width="56"
              x="8"
              y="42"
            />
            <text
              fill="var(--foreground)"
              fontSize="11"
              fontWeight="500"
              textAnchor="middle"
              x="36"
              y="64"
            >
              App
            </text>

            {/* Arrow: App -> Strait */}
            <line
              stroke="var(--border)"
              strokeWidth="1.5"
              x1="64"
              x2="96"
              y1="60"
              y2="60"
            />
            <polygon fill="var(--border)" points="93,56 100,60 93,64" />

            {/* Strait group box */}
            <rect
              fill="var(--muted)"
              height="104"
              rx="8"
              stroke="var(--primary)"
              strokeDasharray="4 3"
              strokeWidth="1.5"
              width="132"
              x="100"
              y="8"
            />
            <text
              fill="var(--primary)"
              fontSize="10"
              fontWeight="600"
              textAnchor="middle"
              x="166"
              y="24"
            >
              Strait
            </text>

            {/* Queue */}
            <rect
              fill="var(--background, #fff)"
              height="26"
              rx="4"
              stroke="var(--border)"
              strokeWidth="1"
              width="52"
              x="112"
              y="34"
            />
            <text
              fill="var(--foreground)"
              fontSize="9"
              fontWeight="500"
              textAnchor="middle"
              x="138"
              y="51"
            >
              Queue
            </text>

            {/* Worker */}
            <rect
              fill="var(--background, #fff)"
              height="26"
              rx="4"
              stroke="var(--border)"
              strokeWidth="1"
              width="52"
              x="112"
              y="68"
            />
            <text
              fill="var(--foreground)"
              fontSize="9"
              fontWeight="500"
              textAnchor="middle"
              x="138"
              y="85"
            >
              Worker
            </text>

            {/* Scheduler */}
            <rect
              fill="var(--background, #fff)"
              height="26"
              rx="4"
              stroke="var(--border)"
              strokeWidth="1"
              width="52"
              x="172"
              y="51"
            />
            <text
              fill="var(--foreground)"
              fontSize="9"
              fontWeight="500"
              textAnchor="middle"
              x="198"
              y="68"
            >
              Scheduler
            </text>

            {/* Internal connections */}
            <line
              stroke="var(--border)"
              strokeWidth="1"
              x1="138"
              x2="138"
              y1="60"
              y2="68"
            />
            <line
              stroke="var(--border)"
              strokeWidth="1"
              x1="164"
              x2="172"
              y1="47"
              y2="58"
            />
            <line
              stroke="var(--border)"
              strokeWidth="1"
              x1="164"
              x2="172"
              y1="81"
              y2="72"
            />

            {/* Arrow: Strait -> Postgres */}
            <line
              stroke="var(--border)"
              strokeWidth="1.5"
              x1="232"
              x2="260"
              y1="60"
              y2="60"
            />
            <polygon fill="var(--border)" points="257,56 264,60 257,64" />

            {/* Postgres node */}
            <rect
              fill="var(--muted)"
              height="36"
              rx="6"
              stroke="var(--border)"
              strokeWidth="1.5"
              width="52"
              x="264"
              y="42"
            />
            <text
              fill="var(--foreground)"
              fontSize="9"
              fontWeight="500"
              textAnchor="middle"
              x="290"
              y="58"
            >
              Postgres
            </text>
            <text
              fill="var(--muted-foreground)"
              fontSize="7"
              textAnchor="middle"
              x="290"
              y="70"
            >
              (durable store)
            </text>
          </svg>
          <ArchitectureList />
        </div>

        {/* Why teams switch */}
        <div className="border-border border-t p-6 sm:p-8 lg:border-t-0 lg:border-l">
          <h3 className="mb-4 font-semibold text-foreground">
            Why teams switch
          </h3>
          <ul className="space-y-3">
            <li className="flex items-start gap-2 text-muted-foreground text-sm leading-relaxed">
              <span className="mt-1 inline-block size-1.5 shrink-0 rounded-full bg-primary" />
              Replace 4+ services with one platform
            </li>
            <li className="flex items-start gap-2 text-muted-foreground text-sm leading-relaxed">
              <span className="mt-1 inline-block size-1.5 shrink-0 rounded-full bg-primary" />
              Go from zero to running jobs in under 5 minutes
            </li>
            <li className="flex items-start gap-2 text-muted-foreground text-sm leading-relaxed">
              <span className="mt-1 inline-block size-1.5 shrink-0 rounded-full bg-primary" />
              Built-in retries, workflows, and cost tracking
            </li>
            <li className="flex items-start gap-2 text-muted-foreground text-sm leading-relaxed">
              <span className="mt-1 inline-block size-1.5 shrink-0 rounded-full bg-primary" />
              Full visibility into every job and workflow run
            </li>
            <li className="flex items-start gap-2 text-muted-foreground text-sm leading-relaxed">
              <span className="mt-1 inline-block size-1.5 shrink-0 rounded-full bg-primary" />
              5 language SDKs, so every team can use it
            </li>
          </ul>
        </div>
      </div>
    </Shell>
  </section>
);

export default CredibilitySection;
