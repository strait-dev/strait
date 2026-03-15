import type { Transition, Variants } from "motion/react";

export const SPRING_SNAPPY: Transition = {
  type: "spring",
  stiffness: 300,
  damping: 24,
};

export const SPRING_SMOOTH: Transition = {
  type: "spring",
  stiffness: 100,
  damping: 20,
};

export const EASE_OUT_EXPO: [number, number, number, number] = [
  0.16, 1, 0.3, 1,
];

export function staggerContainer(delay = 0.08): Variants {
  return {
    hidden: {},
    visible: {
      transition: {
        staggerChildren: delay,
      },
    },
  };
}

export function fadeSlideUp(distance = 24): Variants {
  return {
    hidden: { opacity: 0, y: distance },
    visible: {
      opacity: 1,
      y: 0,
      transition: {
        duration: 0.6,
        ease: EASE_OUT_EXPO,
      },
    },
  };
}

export function scaleReveal(): Variants {
  return {
    hidden: { opacity: 0, scale: 0.95 },
    visible: {
      opacity: 1,
      scale: 1,
      transition: SPRING_SMOOTH,
    },
  };
}
