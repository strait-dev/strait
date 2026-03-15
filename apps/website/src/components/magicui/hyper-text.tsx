"use client";

import { cn } from "@strait/ui/utils";
import { useCallback, useEffect, useRef, useState } from "react";

type HyperTextProps = {
  children: string;
  className?: string;
  duration?: number;
  startOnView?: boolean;
  as?: React.ElementType;
};

const CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";

const HyperText = ({
  children,
  className,
  duration = 800,
  startOnView = true,
  as: Tag = "span",
}: HyperTextProps) => {
  const [displayText, setDisplayText] = useState(children);
  const [isAnimating, setIsAnimating] = useState(false);
  const ref = useRef<HTMLElement>(null);
  const hasAnimated = useRef(false);

  const runAnimation = useCallback(() => {
    if (isAnimating) {
      return;
    }
    setIsAnimating(true);

    const text = children;
    const totalFrames = Math.ceil(duration / 30);
    let frame = 0;

    const interval = setInterval(() => {
      frame++;
      const progress = frame / totalFrames;

      const result = text
        .split("")
        .map((char, i) => {
          if (char === " ") {
            return " ";
          }
          const charProgress = i / text.length;
          if (progress > charProgress + 0.3) {
            return char;
          }
          return CHARS[Math.floor(Math.random() * CHARS.length)] ?? char;
        })
        .join("");

      setDisplayText(result);

      if (frame >= totalFrames) {
        clearInterval(interval);
        setDisplayText(text);
        setIsAnimating(false);
      }
    }, 30);
  }, [children, duration, isAnimating]);

  useEffect(() => {
    if (!startOnView) {
      runAnimation();
      return;
    }

    const el = ref.current;
    if (!el) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting && !hasAnimated.current) {
          hasAnimated.current = true;
          runAnimation();
          observer.disconnect();
        }
      },
      { threshold: 0.5 }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [startOnView, runAnimation]);

  return (
    <Tag className={cn("inline", className)} ref={ref}>
      {displayText}
    </Tag>
  );
};

export default HyperText;
