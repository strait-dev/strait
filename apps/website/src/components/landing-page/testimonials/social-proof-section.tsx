import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";

const TESTIMONIALS = [
  {
    quote:
      "Replaced Redis + BullMQ + custom retry with Strait in a weekend. On-call pages dropped 80%.",
    role: "Platform Engineer",
  },
  {
    quote:
      "The DAG engine is exactly what our data pipeline needed. No more Airflow YAML.",
    role: "Data Engineer",
  },
  {
    quote:
      "Cost budgets on AI agent runs saved us from a $2K billing surprise on day one.",
    role: "Founding Engineer",
  },
];

const SocialProofSection = () => (
  <section className="py-20 sm:py-28">
    <Shell variant="wide">
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              Developers ship faster with Strait.
            </span>{" "}
            <span className="text-muted-foreground">
              Join the growing community.
            </span>
          </h2>
        </div>
      </Reveal>

      <StaggerGroup className="grid grid-cols-1 gap-6 md:grid-cols-3">
        {TESTIMONIALS.map((testimonial) => (
          <StaggerItem key={testimonial.role}>
            <div className="flex h-full flex-col rounded-2xl border border-border/40 bg-card/50 p-6 sm:p-8">
              <blockquote className="flex-1 text-pretty text-foreground leading-relaxed">
                &ldquo;{testimonial.quote}&rdquo;
              </blockquote>
              <p className="mt-6 text-muted-foreground text-sm">
                -- {testimonial.role}
              </p>
            </div>
          </StaggerItem>
        ))}
      </StaggerGroup>
    </Shell>
  </section>
);

export default SocialProofSection;
