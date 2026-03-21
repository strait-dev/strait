import type { Easing, Transition, Variants } from "motion/react";

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

export const SPRING_BOUNCY: Transition = {
  type: "spring",
  stiffness: 400,
  damping: 15,
};

export const SPRING_GENTLE: Transition = {
  type: "spring",
  stiffness: 60,
  damping: 18,
};

export const EASE_OUT: Easing = "easeOut";

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
        ease: EASE_OUT,
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

export function blurFadeIn(): Variants {
  return {
    hidden: { opacity: 0, filter: "blur(8px)" },
    visible: {
      opacity: 1,
      filter: "blur(0px)",
      transition: {
        duration: 0.6,
        ease: EASE_OUT,
      },
    },
  };
}

export function fadeSlideLeft(distance = 24): Variants {
  return {
    hidden: { opacity: 0, x: -distance },
    visible: {
      opacity: 1,
      x: 0,
      transition: {
        duration: 0.6,
        ease: EASE_OUT,
      },
    },
  };
}

export function fadeSlideRight(distance = 24): Variants {
  return {
    hidden: { opacity: 0, x: distance },
    visible: {
      opacity: 1,
      x: 0,
      transition: {
        duration: 0.6,
        ease: EASE_OUT,
      },
    },
  };
}
