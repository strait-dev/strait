import { cn } from "@strait/ui/utils";
import { useEffect, useRef, useState } from "react";
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

    let cancelled = false;
    let observer: IntersectionObserver | null = null;

    import("rough-notation").then(({ annotate }) => {
      if (cancelled) {
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

      observer = new IntersectionObserver(
        ([entry]) => {
          if (entry?.isIntersecting && !hasShown) {
            setHasShown(true);
            annotationRef.current?.show();
            observer?.disconnect();
          }
        },
        { threshold: 0.5 }
      );
      observer.observe(el);
    });

    return () => {
      cancelled = true;
      observer?.disconnect();
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
