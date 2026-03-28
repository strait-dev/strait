import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import type { ComparisonHighlight } from "@/data/comparisons.ts";

type ComparisonHighlightsProps = {
  highlights: ComparisonHighlight[];
  competitorName: string;
};

const ComparisonHighlights = ({
  highlights,
  competitorName,
}: ComparisonHighlightsProps) => {
  if (highlights.length === 0) {
    return null;
  }

  return (
    <Reveal>
      <div className="mb-10">
        <h2 className="text-2xl sm:text-3xl">What Strait does differently</h2>
        <p className="mt-3 text-muted-foreground text-sm leading-relaxed sm:text-base">
          Features where Strait offers a distinct advantage over{" "}
          {competitorName}.
        </p>
      </div>
      <StaggerGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {highlights.map((highlight) => (
          <StaggerItem key={highlight.title}>
            <div className="rounded-xl border border-border/40 bg-card/50 p-6">
              <div className="mb-3 flex items-center gap-2">
                <span className="rounded-md bg-primary/10 px-2 py-0.5 font-medium text-primary text-xs">
                  Strait
                </span>
              </div>
              <h3 className="font-semibold text-foreground">
                {highlight.title}
              </h3>
              <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
                {highlight.description}
              </p>
            </div>
          </StaggerItem>
        ))}
      </StaggerGroup>
    </Reveal>
  );
};

export default ComparisonHighlights;
