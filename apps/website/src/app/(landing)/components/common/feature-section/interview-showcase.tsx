import {
  Chatting01Icon,
  FileEditIcon,
  MessageEdit01Icon,
  SparklesIcon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls";
import FeatureShowcase from "./feature-showcase";
import {
  InterviewVisual1,
  InterviewVisual2,
  InterviewVisual3,
  InterviewVisual4,
} from "./visuals/interview-visual";

const InterviewShowcase = () => (
  <FeatureShowcase
    className="border-border/40 border-y bg-muted/20"
    cta={{
      href: dashboardHref("/login"),
      label: "Set up your first workflow",
    }}
    description="Get a stable foundation for background work so your team can ship faster with fewer production surprises."
    features={[
      {
        title: "Simple setup for each job",
        description:
          "Define what runs and how it should behave in one clear setup flow.",
        icon: Chatting01Icon,
      },
      {
        title: "Reliable queueing",
        description:
          "Keep jobs moving smoothly even as traffic grows and worker demand spikes.",
        icon: SparklesIcon,
      },
      {
        title: "Clear run status at every step",
        description:
          "Know exactly where work stands so incidents are easier to spot and fix.",
        icon: MessageEdit01Icon,
      },
      {
        title: "Recover quickly when runs fail",
        description:
          "Replay failed work without rebuilding everything from scratch.",
        icon: FileEditIcon,
      },
    ]}
    id="features"
    title="Launch dependable job execution without platform overhead"
    visuals={[
      <InterviewVisual1 key="iv1" />,
      <InterviewVisual2 key="iv2" />,
      <InterviewVisual3 key="iv3" />,
      <InterviewVisual4 key="iv4" />,
    ]}
  />
);

export default InterviewShowcase;
