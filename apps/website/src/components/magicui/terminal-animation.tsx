
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
    let i = 0;
    setDisplayedCode("");
    setIsComplete(false);

    const interval = setInterval(() => {
      i++;
      setDisplayedCode(code.slice(0, i));
      if (i >= code.length) {
        clearInterval(interval);
        setIsComplete(true);
      }
    }, typingSpeed);

    return () => clearInterval(interval);
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
