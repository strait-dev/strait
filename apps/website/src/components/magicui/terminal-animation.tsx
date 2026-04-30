import { cn } from "@strait/ui/utils";
import { useReducedMotion } from "motion/react";
import { useCallback, useEffect, useRef, useState } from "react";

type TerminalAnimationProps = {
  code: string;
  className?: string;
  typingSpeed?: number;
  startOnView?: boolean;
};

const TerminalAnimation = ({
  code,
  className,
  typingSpeed = 30,
  startOnView = true,
}: TerminalAnimationProps) => {
  const [displayedCode, setDisplayedCode] = useState("");
  const [isComplete, setIsComplete] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const hasStarted = useRef(false);
  const prefersReducedMotion = useReducedMotion();

  const startTyping = useCallback(() => {
    if (prefersReducedMotion) {
      setDisplayedCode(code);
      setIsComplete(true);
      return;
    }
    setDisplayedCode("");
    setIsComplete(false);

    const msPerChar = typingSpeed;
    let rafId: number;
    let startTime: number | null = null;
    let lastCharCount = 0;

    const tick = (timestamp: number) => {
      if (startTime === null) {
        startTime = timestamp;
      }
      const elapsed = timestamp - startTime;
      const charCount = Math.min(Math.floor(elapsed / msPerChar), code.length);

      if (charCount !== lastCharCount) {
        lastCharCount = charCount;
        setDisplayedCode(code.slice(0, charCount));
        if (charCount >= code.length) {
          setIsComplete(true);
          return;
        }
      }

      rafId = requestAnimationFrame(tick);
    };

    rafId = requestAnimationFrame(tick);

    return () => cancelAnimationFrame(rafId);
  }, [code, typingSpeed, prefersReducedMotion]);

  useEffect(() => {
    if (!startOnView) {
      const cleanup = startTyping();
      return cleanup;
    }

    const el = ref.current;
    if (!el) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting && !hasStarted.current) {
          hasStarted.current = true;
          startTyping();
          observer.disconnect();
        }
      },
      { threshold: 0.3 }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [startOnView, startTyping]);

  return (
    <div className={cn("font-mono text-sm", className)} ref={ref}>
      <code className="whitespace-pre-wrap">
        {displayedCode}
        {!isComplete && (
          <span className="inline-block h-4 w-0.5 animate-pulse bg-current" />
        )}
      </code>
    </div>
  );
};

export default TerminalAnimation;
