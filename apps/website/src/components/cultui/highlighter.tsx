"use client";

import { cn } from "@strait/ui/utils";
import { useEffect, useRef, useState } from "react";
import { annotate } from "rough-notation";
import type { RoughAnnotation } from "rough-notation/lib/model.js";

type HighlighterProps = {
  children: React.ReactNode;
  className?: string;
  type?: "underline" | "circle" | "box" | "highlight" | "strike-through";
  color?: string;
  strokeWidth?: number;
  isView?: boolean;
  animationDuration?: number;
};

const Highlighter = ({
  children,
  className,
  type = "underline",
  color = "currentColor",
  strokeWidth = 2,
  isView = false,
  animationDuration = 800,
}: HighlighterProps) => {
  const ref = useRef<HTMLSpanElement>(null);
  const annotationRef = useRef<RoughAnnotation | null>(null);
  const [hasShown, setHasShown] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }

    annotationRef.current = annotate(el, {
      type,
      color,
      strokeWidth,
      animationDuration,
      multiline: true,
    });

    if (!isView) {
      annotationRef.current.show();
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting && !hasShown) {
          setHasShown(true);
          annotationRef.current?.show();
          observer.disconnect();
        }
      },
      { threshold: 0.5 }
    );
    observer.observe(el);

    return () => {
      observer.disconnect();
      annotationRef.current?.remove();
    };
  }, [type, color, strokeWidth, isView, animationDuration, hasShown]);

  return (
    <span className={cn("inline", className)} ref={ref}>
      {children}
    </span>
  );
};

export default Highlighter;
