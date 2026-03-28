
import { cn } from "@strait/ui/utils";
import { useReducedMotion } from "motion/react";
import { useCallback, useEffect, useMemo, useRef } from "react";

type FlickeringGridProps = {
  squareSize?: number;
  gridGap?: number;
  flickerChance?: number;
  color?: string;
  maxOpacity?: number;
  className?: string;
};

const FlickeringGrid = ({
  squareSize = 4,
  gridGap = 6,
  flickerChance = 0.3,
  color = "rgb(0, 0, 0)",
  maxOpacity = 0.3,
  className,
}: FlickeringGridProps) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const dimensionsRef = useRef({ width: 0, height: 0 });
  const isVisibleRef = useRef(true);
  const prefersReducedMotion = useReducedMotion();
  const reducedMotionRef = useRef(prefersReducedMotion);
  reducedMotionRef.current = prefersReducedMotion;

  const memoizedColor = useMemo(() => {
    const match = color.match(/\d+/g);
    if (match && match.length >= 3) {
      return `${match[0]}, ${match[1]}, ${match[2]}`;
    }
    return "0, 0, 0";
  }, [color]);

  const setupCanvas = useCallback(
    (canvas: HTMLCanvasElement, width: number, height: number) => {
      const dpr = window.devicePixelRatio || 1;
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      const ctx = canvas.getContext("2d");
      if (!ctx) {
        return;
      }
      ctx.scale(dpr, dpr);
    },
    []
  );

  const drawGrid = useCallback(
    (
      ctx: CanvasRenderingContext2D,
      width: number,
      height: number,
      opacities: number[][]
    ) => {
      ctx.clearRect(0, 0, width, height);
      const cols = Math.ceil(width / (squareSize + gridGap));
      const rows = Math.ceil(height / (squareSize + gridGap));

      for (let i = 0; i < cols; i++) {
        for (let j = 0; j < rows; j++) {
          const opacity = opacities[i]?.[j] ?? 0;
          ctx.fillStyle = `rgba(${memoizedColor}, ${opacity})`;
          ctx.fillRect(
            i * (squareSize + gridGap),
            j * (squareSize + gridGap),
            squareSize,
            squareSize
          );
        }
      }
    },
    [squareSize, gridGap, memoizedColor]
  );

  useEffect(() => {
    const container = containerRef.current;
    const canvas = canvasRef.current;
    if (!(container && canvas)) {
      return;
    }

    let cols = 0;
    let rows = 0;
    let opacities: number[][] = [];
    let raf: number;

    const initGrid = () => {
      const { width, height } = dimensionsRef.current;
      if (width === 0) {
        return;
      }
      setupCanvas(canvas, width, height);
      cols = Math.ceil(width / (squareSize + gridGap));
      rows = Math.ceil(height / (squareSize + gridGap));
      opacities = Array.from({ length: cols }, () =>
        Array.from({ length: rows }, () => Math.random() * maxOpacity)
      );
    };

    const updateOpacities = () => {
      for (let i = 0; i < cols; i++) {
        for (let j = 0; j < rows; j++) {
          if (Math.random() < flickerChance * 0.016) {
            const row = opacities[i];
            if (row) {
              row[j] = Math.random() * maxOpacity;
            }
          }
        }
      }
    };

    const animate = () => {
      raf = requestAnimationFrame(animate);
      if (!isVisibleRef.current || reducedMotionRef.current) {
        return;
      }
      const ctx = canvas.getContext("2d");
      if (!(ctx && dimensionsRef.current.width > 0)) {
        return;
      }
      updateOpacities();
      drawGrid(
        ctx,
        dimensionsRef.current.width,
        dimensionsRef.current.height,
        opacities
      );
    };

    const resizeObs = new ResizeObserver(([entry]) => {
      if (entry) {
        dimensionsRef.current = {
          width: entry.contentRect.width,
          height: entry.contentRect.height,
        };
        initGrid();
      }
    });
    resizeObs.observe(container);

    const intersectionObs = new IntersectionObserver(
      ([entry]) => {
        isVisibleRef.current = entry?.isIntersecting ?? false;
      },
      { threshold: 0 }
    );
    intersectionObs.observe(container);

    initGrid();
    raf = requestAnimationFrame(animate);

    return () => {
      cancelAnimationFrame(raf);
      resizeObs.disconnect();
      intersectionObs.disconnect();
    };
  }, [squareSize, gridGap, flickerChance, maxOpacity, setupCanvas, drawGrid]);

  return (
    <div className={cn("absolute inset-0", className)} ref={containerRef}>
      <canvas
        className="pointer-events-none absolute inset-0"
        ref={canvasRef}
      />
    </div>
  );
};

export default FlickeringGrid;
