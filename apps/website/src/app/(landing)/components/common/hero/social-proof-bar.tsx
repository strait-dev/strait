const STATS = [
  { value: "500+", label: "writers using Strait" },
  { value: "10", label: "content types supported" },
  { value: "4.9/5", label: "average user rating" },
] as const;

const SocialProofBar = () => (
  <section
    aria-label="Social proof"
    className="border-border/40 border-y bg-muted/30"
  >
    <div className="mx-auto max-w-[1600px] px-4 py-5 sm:px-6 sm:py-6 lg:px-8">
      <div className="flex flex-col items-center justify-center gap-6 sm:flex-row sm:gap-12 lg:gap-16">
        {STATS.map((stat) => (
          <div
            className="flex items-center gap-2.5 text-center sm:text-left"
            key={stat.label}
          >
            <span className="text-foreground text-lg sm:text-xl">
              {stat.value}
            </span>
            <span className="text-muted-foreground text-sm">{stat.label}</span>
          </div>
        ))}
      </div>
    </div>
  </section>
);

export default SocialProofBar;
