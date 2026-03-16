"use client";

import {
  type HTMLMotionProps,
  motion,
  useInView,
  useReducedMotion,
} from "motion/react";
import { type ReactNode, useRef } from "react";
import { EASE_OUT, SPRING_BOUNCY, SPRING_SMOOTH } from "@/lib/motion.ts";

type RevealVariant = "fade-up" | "fade-left" | "fade-right" | "scale" | "blur";

type RevealProps = {
  children: ReactNode;
  className?: string;
  delay?: number;
  distance?: number;
  direction?: "up" | "down" | "left" | "right";
  variant?: RevealVariant;
  spring?: boolean;
  once?: boolean;
} & Omit<HTMLMotionProps<"div">, "initial" | "animate">;

const directionMap = {
  up: { y: 1, x: 0 },
  down: { y: -1, x: 0 },
  left: { x: 1, y: 0 },
  right: { x: -1, y: 0 },
};

function getVariantStyles(
  variant: RevealVariant,
  distance: number,
  direction: "up" | "down" | "left" | "right"
) {
  switch (variant) {
    case "blur":
      return {
        hidden: { opacity: 0, filter: "blur(8px)" },
        visible: { opacity: 1, filter: "blur(0px)" },
      };
    case "scale":
      return {
        hidden: { opacity: 0, scale: 0.92 },
        visible: { opacity: 1, scale: 1 },
      };
    case "fade-left":
      return {
        hidden: { opacity: 0, x: -distance },
        visible: { opacity: 1, x: 0 },
      };
    case "fade-right":
      return {
        hidden: { opacity: 0, x: distance },
        visible: { opacity: 1, x: 0 },
      };
    default: {
      const dir = directionMap[direction];
      return {
        hidden: {
          opacity: 0,
          x: dir.x * distance,
          y: dir.y * distance,
        },
        visible: { opacity: 1, x: 0, y: 0 },
      };
    }
  }
}

function getTransition(variant: RevealVariant, spring: boolean, delay: number) {
  if (variant === "scale") {
    return { ...SPRING_SMOOTH, delay };
  }
  if (spring) {
    return { ...SPRING_BOUNCY, delay };
  }
  return {
    duration: 0.6,
    ease: EASE_OUT,
    delay,
  };
}

const Reveal = ({
  children,
  className,
  delay = 0,
  distance = 24,
  direction = "up",
  variant = "fade-up",
  spring = false,
  once = true,
  ...rest
}: RevealProps) => {
  const ref = useRef<HTMLDivElement>(null);
  const isInView = useInView(ref, { once, margin: "-64px" });
  const prefersReduced = useReducedMotion();
  const styles = getVariantStyles(variant, distance, direction);
  const transition = getTransition(variant, spring, delay);

  return (
    <motion.div
      animate={prefersReduced || isInView ? styles.visible : styles.hidden}
      className={className}
      initial={prefersReduced ? styles.visible : styles.hidden}
      ref={ref}
      transition={prefersReduced ? { duration: 0 } : transition}
      {...rest}
    >
      {children}
    </motion.div>
  );
};

export default Reveal;
