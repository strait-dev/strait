import { motion, useInView } from "motion/react";
import { type ReactNode, useRef } from "react";
import { fadeSlideUp, staggerContainer } from "@/lib/motion.ts";

type StaggerGroupProps = {
  children: ReactNode;
  className?: string;
  delay?: number;
  once?: boolean;
};

const StaggerGroup = ({
  children,
  className,
  delay = 0.08,
  once = true,
}: StaggerGroupProps) => {
  const ref = useRef<HTMLDivElement>(null);
  const isInView = useInView(ref, { once, margin: "-64px" });

  return (
    <motion.div
      animate={isInView ? "visible" : "hidden"}
      className={className}
      initial="hidden"
      ref={ref}
      variants={staggerContainer(delay)}
    >
      {children}
    </motion.div>
  );
};

const StaggerItem = ({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) => (
  <motion.div className={className} variants={fadeSlideUp()}>
    {children}
  </motion.div>
);

export { StaggerGroup, StaggerItem };
