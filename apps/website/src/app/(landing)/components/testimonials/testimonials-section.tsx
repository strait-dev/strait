import type { TestimonialItem } from "./testimonial";
import TestimonialCarousel from "./testimonial-carousel";

const TESTIMONIALS: TestimonialItem[] = [
  {
    _id: "t-1",
    _title: "Reliability",
    text: "Stay confident during peak traffic with predictable run outcomes and built-in failure handling.",
    authorName: "Reliability",
    authorCompany: "Strait Platform",
    authorPosition: "Core Capability",
    avatar: null,
  },
  {
    _id: "t-2",
    _title: "Orchestration",
    text: "Coordinate multi-step workflows in one place so teams spend less time wiring systems together.",
    authorName: "Orchestration",
    authorCompany: "Strait Platform",
    authorPosition: "Core Capability",
    avatar: null,
  },
  {
    _id: "t-3",
    _title: "Operations",
    text: "Diagnose issues quickly and replay failed runs without slowing down product delivery.",
    authorName: "Operations",
    authorCompany: "Strait Platform",
    authorPosition: "Core Capability",
    avatar: null,
  },
];

const TestimonialsSection = () => {
  const headingId = "testimonials-title";

  return (
    <section aria-labelledby={headingId} className="py-20 sm:py-28">
      <div className="mx-auto max-w-[1600px] px-4 sm:px-6 lg:px-8">
        <div className="mb-14 max-w-3xl animate-on-scroll">
          <h2
            className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            <span className="font-bold text-foreground">
              Built to keep your team moving when it matters.
            </span>{" "}
            <span className="text-muted-foreground">
              Get dependable execution, clearer visibility, and faster recovery
              from day one.
            </span>
          </h2>
        </div>
      </div>

      <div className="border-border/50 border-y">
        <div className="mx-auto max-w-[1600px]">
          <TestimonialCarousel testimonials={TESTIMONIALS} />
        </div>
      </div>
    </section>
  );
};

export default TestimonialsSection;
