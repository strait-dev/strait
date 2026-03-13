import type { HugeiconsIconProps } from "@hugeicons/react";

import FeatureShowcase from "./feature-showcase.tsx";

type FeatureOrientation = "visual-left" | "visual-right";

const allowedOrientations: FeatureOrientation[] = [
  "visual-left",
  "visual-right",
];

type Icon = HugeiconsIconProps["icon"];

type FeatureEntry = {
  title: string;
  description: string;
  icon: Icon;
};

type FeatureShowcaseOptions = {
  slug: string;
  iconMap?: Record<string, Icon>;
  defaultOrientation?: FeatureOrientation;
  content?: {
    title?: string;
    description?: string;
    orientation?: FeatureOrientation;
    cta?: {
      href: string;
      label: string;
    };
    features?: Array<{
      title: string;
      description: string;
      iconKey: string;
    }>;
  };
};

const createFeatureShowcase = ({
  iconMap = {},
  defaultOrientation = "visual-right",
  content,
}: FeatureShowcaseOptions) => {
  const Component = () => {
    const title = content?.title?.trim();
    const description = content?.description?.trim();
    const cta = content?.cta;

    const features: FeatureEntry[] =
      content?.features
        ?.map((feature) => {
          const icon = iconMap[feature.iconKey];
          if (!icon) {
            return null;
          }

          return {
            title: feature.title,
            description: feature.description,
            icon,
          };
        })
        .filter((value): value is FeatureEntry => value !== null) ?? [];

    if (!(title && description && cta) || features.length === 0) {
      return null;
    }

    const orientation =
      content?.orientation && allowedOrientations.includes(content.orientation)
        ? content.orientation
        : defaultOrientation;

    return (
      <FeatureShowcase
        cta={cta}
        description={description}
        features={features}
        orientation={orientation}
        title={title}
      />
    );
  };

  return Component;
};

export default createFeatureShowcase;
