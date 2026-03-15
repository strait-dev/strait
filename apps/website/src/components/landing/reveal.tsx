"use client";

import { type HTMLMotionProps, motion, useInView } from "motion/react";
import { type ReactNode, useRef } from "react";
import { EASE_OUT_EXPO } from "@/lib/motion.ts";

type RevealProps = {
  children: ReactNode;
  className?: string;
  delay?: number;
  distance?: number;
  direction?: "up" | "down" | "left" | "right";
  once?: boolean;
} & Omit<HTMLMotionProps<"div">, "initial" | "animate">;

const directionMap = {
  up: { y: 1, x: 0 },
  down: { y: -1, x: 0 },
  left: { x: 1, y: 0 },
  right: { x: -1, y: 0 },
};

const Reveal = ({
  children,
  className,
  delay = 0,
  distance = 24,
  direction = "up",
  once = true,
  ...rest
}: RevealProps) => {
  const ref = useRef<HTMLDivElement>(null);
  const isInView = useInView(ref, { once, margin: "-64px" });
  const dir = directionMap[direction];

  return (
    <motion.div
      ref={ref}
      className={className}
      initial={{
        opacity: 0,
        x: dir.x * distance,
        y: dir.y * distance,
      }}
      animate={
        isInView
          ? { opacity: 1, x: 0, y: 0 }
          : { opacity: 0, x: dir.x * distance, y: dir.y * distance }
      }
      transition={{
        duration: 0.6,
        ease: EASE_OUT_EXPO,
        delay,
      }}
      {...rest}
    >
      {children}
    </motion.div>
  );
};

export default Reveal;
