"use client";

import { motion, useMotionValue, useSpring } from "motion/react";
import { useEffect, useRef } from "react";

const SmoothCursor = () => {
  const cursorX = useMotionValue(-100);
  const cursorY = useMotionValue(-100);
  const isVisible = useRef(false);
  const opacityRef = useRef(0);

  const springConfig = { damping: 25, stiffness: 200, mass: 0.5 };
  const x = useSpring(cursorX, springConfig);
  const y = useSpring(cursorY, springConfig);

  useEffect(() => {
    // Only show on devices with fine pointer (desktop)
    if (!window.matchMedia("(pointer: fine)").matches) {
      return;
    }

    const handleMouseMove = (e: MouseEvent) => {
      cursorX.set(e.clientX - 8);
      cursorY.set(e.clientY - 8);
      if (!isVisible.current) {
        isVisible.current = true;
        opacityRef.current = 1;
      }
    };

    const handleMouseLeave = () => {
      isVisible.current = false;
    };

    window.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseleave", handleMouseLeave);

    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseleave", handleMouseLeave);
    };
  }, [cursorX, cursorY]);

  // Don't render on touch devices
  if (
    typeof window !== "undefined" &&
    !window.matchMedia("(pointer: fine)").matches
  ) {
    return null;
  }

  return (
    <motion.div
      className="pointer-events-none fixed top-0 left-0 z-[9999] size-4 rounded-full bg-primary/30 mix-blend-difference"
      style={{ x, y }}
    />
  );
};

export default SmoothCursor;
