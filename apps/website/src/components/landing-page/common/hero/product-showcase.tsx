const CAPABILITIES = [
  "Postgres Queue",
  "Workflow DAGs",
  "Retry Strategies",
  "Approval Gates",
  "Idempotent Triggers",
  "Debug Bundles",
  "SDK Endpoints",
  "Cost Budgets",
] as const;

const MarqueeRow = ({ speed = "30s" }: { speed?: string }) => (
  <div className="flex overflow-hidden">
    <div
      className="flex shrink-0 animate-marquee gap-6 pr-6"
      style={{ animationDuration: speed }}
    >
      {CAPABILITIES.map((item) => (
        <span
          className="whitespace-nowrap rounded-full border border-primary-foreground/20 bg-primary-foreground/10 px-5 py-2.5 font-medium text-primary-foreground text-sm"
          key={item}
        >
          {item}
        </span>
      ))}
    </div>
    <div
      aria-hidden="true"
      className="flex shrink-0 animate-marquee gap-6 pr-6"
      style={{ animationDuration: speed }}
    >
      {CAPABILITIES.map((item) => (
        <span
          className="whitespace-nowrap rounded-full border border-primary-foreground/20 bg-primary-foreground/10 px-5 py-2.5 font-medium text-primary-foreground text-sm"
          key={`dup-${item}`}
        >
          {item}
        </span>
      ))}
    </div>
  </div>
);

const ProductShowcase = () => {
  return (
    <section className="relative border-border/40 border-y bg-primary py-20 sm:py-28">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_50%,_rgba(255,255,255,0.08)_0%,_transparent_50%),radial-gradient(circle_at_80%_50%,_rgba(255,255,255,0.05)_0%,_transparent_50%)]" />
      <div className="showcase-dots pointer-events-none absolute inset-0" />
      <div className="relative mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-primary-foreground">
              Everything you need to keep background work moving.
            </span>{" "}
            <span className="text-primary-foreground/70">
              Strait brings setup, visibility, and recovery together so your
              team can deliver faster.
            </span>
          </h2>
        </div>

        <div className="fade-edges relative">
          <MarqueeRow speed="35s" />
        </div>
      </div>
    </section>
  );
};

export default ProductShowcase;
