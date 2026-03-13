import {
  FileSearchIcon,
  Link01Icon,
  PaintBrush01Icon,
  UserIcon,
} from "@hugeicons/core-free-icons";
import { dashboardHref } from "@/lib/urls.ts";
import FeatureShowcase from "./feature-showcase.tsx";
import {
  StylesVisual1,
  StylesVisual2,
  StylesVisual3,
  StylesVisual4,
} from "./visuals/styles-visual.tsx";

const StylesShowcase = () => (
  <FeatureShowcase
    cta={{
      href: dashboardHref("/login"),
      label: "See workflow automation",
    }}
    description="Turn messy, manual process chains into clear workflows your team can trust."
    features={[
      {
        title: "Map each step in one flow",
        description:
          "Design your workflow once and let each step run in the right order.",
        icon: UserIcon,
      },
      {
        title: "Handle edge cases without chaos",
        description:
          "Control what happens next when a step succeeds, fails, or needs a different path.",
        icon: FileSearchIcon,
      },
      {
        title: "Add approval checkpoints",
        description:
          "Pause for human review only where needed, then continue automatically.",
        icon: Link01Icon,
      },
      {
        title: "Reuse proven workflow blocks",
        description:
          "Compose repeatable workflow pieces so new automations ship much faster.",
        icon: PaintBrush01Icon,
      },
    ]}
    orientation="visual-left"
    title="Move from fragile chains to confident workflow automation"
    visuals={[
      <StylesVisual1 key="sv1" />,
      <StylesVisual2 key="sv2" />,
      <StylesVisual3 key="sv3" />,
      <StylesVisual4 key="sv4" />,
    ]}
  />
);

export default StylesShowcase;
