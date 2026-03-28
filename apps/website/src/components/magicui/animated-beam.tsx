import { cn } from "@strait/ui/utils";
import { motion, useReducedMotion } from "motion/react";
import { type RefObject, useEffect, useId, useRef, useState } from "react";

type AnimatedBeamProps = {
  className?: string;
  containerRef: RefObject<HTMLElement | null>;
  fromRef: RefObject<HTMLElement | null>;
  toRef: RefObject<HTMLElement | null>;
  pathColor?: string;
  pathOpacity?: number;
  pathWidth?: number;
  gradientStartColor?: string;
  gradientStopColor?: string;
  duration?: number;
  curvature?: number;
};

const AnimatedBeam = ({
  className,
  containerRef,
  fromRef,
  toRef,
  pathColor = "gray",
  pathOpacity = 0.2,
  pathWidth = 2,
  gradientStartColor = "#ffaa40",
  gradientStopColor = "#9c40ff",
  duration = 3,
  curvature = 0,
}: AnimatedBeamProps) => {
  const id = useId();
  const [pathD, setPathD] = useState("");
  const [svgDimensions, setSvgDimensions] = useState({ width: 0, height: 0 });
  const [isVisible, setIsVisible] = useState(false);
  const svgRef = useRef<SVGSVGElement>(null);
  const prefersReducedMotion = useReducedMotion();

  useEffect(() => {
    const updatePath = () => {
      if (!(containerRef.current && fromRef.current && toRef.current)) {
        return;
      }

      const containerRect = containerRef.current.getBoundingClientRect();
      const fromRect = fromRef.current.getBoundingClientRect();
      const toRect = toRef.current.getBoundingClientRect();

      setSvgDimensions({
        width: containerRect.width,
        height: containerRect.height,
      });

      const startX = fromRect.left - containerRect.left + fromRect.width / 2;
      const startY = fromRect.top - containerRect.top + fromRect.height / 2;
      const endX = toRect.left - containerRect.left + toRect.width / 2;
      const endY = toRect.top - containerRect.top + toRect.height / 2;

      const midX = (startX + endX) / 2;
      const controlY = (startY + endY) / 2 + curvature;

      setPathD(`M ${startX},${startY} Q ${midX},${controlY} ${endX},${endY}`);
    };

    updatePath();

    const observer = new ResizeObserver(updatePath);
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }
    return () => observer.disconnect();
  }, [containerRef, fromRef, toRef, curvature]);

  useEffect(() => {
    const el = svgRef.current;
    if (!el) {
      return;
    }
    const obs = new IntersectionObserver(
      ([entry]) => setIsVisible(!!entry?.isIntersecting),
      { threshold: 0.1 }
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  if (!pathD) {
    return null;
  }

  return (
    <svg
      className={cn("pointer-events-none absolute top-0 left-0", className)}
      fill="none"
      height={svgDimensions.height}
      ref={svgRef}
      width={svgDimensions.width}
    >
      <path
        d={pathD}
        stroke={pathColor}
        strokeOpacity={pathOpacity}
        strokeWidth={pathWidth}
      />
      <path
        d={pathD}
        stroke={`url(#${id})`}
        strokeLinecap="round"
        strokeWidth={pathWidth}
      />
      <defs>
        <motion.linearGradient
          animate={
            isVisible && !prefersReducedMotion
              ? { x1: ["0%", "100%"], x2: ["10%", "110%"] }
              : { x1: "0%", x2: "100%" }
          }
          id={id}
          transition={{
            duration,
            ease: "linear",
            repeat: Number.POSITIVE_INFINITY,
          }}
        >
          <stop stopColor={gradientStartColor} stopOpacity="0" />
          <stop stopColor={gradientStartColor} />
          <stop offset="0.325" stopColor={gradientStopColor} />
          <stop offset="1" stopColor={gradientStopColor} stopOpacity="0" />
        </motion.linearGradient>
      </defs>
    </svg>
  );
};

export default AnimatedBeam;
