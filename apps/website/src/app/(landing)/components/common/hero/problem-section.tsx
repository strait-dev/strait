const PAIN_POINTS = [
  {
    step: "1",
    text: "You maintain one system for API logic, another for queueing, and another for scheduling.",
  },
  {
    step: "2",
    text: "Retries, dead letters, and timeouts behave differently across services and are hard to reason about.",
  },
  {
    step: "3",
    text: "Workflow dependencies and approvals become custom glue code that is brittle under load.",
  },
  {
    step: "4",
    text: "When a run fails, teams lose time stitching together logs, traces, and partial state.",
  },
] as const;

const ProblemSection = () => (
  <section
    aria-label="The problem with background job systems"
    className="border-border/40 border-y py-20 sm:py-28"
  >
    <div className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <div className="mb-14">
          <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              Too much firefighting, not enough shipping.
            </span>{" "}
            <span className="text-muted-foreground">
              When jobs live across disconnected tools, small failures turn into
              long incident nights.
            </span>
          </h2>
        </div>

        <div className="space-y-4">
          {PAIN_POINTS.map((point) => (
            <div
              className="flex items-start gap-4 rounded-xl border border-border/60 bg-card p-4 sm:p-5"
              key={point.step}
            >
              <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted font-medium text-muted-foreground text-xs">
                {point.step}
              </span>
              <p className="text-base text-muted-foreground leading-relaxed">
                {point.text}
              </p>
            </div>
          ))}
        </div>

        <p className="mt-8 text-center font-medium text-foreground text-lg">
          Strait brings execution, visibility, and recovery into one place so
          your team can move with confidence.
        </p>
      </div>
    </div>
  </section>
);

export default ProblemSection;
